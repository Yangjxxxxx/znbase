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
	"math/rand"
	"reflect"
	"testing"

	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types1"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

const (
	selectivity     = .5
	nullProbability = .1
)

func TestSelLTInt64Int64ConstOp(t *testing.T) {
	defer leaktest.AfterTest(t)()
	tups := tuples{{0}, {1}, {2}, {nil}}
	runTests(t, []tuples{tups}, tuples{{0}, {1}}, orderedVerifier, func(input []Operator) (Operator, error) {
		return &selLTInt64Int64ConstOp{
			selConstOpBase: selConstOpBase{
				OneInputNode: NewOneInputNode(input[0]),
				colIdx:       0,
			},
			constArg: 2,
		}, nil
	})
}

func TestSelLTInt64Int64(t *testing.T) {
	defer leaktest.AfterTest(t)()
	tups := tuples{
		{0, 0},
		{0, 1},
		{1, 0},
		{1, 1},
		{nil, 1},
		{-1, nil},
		{nil, nil},
	}
	runTests(t, []tuples{tups}, tuples{{0, 1}}, orderedVerifier, func(input []Operator) (Operator, error) {
		return &selLTInt64Int64Op{
			selOpBase: selOpBase{
				OneInputNode: NewOneInputNode(input[0]),
				col1Idx:      0,
				col2Idx:      1,
			},
		}, nil
	})
}

func TestGetSelectionConstOperator(t *testing.T) {
	defer leaktest.AfterTest(t)()
	cmpOp := tree.LT
	var input Operator
	colIdx := 3
	constVal := int64(31)
	constArg := tree.NewDInt(tree.DInt(constVal))
	op, err := GetSelectionConstOperator(types1.Int, types1.Int, cmpOp, input, colIdx, constArg)
	if err != nil {
		t.Error(err)
	}
	expected := &selLTInt64Int64ConstOp{
		selConstOpBase: selConstOpBase{
			OneInputNode: NewOneInputNode(input),
			colIdx:       colIdx,
		},
		constArg: constVal,
	}
	if !reflect.DeepEqual(op, expected) {
		t.Errorf("got %+v, expected %+v", op, expected)
	}
}

func TestGetSelectionConstMixedTypeOperator(t *testing.T) {
	defer leaktest.AfterTest(t)()
	cmpOp := tree.LT
	var input Operator
	colIdx := 3
	constVal := int16(31)
	constArg := tree.NewDInt(tree.DInt(constVal))
	op, err := GetSelectionConstOperator(types1.Int, types1.Int2, cmpOp, input, colIdx, constArg)
	if err != nil {
		t.Error(err)
	}
	expected := &selLTInt64Int16ConstOp{
		selConstOpBase: selConstOpBase{
			OneInputNode: NewOneInputNode(input),
			colIdx:       colIdx,
		},
		constArg: constVal,
	}
	if !reflect.DeepEqual(op, expected) {
		t.Errorf("got %+v, expected %+v", op, expected)
	}
}

func TestGetSelectionOperator(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ct := types1.Int2
	cmpOp := tree.GE
	var input Operator
	col1Idx := 5
	col2Idx := 7
	op, err := GetSelectionOperator(ct, ct, cmpOp, input, col1Idx, col2Idx)
	if err != nil {
		t.Error(err)
	}
	expected := &selGEInt16Int16Op{
		selOpBase: selOpBase{
			OneInputNode: NewOneInputNode(input),
			col1Idx:      col1Idx,
			col2Idx:      col2Idx,
		},
	}
	if !reflect.DeepEqual(op, expected) {
		t.Errorf("got %+v, expected %+v", op, expected)
	}
}

func benchmarkSelLTInt64Int64ConstOp(b *testing.B, useSelectionVector bool, hasNulls bool) {
	ctx := context.Background()

	batch := testAllocator.NewMemBatch([]coltypes.T{coltypes.Int64})
	col := batch.ColVec(0).Int64()
	for i := 0; i < int(coldata.BatchSize()); i++ {
		if float64(i) < float64(coldata.BatchSize())*selectivity {
			col[i] = -1
		} else {
			col[i] = 1
		}
	}
	if hasNulls {
		for i := 0; i < int(coldata.BatchSize()); i++ {
			if rand.Float64() < nullProbability {
				batch.ColVec(0).Nulls().SetNull(uint16(i))
			}
		}
	}
	batch.SetLength(coldata.BatchSize())
	if useSelectionVector {
		batch.SetSelection(true)
		sel := batch.Selection()
		for i := uint16(0); i < coldata.BatchSize(); i++ {
			sel[i] = i
		}
	}
	source := NewRepeatableBatchSource(batch)
	source.Init()

	plusOp := &selLTInt64Int64ConstOp{
		selConstOpBase: selConstOpBase{
			OneInputNode: NewOneInputNode(source),
			colIdx:       0,
		},
		constArg: 0,
	}
	plusOp.Init()

	b.SetBytes(int64(8 * coldata.BatchSize()))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plusOp.Next(ctx)
	}
}

func BenchmarkSelLTInt64Int64ConstOp(b *testing.B) {
	for _, useSel := range []bool{true, false} {
		for _, hasNulls := range []bool{true, false} {
			b.Run(fmt.Sprintf("useSel=%t,hasNulls=%t", useSel, hasNulls), func(b *testing.B) {
				benchmarkSelLTInt64Int64ConstOp(b, useSel, hasNulls)
			})
		}
	}
}

func benchmarkSelLTInt64Int64Op(b *testing.B, useSelectionVector bool, hasNulls bool) {
	ctx := context.Background()

	batch := testAllocator.NewMemBatch([]coltypes.T{coltypes.Int64, coltypes.Int64})
	col1 := batch.ColVec(0).Int64()
	col2 := batch.ColVec(1).Int64()
	for i := 0; i < int(coldata.BatchSize()); i++ {
		if float64(i) < float64(coldata.BatchSize())*selectivity {
			col1[i], col2[i] = -1, 1
		} else {
			col1[i], col2[i] = 1, -1
		}
	}
	if hasNulls {
		for i := 0; i < int(coldata.BatchSize()); i++ {
			if rand.Float64() < nullProbability {
				batch.ColVec(0).Nulls().SetNull(uint16(i))
			}
			if rand.Float64() < nullProbability {
				batch.ColVec(1).Nulls().SetNull(uint16(i))
			}
		}
	}
	batch.SetLength(coldata.BatchSize())
	if useSelectionVector {
		batch.SetSelection(true)
		sel := batch.Selection()
		for i := uint16(0); i < coldata.BatchSize(); i++ {
			sel[i] = i
		}
	}
	source := NewRepeatableBatchSource(batch)
	source.Init()

	plusOp := &selLTInt64Int64Op{
		selOpBase: selOpBase{
			OneInputNode: NewOneInputNode(source),
			col1Idx:      0,
			col2Idx:      1,
		},
	}
	plusOp.Init()

	b.SetBytes(int64(8 * coldata.BatchSize() * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plusOp.Next(ctx)
	}
}

func BenchmarkSelLTInt64Int64Op(b *testing.B) {
	for _, useSel := range []bool{true, false} {
		for _, hasNulls := range []bool{true, false} {
			b.Run(fmt.Sprintf("useSel=%t,hasNulls=%t", useSel, hasNulls), func(b *testing.B) {
				benchmarkSelLTInt64Int64Op(b, useSel, hasNulls)
			})
		}
	}
}
