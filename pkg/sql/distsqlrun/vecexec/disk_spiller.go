// Copyright 2019  The Cockroach Authors.

package vecexec

import (
	"context"
	"fmt"

	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/vecexec/execerror"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

// bufferingInMemoryOperator is an Operator that buffers up intermediate tuples
// in memory and knows how to export them once the memory limit has been
// reached.
type bufferingInMemoryOperator interface {
	Operator

	// ExportBuffered returns all the batches that have been buffered up and have
	// not yet been processed by the operator. It needs to be called once the
	// memory limit has been reached in order to "dump" the buffered tuples into
	// a disk-backed operator. It will return a zero-length batch once the buffer
	// has been emptied.
	//
	// Calling ExportBuffered may invalidate the contents of the last batch
	// returned by ExportBuffered.
	// TODO(yuzefovich): it might be possible to avoid the need for an Allocator
	// when exporting buffered tuples. This will require the refactor of the
	// buffering in-memory operators.
	ExportBuffered(*Allocator) coldata.Batch
}

// oneInputDiskSpiller is an Operator that manages the fallback from an
// in-memory buffering operator to a disk-backed one when the former hits the
// memory limit.
//
// NOTE: if an out of memory error occurs during initialization, this operator
// simply propagates the error further.
//
// The diagram of the components involved is as follows:
//
//        -------------  input  -----------
//       |                ||                | (2nd src)
//       |                ||   (1st src)    ↓
//       |            ----||---> bufferExportingOperator
//       ↓           |    ||                |
//    inMemoryOp ----     ||                ↓
//       |                ||           diskBackedOp
//       |                ||                |
//       |                ↓↓                |
//        ---------> disk spiller <--------
//                        ||
//                        ||
//                        ↓↓
//                      output
//
// Here is the explanation:
// - the main chain of Operators is input -> disk spiller -> output
// - the dist spiller will first try running everything through the left side
//   chain of input -> inMemoryOp. If that succeeds, great! The disk spiller
//   will simply propagate the batch to the output. If that fails with an OOM
//   error, the disk spiller will then initialize the right side chain and will
//   proceed to emit from there
// - the right side chain is bufferExportingOperator -> diskBackedOp. The
//   former will first export all the buffered tuples from inMemoryOp and then
//   will proceed on emitting from input.
type oneInputDiskSpiller struct {
	NonExplainable

	allocator   *Allocator
	initialized bool
	spilled     bool

	input        Operator
	inMemoryOp   bufferingInMemoryOperator
	diskBackedOp Operator
}

var _ Operator = &oneInputDiskSpiller{}

// newOneInputDiskSpiller returns a new oneInputDiskSpiller. It takes the
// following arguments:
// - allocator - this Allocator is used (if spilling occurs) when copying the
//   buffered tuples from the in-memory operator into the disk-backed one.
// - inMemoryOp - the in-memory operator that will be consuming input and doing
//   computations until it either successfully processes the whole input or
//   reaches its memory limit.
// - diskBackedOpConstructor - the function to construct the disk-backed
//   operator when given an input operator. We take in a constructor rather
//   than an already created operator in order to hide the complexity of buffer
//   exporting operator that serves as the input to the disk-backed operator.
func newOneInputDiskSpiller(
	allocator *Allocator,
	input Operator,
	inMemoryOp bufferingInMemoryOperator,
	diskBackedOpConstructor func(input Operator) Operator,
) Operator {
	diskBackedOpInput := newBufferExportingOperator(allocator, inMemoryOp, input)
	return &oneInputDiskSpiller{
		allocator:    allocator,
		input:        input,
		inMemoryOp:   inMemoryOp,
		diskBackedOp: diskBackedOpConstructor(diskBackedOpInput),
	}
}

func (d *oneInputDiskSpiller) Init() {
	if d.initialized {
		return
	}
	d.initialized = true
	// It is possible that Init() call below will hit an out of memory error,
	// but we decide to bail on this query, so we do not catch internal panics.
	//
	// Also note that d.input is the input to d.inMemoryOp, so calling Init()
	// only on the latter is sufficient.
	d.inMemoryOp.Init()
}

func (d *oneInputDiskSpiller) Next(ctx context.Context) coldata.Batch {
	if d.spilled {
		return d.diskBackedOp.Next(ctx)
	}
	var batch coldata.Batch
	if err := execerror.CatchVecRuntimeError(
		func() {
			batch = d.inMemoryOp.Next(ctx)
		},
	); err != nil {
		if sqlbase.IsOutOfMemoryError(err) {
			d.spilled = true
			d.diskBackedOp.Init()
			return d.Next(ctx)
		}
		// Not an out of memory error, so we propagate it further.
		execerror.VectorizedInternalPanic(err)
	}
	return batch
}

func (d *oneInputDiskSpiller) ChildCount(verbose bool) int {
	if verbose {
		return 3
	}
	return 1
}

func (d *oneInputDiskSpiller) Child(nth int, verbose bool) runbase.OpNode {
	// Note: although the main chain is d.input -> diskSpiller -> output (and the
	// main chain should be under nth == 0), in order to make the output of
	// EXPLAIN (VEC) less confusing we return the in-memory operator as being on
	// the main chain.
	if verbose {
		switch nth {
		case 0:
			return d.inMemoryOp
		case 1:
			return d.input
		case 2:
			return d.diskBackedOp
		default:
			execerror.VectorizedInternalPanic(fmt.Sprintf("invalid index %d", nth))
			// This code is unreachable, but the compiler cannot infer that.
			return nil
		}
	}
	switch nth {
	case 0:
		return d.inMemoryOp
	default:
		execerror.VectorizedInternalPanic(fmt.Sprintf("invalid index %d", nth))
		// This code is unreachable, but the compiler cannot infer that.
		return nil
	}
}

// bufferExportingOperator is an Operator that first returns all batches from
// firstSource, and once firstSource is exhausted, it proceeds on returning all
// batches from the second source.
//
// NOTE: bufferExportingOperator assumes that both sources will have been
// initialized when bufferExportingOperator.Init() is called.
type bufferExportingOperator struct {
	ZeroInputNode
	NonExplainable

	allocator       *Allocator
	firstSource     bufferingInMemoryOperator
	secondSource    Operator
	firstSourceDone bool
}

var _ Operator = &bufferExportingOperator{}

func newBufferExportingOperator(
	allocator *Allocator, firstSource bufferingInMemoryOperator, secondSource Operator,
) Operator {
	return &bufferExportingOperator{
		allocator:    allocator,
		firstSource:  firstSource,
		secondSource: secondSource,
	}
}

func (b *bufferExportingOperator) Init() {
	// Init here is a noop because the operator assumes that both sources have
	// already been initialized.
}

func (b *bufferExportingOperator) Next(ctx context.Context) coldata.Batch {
	if b.firstSourceDone {
		return b.secondSource.Next(ctx)
	}
	batch := b.firstSource.ExportBuffered(b.allocator)
	if batch.Length() == 0 {
		b.firstSourceDone = true
		return b.Next(ctx)
	}
	return batch
}
