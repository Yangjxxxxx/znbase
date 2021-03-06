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

package rditer

import (
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
)

// ComputeStatsForRange computes the stats for a given range by
// iterating over all key ranges for the given range that should
// be accounted for in its stats.
func ComputeStatsForRange(
	d *roachpb.RangeDescriptor, e engine.Reader, nowNanos int64,
) (enginepb.MVCCStats, error) {
	iter := e.NewIterator(engine.IterOptions{UpperBound: d.EndKey.AsRawKey()})
	defer iter.Close()

	ms := enginepb.MVCCStats{}
	for _, keyRange := range MakeReplicatedKeyRanges(d) {
		msDelta, err := iter.ComputeStats(keyRange.Start, keyRange.End, nowNanos)
		if err != nil {
			return enginepb.MVCCStats{}, err
		}
		ms.Add(msDelta)
	}
	return ms, nil
}
