// Copyright 2017 The Cockroach Authors.
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

package batcheval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/log"
)

type wrappedBatch struct {
	engine.Batch
	clearCount      int
	clearRangeCount int
}

func (wb *wrappedBatch) Clear(key engine.MVCCKey) error {
	wb.clearCount++
	return wb.Batch.Clear(key)
}

func (wb *wrappedBatch) ClearRange(start, end engine.MVCCKey) error {
	wb.clearRangeCount++
	return wb.Batch.ClearRange(start, end)
}

// TestCmdClearRangeBytesThreshold verifies that clear range resorts to
// clearing keys individually if under the bytes threshold and issues a
// clear range command to the batch otherwise.
func TestCmdClearRangeBytesThreshold(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	startKey := roachpb.Key("0000")
	endKey := roachpb.Key("9999")
	desc := roachpb.RangeDescriptor{
		RangeID:  99,
		StartKey: roachpb.RKey(startKey),
		EndKey:   roachpb.RKey(endKey),
	}
	valueStr := strings.Repeat("0123456789", 1024)
	var value roachpb.Value
	value.SetString(valueStr) // 10KiB
	halfFull := ClearRangeBytesThreshold / (2 * len(valueStr))
	overFull := ClearRangeBytesThreshold/len(valueStr) + 1
	tests := []struct {
		keyCount           int
		expClearCount      int
		expClearRangeCount int
	}{
		{
			keyCount:           1,
			expClearCount:      1,
			expClearRangeCount: 0,
		},
		// More than a single key, but not enough to use ClearRange.
		{
			keyCount:           halfFull,
			expClearCount:      halfFull,
			expClearRangeCount: 0,
		},
		// With key sizes requiring additional space, this will overshoot.
		{
			keyCount:           overFull,
			expClearCount:      0,
			expClearRangeCount: 1,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			ctx := context.Background()
			eng := engine.NewInMem(roachpb.Attributes{}, 1<<20)
			defer eng.Close()

			var stats enginepb.MVCCStats
			for i := 0; i < test.keyCount; i++ {
				key := roachpb.Key(fmt.Sprintf("%04d", i))
				if err := engine.MVCCPut(ctx, eng, &stats, key, hlc.Timestamp{WallTime: int64(i % 2)}, value, nil); err != nil {
					t.Fatal(err)
				}
			}

			batch := &wrappedBatch{Batch: eng.NewBatch()}
			defer batch.Close()

			var h roachpb.Header
			h.RangeID = desc.RangeID

			cArgs := CommandArgs{Header: h}
			cArgs.EvalCtx = &mockEvalCtx{desc: &desc, clock: hlc.NewClock(hlc.UnixNano, time.Nanosecond), stats: stats}
			cArgs.Args = &roachpb.ClearRangeRequest{
				RequestHeader: roachpb.RequestHeader{
					Key:    startKey,
					EndKey: endKey,
				},
			}
			cArgs.Stats = &enginepb.MVCCStats{}

			if _, err := ClearRange(ctx, batch, cArgs, &roachpb.ClearRangeResponse{}); err != nil {
				t.Fatal(err)
			}

			// Verify cArgs.Stats is equal to the stats we wrote.
			newStats := stats
			newStats.SysBytes, newStats.SysCount = 0, 0       // ignore these values
			cArgs.Stats.SysBytes, cArgs.Stats.SysCount = 0, 0 // these too, as GC threshold is updated
			newStats.Add(*cArgs.Stats)
			newStats.AgeTo(0) // pin at LastUpdateNanos==0
			if !newStats.Equal(enginepb.MVCCStats{}) {
				t.Errorf("expected stats on original writes to be negated on clear range: %+v vs %+v", stats, *cArgs.Stats)
			}

			// Verify we see the correct counts for Clear and ClearRange.
			if a, e := batch.clearCount, test.expClearCount; a != e {
				t.Errorf("expected %d clears; got %d", e, a)
			}
			if a, e := batch.clearRangeCount, test.expClearRangeCount; a != e {
				t.Errorf("expected %d clear ranges; got %d", e, a)
			}

			// Now ensure that the data is gone, whether it was a ClearRange or individual calls to clear.
			if err := batch.Commit(true /* commit */); err != nil {
				t.Fatal(err)
			}
			if err := eng.Iterate(
				engine.MVCCKey{Key: startKey}, engine.MVCCKey{Key: endKey},
				func(kv engine.MVCCKeyValue) (bool, error) {
					return true, errors.New("expected no data in underlying engine")
				},
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}
