// Copyright 2014  The Cockroach Authors.
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

package storage

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/google/btree"
	"github.com/kr/pretty"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/rpc"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/storage/abortspan"
	"github.com/znbasedb/znbase/pkg/storage/batcheval"
	"github.com/znbasedb/znbase/pkg/storage/closedts/ctpb"
	"github.com/znbasedb/znbase/pkg/storage/concurrency"
	"github.com/znbasedb/znbase/pkg/storage/dumpsink"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/storage/rangefeed"
	"github.com/znbasedb/znbase/pkg/storage/spanset"
	"github.com/znbasedb/znbase/pkg/storage/split"
	"github.com/znbasedb/znbase/pkg/storage/stateloader"
	"github.com/znbasedb/znbase/pkg/storage/storagebase"
	"github.com/znbasedb/znbase/pkg/storage/storagepb"
	"github.com/znbasedb/znbase/pkg/storage/txnwait"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/envutil"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/humanizeutil"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/protoutil"
	"github.com/znbasedb/znbase/pkg/util/retry"
	"github.com/znbasedb/znbase/pkg/util/stop"
	"github.com/znbasedb/znbase/pkg/util/syncutil"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/util/tracing"
	"github.com/znbasedb/znbase/pkg/util/uuid"
	"go.etcd.io/etcd/raft"
)

const (
	// configGossipTTL is the time-to-live for configuration maps.

	// optimizePutThreshold is the minimum length of a contiguous run
	// of batched puts or conditional puts, after which the constituent
	// put operations will possibly be optimized by determining whether
	// the key space being written is starting out empty.
	optimizePutThreshold = 10

	replicaChangeTxnName = "change-replica"
	splitTxnName         = "split"
	mergeTxnName         = "merge"

	defaultReplicaRaftMuWarnThreshold = 500 * time.Millisecond
)

var testingDisableQuiescence = envutil.EnvOrDefaultBool("ZNBASE_DISABLE_QUIESCENCE", false)

var disableSyncRaftLog = settings.RegisterBoolSetting(
	"kv.raft_log.disable_synchronization_unsafe",
	"set to true to disable synchronization on Raft log writes to persistent storage. "+
		"Setting to true risks data loss or data corruption on server crashes. "+
		"The setting is meant for internal testing only and SHOULD NOT be used in production.",
	false,
)

// MaxCommandSizeFloor is the minimum allowed value for the MaxCommandSize
// cluster setting.
const MaxCommandSizeFloor = 4 << 20 // 4MB

// MaxCommandSize wraps "kv.raft.command.max_size".
var MaxCommandSize = settings.RegisterValidatedByteSizeSetting(
	"kv.raft.command.max_size",
	"maximum size of a raft command",
	64<<20,
	func(size int64) error {
		if size < MaxCommandSizeFloor {
			return fmt.Errorf("max_size must be greater than %s", humanizeutil.IBytes(MaxCommandSizeFloor))
		}
		return nil
	},
)

type proposalReevaluationReason int

const (
	proposalNoReevaluation proposalReevaluationReason = iota
	// proposalIllegalLeaseIndex indicates the proposal failed to apply at
	// a Lease index it was not legal for. The command should be re-evaluated.
	proposalIllegalLeaseIndex
)

type atomicDescString struct {
	strPtr unsafe.Pointer
}

// store atomically updates d.strPtr with the string representation of desc.
func (d *atomicDescString) store(replicaID roachpb.ReplicaID, desc *roachpb.RangeDescriptor) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d/", desc.RangeID)
	if replicaID == 0 {
		fmt.Fprintf(&buf, "?:")
	} else {
		fmt.Fprintf(&buf, "%d:", replicaID)
	}

	if !desc.IsInitialized() {
		buf.WriteString("{-}")
	} else {
		const maxRangeChars = 30
		rngStr := keys.PrettyPrintRange(roachpb.Key(desc.StartKey), roachpb.Key(desc.EndKey), maxRangeChars)
		buf.WriteString(rngStr)
	}

	str := buf.String()
	atomic.StorePointer(&d.strPtr, unsafe.Pointer(&str))
}

// String returns the string representation of the range; since we are not
// using a lock, the copy might be inconsistent.
func (d *atomicDescString) String() string {
	return *(*string)(atomic.LoadPointer(&d.strPtr))
}

// atomicConnectionClass stores an rpc.ConnectionClass atomically.
type atomicConnectionClass uint32

// get reads the current value of the ConnectionClass.
func (c *atomicConnectionClass) get() rpc.ConnectionClass {
	return rpc.ConnectionClass(atomic.LoadUint32((*uint32)(c)))
}

// set updates the current value of the ConnectionClass.
func (c *atomicConnectionClass) set(cc rpc.ConnectionClass) {
	atomic.StoreUint32((*uint32)(c), uint32(cc))
}

// A Replica is a contiguous keyspace with writes managed via an
// instance of the Raft consensus algorithm. Many ranges may exist
// in a store and they are unlikely to be contiguous. Ranges are
// independent units and are responsible for maintaining their own
// integrity by replacing failed replicas, splitting and merging
// as appropriate.
type Replica struct {
	log.AmbientContext

	// TODO(tschottdorf): Duplicates r.mu.state.desc.RangeID; revisit that.
	RangeID roachpb.RangeID // Only set by the constructor

	store     *Store
	abortSpan *abortspan.AbortSpan // Avoids anomalous reads after abort

	// leaseholderStats tracks all incoming BatchRequests to the replica and which
	// localities they come from in order to aid in lease rebalancing decisions.
	leaseholderStats *replicaStats
	// writeStats tracks the number of keys written by applied raft commands
	// in order to aid in replica rebalancing decisions.
	writeStats *replicaStats

	// creatingReplica is set when a replica is created as uninitialized
	// via a raft message.
	creatingReplica *roachpb.ReplicaDescriptor

	// Held in read mode during read-only commands. Held in exclusive mode to
	// prevent read-only commands from executing. Acquired before the embedded
	// RWMutex.
	readOnlyCmdMu syncutil.RWMutex

	// rangeStr is a string representation of a RangeDescriptor that can be
	// atomically read and updated without needing to acquire the replica.mu lock.
	// All updates to state.Desc should be duplicated here.
	rangeStr atomicDescString

	// connectionClass controls the ConnectionClass used to send raft messages.
	connectionClass atomicConnectionClass

	// schedulerCtx is a cached instance of an annotated Raft scheduler context.
	schedulerCtx atomic.Value // context.Context

	// raftMu protects Raft processing the replica.
	//
	// Locking notes: Replica.raftMu < Replica.mu
	raftMu struct {
		syncutil.Mutex

		// Note that there are two StateLoaders, in raftMu and mu,
		// depending on which lock is being held.
		stateLoader stateloader.StateLoader
		// on-disk storage for sideloaded SSTables. nil when there's no ReplicaID.
		sideloaded SideloadStorage

		// rangefeed is an instance of a rangefeed Processor that is capable of
		// routing rangefeed events to a set of subscribers. Will be nil if no
		// subscribers are registered.
		rangefeed *rangefeed.Processor
	}

	// Contains the lease history when enabled.
	leaseHistory *leaseHistory

	// concMgr sequences incoming requests and provides isolation between
	// requests that intend to perform conflicting operations. It is the
	// centerpiece of transaction contention handling.
	concMgr concurrency.Manager

	mu struct {
		// Protects all fields in the mu struct.
		syncutil.RWMutex
		// The destroyed status of a replica indicating if it's alive, corrupt,
		// scheduled for destruction or has been GCed.
		destroyStatus
		// Is the range quiescent? Quiescent ranges are not Tick()'d and unquiesce
		// whenever a Raft operation is performed.
		quiescent bool
		// mergeComplete is non-nil if a merge is in-progress, in which case any
		// requests should be held until the completion of the merge is signaled by
		// the closing of the channel.
		mergeComplete chan struct{}
		// The state of the Raft state machine.
		state storagepb.ReplicaState
		// Last index/term persisted to the raft log (not necessarily
		// committed). Note that lastTerm may be 0 (and thus invalid) even when
		// lastIndex is known, in which case the term will have to be retrieved
		// from the Raft log entry. Use the invalidLastTerm constant for this
		// case.
		lastIndex, lastTerm uint64
		// A map of raft log index of pending preemptive snapshots to deadlines.
		// Used to prohibit raft log truncations that would leave a gap between
		// the snapshot and the new first index. The map entry has a zero
		// deadline while the snapshot is being sent and turns nonzero when the
		// snapshot has completed, preventing truncation for a grace period
		// (since there is a race between the snapshot completing and its being
		// reflected in the raft status used to make truncation decisions).
		//
		// NB: If we kept only one value, we could end up in situations in which
		// we're either giving some snapshots no grace period, or keep an
		// already finished snapshot "pending" for extended periods of time
		// (preventing log truncation).
		snapshotLogTruncationConstraints map[uuid.UUID]snapTruncationInfo
		// raftLogSize is the approximate size in bytes of the persisted raft
		// log, including sideloaded entries' payloads. The value itself is not
		// persisted and is computed lazily, paced by the raft log truncation
		// queue which will recompute the log size when it finds it
		// uninitialized. This recomputation mechanism isn't relevant for ranges
		// which see regular write activity (for those the log size will deviate
		// from zero quickly, and so it won't be recomputed but will undercount
		// until the first truncation is carried out), but it prevents a large
		// dormant Raft log from sitting around forever, which has caused problems
		// in the past.
		raftLogSize int64
		// If raftLogSizeTrusted is false, don't trust the above raftLogSize until
		// it has been recomputed.
		raftLogSizeTrusted bool
		// raftLogLastCheckSize is the value of raftLogSize the last time the Raft
		// log was checked for truncation or at the time of the last Raft log
		// truncation.
		raftLogLastCheckSize int64
		// pendingLeaseRequest is used to coalesce RequestLease requests.
		pendingLeaseRequest pendingLeaseRequest
		// minLeaseProposedTS is the minimum acceptable lease.ProposedTS; only
		// leases proposed after this timestamp can be used for proposing commands.
		// This is used to protect against several hazards:
		// - leases held (or even proposed) before a restart cannot be used after a
		// restart. This is because:
		// 		a) the spanlatch manager is wiped during the restart; there might be
		// 		writes in flight that do not have the latches they held reflected. So,
		// 		we need to synchronize all new reads with those old in-flight writes.
		// 		Forcing acquisition of a new lease essentially flushes all the
		// 		previous raft commands.
		// 		b) a lease transfer might have been in progress at the time of the
		// 		restart. Using the existing lease after the restart would break the
		// 		transfer proposer's promise to not use the existing lease.
		// - a lease cannot be used after a transfer is initiated. Moreover, even
		// lease extension that were in flight at the time of the transfer cannot be
		// used, if they eventually apply.
		minLeaseProposedTS hlc.Timestamp
		// A pointer to the zone config for this replica.
		zone        *config.ZoneConfig
		locateSpace *roachpb.LocationValue
		// proposalBuf buffers Raft commands as they are passed to the Raft
		// replication subsystem. The buffer is populated by requests after
		// evaluation and is consumed by the Raft processing thread. Once
		// consumed, commands are proposed through Raft and moved to the
		// proposals map.
		proposalBuf propBuf
		// proposals stores the Raft in-flight commands which originated at
		// this Replica, i.e. all commands for which propose has been called,
		// but which have not yet applied.
		//
		// The *ProposalData in the map are "owned" by it. Elements from the
		// map must only be referenced while Replica.mu is held, except if the
		// element is removed from the map first.
		//
		// Due to Raft reproposals, multiple in-flight Raft entries can have
		// the same CmdIDKey, all corresponding to the same KV request. However,
		// not all Raft entries with a given command ID will correspond directly
		// to the *RaftCommand contained in its associated *ProposalData. This
		// is because the *RaftCommand can be mutated during reproposals by
		// Replica.tryReproposeWithNewLeaseIndex.
		proposals         map[storagebase.CmdIDKey]*ProposalData
		internalRaftGroup *raft.RawNode
		// The ID of the replica within the Raft group. May be 0 if the replica has
		// been created from a preemptive snapshot (i.e. before being added to the
		// Raft group). The replica ID will be non-zero whenever the replica is
		// part of a Raft group.
		replicaID roachpb.ReplicaID
		// The minimum allowed ID for this replica. Initialized from
		// RaftTombstone.NextReplicaID.
		minReplicaID roachpb.ReplicaID
		// The ID of the leader replica within the Raft group. Used to determine
		// when the leadership changes.
		leaderID roachpb.ReplicaID
		// The most recently added replica for the range and when it was added.
		// Used to determine whether a replica is new enough that we shouldn't
		// penalize it for being slightly behind. These field gets cleared out once
		// we know that the replica has caught up.
		lastReplicaAdded     roachpb.ReplicaID
		lastReplicaAddedTime time.Time
		// initialMaxClosed is the initial maxClosed timestamp for the replica as known
		// from its left-hand-side upon creation.
		initialMaxClosed hlc.Timestamp

		// The most recently updated time for each follower of this range. This is updated
		// every time a Raft message is received from a peer.
		// Note that superficially it seems that similar information is contained in the
		// Progress of a RaftStatus, which has a RecentActive field. However, that field
		// is always true unless CheckQuorum is active, which at the time of writing in
		// ZNBaseDB is not the case.
		//
		// The lastUpdateTimes map is also updated when a leaseholder steps up
		// (making the assumption that all followers are live at that point),
		// and when the range unquiesces (marking all replicating followers as
		// live).
		//
		// TODO(tschottdorf): keeping a map on each replica seems to be
		// overdoing it. We should map the replicaID to a NodeID and then use
		// node liveness (or any sensible measure of the peer being around).
		// The danger in doing so is that a single stuck replica on an otherwise
		// functioning node could fill up the quota pool. We are already taking
		// this kind of risk though: a replica that gets stuck on an otherwise
		// live node will not lose leaseholdership.
		lastUpdateTimes lastUpdateTimesMap

		// The last seen replica descriptors from incoming Raft messages. These are
		// stored so that the replica still knows the replica descriptors for itself
		// and for its message recipients in the circumstances when its RangeDescriptor
		// is out of date.
		//
		// Normally, a replica knows about the other replica descriptors for a
		// range via the RangeDescriptor stored in Replica.mu.state.Desc. But that
		// descriptor is only updated during a Split or ChangeReplicas operation.
		// There are periods during a Replica's lifetime when that information is
		// out of date:
		//
		// 1. When a replica is being newly created as the result of an incoming
		// Raft message for it. This is the common case for ChangeReplicas and an
		// uncommon case for Splits. The leader will be sending the replica
		// messages and the replica needs to be able to respond before it can
		// receive an updated range descriptor (via a snapshot,
		// changeReplicasTrigger, or splitTrigger).
		//
		// 2. If the node containing a replica is partitioned or down while the
		// replicas for the range are updated. When the node comes back up, other
		// replicas may begin communicating with it and it needs to be able to
		// respond. Unlike 1 where there is no range descriptor, in this situation
		// the replica has a range descriptor but it is out of date. Note that a
		// replica being removed from a node and then quickly re-added before the
		// replica has been GC'd will also use the last seen descriptors. In
		// effect, this is another path for which the replica's local range
		// descriptor is out of date.
		//
		// The last seen replica descriptors are updated on receipt of every raft
		// message via Replica.setLastReplicaDescriptors (see
		// Store.HandleRaftRequest). These last seen descriptors are used when
		// the replica's RangeDescriptor contains missing or out of date descriptors
		// for a replica (see Replica.sendRaftMessage).
		//
		// Removing a replica from Store.mu.replicas is not a problem because
		// when a replica is completely removed, it won't be recreated until
		// there is another event that will repopulate the replicas map in the
		// range descriptor. When it is temporarily dropped and recreated, the
		// newly recreated replica will have a complete range descriptor.
		lastToReplica, lastFromReplica roachpb.ReplicaDescriptor

		// Computed checksum at a snapshot UUID.
		checksums map[uuid.UUID]ReplicaChecksum

		// proposalQuota is the quota pool maintained by the lease holder where
		// incoming writes acquire quota from a fixed quota pool before going
		// through. If there is no quota available, the write is throttled
		// until quota is made available to the pool.
		// Acquired quota for a given command is only released when all the
		// replicas have persisted the corresponding entry into their logs.
		proposalQuota *quotaPool

		proposalQuotaBaseIndex uint64

		// Once the leader observes a proposal come 'out of Raft', we add the
		// size of the associated command to a queue of quotas we have yet to
		// release back to the quota pool. We only do so when all replicas have
		// persisted the corresponding entry into their logs.
		quotaReleaseQueue []int64

		// Counts calls to Replica.tick()
		ticks int

		// Counts Raft messages refused due to queue congestion.
		droppedMessages int

		// Note that there are two replicaStateLoaders, in raftMu and mu,
		// depending on which lock is being held.
		stateLoader stateloader.StateLoader

		// draining specifies whether this replica is draining. Raft leadership
		// transfers due to a lease change will be attempted even if the target does
		// not have all the log entries.
		draining bool
	}

	// Throttle how often we offer this Replica to the split and merge queues.
	// We have triggers downstream of Raft that do so based on limited
	// information and without explicit throttling some replicas will offer once
	// per applied Raft command, which is silly and also clogs up the queues'
	// semaphores.
	splitQueueThrottle, mergeQueueThrottle util.EveryN

	// loadBasedSplitter keeps information about load-based splitting.
	loadBasedSplitter split.Decider

	unreachablesMu struct {
		syncutil.Mutex
		remotes map[roachpb.ReplicaID]struct{}
	}
}

var _ batcheval.EvalContext = &Replica{}

// KeyRange is an interface type for the replicasByKey BTree, to compare
// Replica and ReplicaPlaceholder.
type KeyRange interface {
	Desc() *roachpb.RangeDescriptor
	rangeKeyItem
	btree.Item
	fmt.Stringer
}

var _ KeyRange = &Replica{}

var _ client.Sender = &Replica{}

// NewReplica initializes the replica using the given metadata. If the
// replica is initialized (i.e. desc contains more than a RangeID),
// replicaID should be 0 and the replicaID will be discovered from the
// descriptor.
func NewReplica(
	desc *roachpb.RangeDescriptor, store *Store, replicaID roachpb.ReplicaID,
) (*Replica, error) {
	r := newReplica(desc.RangeID, desc, store)
	return r, r.init(desc, store.Clock(), replicaID)
}

// Send executes a command on this range, dispatching it to the
// read-only, read-write, or admin execution path as appropriate.
// ctx should contain the log tags from the store (and up).
// TODO(nvanbenschoten): Move Replica.Send and it's callees into
// the new replica_send.go file once this review is complete.
func (r *Replica) Send(
	ctx context.Context, ba roachpb.BatchRequest,
) (*roachpb.BatchResponse, *roachpb.Error) {
	return r.sendWithRangeID(ctx, r.RangeID, &ba)
}

// sendWithRangeID takes an unused rangeID argument so that the range
// ID will be accessible in stack traces (both in panics and when
// sampling goroutines from a live server). This line is subject to
// the whims of the compiler and it can be difficult to find the right
// value, but as of this writing the following example shows a stack
// while processing range 21 (0x15) (the first occurrence of that
// number is the rangeID argument, the second is within the encoded
// BatchRequest, although we don't want to rely on that occurring
// within the portion printed in the stack trace):
//
// github.com/znbasedb/znbase/pkg/storage.(*Replica).sendWithRangeID(0xc420d1a000, 0x64bfb80, 0xc421564b10, 0x15, 0x153fd4634aeb0193, 0x0, 0x100000001, 0x1, 0x15, 0x0, ...)
func (r *Replica) sendWithRangeID(
	ctx context.Context, rangeID roachpb.RangeID, ba *roachpb.BatchRequest,
) (*roachpb.BatchResponse, *roachpb.Error) {
	var br *roachpb.BatchResponse
	if r.leaseholderStats != nil && ba.Header.GatewayNodeID != 0 {
		r.leaseholderStats.record(ba.Header.GatewayNodeID)
	}

	// Add the range log tag.
	ctx = r.AnnotateCtx(ctx)
	ctx, cleanup := tracing.EnsureContext(ctx, r.AmbientContext.Tracer, "replica send")
	defer cleanup()

	// If the internal Raft group is not initialized, create it and wake the leader.
	r.maybeInitializeRaftGroup(ctx)

	isReadOnly := ba.IsReadOnly()

	if err := r.checkBatchRequest(ba, isReadOnly); err != nil {
		return nil, roachpb.NewError(err)
	}

	if err := r.maybeBackpressureBatch(ctx, ba); err != nil {
		return nil, roachpb.NewError(err)
	}
	// NB: must be performed before collecting request spans.
	ba, err := maybeStripInFlightWrites(ba)
	if err != nil {
		return nil, roachpb.NewError(err)
	}

	if filter := r.store.cfg.TestingKnobs.TestingRequestFilter; filter != nil {
		if pErr := filter(*ba); pErr != nil {
			return nil, pErr
		}
	}

	// Differentiate between read-write, read-only and admin.
	var pErr *roachpb.Error
	if isReadOnly {
		log.Event(ctx, "read-only path")
		fn := (*Replica).executeReadOnlyBatch
		br, pErr = r.executeBatchWithConcurrencyRetries(ctx, ba, fn, isReadOnly)
	} else if ba.IsWrite() {
		log.Event(ctx, "read-write path")
		//br, pErr = r.executeWriteBatch(ctx, ba)
		fn := (*Replica).executeWriteBatch
		br, pErr = r.executeBatchWithConcurrencyRetries(ctx, ba, fn, isReadOnly)
	} else if ba.IsAdmin() {
		log.Event(ctx, "admin path")
		br, pErr = r.executeAdminBatch(ctx, ba)
	} else if len(ba.Requests) == 0 {
		// empty batch; shouldn't happen (we could handle it, but it hints
		// at someone doing weird things, and once we drop the key range
		// from the header it won't be clear how to route those requests).
		log.Fatalf(ctx, "empty batch")
	} else {
		log.Fatalf(ctx, "don't know how to handle command %s", ba)
	}
	if pErr != nil {
		log.Eventf(ctx, "replica.Send got error: %s", pErr)
	} else {
		if filter := r.store.cfg.TestingKnobs.TestingResponseFilter; filter != nil {
			pErr = filter(*ba, br)
		}
	}
	return br, pErr
}

// batchExecutionFn is a method on Replica that is able to execute a
// BatchRequest. It is called with the batch, along with the span bounds that
// the batch will operate over and a guard for the latches protecting the span
// bounds. The function must ensure that the latch guard is eventually released.
type batchExecutionFn func(
	*Replica, context.Context, *roachpb.BatchRequest, storagepb.LeaseStatus, *concurrency.Guard,
) (*roachpb.BatchResponse, *concurrency.Guard, *roachpb.Error)

var _ batchExecutionFn = (*Replica).executeWriteBatch
var _ batchExecutionFn = (*Replica).executeReadOnlyBatch

// executeBatchWithConcurrencyRetries is the entry point for client (non-admin)
// requests that execute against the range's state. The method coordinates the
// execution of requests that may require multiple retries due to interactions
// with concurrent transactions.
//
// The method acquires latches for the request, which synchronizes it with
// conflicting requests. This permits the execution function to run without
// concern of coordinating with logically conflicting operations, although it
// still needs to worry about coordinating with non-conflicting operations when
// accessing shared data structures.
//
// If the execution function hits a concurrency error like a WriteIntentError or
// a TransactionPushError it will propagate the error back to this method, which
// handles the process of retrying batch execution after addressing the error.
func (r *Replica) executeBatchWithConcurrencyRetries(
	ctx context.Context, ba *roachpb.BatchRequest, fn batchExecutionFn, isReadOnly bool,
) (br *roachpb.BatchResponse, pErr *roachpb.Error) {
	// Determine the maximal set of key spans that the batch will operate on.
	latchSpans, lockSpans, err := r.collectSpans(ba)
	if err != nil {
		return nil, roachpb.NewError(err)
	}
	//replication table read -48s timestamp for serving followRead
	if ba.Txn != nil {
		if t, ok := ba.Requests[0].GetInner().(*roachpb.ScanRequest); ok && t.ReplicationTable {
			backwardTimeStamp(ba, 48)
		}
	}

	// Handle load-based splitting.
	r.recordBatchForLoadBasedSplitting(ctx, ba, latchSpans)

	// TODO(nvanbenschoten): Clean this up once it's pulled inside the
	// concurrency manager.
	// Try to execute command; exit retry loop on success.
	var g *concurrency.Guard
	defer func() {
		// NB: wrapped to delay g evaluation to its value when returning.
		if g != nil {
			r.concMgr.FinishReq(g)
		}
	}()
	// Try to execute command; exit retry loop on success.
	for {
		// Exit loop if context has been canceled or timed out.
		if err := ctx.Err(); err != nil {
			return nil, roachpb.NewError(errors.Wrap(err, "aborted during Replica.Send"))
		}

		// Determine the lease under which to evaluate the request.
		var status storagepb.LeaseStatus
		if !ba.ReadConsistency.RequiresReadLease() {
			// Get a clock reading for checkExecutionCanProceed.
			status.Timestamp = r.Clock().Now()
		} else if ba.IsSingleSkipLeaseCheckRequest() {
			// For lease commands, use the provided previous lease for verification.
			status.Lease = ba.GetPrevLeaseForLeaseRequest()
			status.Timestamp = r.Clock().Now()
		} else {
			// If the request is a write or a consistent read, it requires the
			// range lease or permission to serve via follower reads.
			if status, pErr = r.redirectOnOrAcquireLease(ctx); pErr != nil {
				if nErr := r.canServeFollowerRead(ctx, ba, pErr); nErr != nil {
					return nil, nErr
				}
			}
		}
		// Limit the transaction's maximum timestamp using observed timestamps.
		r.limitTxnMaxTimestamp(ctx, ba, status)
		// Acquire latches to prevent overlapping requests from executing until
		// this request completes. After latching, wait on any conflicting locks
		// to ensure that the request has full isolation during evaluation. This
		// returns a request guard that must be eventually released.
		var resp []roachpb.ResponseUnion
		g, resp, pErr = r.concMgr.SequenceReq(ctx, g, concurrency.Request{
			Txn:             ba.Txn,
			Timestamp:       ba.Timestamp,
			Priority:        ba.UserPriority,
			ReadConsistency: ba.ReadConsistency,
			WaitPolicy:      ba.WaitPolicy,
			WaitTime:        ba.WaitTime.WallTime,
			Requests:        ba.Requests,
			LatchSpans:      latchSpans,
			LockSpans:       lockSpans,
			ReadOnly:        isReadOnly,
			BackFill:        ba.BackFill,
		})
		if pErr != nil {
			return nil, pErr
		} else if resp != nil {
			br = new(roachpb.BatchResponse)
			br.Responses = resp
			return br, nil
		}

		if filter := r.store.cfg.TestingKnobs.TestingLatchFilter; filter != nil {
			if pErr := filter(*ba); pErr != nil {
				return nil, pErr
			}
		}
		br, g, pErr = fn(r, ctx, ba, status, g)
		//replication table read +48s timestamp after serving followRead for compensation
		if br != nil {
			if t, ok := ba.Requests[0].GetInner().(*roachpb.ScanRequest); ok && t.ReplicationTable {
				forwardTimeStamp(br, 48)
			}
		}
		if pErr == nil {
			// Success.
			return br, nil
		} else if !isConcurrencyRetryError(pErr) {
			// Propagate error.
			return nil, pErr
		}

		// The batch execution func returned a server-side concurrency retry
		// error. It must have also handed back ownership of the concurrency
		// guard without having already released the guard's latches.
		g.AssertLatches()
		switch t := pErr.GetDetail().(type) {
		case *roachpb.WriteIntentError:
			if _, has := ba.GetArg(roachpb.DumpOnline); has {
				return nil, pErr
			}
			if g, pErr = r.handleWriteIntentError(ctx, ba, g, status.Lease, pErr, t); pErr != nil {
				return nil, pErr
			}
		case *roachpb.TransactionPushError:
			if g, pErr = r.handleTransactionPushError(ctx, ba, g, pErr, t); pErr != nil {
				return nil, pErr
			}
		case *roachpb.IndeterminateCommitError:
			r.concMgr.FinishReq(g)
			g = nil
			// Then launch a task to handle the indeterminate commit error.
			if pErr = r.handleIndeterminateCommitError(ctx, ba, pErr, t); pErr != nil {
				return nil, pErr
			}
		case *roachpb.MergeInProgressError:
			r.concMgr.FinishReq(g)
			g = nil
			// Then listen for the merge to complete.
			if pErr = r.handleMergeInProgressError(ctx, ba, pErr, t); pErr != nil {
				return nil, pErr
			}
		default:
			log.Fatalf(ctx, "unexpected concurrency retry error %T", t)
		}
		// Retry...
	}
}

// isConcurrencyRetryError returns whether or not the provided error is a
// "server-side concurrency retry error" that will be captured and retried by
// executeBatchWithConcurrencyRetries. Server-side concurrency retry errors are
// handled by dropping a request's latches, waiting for and/or ensuring that the
// condition which caused the error is handled, re-sequencing through the
// concurrency manager, and executing the request again.
func isConcurrencyRetryError(pErr *roachpb.Error) bool {
	switch pErr.GetDetail().(type) {
	case *roachpb.WriteIntentError:
		// If a request hits a WriteIntentError, it adds the conflicting intent
		// to the lockTable through a process called "lock discovery". It then
		// waits in the lock's wait-queue during its next sequencing pass.
	case *roachpb.TransactionPushError:
		// If a PushTxn request hits a TransactionPushError, it attempted to
		// push another transactions record but did not succeed. It enqueues the
		// pushee transaction in the txnWaitQueue and waits on the record to
		// change or expire during its next sequencing pass.
	case *roachpb.IndeterminateCommitError:
		// If a PushTxn hits a IndeterminateCommitError, it attempted to push an
		// expired transaction record in the STAGING state. It's unclear whether
		// the pushee is aborted or committed, so the request must kick off the
		// "transaction recovery procedure" to resolve this ambiguity before
		// retrying.
	case *roachpb.MergeInProgressError:
		// If a request hits a MergeInProgressError, the replica it is being
		// evaluted against is in the process of being merged into its left-hand
		// neighbor. The request cannot proceed until the range merge completes,
		// either successfully or unsuccessfully, so it waits before retrying.
		// If the merge does complete successfully, the retry will be rejected
		// with an error that will propagate back to the client.
	default:
		return false
	}
	return true
}

func (r *Replica) handleWriteIntentError(
	ctx context.Context,
	ba *roachpb.BatchRequest,
	g *concurrency.Guard,
	lease roachpb.Lease,
	pErr *roachpb.Error,
	t *roachpb.WriteIntentError,
) (*concurrency.Guard, *roachpb.Error) {
	if r.store.cfg.TestingKnobs.DontPushOnWriteIntentError {
		return g, pErr
	}
	// g's latches will be dropped, but it retains its spot in lock wait-queues.
	return r.concMgr.HandleWriterIntentError(ctx, g, lease.Sequence, t)
}

func (r *Replica) handleTransactionPushError(
	ctx context.Context,
	ba *roachpb.BatchRequest,
	g *concurrency.Guard,
	pErr *roachpb.Error,
	t *roachpb.TransactionPushError,
) (*concurrency.Guard, *roachpb.Error) {
	// On a transaction push error, retry immediately if doing so will enqueue
	// into the txnWaitQueue in order to await further updates to the unpushed
	// txn's status. We check ShouldPushImmediately to avoid retrying
	// non-queueable PushTxnRequests (see #18191).
	dontRetry := r.store.cfg.TestingKnobs.DontRetryPushTxnFailures
	if !dontRetry && ba.IsSinglePushTxnRequest() {
		pushReq := ba.Requests[0].GetInner().(*roachpb.PushTxnRequest)
		dontRetry = txnwait.ShouldPushImmediately(pushReq)
	}
	if dontRetry {
		return g, pErr
	}
	// g's latches will be dropped, but it retains its spot in lock wait-queues
	// (though a PushTxn shouldn't be in any lock wait-queues).
	return r.concMgr.HandleTransactionPushError(ctx, g, t), nil
}

func (r *Replica) handleIndeterminateCommitError(
	ctx context.Context,
	ba *roachpb.BatchRequest,
	pErr *roachpb.Error,
	t *roachpb.IndeterminateCommitError,
) *roachpb.Error {
	if r.store.cfg.TestingKnobs.DontRecoverIndeterminateCommits {
		return pErr
	}
	// On an indeterminate commit error, attempt to recover and finalize the
	// stuck transaction. Retry immediately if successful.
	if _, err := r.store.recoveryMgr.ResolveIndeterminateCommit(ctx, t); err != nil {
		// Do not propagate ambiguous results; assume success and retry original op.
		if _, ok := err.(*roachpb.AmbiguousResultError); ok {
			return nil
		}
		// Propagate new error. Preserve the error index.
		newPErr := roachpb.NewError(err)
		newPErr.Index = pErr.Index
		return newPErr
	}
	// We've recovered the transaction that blocked the push; retry command.
	return nil
}

func (r *Replica) handleMergeInProgressError(
	ctx context.Context,
	ba *roachpb.BatchRequest,
	pErr *roachpb.Error,
	t *roachpb.MergeInProgressError,
) *roachpb.Error {
	// A merge was in progress. We need to retry the command after the merge
	// completes, as signaled by the closing of the replica's mergeComplete
	// channel. Note that the merge may have already completed, in which case
	// its mergeComplete channel will be nil.
	mergeCompleteCh := r.getMergeCompleteCh()
	if mergeCompleteCh == nil {
		// Merge no longer in progress. Retry the command.
		return nil
	}
	log.Event(ctx, "waiting on in-progress merge")
	select {
	case <-mergeCompleteCh:
		// Merge complete. Retry the command.
		return nil
	case <-ctx.Done():
		return roachpb.NewError(errors.Wrap(ctx.Err(), "aborted during merge"))
	case <-r.store.stopper.ShouldQuiesce():
		return roachpb.NewError(&roachpb.NodeUnavailableError{})
	}
}

// String returns the string representation of the replica using an
// inconsistent copy of the range descriptor. Therefore, String does not
// require a lock and its output may not be atomic with other ongoing work in
// the replica. This is done to prevent deadlocks in logging sites.
func (r *Replica) String() string {
	return fmt.Sprintf("[n%d,s%d,r%s]", r.store.Ident.NodeID, r.store.Ident.StoreID, &r.rangeStr)
}

// ReplicaID returns the ID for the Replica. It may be zero if the replica does
// not know its ID. Once a Replica has a non-zero ReplicaID it will never change.
func (r *Replica) ReplicaID() roachpb.ReplicaID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mu.replicaID
}

// cleanupFailedProposal cleans up after a proposal that has failed. It
// clears any references to the proposal and releases associated quota.
func (r *Replica) cleanupFailedProposal(p *ProposalData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupFailedProposalLocked(p)
}

// cleanupFailedProposalLocked is like cleanupFailedProposal, but requires
// the Replica mutex to be exclusively held.
func (r *Replica) cleanupFailedProposalLocked(p *ProposalData) {
	// Clear the proposal from the proposals map. May be a no-op if the
	// proposal has not yet been inserted into the map.
	delete(r.mu.proposals, p.idKey)
	// Release associated quota pool resources if we have been tracking
	// this command.
	//
	// NB: We may be double free-ing here in cases where proposals are
	// duplicated. To counter this our quota pool is capped at the initial
	// quota size.
	if r.mu.proposalQuota != nil {
		r.mu.proposalQuota.add(p.quotaSize)
	}
}

// GetMinBytes gets the replica's minimum byte threshold.
func (r *Replica) GetMinBytes() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return *r.mu.zone.RangeMinBytes
}

// GetMaxBytes gets the replica's maximum byte threshold.
func (r *Replica) GetMaxBytes() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return *r.mu.zone.RangeMaxBytes
}

// SetZoneConfig sets the replica's zone config.
func (r *Replica) SetZoneConfig(zone *config.ZoneConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mu.zone = zone
}

// SetLocateSpace sets the replica's locateSpace.
func (r *Replica) SetLocateSpace(locateSpace *roachpb.LocationValue) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mu.locateSpace = locateSpace
}

// GetLocateSpace returns the locateSpace for the replica.
func (r *Replica) GetLocateSpace() *roachpb.LocationValue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mu.locateSpace
}

// IsFirstRange returns true if this is the first range.
func (r *Replica) IsFirstRange() bool {
	return r.RangeID == 1
}

// IsDestroyed returns a non-nil error if the replica has been destroyed
// and the reason if it has.
func (r *Replica) IsDestroyed() (DestroyReason, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isDestroyedRLocked()
}

func (r *Replica) isDestroyedRLocked() (DestroyReason, error) {
	return r.mu.destroyStatus.reason, r.mu.destroyStatus.err
}

// DescAndZone returns the authoritative range descriptor as well
// as the zone config for the replica.
func (r *Replica) DescAndZone() (*roachpb.RangeDescriptor, *config.ZoneConfig) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mu.state.Desc, r.mu.zone
}

// Desc returns the authoritative range descriptor, acquiring a replica lock in
// the process.
func (r *Replica) Desc() *roachpb.RangeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mu.state.Desc
}

// Zone returns the zone config for the replica.
func (r *Replica) Zone() *config.ZoneConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mu.zone
}

func (r *Replica) descRLocked() *roachpb.RangeDescriptor {
	return r.mu.state.Desc
}

// NodeID returns the ID of the node this replica belongs to.
func (r *Replica) NodeID() roachpb.NodeID {
	return r.store.nodeDesc.NodeID
}

// ClusterSettings returns the node's ClusterSettings.
func (r *Replica) ClusterSettings() *cluster.Settings {
	return r.store.cfg.Settings
}

// StoreID returns the Replica's StoreID.
func (r *Replica) StoreID() roachpb.StoreID {
	return r.store.StoreID()
}

// EvalKnobs returns the EvalContext's Knobs.
func (r *Replica) EvalKnobs() storagebase.BatchEvalTestingKnobs {
	return r.store.cfg.TestingKnobs.EvalKnobs
}

// Clock returns the hlc clock shared by this replica.
func (r *Replica) Clock() *hlc.Clock {
	return r.store.Clock()
}

// DB returns the Replica's client DB.
func (r *Replica) DB() *client.DB {
	return r.store.DB()
}

// Engine returns the Replica's underlying Engine. In most cases the
// evaluation Batch should be used instead.
func (r *Replica) Engine() engine.Engine {
	return r.store.Engine()
}

// AbortSpan returns the Replica's AbortSpan.
func (r *Replica) AbortSpan() *abortspan.AbortSpan {
	// Despite its name, the AbortSpan doesn't hold on-disk data in
	// memory. It just provides methods that take a Batch, so SpanSet
	// declarations are enforced there.
	return r.abortSpan
}

// GetLimiters returns the Replica's limiters.
func (r *Replica) GetLimiters() *batcheval.Limiters {
	return &r.store.limiters
}

// GetConcurrencyManager returns the Replica's concurrency.Manager.
func (r *Replica) GetConcurrencyManager() concurrency.Manager {
	return r.concMgr
}

// GetTerm returns the term of the given index in the raft log.
func (r *Replica) GetTerm(i uint64) (uint64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.raftTermRLocked(i)
}

// GetRangeID returns the Range ID.
func (r *Replica) GetRangeID() roachpb.RangeID {
	return r.RangeID
}

// GetGCThreshold returns the GC threshold.
func (r *Replica) GetGCThreshold() hlc.Timestamp {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return *r.mu.state.GCThreshold
}

// GetTxnSpanGCThreshold returns the time of the replica's last transaction span
// GC.
func (r *Replica) GetTxnSpanGCThreshold() hlc.Timestamp {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return *r.mu.state.TxnSpanGCThreshold
}

func maxReplicaID(desc *roachpb.RangeDescriptor) roachpb.ReplicaID {
	if desc == nil || !desc.IsInitialized() {
		return 0
	}
	var maxID roachpb.ReplicaID
	for _, repl := range desc.Replicas {
		if repl.ReplicaID > maxID {
			maxID = repl.ReplicaID
		}
	}
	return maxID
}

// LastReplicaAdded returns the ID of the most recently added replica and the
// time at which it was added.
func (r *Replica) LastReplicaAdded() (roachpb.ReplicaID, time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mu.lastReplicaAdded, r.mu.lastReplicaAddedTime
}

// GetReplicaDescriptor returns the replica for this range from the range
// descriptor. Returns a *RangeNotFoundError if the replica is not found.
// No other errors are returned.
func (r *Replica) GetReplicaDescriptor() (roachpb.ReplicaDescriptor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getReplicaDescriptorRLocked()
}

// getReplicaDescriptorRLocked is like getReplicaDescriptor, but assumes that
// r.mu is held for either reading or writing.
func (r *Replica) getReplicaDescriptorRLocked() (roachpb.ReplicaDescriptor, error) {
	repDesc, ok := r.mu.state.Desc.GetReplicaDescriptor(r.store.StoreID())
	if ok {
		return repDesc, nil
	}
	return roachpb.ReplicaDescriptor{}, roachpb.NewRangeNotFoundError(r.RangeID, r.store.StoreID())
}

func (r *Replica) getMergeCompleteCh() chan struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getMergeCompleteChRLocked()
}

func (r *Replica) getMergeCompleteChRLocked() chan struct{} {
	return r.mu.mergeComplete
}

// setLastReplicaDescriptors sets the the most recently seen replica
// descriptors to those contained in the *RaftMessageRequest, acquiring r.mu
// to do so.
func (r *Replica) setLastReplicaDescriptors(req *RaftMessageRequest) {
	r.mu.Lock()
	r.mu.lastFromReplica = req.FromReplica
	r.mu.lastToReplica = req.ToReplica
	r.mu.Unlock()
}

// GetMVCCStats returns a copy of the MVCC stats object for this range.
// This accessor is thread-safe, but provides no guarantees about its
// synchronization with any concurrent writes.
func (r *Replica) GetMVCCStats() enginepb.MVCCStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return *r.mu.state.Stats
}

// GetSplitQPS returns the Replica's queries/s request rate.
//
// NOTE: This should only be used for load based splitting, only
// works when the load based splitting cluster setting is enabled.
//
// Use QueriesPerSecond() for current QPS stats for all other purposes.
func (r *Replica) GetSplitQPS() float64 {
	return r.loadBasedSplitter.LastQPS(timeutil.Now())
}

// ContainsKey returns whether this range contains the specified key.
//
// TODO(bdarnell): This is not the same as RangeDescriptor.ContainsKey.
func (r *Replica) ContainsKey(key roachpb.Key) bool {
	return storagebase.ContainsKey(r.Desc(), key)
}

// ContainsKeyRange returns whether this range contains the specified
// key range from start to end.
func (r *Replica) ContainsKeyRange(start, end roachpb.Key) bool {
	return storagebase.ContainsKeyRange(*r.Desc(), start, end)
}

// GetLastReplicaGCTimestamp reads the timestamp at which the replica was
// last checked for removal by the replica gc queue.
func (r *Replica) GetLastReplicaGCTimestamp(ctx context.Context) (hlc.Timestamp, error) {
	key := keys.RangeLastReplicaGCTimestampKey(r.RangeID)
	var timestamp hlc.Timestamp
	_, err := engine.MVCCGetProto(ctx, r.store.Engine(), key, hlc.Timestamp{}, &timestamp,
		engine.MVCCGetOptions{})
	if err != nil {
		return hlc.Timestamp{}, err
	}
	return timestamp, nil
}

func (r *Replica) setLastReplicaGCTimestamp(ctx context.Context, timestamp hlc.Timestamp) error {
	key := keys.RangeLastReplicaGCTimestampKey(r.RangeID)
	return engine.MVCCPutProto(ctx, r.store.Engine(), nil, key, hlc.Timestamp{}, nil, &timestamp)
}

// getQueueLastProcessed returns the last processed timestamp for the
// specified queue, or the zero timestamp if not available.
func (r *Replica) getQueueLastProcessed(ctx context.Context, queue string) (hlc.Timestamp, error) {
	key := keys.QueueLastProcessedKey(r.Desc().StartKey, queue)
	var timestamp hlc.Timestamp
	if r.store != nil {
		_, err := engine.MVCCGetProto(ctx, r.store.Engine(), key, hlc.Timestamp{}, &timestamp,
			engine.MVCCGetOptions{})
		if err != nil {
			log.VErrEventf(ctx, 2, "last processed timestamp unavailable: %s", err)
			return hlc.Timestamp{}, err
		}
	}
	log.VEventf(ctx, 2, "last processed timestamp: %s", timestamp)
	return timestamp, nil
}

// setQueueLastProcessed writes the last processed timestamp for the
// specified queue.
func (r *Replica) setQueueLastProcessed(
	ctx context.Context, queue string, timestamp hlc.Timestamp,
) error {
	key := keys.QueueLastProcessedKey(r.Desc().StartKey, queue)
	return r.store.DB().PutInline(ctx, key, &timestamp)
}

// RaftStatus returns the current raft status of the replica. It returns nil
// if the Raft group has not been initialized yet.
func (r *Replica) RaftStatus() *raft.Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.raftStatusRLocked()
}

func (r *Replica) raftStatusRLocked() *raft.Status {
	if rg := r.mu.internalRaftGroup; rg != nil {
		s := rg.Status()
		return &s
	}
	return nil
}

// State returns a copy of the internal state of the Replica, along with some
// auxiliary information.
func (r *Replica) State() storagepb.RangeInfo {
	var ri storagepb.RangeInfo

	// NB: this acquires an RLock(). Reentrant RLocks are deadlock prone, so do
	// this first before RLocking below. Performance of this extra lock
	// acquisition is not a concern.
	ri.ActiveClosedTimestamp = r.maxClosed(context.Background())

	r.mu.RLock()
	defer r.mu.RUnlock()
	ri.ReplicaState = *(protoutil.Clone(&r.mu.state)).(*storagepb.ReplicaState)
	ri.LastIndex = r.mu.lastIndex
	ri.NumPending = uint64(r.numPendingProposalsRLocked())
	ri.RaftLogSize = r.mu.raftLogSize
	ri.RaftLogSizeTrusted = r.mu.raftLogSizeTrusted
	ri.NumDropped = uint64(r.mu.droppedMessages)
	if r.mu.proposalQuota != nil {
		ri.ApproximateProposalQuota = r.mu.proposalQuota.approximateQuota()
	}
	ri.RangeMaxBytes = *r.mu.zone.RangeMaxBytes
	if desc := ri.ReplicaState.Desc; desc != nil {
		for _, replDesc := range desc.Replicas {
			r.store.cfg.ClosedTimestamp.Storage.VisitDescending(replDesc.NodeID, func(e ctpb.Entry) (done bool) {
				mlai, found := e.MLAI[r.RangeID]
				if !found {
					return false // not done
				}
				if ri.NewestClosedTimestamp.ClosedTimestamp.Less(e.ClosedTimestamp) {
					ri.NewestClosedTimestamp.NodeID = replDesc.NodeID
					ri.NewestClosedTimestamp.MLAI = int64(mlai)
					ri.NewestClosedTimestamp.ClosedTimestamp = e.ClosedTimestamp
				}
				return true // done
			})
		}
	}
	return ri
}

// assertStateLocked can be called from the Raft goroutine to check that the
// in-memory and on-disk states of the Replica are congruent.
// Requires that both r.raftMu and r.mu are held.
//
// TODO(tschottdorf): Consider future removal (for example, when #7224 is resolved).
func (r *Replica) assertStateLocked(ctx context.Context, reader engine.Reader) {
	diskState, err := r.mu.stateLoader.Load(ctx, reader, r.mu.state.Desc)
	if err != nil {
		log.Fatal(ctx, err)
	}
	if !diskState.Equal(r.mu.state) {
		// The roundabout way of printing here is to expose this information in sentry.io.
		//
		// TODO(dt): expose properly once #15892 is addressed.
		log.Errorf(ctx, "on-disk and in-memory state diverged:\n%s", pretty.Diff(diskState, r.mu.state))
		r.mu.state.Desc, diskState.Desc = nil, nil
		log.Fatal(ctx, log.Safe(
			fmt.Sprintf("on-disk and in-memory state diverged: %s",
				pretty.Diff(diskState, r.mu.state)),
		))
	}
}

// checkExecutionCanProceed returns an error if a batch request cannot be
// executed by the Replica. An error indicates that the Replica is not live and
// able to serve traffic or that the request is not compatible with the state of
// the Range.
//
// The method accepts a concurrency Guard and a LeaseStatus parameter. These are
// used to indicate whether the caller has acquired latches and checked the
// Range lease. The method will only check for a pending merge if both of these
// conditions are true. If either !g.HoldingLatches() or st == nil then the
// method will not check for a pending merge. Callers might be ok with this if
// they know that they will end up checking for a pending merge at some later
// time.
func (r *Replica) checkExecutionCanProceed(
	ba *roachpb.BatchRequest, g *concurrency.Guard, st *storagepb.LeaseStatus,
) error {
	rSpan, err := keys.Range(ba.Requests)
	if err != nil {
		return err
	}
	r.mu.RLock()

	defer r.mu.RUnlock()
	if _, err := r.isDestroyedRLocked(); err != nil {
		return err
	} else if err := r.checkSpanInRangeRLocked(rSpan); err != nil {
		return err
	} else if err := r.checkTSAboveGCThresholdRLocked(ba.Timestamp); err != nil {
		return err
	} else if g.HoldingLatches() && st != nil {
		// Only check for a pending merge if latches are held and the Range
		// lease is held by this Replica. Without both of these conditions,
		// checkForPendingMergeRLocked could return false negatives.
		return r.checkForPendingMergeRLocked(ba)
	}
	return nil
}

// checkExecutionCanProceedForRangeFeed returns an error if a rangefeed request
// cannot be executed by the Replica.
func (r *Replica) checkExecutionCanProceedForRangeFeed(
	rSpan roachpb.RSpan, ts hlc.Timestamp,
) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, err := r.isDestroyedRLocked(); err != nil {
		return err
	} else if err := r.checkSpanInRangeRLocked(rSpan); err != nil {
		return err
	} else if err := r.checkTSAboveGCThresholdRLocked(ts); err != nil {
		return err
	} else if r.requiresExpiringLeaseRLocked() {
		// Ensure that the range does not require an expiration-based lease. If it
		// does, it will never get closed timestamp updates and the rangefeed will
		// never be able to advance its resolved timestamp.
		return errors.New("expiration-based leases are incompatible with rangefeeds")
	}
	return nil
}

// checkSpanInRangeRLocked returns an error if a request (identified by its
// key span) can be run on the replica.
func (r *Replica) checkSpanInRangeRLocked(rspan roachpb.RSpan) error {
	desc := r.mu.state.Desc
	if desc.ContainsKeyRange(rspan.Key, rspan.EndKey) {
		return nil
	}
	return roachpb.NewRangeKeyMismatchError(
		rspan.Key.AsRawKey(), rspan.EndKey.AsRawKey(), desc,
	)
}

// checkTSAboveGCThresholdRLocked returns an error if a request (identified
// by its MVCC timestamp) can be run on the replica.
func (r *Replica) checkTSAboveGCThresholdRLocked(ts hlc.Timestamp) error {
	threshold := r.mu.state.GCThreshold
	if threshold.Less(ts) {
		return nil
	}
	// allow query snapshot data
	_ = &roachpb.BatchTimestampBeforeGCError{
		Timestamp: ts,
		Threshold: *threshold,
	}
	return nil
}

// checkForPendingMergeRLocked determines whether the replica is being merged
// into its left-hand neighbor. If so, an error is returned to prevent the
// request from proceeding until the merge completes.
func (r *Replica) checkForPendingMergeRLocked(ba *roachpb.BatchRequest) error {
	if r.getMergeCompleteChRLocked() == nil {
		return nil
	}
	if ba.IsSingleSubsumeRequest() {
		return nil
	}
	// The replica is being merged into its left-hand neighbor. This request
	// cannot proceed until the merge completes, signaled by the closing of the
	// channel.
	//
	// It is very important that this check occur after we have acquired latches
	// from the spanlatch manager. Only after we release these latches are we
	// guaranteed that we're not racing with a Subsume command. (Subsume
	// commands declare a conflict with all other commands.) It is also
	// important that this check occur after we have verified that this replica
	// is the leaseholder. Only the leaseholder will have its merge complete
	// channel set.
	//
	// Note that Subsume commands are exempt from waiting on the mergeComplete
	// channel. This is necessary to avoid deadlock. While normally a Subsume
	// request will trigger the installation of a mergeComplete channel after it
	// is executed, it may sometimes execute after the mergeComplete channel has
	// been installed. Consider the case where the RHS replica acquires a new
	// lease after the merge transaction deletes its local range descriptor but
	// before the Subsume command is sent. The lease acquisition request will
	// notice the intent on the local range descriptor and install a
	// mergeComplete channel. If the forthcoming Subsume blocked on that
	// channel, the merge transaction would deadlock.
	//
	// This exclusion admits a small race condition. If a Subsume request is
	// sent to the right-hand side of a merge, outside of a merge transaction,
	// after the merge has committed but before the RHS has noticed that the
	// merge has committed, the request may return stale data. Since the merge
	// has committed, the LHS may have processed writes to the keyspace
	// previously owned by the RHS that the RHS is unaware of. This window
	// closes quickly, as the RHS will soon notice the merge transaction has
	// committed and mark itself as destroyed, which prevents it from serving
	// all traffic, including Subsume requests.
	//
	// In our current, careful usage of Subsume, this race condition is
	// irrelevant. Subsume is only sent from within a merge transaction, and
	// merge transactions read the RHS descriptor at the beginning of the
	// transaction to verify that it has not already been merged away.
	//
	// We can't wait for the merge to complete here, though. The replica might
	// need to respond to a Subsume request in order for the merge to complete,
	// and blocking here would force that Subsume request to sit in hold its
	// latches forever, deadlocking the merge. Instead, we release the latches
	// we acquired above and return a MergeInProgressError. The store will catch
	// that error and resubmit the request after mergeCompleteCh closes. See
	// #27442 for the full context.
	return &roachpb.MergeInProgressError{}
}

// checkBatchRequest verifies BatchRequest validity requirements. In particular,
// the batch must have an assigned timestamp, and either all requests must be
// read-only, or none.
//
// TODO(tschottdorf): should check that request is contained in range
// and that EndTransaction only occurs at the very end.
func (r *Replica) checkBatchRequest(ba *roachpb.BatchRequest, isReadOnly bool) error {
	if ba.Timestamp == (hlc.Timestamp{}) {
		// For transactional requests, Store.Send sets the timestamp. For non-
		// transactional requests, the client sets the timestamp. Either way, we
		// need to have a timestamp at this point.
		return errors.New("Replica.checkBatchRequest: batch does not have timestamp assigned")
	}
	consistent := ba.ReadConsistency == roachpb.CONSISTENT
	if isReadOnly {
		if !consistent && ba.Txn != nil {
			// Disallow any inconsistent reads within txns.
			return errors.Errorf("cannot allow %v reads within a transaction", ba.ReadConsistency)
		}
	} else if !consistent {
		return errors.Errorf("%v mode is only available to reads", ba.ReadConsistency)
	}

	return nil
}

// ec holds necessary information to end a batch after Raft
// command processing.
type endCmds struct {
	repl *Replica
	g    *concurrency.Guard
}

// move moves the ec into the return value, clearing and making
// a call to done on the receiver a no-op.
func (ec *endCmds) move() endCmds {
	res := *ec
	*ec = endCmds{}
	return res
}

// done releases the latches acquired by the command and updates
// the timestamp cache using the final timestamp of each command.
func (ec *endCmds) done(ba *roachpb.BatchRequest, br *roachpb.BatchResponse, pErr *roachpb.Error) {
	if ec.repl == nil {
		// The ec were cleared.
		return
	}
	defer ec.move() // clear

	// Update the timestamp cache if the request is not being re-evaluated. Each
	// request is considered in turn; only those marked as affecting the cache are
	// processed.
	ec.repl.updateTimestampCache(ba, br, pErr)

	// Release the latches acquired by the request and exit lock wait-queues.
	// Must be done AFTER the timestamp cache is updated. ec.g is only set when
	// the Raft proposal has assumed responsibility for the request.
	if ec.g != nil {
		ec.repl.concMgr.FinishReq(ec.g)
	}

}

func (r *Replica) collectSpans(
	ba *roachpb.BatchRequest,
) (latchSpans, lockSpans *spanset.SpanSet, _ error) {
	latchSpans, lockSpans = new(spanset.SpanSet), new(spanset.SpanSet)
	if _, ok := ba.GetArg(roachpb.DumpOnline); ok {
		latchSpans.Reserve(spanset.SpanReadOnly, spanset.SpanGlobal, len(ba.Requests))
		return latchSpans, lockSpans, nil
	}
	// TODO(bdarnell): need to make this less global when local
	// latches are used more heavily. For example, a split will
	// have a large read-only span but also a write (see #10084).
	// Currently local spans are the exception, so preallocate for the
	// common case in which all are global. We rarely mix read and
	// write commands, so preallocate for writes if there are any
	// writes present in the batch.
	//
	// TODO(bdarnell): revisit as the local portion gets its appropriate
	// use.
	if ba.IsLocking() {
		latchGuess := len(ba.Requests)
		if et, ok := ba.GetArg(roachpb.EndTransaction); ok {
			// EndTxn declares a global write for each of its lock spans.
			latchGuess += len(et.(*roachpb.EndTransactionRequest).LockSpans) - 1
		}
		latchSpans.Reserve(spanset.SpanReadWrite, spanset.SpanGlobal, latchGuess)
		lockSpans.Reserve(spanset.SpanReadWrite, spanset.SpanGlobal, len(ba.Requests))
	} else {
		latchSpans.Reserve(spanset.SpanReadOnly, spanset.SpanGlobal, len(ba.Requests))
		lockSpans.Reserve(spanset.SpanReadOnly, spanset.SpanGlobal, len(ba.Requests))
	}

	desc := r.Desc()
	batcheval.DeclareKeysForBatch(desc, ba.Header, latchSpans)
	for _, union := range ba.Requests {
		inner := union.GetInner()
		if cmd, ok := batcheval.LookupCommand(inner.Method()); ok {
			cmd.DeclareKeys(desc, ba.Header, inner, latchSpans, lockSpans)
		} else {
			return nil, nil, errors.Errorf("unrecognized command %s", inner.Method())
		}
	}

	// Commands may create a large number of duplicate spans. De-duplicate
	// them to reduce the number of spans we pass to the spanlatch manager.
	for _, s := range [...]*spanset.SpanSet{latchSpans, lockSpans} {
		s.SortAndDedup()

		// If any command gave us spans that are invalid, bail out early
		// (before passing them to the spanlatch manager, which may panic).
		if err := s.Validate(); err != nil {
			return nil, nil, err
		}
	}

	return latchSpans, lockSpans, nil
}

// beginCmds waits for any in-flight, conflicting commands to complete. More
// specifically, beginCmds acquires latches for the request based on keys
// affected by the batched commands. This gates subsequent commands with
// overlapping keys or key ranges. It returns a cleanup function to be called
// when the commands are done and can release their latches.

// executeAdminBatch executes the command directly. There is no interaction
// with the spanlatch manager or the timestamp cache, as admin commands
// are not meant to consistently access or modify the underlying data.
// Admin commands must run on the lease holder replica. Batch support here is
// limited to single-element batches; everything else catches an error.
func (r *Replica) executeAdminBatch(
	ctx context.Context, ba *roachpb.BatchRequest,
) (*roachpb.BatchResponse, *roachpb.Error) {
	if len(ba.Requests) != 1 {
		return nil, roachpb.NewErrorf("only single-element admin batches allowed")
	}

	args := ba.Requests[0].GetInner()
	if sp := opentracing.SpanFromContext(ctx); sp != nil {
		sp.SetOperationName(reflect.TypeOf(args).String())
	}

	// Admin commands always require the range lease.

	status, pErr := r.redirectOnOrAcquireLease(ctx)
	if pErr != nil {
		return nil, pErr
	}
	// Note there is no need to limit transaction max timestamp on admin requests.
	// Verify that the batch can be executed.
	// NB: we pass nil for the spanlatch guard because we haven't acquired
	// latches yet. This is ok because each individual request that the admin
	// request sends will acquire latches.
	if err := r.checkExecutionCanProceed(ba, nil /* lg */, &status); err != nil {
		return nil, roachpb.NewError(err)
	}

	var resp roachpb.Response
	switch tArgs := args.(type) {
	case *roachpb.AdminSplitRequest:
		var reply roachpb.AdminSplitResponse
		reply, pErr = r.AdminSplit(ctx, *tArgs, "manual", ba.BackFill)
		resp = &reply

	case *roachpb.AdminMergeRequest:
		var reply roachpb.AdminMergeResponse
		reply, pErr = r.AdminMerge(ctx, *tArgs, "manual", ba.BackFill)
		resp = &reply

	case *roachpb.AdminTransferLeaseRequest:
		pErr = roachpb.NewError(r.AdminTransferLease(ctx, tArgs.Target))
		resp = &roachpb.AdminTransferLeaseResponse{}

	case *roachpb.AdminChangeReplicasRequest:
		var err error
		expDesc := tArgs.ExpDesc
		if expDesc == nil {
			expDesc = r.Desc()
		}
		for _, target := range tArgs.Targets {
			// Update expDesc to the outcome of the previous run to enable detection
			// of concurrent updates while applying a series of changes.
			expDesc, err = r.ChangeReplicas(
				ctx, tArgs.ChangeType, target, expDesc, storagepb.ReasonAdminRequest, "", ba.BackFill)
			if err != nil {
				break
			}
		}
		pErr = roachpb.NewError(err)
		if err != nil {
			resp = &roachpb.AdminChangeReplicasResponse{}
		} else {
			resp = &roachpb.AdminChangeReplicasResponse{
				Desc: expDesc,
			}
		}

	case *roachpb.AdminRelocateRangeRequest:
		err := r.store.AdminRelocateRange(ctx, *r.Desc(), tArgs.Targets, ba.BackFill)
		pErr = roachpb.NewError(err)
		resp = &roachpb.AdminRelocateRangeResponse{}

	case *roachpb.CheckConsistencyRequest:
		var reply roachpb.CheckConsistencyResponse
		reply, pErr = r.CheckConsistency(ctx, *tArgs, ba.BackFill)
		resp = &reply

	case *roachpb.ImportRequest:
		cArgs := batcheval.CommandArgs{
			EvalCtx: NewReplicaEvalContext(r, &spanset.SpanSet{}),
			Header:  ba.Header,
			Args:    args,
		}
		var err error
		resp, err = importCmdFn(ctx, cArgs)
		pErr = roachpb.NewError(err)

	case *roachpb.LoadRequest:
		cArgs := batcheval.CommandArgs{
			EvalCtx: NewReplicaEvalContext(r, &spanset.SpanSet{}),
			Header:  ba.Header,
			Args:    args,
		}
		var err error
		resp, err = importCmdFn(ctx, cArgs)
		pErr = roachpb.NewError(err)

	case *roachpb.AdminScatterRequest:
		reply, err := r.adminScatter(ctx, *tArgs, ba.BackFill)
		pErr = roachpb.NewError(err)
		resp = &reply

	default:
		return nil, roachpb.NewErrorf("unrecognized admin command: %T", args)
	}

	if pErr != nil {
		return nil, pErr
	}

	if ba.Header.ReturnRangeInfo {
		returnRangeInfo(resp, r)
	}

	br := &roachpb.BatchResponse{}
	br.Add(resp)
	br.Txn = resp.Header().Txn
	return br, nil
}

// limitTxnMaxTimestamp limits the batch transaction's max timestamp
// so that it respects any timestamp already observed on this node.
// This prevents unnecessary uncertainty interval restarts caused by
// reading a value written at a timestamp between txn.Timestamp and
// txn.MaxTimestamp. The replica lease's start time is also taken into
// consideration to ensure that a lease transfer does not result in
// the observed timestamp for this node being inapplicable to data
// previously written by the former leaseholder. To wit:
//
// 1. put(k on leaseholder n1), gateway chooses t=1.0
// 2. begin; read(unrelated key on n2); gateway chooses t=0.98
// 3. pick up observed timestamp for n2 of t=0.99
// 4. n1 transfers lease for range with k to n2 @ t=1.1
// 5. read(k) on leaseholder n2 at ReadTimestamp=0.98 should get
//    ReadWithinUncertaintyInterval because of the write in step 1, so
//    even though we observed n2's timestamp in step 3 we must expand
//    the uncertainty interval to the lease's start time, which is
//    guaranteed to be greater than any write which occurred under
//    the previous leaseholder.
func (r *Replica) limitTxnMaxTimestamp(
	ctx context.Context, ba *roachpb.BatchRequest, status storagepb.LeaseStatus,
) {
	if ba.Txn == nil {
		return
	}
	// For calls that read data within a txn, we keep track of timestamps
	// observed from the various participating nodes' HLC clocks. If we have
	// a timestamp on file for this Node which is smaller than MaxTimestamp,
	// we can lower MaxTimestamp accordingly. If MaxTimestamp drops below
	// ReadTimestamp, we effectively can't see uncertainty restarts anymore.
	obsTS, ok := ba.Txn.GetObservedTimestamp(ba.Replica.NodeID)
	if !ok {
		return
	}
	// If the lease is valid, we use the greater of the observed
	// timestamp and the lease start time, up to the max timestamp. This
	// ensures we avoid incorrect assumptions about when data was
	// written, in absolute time on a different node, which held the
	// lease before this replica acquired it.
	if status.State == storagepb.LeaseState_VALID {
		obsTS.Forward(status.Lease.Start)
	}
	if obsTS.Less(ba.Txn.MaxTimestamp) {
		// Copy-on-write to protect others we might be sharing the Txn with.
		txnClone := ba.Txn.Clone()
		// The uncertainty window is [ReadTimestamp, maxTS), so if that window
		// is empty, there won't be any uncertainty restarts.
		if !ba.Txn.ReadTimestamp.Less(obsTS) {
			log.Event(ctx, "read has no clock uncertainty")
		}
		txnClone.MaxTimestamp.Backward(obsTS)
		ba.Txn = txnClone
	}
}

// maybeWatchForMerge checks whether a merge of this replica into its left
// neighbor is in its critical phase and, if so, arranges to block all requests
// until the merge completes.
func (r *Replica) maybeWatchForMerge(ctx context.Context) error {
	desc := r.Desc()
	descKey := keys.RangeDescriptorKey(desc.StartKey)
	_, intent, err := engine.MVCCGet(ctx, r.Engine(), descKey, r.Clock().Now(),
		engine.MVCCGetOptions{Inconsistent: true})
	if err != nil {
		return err
	} else if intent == nil {
		return nil
	}
	val, _, err := engine.MVCCGetAsTxn(
		ctx, r.Engine(), descKey, intent.Txn.WriteTimestamp, intent.Txn)
	if err != nil {
		return err
	} else if val != nil {
		return nil
	}

	// At this point, we know we have a deletion intent on our range descriptor.
	// That means a merge is in progress. Block all commands until we can
	// retrieve an updated range descriptor from meta2, which will indicate
	// whether the merge succeeded or not.

	mergeCompleteCh := make(chan struct{})
	r.mu.Lock()
	if r.mu.mergeComplete != nil {
		// Another request already noticed the merge, installed a mergeComplete
		// channel, and launched a goroutine to watch for the merge's completion.
		// Nothing more to do.
		r.mu.Unlock()
		return nil
	}
	r.mu.mergeComplete = mergeCompleteCh
	// The RHS of a merge is not permitted to quiesce while a mergeComplete
	// channel is installed. (If the RHS is quiescent when the merge commits, any
	// orphaned followers would fail to queue themselves for GC.) Unquiesce the
	// range in case it managed to quiesce between when the Subsume request
	// arrived and now, which is rare but entirely legal.
	r.unquiesceLocked()
	r.mu.Unlock()

	taskCtx := r.AnnotateCtx(context.Background())
	err = r.store.stopper.RunAsyncTask(taskCtx, "wait-for-merge", func(ctx context.Context) {
		var pushTxnRes *roachpb.PushTxnResponse
		for retry := retry.Start(base.DefaultRetryOptions()); retry.Next(); {
			// Wait for the merge transaction to complete by attempting to push it. We
			// don't want to accidentally abort the merge transaction, so we use the
			// minimum transaction priority. Note that a push type of
			// roachpb.PUSH_TOUCH, though it might appear more semantically correct,
			// returns immediately and causes us to spin hot, whereas
			// roachpb.PUSH_ABORT efficiently blocks until the transaction completes.
			b := &client.Batch{}
			b.Header.Timestamp = r.Clock().Now()
			b.AddRawRequest(&roachpb.PushTxnRequest{
				RequestHeader: roachpb.RequestHeader{Key: intent.Txn.Key},
				PusherTxn: roachpb.Transaction{
					TxnMeta: enginepb.TxnMeta{Priority: enginepb.MinTxnPriority},
				},
				PusheeTxn:       intent.Txn,
				PushType:        roachpb.PUSH_ABORT,
				InclusivePushTo: true,
			})
			if err := r.DB().Run(ctx, b); err != nil {
				select {
				case <-r.store.stopper.ShouldQuiesce():
					// The server is shutting down. The error while pushing the
					// transaction was probably caused by the shutdown, so ignore it.
					return
				default:
					log.Warningf(ctx, "error while watching for merge to complete: PushTxn: %s", err)
					// We can't safely unblock traffic until we can prove that the merge
					// transaction is committed or aborted. Nothing to do but try again.
					continue
				}
			}
			pushTxnRes = b.RawResponse().Responses[0].GetInner().(*roachpb.PushTxnResponse)
			break
		}

		var mergeCommitted bool
		switch pushTxnRes.PusheeTxn.Status {
		case roachpb.PENDING, roachpb.STAGING:
			log.Fatalf(ctx, "PushTxn returned while merge transaction %s was still %s",
				intent.Txn.ID.Short(), pushTxnRes.PusheeTxn.Status)
		case roachpb.COMMITTED:
			// If PushTxn claims that the transaction committed, then the transaction
			// definitely committed.
			mergeCommitted = true
		case roachpb.ABORTED:
			// If PushTxn claims that the transaction aborted, it's not a guarantee
			// that the transaction actually aborted. It could also mean that the
			// transaction completed, resolved its intents, and GC'd its transaction
			// record before our PushTxn arrived. To figure out what happened, we
			// need to look in meta2.
			var getRes *roachpb.GetResponse
			for retry := retry.Start(base.DefaultRetryOptions()); retry.Next(); {
				metaKey := keys.RangeMetaKey(desc.EndKey)
				res, pErr := client.SendWrappedWith(ctx, r.DB().NonTransactionalSender(), roachpb.Header{
					// Use READ_UNCOMMITTED to avoid trying to resolve intents, since
					// resolving those intents might involve sending requests to this
					// range, and that could deadlock. See the comment on
					// TestStoreRangeMergeConcurrentSplit for details.
					ReadConsistency: roachpb.READ_UNCOMMITTED,
				}, &roachpb.GetRequest{
					RequestHeader: roachpb.RequestHeader{Key: metaKey.AsRawKey()},
				})
				if pErr != nil {
					select {
					case <-r.store.stopper.ShouldQuiesce():
						// The server is shutting down. The error while fetching the range
						// descriptor was probably caused by the shutdown, so ignore it.
						return
					default:
						log.Warningf(ctx, "error while watching for merge to complete: Get %s: %s", metaKey, pErr)
						// We can't safely unblock traffic until we can prove that the merge
						// transaction is committed or aborted. Nothing to do but try again.
						continue
					}
				}
				getRes = res.(*roachpb.GetResponse)
				break
			}
			if getRes.Value == nil {
				// A range descriptor with our end key is no longer present in meta2, so
				// the merge must have committed.
				mergeCommitted = true
			} else {
				// A range descriptor with our end key is still present in meta2. The
				// merge committed iff that range descriptor has a different range ID.
				var meta2Desc roachpb.RangeDescriptor
				if err := getRes.Value.GetProto(&meta2Desc); err != nil {
					log.Fatalf(ctx, "error while watching for merge to complete: "+
						"unmarshaling meta2 range descriptor: %s", err)
				}
				if meta2Desc.RangeID != r.RangeID {
					mergeCommitted = true
				}
			}
		}

		r.mu.Lock()
		if mergeCommitted && r.mu.destroyStatus.IsAlive() {
			// The merge committed but the left-hand replica on this store hasn't
			// subsumed this replica yet. Mark this replica as destroyed so it
			// doesn't serve requests when we close the mergeCompleteCh below.
			r.mu.destroyStatus.Set(roachpb.NewRangeNotFoundError(r.RangeID, r.store.StoreID()), destroyReasonMergePending)
		}
		// Unblock pending requests. If the merge committed, the requests will
		// notice that the replica has been destroyed and return an appropriate
		// error. If the merge aborted, the requests will be handled normally.
		r.mu.mergeComplete = nil
		close(mergeCompleteCh)
		r.mu.Unlock()
	})
	if err == stop.ErrUnavailable {
		// We weren't able to launch a goroutine to watch for the merge's completion
		// because the server is shutting down. Normally failing to launch the
		// watcher goroutine would wedge pending requests on the replica's
		// mergeComplete channel forever, but since we're shutting down those
		// requests will get dropped and retried on another node. Suppress the error.
		err = nil
	}
	return err
}

func (r *Replica) maybeTransferRaftLeadership(ctx context.Context) {
	r.mu.Lock()
	r.maybeTransferRaftLeadershipLocked(ctx)
	r.mu.Unlock()
}

// maybeTransferRaftLeadershipLocked attempts to transfer the leadership away
// from this node to the leaseholder, if this node is the current raft leader
// but not the leaseholder. We don't attempt to transfer leadership if the
// leaseholder is behind on applying the log.
//
// We like it when leases and raft leadership are collocated because that
// facilitates quick command application (requests generally need to make it to
// both the lease holder and the raft leader before being applied by other
// replicas).
func (r *Replica) maybeTransferRaftLeadershipLocked(ctx context.Context) {
	if r.store.TestingKnobs().DisableLeaderFollowsLeaseholder {
		return
	}
	lease := *r.mu.state.Lease
	if lease.OwnedBy(r.StoreID()) || !r.isLeaseValidRLocked(lease, r.Clock().Now()) {
		return
	}
	raftStatus := r.raftStatusRLocked()
	if raftStatus == nil || raftStatus.RaftState != raft.StateLeader {
		return
	}
	lhReplicaID := uint64(lease.Replica.ReplicaID)
	lhProgress, ok := raftStatus.Progress[lhReplicaID]
	if (ok && lhProgress.Match >= raftStatus.Commit) || r.mu.draining {
		log.VEventf(ctx, 1, "transferring raft leadership to replica ID %v", lhReplicaID)
		r.store.metrics.RangeRaftLeaderTransfers.Inc(1)
		r.mu.internalRaftGroup.TransferLeader(lhReplicaID)
	}
}

func (r *Replica) mergeInProgressRLocked() bool {
	return r.mu.mergeComplete != nil
}

func (r *Replica) getReplicaDescriptorByIDRLocked(
	replicaID roachpb.ReplicaID, fallback roachpb.ReplicaDescriptor,
) (roachpb.ReplicaDescriptor, error) {
	if repDesc, ok := r.mu.state.Desc.GetReplicaDescriptorByID(replicaID); ok {
		return repDesc, nil
	}
	if fallback.ReplicaID == replicaID {
		return fallback, nil
	}
	return roachpb.ReplicaDescriptor{},
		errors.Errorf("replica %d not present in %v, %v", replicaID, fallback, r.mu.state.Desc.Replicas)
}

// checkIfTxnAborted checks the txn AbortSpan for the given
// transaction. In case the transaction has been aborted, return a
// transaction abort error.
func checkIfTxnAborted(
	ctx context.Context, rec batcheval.EvalContext, b engine.Reader, txn roachpb.Transaction,
) *roachpb.Error {
	var entry roachpb.AbortSpanEntry
	aborted, err := rec.AbortSpan().Get(ctx, b, txn.ID, &entry)
	if err != nil {
		return roachpb.NewError(roachpb.NewReplicaCorruptionError(
			errors.Wrap(err, "could not read from AbortSpan")))
	}
	if aborted {
		// We hit the cache, so let the transaction restart.
		log.VEventf(ctx, 1, "found AbortSpan entry for %s with priority %d",
			txn.ID.Short(), entry.Priority)
		newTxn := txn.Clone()
		if entry.Priority > newTxn.Priority {
			newTxn.Priority = entry.Priority
		}
		newTxn.Status = roachpb.ABORTED
		return roachpb.NewErrorWithTxn(
			roachpb.NewTransactionAbortedError(roachpb.ABORT_REASON_ABORT_SPAN), newTxn)
	}
	return nil
}

func (r *Replica) startKey() roachpb.RKey {
	return r.Desc().StartKey
}

// Less implements the btree.Item interface.
func (r *Replica) Less(i btree.Item) bool {
	return r.startKey().Less(i.(rangeKeyItem).startKey())
}

// GetLeaseHistory returns the lease history stored on this replica.
func (r *Replica) GetLeaseHistory() []roachpb.Lease {
	if r.leaseHistory == nil {
		return nil
	}

	return r.leaseHistory.get()
}

// EnableLeaseHistory turns on the lease history for testing purposes. Returns
// a function to return it to its original state that can be deferred.
func EnableLeaseHistory(maxEntries int) func() {
	originalValue := leaseHistoryMaxEntries
	leaseHistoryMaxEntries = maxEntries
	return func() {
		leaseHistoryMaxEntries = originalValue
	}
}

// GetDumpSink returns an ExternalStorage object, based on
// information parsed from a URI, stored in `dest`.
func (r *Replica) GetDumpSink(
	ctx context.Context, dest roachpb.DumpSink,
) (dumpsink.DumpSink, error) {
	return r.store.cfg.DumpSink(ctx, dest)
}

// GetDumpSinkFromURI returns an ExternalStorage object, based on the given URI.
func (r *Replica) GetDumpSinkFromURI(ctx context.Context, uri string) (dumpsink.DumpSink, error) {
	return r.store.cfg.DumpSinkFromURI(ctx, uri)
}

// GetExecCfg return An ExecutorConfig,encompasses the auxiliary objects and configuration
// required to create an executor.
func (r *Replica) GetExecCfg() (interface{}, error) {
	return r.store.cfg.SQLExecutor.GetExecCfg()
}

func init() {
	tracing.RegisterTagRemapping("r", "range")
}
