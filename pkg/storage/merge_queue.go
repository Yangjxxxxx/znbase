// Copyright 2018  The Cockroach Authors.
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
	"fmt"
	"math"
	"time"

	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/gossip"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/storage/storagebase"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/humanizeutil"
	"github.com/znbasedb/znbase/pkg/util/log"
)

const (
	// mergeQueuePurgatoryCheckInterval is the interval at which replicas in
	// purgatory make merge attempts. Since merges are relatively untested, the
	// reasons that a range may fail to merge are unknown, so the merge queue has
	// a large purgatory interval.
	mergeQueuePurgatoryCheckInterval = 1 * time.Minute
)

// MergeQueueConcurrency manages a queue of ranges slated to be merged with their right-hand neighbor.
// We allow to adjust the parallelism of merge queue process.
var MergeQueueConcurrency = settings.RegisterPositiveIntSetting(
	"kv.range_merge.queue_concurrency",
	"the current implementation of merges requires rewriting the right-hand data "+
		"onto the left-hand range, even when the ranges are collocated. This is"+
		"expensive, so limit to one merge at a time."+
		"Note that a value lower than 1 is ignored.",
	1,
)

// MergeQueueInterval is a setting that controls how often the merge queue waits
// between processing replicas.
var MergeQueueInterval = func() *settings.DurationSetting {
	s := settings.RegisterNonNegativeDurationSetting(
		"kv.range_merge.queue_interval",
		"how long the merge queue waits between processing replicas",
		time.Second,
	)
	s.SetSensitive()
	return s
}()

// mergeQueue manages a queue of ranges slated to be merged with their right-
// hand neighbor.
//
// A range will only be queued if it is beneath the minimum size threshold. Once
// queued, the size of the right-hand neighbor will additionally be checked;
// merges can only proceed if a) the right-hand neighbor is beneath the minimum
// size threshold, and b) the merged range would not need to be immediately
// split, e.g. because the new range would exceed the maximum size threshold.
//
// Note that the merge queue is not capable of initiating all possible merges.
// Consider the example below:
//
//      /Table/51/1    /Table/51/2    /Table/52
//         32MB            0MB           32MB
//
// The range beginning at /Table/51/2 is empty and would, ideally, be merged
// away. The range to its left, /Table/51/1, will not propose a merge because it
// is over the minimum size threshold. And /Table/51/2 will not propose a merge
// because the next range, /Table/52, is a new table and thus the split is
// mandatory.
//
// There are several ways to solve this. /Table/51/2 could look both left and
// right to find a merge partner, but discovering ones left neighbor is rather
// difficult and involves scanning the meta ranges. /Table/51/1 could propose a
// merge even though it's over the minimum size threshold, but this would result
// in a lot more RangeStats requests--essentially every range would send a
// RangeStats request on every scanner cycle.
//
// The current approach seems to be a nice balance of finding nearly all
// mergeable ranges without sending many RPCs. It has the additional nice
// property of not sending any RPCs to meta ranges until a merge is actually
// initiated.
type mergeQueue struct {
	*baseQueue
	db       *client.DB
	purgChan <-chan time.Time
}

func newMergeQueue(store *Store, db *client.DB, gossip *gossip.Gossip) *mergeQueue {
	mq := &mergeQueue{
		db:       db,
		purgChan: time.NewTicker(mergeQueuePurgatoryCheckInterval).C,
	}
	mq.baseQueue = newBaseQueue(
		"merge", mq, store, gossip,
		queueConfig{
			maxSize:              defaultQueueMaxSize,
			maxConcurrency:       MergeQueueConcurrency.Get(&store.ClusterSettings().SV),
			needsLease:           true,
			needsSystemConfig:    true,
			acceptsUnsplitRanges: false,
			successes:            store.metrics.MergeQueueSuccesses,
			failures:             store.metrics.MergeQueueFailures,
			pending:              store.metrics.MergeQueuePending,
			processingNanos:      store.metrics.MergeQueueProcessingNanos,
			purgatory:            store.metrics.MergeQueuePurgatory,
		},
	)

	confChanged := func() {
		mq.maxConcurrency = MergeQueueConcurrency.Get(&store.ClusterSettings().SV)
		mq.confCh <- struct{}{}
	}
	MergeQueueConcurrency.SetOnChange(&store.cfg.Settings.SV, confChanged)
	return mq
}

func (mq *mergeQueue) enabled() bool {
	st := mq.store.ClusterSettings()
	return storagebase.MergeQueueEnabled.Get(&st.SV)
}

func (mq *mergeQueue) mergesDisabledForRange(desc *roachpb.RangeDescriptor) bool {
	_, tableID, err := keys.DecodeTablePrefix(desc.StartKey.AsRawKey())
	if err == nil {
		_, err = mq.gossip.GetInfo(gossip.MakeTableDisableMergesKey(uint32(tableID)))
		if err == nil {
			return true
		}
	}
	_, tableID2, err := keys.DecodeTablePrefix(desc.EndKey.AsRawKey())
	if err != nil {
		return false
	}
	if tableID == tableID2 {
		return false
	}
	_, err = mq.gossip.GetInfo(gossip.MakeTableDisableMergesKey(uint32(tableID2)))
	return err == nil
}

func (mq *mergeQueue) shouldQueue(
	ctx context.Context, now hlc.Timestamp, repl *Replica, sysCfg *config.SystemConfig,
) (shouldQ bool, priority float64) {
	if !mq.enabled() {
		return false, 0
	}

	desc := repl.Desc()

	if desc.EndKey.Equal(roachpb.RKeyMax) {
		// The last range has no right-hand neighbor to merge with.
		return false, 0
	}

	if sysCfg.NeedsSplit(desc.StartKey, desc.EndKey.Next()) {
		// This range would need to be split if it extended just one key further.
		// There is thus no possible right-hand neighbor that it could be merged
		// with.
		return false, 0
	}

	if mq.mergesDisabledForRange(desc) {
		return false, 0
	}

	sizeRatio := float64(repl.GetMVCCStats().Total()) / float64(repl.GetMinBytes())
	if math.IsNaN(sizeRatio) || sizeRatio >= 1 {
		// This range is above the minimum size threshold. It does not need to be
		// merged.
		return false, 0
	}

	// Invert sizeRatio to compute the priority so that smaller ranges are merged
	// before larger ranges.
	priority = 1 - sizeRatio
	return true, priority
}

// rangeMergePurgatoryError wraps an error that occurs during merging to
// indicate that the error should send the range to purgatory.
type rangeMergePurgatoryError struct{ error }

func (rangeMergePurgatoryError) purgatoryErrorMarker() {}

var _ purgatoryError = rangeMergePurgatoryError{}

func (mq *mergeQueue) requestRangeStats(
	ctx context.Context, key roachpb.Key,
) (roachpb.RangeDescriptor, enginepb.MVCCStats, float64, error) {
	res, pErr := client.SendWrappedWith(ctx, mq.db.NonTransactionalSender(), roachpb.Header{
		ReturnRangeInfo: true,
	}, &roachpb.RangeStatsRequest{
		RequestHeader: roachpb.RequestHeader{Key: key},
	})
	if pErr != nil {
		return roachpb.RangeDescriptor{}, enginepb.MVCCStats{}, 0, pErr.GoError()
	}
	rangeInfos := res.Header().RangeInfos
	if len(rangeInfos) != 1 {
		return roachpb.RangeDescriptor{}, enginepb.MVCCStats{}, 0, fmt.Errorf(
			"mergeQueue.requestRangeStats: response had %d range infos but exactly one was expected",
			len(rangeInfos))
	}
	return rangeInfos[0].Desc, res.(*roachpb.RangeStatsResponse).MVCCStats,
		res.(*roachpb.RangeStatsResponse).QueriesPerSecond, nil
}

func (mq *mergeQueue) process(
	ctx context.Context, lhsRepl *Replica, sysCfg *config.SystemConfig,
) error {
	if !mq.enabled() {
		log.VEventf(ctx, 2, "skipping merge: queue has been disabled")
		return nil
	}

	lhsDesc := lhsRepl.Desc()
	if mq.mergesDisabledForRange(lhsDesc) {
		log.VEventf(ctx, 2, "skipping merge: merges are temporarily disabled for this table")
		return nil
	}

	lhsStats := lhsRepl.GetMVCCStats()
	minBytes := lhsRepl.GetMinBytes()
	if lhsStats.Total() >= minBytes {
		log.VEventf(ctx, 2, "skipping merge: LHS meets minimum size threshold %d with %d bytes",
			minBytes, lhsStats.Total())
		return nil
	}

	lhsQPS := lhsRepl.GetSplitQPS()
	rhsDesc, rhsStats, rhsQPS, err := mq.requestRangeStats(ctx, lhsDesc.EndKey.AsRawKey())
	if err != nil {
		return err
	}
	if rhsStats.Total() >= minBytes {
		log.VEventf(ctx, 2, "skipping merge: RHS meets minimum size threshold %d with %d bytes",
			minBytes, lhsStats.Total())
		return nil
	}

	mergedDesc := &roachpb.RangeDescriptor{
		StartKey: lhsDesc.StartKey,
		EndKey:   rhsDesc.EndKey,
	}
	mergedStats := lhsStats
	mergedStats.Add(rhsStats)

	var mergedQPS float64
	if lhsRepl.SplitByLoadEnabled() {
		mergedQPS = lhsQPS + rhsQPS
	}

	// Check if the merged range would need to be split, if so, skip merge.
	// Use a lower threshold for load based splitting so we don't find ourselves
	// in a situation where we keep merging ranges that would be split soon after
	// by a small increase in load.
	loadBasedSplitPossible := lhsRepl.SplitByLoadQPSThreshold() < 2*mergedQPS
	if ok, _ := shouldSplitRange(mergedDesc, mergedStats, lhsRepl.GetMaxBytes(), sysCfg); ok || loadBasedSplitPossible {
		log.VEventf(ctx, 2,
			"skipping merge to avoid thrashing: merged range %s may split "+
				"(estimated size, estimated QPS: %d, %v)",
			mergedDesc, mergedStats.Total(), mergedQPS)
		return nil
	}

	if !replicaSetsEqual(lhsDesc.Replicas, rhsDesc.Replicas) {
		var targets []roachpb.ReplicationTarget
		for _, lhsReplDesc := range lhsDesc.Replicas {
			targets = append(targets, roachpb.ReplicationTarget{
				NodeID: lhsReplDesc.NodeID, StoreID: lhsReplDesc.StoreID,
			})
		}
		lease, _ := lhsRepl.GetLease()
		for i := range targets {
			if targets[i].NodeID == lease.Replica.NodeID && targets[i].StoreID == lease.Replica.StoreID {
				if i > 0 {
					targets[0], targets[i] = targets[i], targets[0]
				}
				break
			}
		}
		// TODO(benesch): RelocateRange can sometimes fail if it needs to move a replica
		// from one store to another store on the same node.
		if err := mq.store.DB().AdminRelocateRange(ctx, rhsDesc.StartKey, targets); err != nil {
			return err
		}
	}

	log.VEventf(ctx, 2, "merging to produce range: %s-%s", mergedDesc.StartKey, mergedDesc.EndKey)
	reason := fmt.Sprintf("lhs+rhs has (size=%s+%s qps=%.2f+%.2f --> %.2fqps) below threshold (size=%s, qps=%.2f)",
		humanizeutil.IBytes(lhsStats.Total()),
		humanizeutil.IBytes(rhsStats.Total()),
		lhsQPS,
		rhsQPS,
		mergedQPS,
		humanizeutil.IBytes(mergedStats.Total()),
		mergedQPS,
	)
	_, pErr := lhsRepl.AdminMerge(ctx, roachpb.AdminMergeRequest{}, reason, false)
	switch err := pErr.GoError(); err.(type) {
	case nil:
	case *roachpb.ConditionFailedError:
		// ConditionFailedErrors are an expected outcome for range merge
		// attempts because merges can race with other descriptor modifications.
		// On seeing a ConditionFailedError, don't return an error and enqueue
		// this replica again in case it still needs to be merged.
		log.Infof(ctx, "merge saw concurrent descriptor modification; maybe retrying")
		mq.MaybeAddAsync(ctx, lhsRepl, mq.store.Clock().Now())
	default:
		// While range merges are unstable, be extra cautious and mark every error
		// as purgatory-worthy.
		return rangeMergePurgatoryError{err}
	}
	if testingAggressiveConsistencyChecks {
		if err := mq.store.consistencyQueue.process(ctx, lhsRepl, sysCfg); err != nil {
			log.Warning(ctx, err)
		}
	}
	return nil
}

func (mq *mergeQueue) timer(time.Duration) time.Duration {
	return MergeQueueInterval.Get(&mq.store.ClusterSettings().SV)
}

func (mq *mergeQueue) purgatoryChan() <-chan time.Time {
	return mq.purgChan
}
