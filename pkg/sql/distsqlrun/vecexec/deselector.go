// Copyright 2019  The Cockroach Authors.

package vecexec

import (
	"context"

	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
)

// deselectorOp consumes the input operator, and if resulting batches have a
// selection vector, it coalesces them (meaning that tuples will be reordered
// or omitted according to the selection vector). If the batches come with no
// selection vector, it is a noop.
type deselectorOp struct {
	OneInputNode
	NonExplainable
	allocator  *Allocator
	inputTypes []coltypes.T

	output coldata.Batch
}

var _ Operator = &deselectorOp{}

// NewDeselectorOp creates a new deselector operator on the given input
// operator with the given column coltypes.
func NewDeselectorOp(allocator *Allocator, input Operator, colTypes []coltypes.T) Operator {
	return &deselectorOp{
		OneInputNode: NewOneInputNode(input),
		allocator:    allocator,
		inputTypes:   colTypes,
	}
}

func (p *deselectorOp) Init() {
	p.input.Init()
	p.output = p.allocator.NewMemBatch(p.inputTypes)
}

func (p *deselectorOp) Next(ctx context.Context) coldata.Batch {
	batch := p.input.Next(ctx)
	if batch.Selection() == nil {
		return batch
	}

	p.output.SetLength(batch.Length())
	p.output.ResetInternalBatch()
	sel := batch.Selection()
	for i, t := range p.inputTypes {
		toCol := p.output.ColVec(i)
		fromCol := batch.ColVec(i)
		p.allocator.Copy(
			toCol,
			coldata.CopySliceArgs{
				SliceArgs: coldata.SliceArgs{
					ColType:   t,
					Src:       fromCol,
					Sel:       sel,
					SrcEndIdx: uint64(batch.Length()),
				},
			},
		)
	}
	return p.output
}
