// Copyright 2019  The Cockroach Authors.

package vecexec

import (
	"context"
	"fmt"

	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/vecexec/execerror"
	"github.com/znbasedb/znbase/pkg/sql/sem"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types1"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

type defaultBuiltinFuncOperator struct {
	OneInputNode
	allocator      *Allocator
	evalCtx        *tree.EvalContext
	funcExpr       *tree.FuncExpr
	columnTypes    []types1.T
	argumentCols   []int
	outputIdx      int
	outputType     *types1.T
	outputPhysType coltypes.T
	converter      func(tree.Datum) (interface{}, error)

	row tree.Datums
	da  sqlbase.DatumAlloc
}

var _ Operator = &defaultBuiltinFuncOperator{}

func (b *defaultBuiltinFuncOperator) Init() {
	b.input.Init()
}

func (b *defaultBuiltinFuncOperator) Next(ctx context.Context) coldata.Batch {
	batch := b.input.Next(ctx)
	n := batch.Length()
	if b.outputIdx == batch.Width() {
		b.allocator.AppendColumn(batch, b.outputPhysType)
	}
	if n == 0 {
		return batch
	}

	sel := batch.Selection()
	output := batch.ColVec(b.outputIdx)
	b.allocator.performOperation(
		[]coldata.Vec{output},
		func() {
			for i := uint16(0); i < n; i++ {
				rowIdx := i
				if sel != nil {
					rowIdx = sel[i]
				}

				hasNulls := false

				for j := range b.argumentCols {
					col := batch.ColVec(b.argumentCols[j])
					b.row[j] = PhysicalTypeColElemToDatum(col, rowIdx, b.da, &b.columnTypes[b.argumentCols[j]])
					hasNulls = hasNulls || b.row[j] == tree.DNull
				}

				var (
					res tree.Datum
					err error
				)
				// Some functions cannot handle null arguments.
				if hasNulls && !b.funcExpr.CanHandleNulls() {
					res = tree.DNull
				} else {
					res, err = b.funcExpr.ResolvedOverload().Fn(b.evalCtx, b.row)
					if err != nil {
						execerror.NonVectorizedPanic(err)
					}
				}

				// Convert the datum into a physical type and write it out.
				if res == tree.DNull {
					batch.ColVec(b.outputIdx).Nulls().SetNull(rowIdx)
				} else {
					converted, err := b.converter(res)
					if err != nil {
						execerror.VectorizedInternalPanic(err)
					}
					coldata.SetValueAt(output, converted, rowIdx, b.outputPhysType)
				}
			}
		},
	)
	return batch
}

type substringFunctionOperator struct {
	OneInputNode
	allocator    *Allocator
	argumentCols []int
	outputIdx    int
}

var _ Operator = &substringFunctionOperator{}

func (s *substringFunctionOperator) Init() {
	s.input.Init()
}

func (s *substringFunctionOperator) Next(ctx context.Context) coldata.Batch {
	batch := s.input.Next(ctx)
	if s.outputIdx == batch.Width() {
		s.allocator.AppendColumn(batch, coltypes.Bytes)
	}

	n := batch.Length()
	if n == 0 {
		return batch
	}

	sel := batch.Selection()
	runeVec := batch.ColVec(s.argumentCols[0]).Bytes()
	startVec := batch.ColVec(s.argumentCols[1]).Int64()
	lengthVec := batch.ColVec(s.argumentCols[2]).Int64()
	outputVec := batch.ColVec(s.outputIdx)
	outputCol := outputVec.Bytes()
	s.allocator.performOperation(
		[]coldata.Vec{outputVec},
		func() {
			for i := uint16(0); i < n; i++ {
				rowIdx := i
				if sel != nil {
					rowIdx = sel[i]
				}

				// The substring operator does not support nulls. If any of the arguments
				// are NULL, we output NULL.
				isNull := false
				for _, col := range s.argumentCols {
					if batch.ColVec(col).Nulls().NullAt(rowIdx) {
						isNull = true
						break
					}
				}
				if isNull {
					batch.ColVec(s.outputIdx).Nulls().SetNull(rowIdx)
					continue
				}

				runes := runeVec.Get(int(rowIdx))
				// Substring start is 1 indexed.
				start := int(startVec[rowIdx]) - 1
				length := int(lengthVec[rowIdx])
				if length < 0 {
					execerror.VectorizedInternalPanic(fmt.Sprintf("negative substring length %d not allowed", length))
				}

				end := start + length
				// Check for integer overflow.
				if end < start {
					end = len(runes)
				} else if end < 0 {
					end = 0
				} else if end > len(runes) {
					end = len(runes)
				}

				if start < 0 {
					start = 0
				} else if start > len(runes) {
					start = len(runes)
				}
				outputCol.Set(int(rowIdx), runes[start:end])
			}
		},
	)
	return batch
}

// NewBuiltinFunctionOperator returns an operator that applies builtin functions.
func NewBuiltinFunctionOperator(
	allocator *Allocator,
	evalCtx *tree.EvalContext,
	funcExpr *tree.FuncExpr,
	columnTypes []types1.T,
	argumentCols []int,
	outputIdx int,
	input Operator,
) (Operator, error) {

	switch funcExpr.ResolvedOverload().SpecializedVecBuiltin {
	case tree.SubstringStringIntInt:
		return &substringFunctionOperator{
			OneInputNode: NewOneInputNode(input),
			allocator:    allocator,
			argumentCols: argumentCols,
			outputIdx:    outputIdx,
		}, nil
	default:
		outputType := funcExpr.ResolvedType()
		newType := sem.ToNewType(outputType)
		outputPhysType := sem.FromColumnType(newType)
		if outputPhysType == coltypes.Unhandled {
			return nil, errors.Errorf(
				"unsupported output type %q of %s",
				outputType.String(), funcExpr.String(),
			)
		}
		return &defaultBuiltinFuncOperator{
			OneInputNode:   NewOneInputNode(input),
			allocator:      allocator,
			evalCtx:        evalCtx,
			funcExpr:       funcExpr,
			outputIdx:      outputIdx,
			columnTypes:    columnTypes,
			outputType:     newType,
			outputPhysType: outputPhysType,
			converter:      sem.GetDatumToPhysicalFn(newType),
			row:            make(tree.Datums, len(argumentCols)),
			argumentCols:   argumentCols,
		}, nil
	}
}
