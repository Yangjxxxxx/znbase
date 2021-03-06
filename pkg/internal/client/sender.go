// Copyright 2015  The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package client

import (
	"context"

	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// TxnType specifies whether a transaction is the root (parent)
// transaction, or a leaf (child) in a tree of client.Txns, as
// is used in a DistSQL flow.
type TxnType int

// TxnTypeName 事务类型名称
var TxnTypeName = map[TxnType]string{
	RootTxn: "ROOT TXN",
	LeafTxn: "LEAF TXN",
}

const (
	_ TxnType = iota
	// RootTxn specifies this sender is the root transaction, and is
	// responsible for aggregating all transactional state (see
	// TxnCoordMeta) and finalizing the transaction. The root txn is
	// responsible for heartbeating the transaction record.
	RootTxn
	// LeafTxn specifies this sender is for one of potentially many
	// distributed client transactions. The state from this transaction
	// must be propagated back to the root transaction and used to
	// augment its state before the transaction can be finalized. Leaf
	// transactions do not heartbeat the transaction record.
	//
	// Note: As leaves don't perform heartbeats, the transaction might be
	// cleaned up while this leaf is executing an operation. So data read
	// by a leaf txn is not guaranteed to not miss writes performed by the
	// transaction before the cleanup (at least not after the expiration
	// of the GC period / abort span entry timeout). If the client cares
	// about this hazard, the state of the heartbeats should be checked
	// using the root txn before delivering results to the client. DistSQL
	// does this.
	LeafTxn
)

// Sender is implemented by modules throughout the znbase stack, on both the
// "client" and the "server", involved in passing along and ultimately
// evaluating requests (batches). The interface is now considered regrettable
// because it's too narrow and at times leaky.
// Notable implementors: client.Txn, kv.TxnCoordSender, storage.Node,
// storage.Store, storage.Replica.
type Sender interface {
	// Send sends a batch for evaluation.
	// The contract about whether both a response and an error can be returned
	// varies between layers.
	//
	// The caller retains ownership of all the memory referenced by the
	// BatchRequest; the callee is not allowed to hold on to any parts of it past
	// after it returns from the call (this is so that the client module can
	// allocate requests from a pool and reuse them). For example, the DistSender
	// makes sure that, if there are concurrent requests, it waits for all of them
	// before returning, even in error cases.
	//
	// Once the request reaches the `transport` module, anothern restriction
	// applies (particularly relevant for the case when the node that the
	// transport is talking to is local, and so there's not gRPC
	// marshaling/unmarshaling):
	// - the callee has to treat everything inside the BatchRequest as
	// read-only. This is so that the client module retains the right to pass
	// pointers into its internals, like for example the Transaction. This
	// wouldn't work if the server would be allowed to change the Transaction
	// willy-nilly.
	// TODO(andrei): The client does not currently use this last guarantee; it
	// clones the txn for every request. Given that a client.Txn can be used
	// concurrently, in order for the client to take advantage of this, it would
	// need to switch to a copy-on-write scheme so that its updates to the txn do
	// not race with the server reading it. We should do this to avoid the cloning
	// allocations. And to be frank, it'd be a good idea for the
	// BatchRequest/Response to generally stop carrying transactions; the requests
	// usually only need a txn id and some timestamp. The responses would ideally
	// contain a list of targeted instructions about what the client should
	// update, as opposed to a full txn that the client is expected to diff with
	// its copy and apply all the updates.
	Send(context.Context, roachpb.BatchRequest) (*roachpb.BatchResponse, *roachpb.Error)
}

// TxnSender is the interface used to call into a ZNBaseDB instance
// when sending transactional requests. In addition to the usual
// Sender interface, TxnSender facilitates marshaling of transaction
// metadata between the "root" client.Txn and "leaf" instances.
type TxnSender interface {
	Sender

	// OnFinish invokes the supplied closure when the sender has finished
	// with the txn (i.e. it's been abandoned, aborted, or committed).
	// The error passed is meant to indicate to an extant distributed
	// SQL receiver that the underlying transaction record has either been
	// aborted (and why), or been committed. Only one callback is set, so
	// if this method is invoked multiple times, the most recent callback
	// is the only one which will be invoked.
	OnFinish(func(error))

	// SetSystemConfigTrigger sets the system db trigger to true on this transaction.
	// This will impact the EndTransactionRequest.
	//
	// NOTE: The system db trigger will only execute correctly if the transaction
	// record is located on the range that contains the system span. If a
	// transaction is created which modifies both system *and* non-system data, it
	// should be ensured that the transaction record itself is on the system span.
	// This can be done by making sure a system key is the first key touched in the
	// transaction.
	SetSystemConfigTrigger() error

	// GetMeta retrieves a copy of the TxnCoordMeta, which can be sent from root
	// to leaf transactions or the other way around. Can be combined via
	// AugmentMeta().
	//
	// If AnyTxnStatus is passed, then this function never returns errors.
	GetMeta(context.Context, TxnStatusOpt) (roachpb.TxnCoordMeta, error)

	// AugmentMeta combines the TxnCoordMeta from another distributed
	// TxnSender which is part of the same transaction.
	AugmentMeta(ctx context.Context, meta roachpb.TxnCoordMeta)

	// SetUserPriority sets the txn's priority.
	SetUserPriority(roachpb.UserPriority) error

	// SetDebugName sets the txn's debug name.
	SetDebugName(name string) error

	SetTxnMetaKeyIsSystemConfig()

	// TxnStatus exports the txn's status.
	TxnStatus() roachpb.TransactionStatus

	// CreateSavepoint establishes a savepoint.
	// This method is only valid when called on RootTxns.
	//
	// Committing (or aborting) the transaction causes every open
	// savepoint to be released (or, respectively, rolled back)
	// implicitly.
	CreateSavepoint(context.Context) (SavepointToken, error)

	// RollbackToSavepoint rolls back to the given savepoint.
	// All savepoints "under" the savepoint being rolled back
	// are also rolled back and their token must not be used any more.
	// The token of the savepoint being rolled back remains valid
	// and can be reused later (e.g. to release or roll back again).
	// Aborting the txn implicitly rolls back all savepoints
	// that are still open.
	//
	// This method is only valid when called on RootTxns.
	RollbackToSavepoint(context.Context, SavepointToken) error

	// ReleaseSavepoint releases the given savepoint.
	// The savepoint must not have been rolled back or released already.
	// All savepoints "under" the savepoint being released
	// are also released and their token must not be used any more.
	// Committing the txn implicitly releases all savepoints
	// that are still open.
	//
	// This method is only valid when called on RootTxns.
	ReleaseSavepoint(context.Context, SavepointToken) error

	// SetFixedTimestamp makes the transaction run in an unusual way, at a "fixed
	// timestamp": Timestamp and ReadTimestamp are set to ts, there's no clock
	// uncertainty, and the txn's deadline is set to ts such that the transaction
	// can't be pushed to a different timestamp.
	//
	// This is used to support historical queries (AS OF SYSTEM TIME queries and
	// backups). This method must be called on every transaction retry (but note
	// that retries should be rare for read-only queries with no clock uncertainty).
	// SetFixedTimestamp使事务以一种不同寻常的方式运行，在一个“固定的时间戳”:timestamp和RefreshedTimestamp被
	// 设置为ts，没有时钟不确定性，txn的deadline被设置为ts，这样事务就不能被推到一个不同的时间戳。
	SetFixedTimestamp(ctx context.Context, ts hlc.Timestamp)

	// 更新最新读写时间
	SetTimeForRC(ts hlc.Timestamp)

	// ManualRestart bumps the transactions epoch, and can upgrade the timestamp
	// and priority.
	// An uninitialized timestamp can be passed to leave the timestamp alone.
	//
	// Used by the SQL layer which sometimes knows that a transaction will not be
	// able to commit and prefers to restart early.
	// It is also used after synchronizing concurrent actors using a txn when a
	// retryable error is seen.
	// TODO(andrei): this second use should go away once we move to a TxnAttempt
	// model.
	ManualRestart(context.Context, roachpb.UserPriority, hlc.Timestamp)

	// UpdateStateOnRemoteRetryableErr updates the txn in response to an error
	// encountered when running a request through the txn.
	UpdateStateOnRemoteRetryableErr(context.Context, *roachpb.Error) *roachpb.Error

	// DisablePipelining instructs the TxnSender not to pipeline requests. It
	// should rarely be necessary to call this method. It is only recommended for
	// transactions that need extremely precise control over the request ordering,
	// like the transaction that merges ranges together.
	DisablePipelining() error

	// ReadTimestamp returns the transaction's current read timestamp.
	// Note a transaction can be internally pushed forward in time before
	// committing so this is not guaranteed to be the commit timestamp.
	// Use CommitTimestamp() when needed.
	// ReadTimestamp返回事务当前读的时间戳。事务可以在被提交之前推进时间戳，因此不能保证ReadTimestamp就是事务提交的时间戳。
	// 如果需要的话，就用CommitTimestamp来进行获取
	ReadTimestamp() hlc.Timestamp

	// CommitTimestamp returns the transaction's start timestamp.
	// The start timestamp can get pushed but the use of this
	// method will guarantee that the caller of this method sees
	// the push and thus calls this method again to receive the new
	// timestamp.
	CommitTimestamp() hlc.Timestamp

	// CommitTimestampFixed returns true if the commit timestamp has
	// been fixed to the start timestamp and cannot be pushed forward.
	CommitTimestampFixed() bool

	// IsSerializablePushAndRefreshNotPossible returns true if the transaction is
	// serializable, its timestamp has been pushed and there's no chance that
	// refreshing the read spans will succeed later (thus allowing the transaction
	// to commit and not be restarted). Used to detect whether the txn is
	// guaranteed to get a retriable error later.
	//
	// Note that this method allows for false negatives: sometimes the client only
	// figures out that it's been pushed when it sends an EndTransaction - i.e.
	// it's possible for the txn to have been pushed asynchoronously by some other
	// operation (usually, but not exclusively, by a high-priority txn with
	// conflicting writes).
	IsSerializablePushAndRefreshNotPossible() bool

	// Active returns true iff some commands have been performed with
	// this txn already.
	//
	// TODO(knz): Remove this, see
	// https://github.com/cockroachdb/cockroach/issues/15012
	Active() bool

	// Epoch returns the txn's epoch.
	Epoch() enginepb.TxnEpoch

	// SerializeTxn returns a clone of the transaction's current proto.
	// This is a nuclear option; generally client code shouldn't deal with protos.
	// However, this is used by DistSQL for sending the transaction over the wire
	// when it creates flows.
	SerializeTxn() *roachpb.Transaction

	// Step creates a sequencing point in the current transaction. A
	// sequencing point establishes a snapshot baseline for subsequent
	// read-only operations: until the next sequencing point, read-only
	// operations observe the data at the time the snapshot was
	// established and ignore writes performed since.
	//
	// Before the first step is taken, the transaction operates as if
	// there was a step after every write: each read to a key is able to
	// see the latest write before it. This makes the step behavior
	// opt-in and backward-compatible with existing code which does not
	// need it.
	// The method is idempotent.
	Step(ctx context.Context) error

	// i.e. uses reads-own-writes by default. This makes the step
	// behavior opt-in and backward-compatible with existing code which
	// does not need it.
	//
	// Calling ConfigureStepping(SteppingEnabled) when the stepping mode
	// is currently disabled implies calling Step(), for convenience.
	ConfigureStepping(ctx context.Context, mode SteppingMode) (prevMode SteppingMode)

	// GetSteppingMode accompanies ConfigureStepping. It is provided
	// for use in tests and assertion checks.
	GetSteppingMode(ctx context.Context) (curMode SteppingMode)
	GetTxnSpans() TxnSpans
}

// SteppingMode is the argument type to ConfigureStepping.
type SteppingMode bool

const (
	// SteppingDisabled is the default mode, where each read can
	// observe the latest write.
	SteppingDisabled SteppingMode = false

	// SteppingEnabled can be set to indicate that read operations
	// operate on a snapshot taken at the latest Step() invocation.
	SteppingEnabled SteppingMode = true
)

// SavepointToken represents a savepoint.
type SavepointToken interface {
	// Initial returns true if this savepoint has been created before performing
	// any KV operations. If so, it is possible to rollback to it after a
	// retriable error. If not, then rolling back to it after a retriable error
	// will return the retriable error again because reads might have been
	// evaluated before the savepoint and such reads cannot have their timestamp
	// forwarded without a refresh.
	Initial() bool
}

// TxnStatusOpt represents options for TxnSender.GetMeta().
type TxnStatusOpt int

const (
	// AnyTxnStatus means GetMeta() will return the info without checking the
	// txn's status.
	AnyTxnStatus TxnStatusOpt = iota
	// OnlyPending means GetMeta() will return an error if the transaction is not
	// in the pending state.
	// This is used when sending the txn from root to leaves so that we don't
	// create leaves that start up in an aborted state - which is not allowed.
	OnlyPending
)

// TxnSenderFactory is the interface used to create new instances
// of TxnSender.
type TxnSenderFactory interface {
	// TransactionalSender returns a sender to be used for transactional requests.
	// typ specifies whether the sender is the root or one of potentially many
	// child "leaf" nodes in a tree of transaction objects, as is created during a
	// DistSQL flow.
	// coordMeta is the TxnCoordMeta which contains the transaction whose requests
	// this sender will carry.
	TransactionalSender(typ TxnType, coordMeta roachpb.TxnCoordMeta) TxnSender
	// NonTransactionalSender returns a sender to be used for non-transactional
	// requests. Generally this is a sender that TransactionalSender() wraps.
	NonTransactionalSender() Sender
}

// SenderFunc is an adapter to allow the use of ordinary functions
// as Senders.
type SenderFunc func(context.Context, roachpb.BatchRequest) (*roachpb.BatchResponse, *roachpb.Error)

// Send calls f(ctx, c).
func (f SenderFunc) Send(
	ctx context.Context, ba roachpb.BatchRequest,
) (*roachpb.BatchResponse, *roachpb.Error) {
	return f(ctx, ba)
}

// MockTransactionalSender allows a function to be used as a TxnSender.
type MockTransactionalSender struct {
	senderFunc func(
		context.Context, *roachpb.Transaction, roachpb.BatchRequest,
	) (*roachpb.BatchResponse, *roachpb.Error)
	txn roachpb.Transaction
}

// NewMockTransactionalSender creates a MockTransactionalSender.
// The passed in txn is cloned.
func NewMockTransactionalSender(
	f func(
		context.Context, *roachpb.Transaction, roachpb.BatchRequest,
	) (*roachpb.BatchResponse, *roachpb.Error),
	txn *roachpb.Transaction,
) *MockTransactionalSender {
	return &MockTransactionalSender{senderFunc: f, txn: *txn}
}

// Send is part of the TxnSender interface.
func (m *MockTransactionalSender) Send(
	ctx context.Context, ba roachpb.BatchRequest,
) (*roachpb.BatchResponse, *roachpb.Error) {
	return m.senderFunc(ctx, &m.txn, ba)
}

// GetMeta is part of the TxnSender interface.
func (m *MockTransactionalSender) GetMeta(
	context.Context, TxnStatusOpt,
) (roachpb.TxnCoordMeta, error) {
	panic("unimplemented")
}

// AugmentMeta is part of the TxnSender interface.
func (m *MockTransactionalSender) AugmentMeta(context.Context, roachpb.TxnCoordMeta) {
	panic("unimplemented")
}

// OnFinish is part of the TxnSender interface.
func (m *MockTransactionalSender) OnFinish(f func(error)) {
	// We accept the nil, as that's commonly used to reset a previously-set
	// closure.
	if f != nil {
		panic("unimplemented")
	}
}

// SetSystemConfigTrigger is part of the TxnSender interface.
func (m *MockTransactionalSender) SetSystemConfigTrigger() error { panic("unimplemented") }

// TxnStatus is part of the TxnSender interface.
func (m *MockTransactionalSender) TxnStatus() roachpb.TransactionStatus {
	return m.txn.Status
}

// SetUserPriority is part of the TxnSender interface.
func (m *MockTransactionalSender) SetUserPriority(pri roachpb.UserPriority) error {
	m.txn.Priority = roachpb.MakePriority(pri)
	return nil
}

// SetDebugName is part of the TxnSender interface.
func (m *MockTransactionalSender) SetDebugName(name string) error {
	m.txn.Name = name
	return nil
}

// SetTxnMetaKeyIsSystemConfig is part of the TxnSender interface.
func (m *MockTransactionalSender) SetTxnMetaKeyIsSystemConfig() {
	log.Eventf(context.Background(), "TxnSender interface implementation")
}

//// OrigTimestamp is part of the TxnSender interface.
//func (m *MockTransactionalSender) OrigTimestamp() hlc.Timestamp {
//	return m.txn.OrigTimestamp
//}

//ReadTimestamp is part of the TxnSender interface.
func (m *MockTransactionalSender) ReadTimestamp() hlc.Timestamp {
	//ts := m.txn.OrigTimestamp
	//ts.Forward(m.txn.RefreshedTimestamp)
	//return ts
	//return m.txn.RefreshedTimestamp
	return m.txn.ReadTimestamp
}

// CommitTimestamp is part of the TxnSender interface.
func (m *MockTransactionalSender) CommitTimestamp() hlc.Timestamp {
	return m.txn.ReadTimestamp
}

// CommitTimestampFixed is part of the TxnSender interface.
func (m *MockTransactionalSender) CommitTimestampFixed() bool {
	panic("unimplemented")
}

// SetFixedTimestamp is part of the TxnSender interface.
func (m *MockTransactionalSender) SetFixedTimestamp(_ context.Context, ts hlc.Timestamp) {
	m.txn.WriteTimestamp = ts
	m.txn.ReadTimestamp = ts
	m.txn.MaxTimestamp = ts
	m.txn.CommitTimestampFixed = true
	m.txn.MinTimestamp.Backward(ts)
}

// SetTimeForRC is part of the client.TxnSender interface.
func (m *MockTransactionalSender) SetTimeForRC(ts hlc.Timestamp) {
	panic("unimplemented")
}

// ManualRestart is part of the TxnSender interface.
func (m *MockTransactionalSender) ManualRestart(
	ctx context.Context, pri roachpb.UserPriority, ts hlc.Timestamp,
) {
	m.txn.Restart(pri, 0 /* upgradePriority */, ts)
}

// IsSerializablePushAndRefreshNotPossible is part of the TxnSender interface.
func (m *MockTransactionalSender) IsSerializablePushAndRefreshNotPossible() bool {
	return false
}

// CreateSavepoint is part of the client.TxnSender interface.
func (m *MockTransactionalSender) CreateSavepoint(context.Context) (SavepointToken, error) {
	panic("unimplemented")
}

// RollbackToSavepoint is part of the client.TxnSender interface.
func (m *MockTransactionalSender) RollbackToSavepoint(context.Context, SavepointToken) error {
	panic("unimplemented")
}

// ReleaseSavepoint is part of the client.TxnSender interface.
func (m *MockTransactionalSender) ReleaseSavepoint(context.Context, SavepointToken) error {
	panic("unimplemented")
}

// Epoch is part of the TxnSender interface.
func (m *MockTransactionalSender) Epoch() enginepb.TxnEpoch { panic("unimplemented") }

// SerializeTxn is part of the TxnSender interface.
func (m *MockTransactionalSender) SerializeTxn() *roachpb.Transaction {
	return m.txn.Clone()
}

// Active is part of the TxnSender interface.
func (m *MockTransactionalSender) Active() bool {
	panic("unimplemented")
}

// UpdateStateOnRemoteRetryableErr is part of the TxnSender interface.
func (m *MockTransactionalSender) UpdateStateOnRemoteRetryableErr(
	ctx context.Context, pErr *roachpb.Error,
) *roachpb.Error {
	panic("unimplemented")
}

// DisablePipelining is part of the client.TxnSender interface.
func (m *MockTransactionalSender) DisablePipelining() error { return nil }

// Step is part of the client.TxnSender interface.
func (m *MockTransactionalSender) Step(_ context.Context) error {
	// At least one test (e.g sql/TestPortalsDestroyedOnTxnFinish) requires
	// the ability to run simple statements that do not access storage,
	// and that requires a non-panicky Step().
	return nil
}

// ConfigureStepping is part of the TxnSender interface.
func (m *MockTransactionalSender) ConfigureStepping(context.Context, SteppingMode) SteppingMode {
	// See Step() above.
	return SteppingDisabled
}

// GetSteppingMode is part of the TxnSender interface.
func (m *MockTransactionalSender) GetSteppingMode(context.Context) SteppingMode {
	return SteppingDisabled
}

// GetTxnSpans is part of the TxnSender interface.
func (m MockTransactionalSender) GetTxnSpans() TxnSpans {
	return TxnSpans{nil, nil}
}

// MockTxnSenderFactory is a TxnSenderFactory producing MockTxnSenders.
type MockTxnSenderFactory struct {
	senderFunc func(context.Context, *roachpb.Transaction, roachpb.BatchRequest) (
		*roachpb.BatchResponse, *roachpb.Error)
}

var _ TxnSenderFactory = MockTxnSenderFactory{}

// MakeMockTxnSenderFactory creates a MockTxnSenderFactory from a sender
// function that receives the transaction in addition to the request. The
// function is responsible for putting the txn inside the batch, if needed.
func MakeMockTxnSenderFactory(
	senderFunc func(
		context.Context, *roachpb.Transaction, roachpb.BatchRequest,
	) (*roachpb.BatchResponse, *roachpb.Error),
) MockTxnSenderFactory {
	return MockTxnSenderFactory{
		senderFunc: senderFunc,
	}
}

// TransactionalSender is part of TxnSenderFactory.
func (f MockTxnSenderFactory) TransactionalSender(
	_ TxnType, coordMeta roachpb.TxnCoordMeta,
) TxnSender {
	return NewMockTransactionalSender(f.senderFunc, &coordMeta.Txn)
}

// NonTransactionalSender is part of TxnSenderFactory.
func (f MockTxnSenderFactory) NonTransactionalSender() Sender {
	return nil
}

// NonTransactionalFactoryFunc is a TxnSenderFactory that cannot, in fact,
// create any transactional senders, only non-transactional ones.
type NonTransactionalFactoryFunc SenderFunc

var _ TxnSenderFactory = NonTransactionalFactoryFunc(nil)

// TransactionalSender is part of the TxnSenderFactory.
func (f NonTransactionalFactoryFunc) TransactionalSender(
	typ TxnType, _ roachpb.TxnCoordMeta,
) TxnSender {
	panic("not supported ")
}

// NonTransactionalSender is part of the TxnSenderFactory.
func (f NonTransactionalFactoryFunc) NonTransactionalSender() Sender {
	return SenderFunc(f)
}

// SendWrappedWith is a convenience function which wraps the request
// in a batch and sends it via the provided Sender and headers. It
// returns the unwrapped response or an error. It's valid to pass a
// `nil` context; an empty one is used in that case.
func SendWrappedWith(
	ctx context.Context, sender Sender, h roachpb.Header, args roachpb.Request,
) (roachpb.Response, *roachpb.Error) {
	ba := roachpb.BatchRequest{}
	ba.Header = h
	ba.Add(args)

	br, pErr := sender.Send(ctx, ba)
	if pErr != nil {
		return nil, pErr
	}
	unwrappedReply := br.Responses[0].GetInner()
	header := unwrappedReply.Header()
	header.Txn = br.Txn
	unwrappedReply.SetHeader(header)
	return unwrappedReply, nil
}

// SendWrapped is identical to SendWrappedWith with a zero header.
// TODO(tschottdorf): should move this to testutils and merge with
// other helpers which are used, for example, in `storage`.
func SendWrapped(
	ctx context.Context, sender Sender, args roachpb.Request,
) (roachpb.Response, *roachpb.Error) {
	return SendWrappedWith(ctx, sender, roachpb.Header{}, args)
}

// Wrap returns a Sender which applies the given function before delegating to
// the supplied Sender.
func Wrap(sender Sender, f func(roachpb.BatchRequest) roachpb.BatchRequest) Sender {
	return SenderFunc(func(ctx context.Context, ba roachpb.BatchRequest) (*roachpb.BatchResponse, *roachpb.Error) {
		return sender.Send(ctx, f(ba))
	})
}
