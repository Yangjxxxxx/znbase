package sql

import (
	"context"

	"github.com/znbasedb/znbase/pkg/sql/rowcontainer"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

// bufferNode consumes its input one row at a time, stores it in the buffer,
// and passes the row through. The buffered rows can be iterated over multiple
// times.
type bufferNode struct {
	plan planNode

	// TODO(yuzefovich): the buffer should probably be backed by disk. If so, the
	// comments about TempStorage suggest that it should be used by DistSQL
	// processors, but this node is local.
	bufferedRows       *rowcontainer.RowContainer
	passThruNextRowIdx int

	// label is a string used to describe the node in an EXPLAIN plan.
	label string
}

func (n *bufferNode) startExec(params runParams) error {
	n.bufferedRows = rowcontainer.NewRowContainer(
		params.EvalContext().Mon.MakeBoundAccount(),
		sqlbase.ColTypeInfoFromResCols(getPlanColumns(n.plan, false /* mut */)),
		0, /* rowCapacity */
	)
	return nil
}

func (n *bufferNode) Next(params runParams) (bool, error) {
	isStartWith := false
	if err := params.p.cancelChecker.Check(); err != nil {
		return false, err
	}
	ok, err := n.plan.Next(params)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	distinct := false
	recursiveNode, ok := n.plan.(*recursiveCTENode)
	if ok {
		distinct = !recursiveNode.IsUnionAll()
	}

	switch t := params.p.stmt.AST.(type) {
	case *tree.Select:
		if t.With != nil && t.With.StartWith {
			isStartWith = true
		}
	case *tree.CreateTable:
		if t.AsSource.With != nil && t.AsSource.With.StartWith {
			isStartWith = true
		}
	}

	if distinct && !isStartWith {
		ret := true
		for i := 0; i < n.bufferedRows.Len(); i++ {
			ret = n.bufferedRows.At(i).IsDistinctFrom(params.EvalContext(), n.plan.Values())
			if !ret {
				return true, nil
			}
		}
	}

	if _, err = n.bufferedRows.AddRow(params.ctx, n.plan.Values()); err != nil {
		return false, err
	}
	n.passThruNextRowIdx++
	return true, nil
}

func (n *bufferNode) Values() tree.Datums {
	return n.bufferedRows.At(n.passThruNextRowIdx - 1)
}

func (n *bufferNode) Close(ctx context.Context) {
	n.plan.Close(ctx)
	if n.bufferedRows != nil {
		n.bufferedRows.Close(ctx)
	}
}

// scanBufferNode behaves like an iterator into the bufferNode it is
// referencing. The bufferNode can be iterated over multiple times
// simultaneously, however, a new scanBufferNode is needed.
type scanBufferNode struct {
	buffer *bufferNode

	nextRowIdx int

	// label is a string used to describe the node in an EXPLAIN plan.
	label string
}

func (n *scanBufferNode) startExec(runParams) error {
	return nil
}

func (n *scanBufferNode) Next(runParams) (bool, error) {
	n.nextRowIdx++
	return n.nextRowIdx <= n.buffer.bufferedRows.Len(), nil
}

func (n *scanBufferNode) Values() tree.Datums {
	return n.buffer.bufferedRows.At(n.nextRowIdx - 1)
}

func (n *scanBufferNode) Close(context.Context) {
}
