// Copyright 2016  The Cockroach Authors.
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
	"context"

	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/storage/abortspan"
	"github.com/znbasedb/znbase/pkg/storage/batcheval"
	"github.com/znbasedb/znbase/pkg/storage/concurrency"
	"github.com/znbasedb/znbase/pkg/storage/dumpsink"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/storage/spanset"
	"github.com/znbasedb/znbase/pkg/storage/storagebase"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/uuid"
)

// SpanSetReplicaEvalContext is a testing-only implementation of
// ReplicaEvalContext which verifies that access to state is registered in the
// SpanSet if one is given.
type SpanSetReplicaEvalContext struct {
	i  batcheval.EvalContext
	ss spanset.SpanSet
}

var _ batcheval.EvalContext = &SpanSetReplicaEvalContext{}

// AbortSpan returns the abort span.
func (rec *SpanSetReplicaEvalContext) AbortSpan() *abortspan.AbortSpan {
	return rec.i.AbortSpan()
}

// EvalKnobs returns the batch evaluation Knobs.
func (rec *SpanSetReplicaEvalContext) EvalKnobs() storagebase.BatchEvalTestingKnobs {
	return rec.i.EvalKnobs()
}

// StoreID returns the StoreID.
func (rec *SpanSetReplicaEvalContext) StoreID() roachpb.StoreID {
	return rec.i.StoreID()
}

// GetRangeID returns the RangeID.
func (rec *SpanSetReplicaEvalContext) GetRangeID() roachpb.RangeID {
	return rec.i.GetRangeID()
}

// ClusterSettings returns the cluster settings.
func (rec *SpanSetReplicaEvalContext) ClusterSettings() *cluster.Settings {
	return rec.i.ClusterSettings()
}

// Clock returns the Replica's clock.
func (rec *SpanSetReplicaEvalContext) Clock() *hlc.Clock {
	return rec.i.Clock()
}

// DB returns the Replica's client DB.
func (rec *SpanSetReplicaEvalContext) DB() *client.DB {
	return rec.i.DB()
}

// GetConcurrencyManager returns the txnwait.Queue.> GetConcurrencyManager returns the concurrency manager
func (rec *SpanSetReplicaEvalContext) GetConcurrencyManager() concurrency.Manager {
	return rec.i.GetConcurrencyManager()
}

//NodeID returns the NodeID.
func (rec *SpanSetReplicaEvalContext) NodeID() roachpb.NodeID {
	return rec.i.NodeID()
}

// Engine returns the engine.
func (rec *SpanSetReplicaEvalContext) Engine() engine.Engine {
	return rec.i.Engine()
}

// GetFirstIndex returns the first index.
func (rec *SpanSetReplicaEvalContext) GetFirstIndex() (uint64, error) {
	return rec.i.GetFirstIndex()
}

// GetTerm returns the term for the given index in the Raft log.
func (rec *SpanSetReplicaEvalContext) GetTerm(i uint64) (uint64, error) {
	return rec.i.GetTerm(i)
}

// GetLeaseAppliedIndex returns the lease index of the last applied command.
func (rec *SpanSetReplicaEvalContext) GetLeaseAppliedIndex() uint64 {
	return rec.i.GetLeaseAppliedIndex()
}

// IsFirstRange returns true iff the replica belongs to the first range.
func (rec *SpanSetReplicaEvalContext) IsFirstRange() bool {
	return rec.i.IsFirstRange()
}

// Desc returns the Replica's RangeDescriptor.
func (rec SpanSetReplicaEvalContext) Desc() *roachpb.RangeDescriptor {
	desc := rec.i.Desc()
	rec.ss.AssertAllowed(spanset.SpanReadOnly,
		roachpb.Span{Key: keys.RangeDescriptorKey(desc.StartKey)},
	)
	return desc
}

// ContainsKey returns true if the given key is within the Replica's range.
//
// TODO(bdarnell): Replace this method with one on Desc(). See comment
// on Replica.ContainsKey.
func (rec SpanSetReplicaEvalContext) ContainsKey(key roachpb.Key) bool {
	desc := rec.Desc() // already asserts
	return storagebase.ContainsKey(desc, key)
}

// GetMVCCStats returns the Replica's MVCCStats.
func (rec SpanSetReplicaEvalContext) GetMVCCStats() enginepb.MVCCStats {
	// Thanks to commutativity, the spanlatch manager does not have to serialize
	// on the MVCCStats key. This means that the key is not included in SpanSet
	// declarations, so there's nothing to assert here.
	return rec.i.GetMVCCStats()
}

// GetSplitQPS returns the Replica's queries/s rate for splitting purposes.
func (rec SpanSetReplicaEvalContext) GetSplitQPS() float64 {
	return rec.i.GetSplitQPS()
}

// CanCreateTxnRecord determines whether a transaction record can be created
// for the provided transaction information. See Replica.CanCreateTxnRecord
// for details about its arguments, return values, and preconditions.
func (rec SpanSetReplicaEvalContext) CanCreateTxnRecord(
	txnID uuid.UUID, txnKey []byte, txnMinTSUpperBound hlc.Timestamp,
) (bool, hlc.Timestamp, roachpb.TransactionAbortedReason) {
	rec.ss.AssertAllowed(spanset.SpanReadOnly,
		roachpb.Span{Key: keys.TransactionKey(txnKey, txnID)},
	)
	return rec.i.CanCreateTxnRecord(txnID, txnKey, txnMinTSUpperBound)
}

//GetMaxRead Returns the maximum read time between the start key and the end key
func (rec SpanSetReplicaEvalContext) GetMaxRead(start, end roachpb.Key) hlc.Timestamp {
	return rec.i.GetMaxRead(start, end)
}

// GetGCThreshold returns the GC threshold of the Range, typically updated when
// keys are garbage collected. Reads and writes at timestamps <= this time will
// not be served.
func (rec SpanSetReplicaEvalContext) GetGCThreshold() hlc.Timestamp {
	rec.ss.AssertAllowed(spanset.SpanReadOnly,
		roachpb.Span{Key: keys.RangeLastGCKey(rec.GetRangeID())},
	)
	return rec.i.GetGCThreshold()
}

// GetTxnSpanGCThreshold returns the time of the Replica's last
// transaction span GC.
func (rec SpanSetReplicaEvalContext) GetTxnSpanGCThreshold() hlc.Timestamp {
	rec.ss.AssertAllowed(spanset.SpanReadOnly,
		roachpb.Span{Key: keys.RangeTxnSpanGCThresholdKey(rec.GetRangeID())},
	)
	return rec.i.GetTxnSpanGCThreshold()
}

// String implements Stringer.
func (rec SpanSetReplicaEvalContext) String() string {
	return rec.i.String()
}

// GetLastReplicaGCTimestamp returns the last time the Replica was
// considered for GC.
func (rec SpanSetReplicaEvalContext) GetLastReplicaGCTimestamp(
	ctx context.Context,
) (hlc.Timestamp, error) {
	if err := rec.ss.CheckAllowed(spanset.SpanReadOnly,
		roachpb.Span{Key: keys.RangeLastReplicaGCTimestampKey(rec.GetRangeID())},
	); err != nil {
		return hlc.Timestamp{}, err
	}
	return rec.i.GetLastReplicaGCTimestamp(ctx)
}

// GetLease returns the Replica's current and next lease (if any).
func (rec SpanSetReplicaEvalContext) GetLease() (roachpb.Lease, roachpb.Lease) {
	rec.ss.AssertAllowed(spanset.SpanReadOnly,
		roachpb.Span{Key: keys.RangeLeaseKey(rec.GetRangeID())},
	)
	return rec.i.GetLease()
}

// IsEndKey is a part of the EvalContext interfaec.
func (rec SpanSetReplicaEvalContext) IsEndKey(key roachpb.Key) error {
	return rec.i.IsEndKey(key)
}

// GetLimiters returns the per-store limiters.
func (rec *SpanSetReplicaEvalContext) GetLimiters() *batcheval.Limiters {
	return rec.i.GetLimiters()
}

// GetDumpSink returns a DumpSink object, based on
// information parsed from a URI, stored in `dest`.
func (rec *SpanSetReplicaEvalContext) GetDumpSink(
	ctx context.Context, dest roachpb.DumpSink,
) (dumpsink.DumpSink, error) {
	return rec.i.GetDumpSink(ctx, dest)
}

// GetDumpSinkFromURI returns a DumpSink object, based on the given URI.
func (rec *SpanSetReplicaEvalContext) GetDumpSinkFromURI(
	ctx context.Context, uri string,
) (dumpsink.DumpSink, error) {
	return rec.i.GetDumpSinkFromURI(ctx, uri)
}

// GetExecCfg return An ExecutorConfig,encompasses the auxiliary objects and configuration
// required to create an executor.
func (rec *SpanSetReplicaEvalContext) GetExecCfg() (interface{}, error) {
	return rec.i.GetExecCfg()
}
