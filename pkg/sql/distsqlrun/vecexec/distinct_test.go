// Copyright 2018 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package vecexec

import (
	"context"
	"fmt"
	"testing"

	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/randutil"
)

func TestSortedDistinct(t *testing.T) {
	defer leaktest.AfterTest(t)()
	tcs := []struct {
		distinctCols []uint32
		colTypes     []coltypes.T
		numCols      int
		tuples       []tuple
		expected     []tuple
	}{
		{
			distinctCols: []uint32{0, 1, 2},
			colTypes:     []coltypes.T{coltypes.Float64, coltypes.Int64, coltypes.Bytes},
			numCols:      4,
			tuples: tuples{
				{nil, nil, nil, nil},
				{nil, nil, nil, nil},
				{nil, nil, "30", nil},
				{1.0, 2, "30", 4},
				{1.0, 2, "30", 5},
				{2.0, 2, "30", 4},
				{2.0, 3, "30", 4},
				{2.0, 3, "40", 4},
				{2.0, 3, "40", 4},
			},
			expected: tuples{
				{nil, nil, nil, nil},
				{nil, nil, "30", nil},
				{1.0, 2, "30", 4},
				{2.0, 2, "30", 4},
				{2.0, 3, "30", 4},
				{2.0, 3, "40", 4},
			},
		},
		{
			distinctCols: []uint32{1, 0, 2},
			colTypes:     []coltypes.T{coltypes.Float64, coltypes.Int64, coltypes.Bytes},
			numCols:      4,
			tuples: tuples{
				{nil, nil, nil, nil},
				{nil, nil, nil, nil},
				{nil, nil, "30", nil},
				{1.0, 2, "30", 4},
				{1.0, 2, "30", 5},
				{2.0, 2, "30", 4},
				{2.0, 3, "30", 4},
				{2.0, 3, "40", 4},
				{2.0, 3, "40", 4},
			},
			expected: tuples{
				{nil, nil, nil, nil},
				{nil, nil, "30", nil},
				{1.0, 2, "30", 4},
				{2.0, 2, "30", 4},
				{2.0, 3, "30", 4},
				{2.0, 3, "40", 4},
			},
		},
		{
			distinctCols: []uint32{0, 1, 2},
			colTypes:     []coltypes.T{coltypes.Float64, coltypes.Int64, coltypes.Bytes},
			numCols:      4,
			tuples: tuples{
				{1.0, 2, "30", 4},
				{1.0, 2, "30", 5},
				{nil, nil, nil, nil},
				{nil, nil, nil, nil},
				{2.0, 2, "30", 4},
				{2.0, 3, "30", 4},
				{nil, nil, "30", nil},
				{2.0, 3, "40", 4},
				{2.0, 3, "40", 4},
			},
			expected: tuples{
				{1.0, 2, "30", 4},
				{nil, nil, nil, nil},
				{2.0, 2, "30", 4},
				{2.0, 3, "30", 4},
				{nil, nil, "30", nil},
				{2.0, 3, "40", 4},
			},
		},
	}

	for _, tc := range tcs {
		runTests(t, []tuples{tc.tuples}, tc.expected, orderedVerifier,
			func(input []Operator) (Operator, error) {
				return NewOrderedDistinct(input[0], tc.distinctCols, tc.colTypes)
			})
	}
}

func BenchmarkSortedDistinct(b *testing.B) {
	rng, _ := randutil.NewPseudoRand()
	ctx := context.Background()

	batch := testAllocator.NewMemBatch([]coltypes.T{coltypes.Int64, coltypes.Int64, coltypes.Int64})
	aCol := batch.ColVec(1).Int64()
	bCol := batch.ColVec(2).Int64()
	lastA := int64(0)
	lastB := int64(0)
	for i := 0; i < int(coldata.BatchSize()); i++ {
		// 1/4 chance of changing each distinct coldata.
		if rng.Float64() > 0.75 {
			lastA++
		}
		if rng.Float64() > 0.75 {
			lastB++
		}
		aCol[i] = lastA
		bCol[i] = lastB
	}
	batch.SetLength(coldata.BatchSize())
	source := NewRepeatableBatchSource(batch)
	source.Init()

	distinct, err := NewOrderedDistinct(source, []uint32{1, 2}, []coltypes.T{coltypes.Int64, coltypes.Int64, coltypes.Int64})
	if err != nil {
		b.Fatal(err)
	}

	// don't count the artificial zeroOp'd column in the throughput
	for _, nulls := range []bool{false, true} {
		b.Run(fmt.Sprintf("nulls=%t", nulls), func(b *testing.B) {
			if nulls {
				n := coldata.NewNulls(int(coldata.BatchSize()))
				// Setting one value to null is enough to trigger the null handling
				// logic for the entire batch.
				n.SetNull(0)
				batch.ColVec(1).SetNulls(&n)
				batch.ColVec(2).SetNulls(&n)
			}
			b.SetBytes(int64(8 * coldata.BatchSize() * 3))
			for i := 0; i < b.N; i++ {
				distinct.Next(ctx)
			}
		})
	}
}
