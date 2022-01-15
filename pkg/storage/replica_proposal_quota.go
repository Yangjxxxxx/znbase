// Copyright 2019  The Cockroach Authors.
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
	"time"

	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/tracker"
)

// MaxQuotaReplicaLivenessDuration is the maximum duration that a replica
// can remain inactive while still being counting against the range's
// available proposal quota.
const MaxQuotaReplicaLivenessDuration = 10 * time.Second

func (r *Replica) maybeAcquireProposalQuota(ctx context.Context, quota int64) error {
	r.mu.RLock()
	quotaPool := r.mu.proposalQuota
	desc := *r.mu.state.Desc
	r.mu.RUnlock()

	// Quota acquisition only takes place on the leader replica,
	// r.mu.proposalQuota is set to nil if a node is a follower (see
	// updateProposalQuotaRaftMuLocked). For the cases where the range lease
	// holder is not the same as the range leader, i.e. the lease holder is a
	// follower, r.mu.proposalQuota == nil. This means all quota acquisitions
	// go through without any throttling whatsoever but given how short lived
	// these scenarios are we don't try to remedy any further.
	//
	// NB: It is necessary to allow proposals with a nil quota pool to go
	// through, for otherwise a follower could never request the lease.

	if quotaPool == nil {
		return nil
	}

	if !quotaPoolEnabledForRange(desc) {
		return nil
	}

	// Trace if we're running low on available proposal quota; it might explain
	// why we're taking so long.
	if log.HasSpanOrEvent(ctx) {
		if q := quotaPool.approximateQuota(); q < quotaPool.maxQuota()/10 {
			log.Eventf(ctx, "quota running low, currently available ~%d", q)
		}
	}

	return quotaPool.acquire(ctx, quota)
}

func quotaPoolEnabledForRange(desc roachpb.RangeDescriptor) bool {
	// The NodeLiveness range does not use a quota pool. We don't want to
	// throttle updates to the NodeLiveness range even if a follower is falling
	// behind because this could result in cascading failures.
	return !bytes.HasPrefix(desc.StartKey, keys.NodeLivenessPrefix)
}

func (r *Replica) updateProposalQuotaRaftMuLocked(
	ctx context.Context, lastLeaderID roachpb.ReplicaID,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mu.replicaID == 0 {
		// The replica was created from preemptive snapshot and has not been
		// added to the Raft group.
		return
	}

	// We need to check if the replica is being destroyed and if so, unblock
	// all ongoing and subsequent quota acquisition goroutines (if any).
	//
	// TODO(irfansharif): There is still a potential problem here that leaves
	// clients hanging if the replica gets destroyed but this code path is
	// never taken. Moving quota pool draining to every point where a
	// replica can get destroyed is an option, alternatively we can clear
	// our leader status and close the proposalQuota whenever the replica is
	// destroyed.
	if r.mu.destroyStatus.Removed() {
		if r.mu.proposalQuota != nil {
			r.mu.proposalQuota.close()
		}
		r.mu.proposalQuota = nil
		r.mu.lastUpdateTimes = nil
		r.mu.quotaReleaseQueue = nil
		return
	}

	status := r.mu.internalRaftGroup.BasicStatus()
	if r.mu.leaderID != lastLeaderID {
		if r.mu.replicaID == r.mu.leaderID {
			// We're becoming the leader.
			// Initialize the proposalQuotaBaseIndex at the applied index.
			// After the proposal quota is enabled all entries applied by this replica
			// will be appended to the quotaReleaseQueue. The proposalQuotaBaseIndex
			// and the quotaReleaseQueue together track status.Applied exactly.
			r.mu.proposalQuotaBaseIndex = status.Applied
			if r.mu.proposalQuota != nil {
				log.Fatal(ctx, "proposalQuota was not nil before becoming the leader")
			}
			if releaseQueueLen := len(r.mu.quotaReleaseQueue); releaseQueueLen != 0 {
				log.Fatalf(ctx, "len(r.mu.quotaReleaseQueue) = %d, expected 0", releaseQueueLen)
			}

			// Raft may propose commands itself (specifically the empty
			// commands when leadership changes), and these commands don't go
			// through the code paths where we acquire quota from the pool. To
			// offset this we reset the quota pool whenever leadership changes
			// hands.
			r.mu.proposalQuota = newQuotaPool(r.store.cfg.RaftProposalQuota)
			r.mu.lastUpdateTimes = make(map[roachpb.ReplicaID]time.Time)
			r.mu.lastUpdateTimes.updateOnBecomeLeader(r.mu.state.Desc.Replicas, timeutil.Now())
		} else if r.mu.proposalQuota != nil {
			// We're becoming a follower.

			// We unblock all ongoing and subsequent quota acquisition
			// goroutines (if any).
			r.mu.proposalQuota.close()
			r.mu.proposalQuota = nil
			r.mu.lastUpdateTimes = nil
			r.mu.quotaReleaseQueue = nil
		}
		return
	} else if r.mu.proposalQuota == nil {
		if r.mu.replicaID == r.mu.leaderID {
			log.Fatal(ctx, "leader has uninitialized proposalQuota pool")
		}
		// We're a follower.
		return
	}

	// We're still the leader.

	// Find the minimum index that active followers have acknowledged.
	now := timeutil.Now()
	// commitIndex is used to determine whether a newly added replica has fully
	// caught up.
	commitIndex := status.Commit
	// Initialize minIndex to the currently applied index. The below progress
	// checks will only decrease the minIndex. Given that the quotaReleaseQueue
	// cannot correspond to values beyond the applied index there's no reason
	// to consider progress beyond it as meaningful.
	minIndex := status.Applied
	r.mu.internalRaftGroup.WithProgress(func(id uint64, _ raft.ProgressType, progress tracker.Progress) {
		rep, ok := r.mu.state.Desc.GetReplicaDescriptorByID(roachpb.ReplicaID(id))
		if !ok {
			return
		}

		// Only consider followers that are active.
		if !r.mu.lastUpdateTimes.isFollowerActive(ctx, rep.ReplicaID, now) {
			return
		}

		// Only consider followers that that have "healthy" RPC connections.
		if err := r.store.cfg.NodeDialer.ConnHealth(rep.NodeID, r.connectionClass.get()); err != nil {
			return
		}

		// Note that the Match field has different semantics depending on
		// the State.
		//
		// In state ProgressStateReplicate, the Match index is optimistically
		// updated whenever a message is *sent* (not received). Due to Raft
		// flow control, only a reasonably small amount of data can be en
		// route to a given follower at any point in time.
		//
		// In state ProgressStateProbe, the Match index equals Next-1, and
		// it tells us the leader's optimistic best guess for the right log
		// index (and will try once per heartbeat interval to update its
		// estimate). In the usual case, the follower responds with a hint
		// when it rejects the first probe and the leader replicates or
		// sends a snapshot. In the case in which the follower does not
		// respond, the leader reduces Match by one each heartbeat interval.
		// But if the follower does not respond, we've already filtered it
		// out above. We use the Match index as is, even though the follower
		// likely isn't there yet because that index won't go up unless the
		// follower is actually catching up, so it won't cause it to fall
		// behind arbitrarily.
		//
		// Another interesting tidbit about this state is that the Paused
		// field is usually true as it is used to limit the number of probes
		// (i.e. appends) sent to this follower to one per heartbeat
		// interval.
		//
		// In state ProgressStateSnapshot, the Match index is the last known
		// (possibly optimistic, depending on previous state) index before
		// the snapshot went out. Once the snapshot applies, the follower
		// will enter ProgressStateReplicate again. So here the Match index
		// works as advertised too.

		// Only consider followers who are in advance of the quota base
		// index. This prevents a follower from coming back online and
		// preventing throughput to the range until it has caught up.
		if progress.Match < r.mu.proposalQuotaBaseIndex {
			return
		}
		if progress.Match > 0 && progress.Match < minIndex {
			minIndex = progress.Match
		}
		// If this is the most recently added replica and it has caught up, clear
		// our state that was tracking it. This is unrelated to managing proposal
		// quota, but this is a convenient place to do so.
		if rep.ReplicaID == r.mu.lastReplicaAdded && progress.Match >= commitIndex {
			r.mu.lastReplicaAdded = 0
			r.mu.lastReplicaAddedTime = time.Time{}
		}
	})

	if r.mu.proposalQuotaBaseIndex < minIndex {
		// We've persisted at least minIndex-r.mu.proposalQuotaBaseIndex entries
		// to the raft log on all 'active' replicas and applied at least minIndex
		// entries locally since last we checked, so we are able to release the
		// difference back to the quota pool.
		numReleases := minIndex - r.mu.proposalQuotaBaseIndex
		sum := int64(0)
		for _, rel := range r.mu.quotaReleaseQueue[:numReleases] {
			sum += rel
		}
		r.mu.proposalQuotaBaseIndex += numReleases
		r.mu.quotaReleaseQueue = r.mu.quotaReleaseQueue[numReleases:]
		r.mu.proposalQuota.add(sum)
	}
	// Assert the sanity of the base index and the queue. Queue entries should
	// correspond to applied entries. It should not be possible for the base
	// index and the not yet released applied entries to not equal the applied
	// index.
	releasableIndex := r.mu.proposalQuotaBaseIndex + uint64(len(r.mu.quotaReleaseQueue))
	if releasableIndex != status.Applied {
		log.Fatalf(ctx, "proposalQuotaBaseIndex (%d) + quotaReleaseQueueLen (%d) = %d"+
			" must equal the applied index (%d)",
			r.mu.proposalQuotaBaseIndex, len(r.mu.quotaReleaseQueue), releasableIndex,
			status.Applied)
	}
}
