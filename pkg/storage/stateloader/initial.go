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

package stateloader

import (
	"context"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/storage/storagepb"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// raftInitialLog{Index,Term} are the starting points for the raft log. We
// bootstrap the raft membership by synthesizing a snapshot as if there were
// some discarded prefix to the log, so we must begin the log at an arbitrary
// index greater than 1.
const (
	raftInitialLogIndex = 10
	raftInitialLogTerm  = 5
)

// WriteInitialReplicaState sets up a new Range, but without writing an
// associated Raft state (which must be written separately via
// synthesizeRaftState before instantiating a Replica). The main task is to
// persist a ReplicaState which does not start from zero but presupposes a few
// entries already having applied. The supplied MVCCStats are used for the Stats
// field after adjusting for persisting the state itself, and the updated stats
// are returned.
//
// Args:
// activeVersion: The cluster's version.
func WriteInitialReplicaState(
	ctx context.Context,
	eng engine.ReadWriter,
	ms enginepb.MVCCStats,
	desc roachpb.RangeDescriptor,
	lease roachpb.Lease,
	gcThreshold hlc.Timestamp,
	txnSpanGCThreshold hlc.Timestamp,
	activeVersion roachpb.Version,
	truncStateType TruncatedStateType,
) (enginepb.MVCCStats, error) {
	rsl := Make(desc.RangeID)
	// NB: be careful using activeVersion here. One caller of this code is the
	// split trigger, and the version with which the split trigger is called can
	// vary across followers. Thus, actions which require coordination cannot
	// use the version as a trigger (this is why this method takes a
	// truncStateType argument).

	var s storagepb.ReplicaState
	s.TruncatedState = &roachpb.RaftTruncatedState{
		Term:  raftInitialLogTerm,
		Index: raftInitialLogIndex,
	}
	s.RaftAppliedIndex = s.TruncatedState.Index
	s.Desc = &roachpb.RangeDescriptor{
		RangeID: desc.RangeID,
	}
	s.Stats = &ms
	s.Lease = &lease
	s.GCThreshold = &gcThreshold
	s.TxnSpanGCThreshold = &txnSpanGCThreshold

	// If the version is high enough to guarantee that all nodes will understand
	// the AppliedStateKey then we can just straight to using it without ever
	// writing the legacy stats and index keys.
	if !activeVersion.Less(cluster.VersionByKey(cluster.VersionRangeAppliedStateKey)) {
		s.UsingAppliedStateKey = true
	} else {
		if err := engine.AccountForLegacyMVCCStats(s.Stats, desc.RangeID); err != nil {
			return enginepb.MVCCStats{}, err
		}
	}

	if existingLease, err := rsl.LoadLease(ctx, eng); err != nil {
		return enginepb.MVCCStats{}, errors.Wrap(err, "error reading lease")
	} else if (existingLease != roachpb.Lease{}) {
		log.Fatalf(ctx, "expected trivial lease, but found %+v", existingLease)
	}

	if existingGCThreshold, err := rsl.LoadGCThreshold(ctx, eng); err != nil {
		return enginepb.MVCCStats{}, errors.Wrap(err, "error reading GCThreshold")
	} else if (*existingGCThreshold != hlc.Timestamp{}) {
		log.Fatalf(ctx, "expected trivial GChreshold, but found %+v", existingGCThreshold)
	}

	if existingTxnSpanGCThreshold, err := rsl.LoadTxnSpanGCThreshold(ctx, eng); err != nil {
		return enginepb.MVCCStats{}, errors.Wrap(err, "error reading TxnSpanGCThreshold")
	} else if (*existingTxnSpanGCThreshold != hlc.Timestamp{}) {
		log.Fatalf(ctx, "expected trivial TxnSpanGCThreshold, but found %+v", existingTxnSpanGCThreshold)
	}

	newMS, err := rsl.Save(ctx, eng, s, truncStateType)
	if err != nil {
		return enginepb.MVCCStats{}, err
	}

	return newMS, nil
}

// WriteInitialState calls WriteInitialReplicaState followed by
// SynthesizeRaftState. It is typically called during bootstrap. The supplied
// MVCCStats are used for the Stats field after adjusting for persisting the
// state itself, and the updated stats are returned.
//
// Args:
// bootstrapVersion: The version at which the cluster is bootstrapped.
func WriteInitialState(
	ctx context.Context,
	eng engine.ReadWriter,
	ms enginepb.MVCCStats,
	desc roachpb.RangeDescriptor,
	lease roachpb.Lease,
	gcThreshold hlc.Timestamp,
	txnSpanGCThreshold hlc.Timestamp,
	bootstrapVersion roachpb.Version,
	truncStateType TruncatedStateType,
) (enginepb.MVCCStats, error) {
	newMS, err := WriteInitialReplicaState(
		ctx, eng, ms, desc, lease, gcThreshold, txnSpanGCThreshold, bootstrapVersion, truncStateType)
	if err != nil {
		return enginepb.MVCCStats{}, err
	}
	if err := Make(desc.RangeID).SynthesizeRaftState(ctx, eng); err != nil {
		return enginepb.MVCCStats{}, err
	}
	return newMS, nil
}
