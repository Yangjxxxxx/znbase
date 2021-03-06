// Copyright 2016  The Cockroach Authors.
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

package sql

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/sql/opt/cat"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

// unionNode is a planNode whose rows are the result of one of three set
// operations (UNION, INTERSECT, or EXCEPT) on left and right. There are two
// variations of each set operation: distinct, which always returns unique
// results, and all, which does no uniqueing.
//
// Ordering of rows is expected to be handled externally to unionNode.
// TODO(dan): In the long run, this is insufficient. If we know both left and
// right are ordered the same way, we can do the set logic without the map
// state. Additionally, if the unionNode has an ordering then we can hint it
// down to left and right and force the condition for this first optimization.
//
// All six of the operations can be completed without cacheing rows by
// iterating one side then the other and keeping counts of unique rows
// in a map. The logic is common for all six. However, because EXCEPT
// needs to iterate the right side first, the common code always reads
// the right operand first. Meanwhile, we invert the operands for the
// non-EXCEPT cases in order to preserve the appearance of the
// original specified order.
//
// The emit logic for each op is represented by implementors of the
// unionNodeEmit interface. The emitRight method is called for each row output
// by the right side and passed a hashable representation of the row. If it
// returns true, the row is emitted. After all right rows are examined, then
// each left row is passed to emitLeft in the same way.
//
// An example: intersectNodeEmitAll
// VALUES (1), (1), (1), (2), (2) INTERSECT ALL VALUES (1), (3), (1)
// ----
// 1
// 1
// There are three 1s on the left and two 1s on the right, so we emit 1, 1.
// Nothing else is in both.
//  emitRight: For each row, increment the map entry.
//  emitLeft: For each row, if the row is not present in the map, it was not in
//    both, don't emit. Otherwise, if the count for the row was > 0, emit and
//    decrement the entry. Otherwise, the row was on the right, but we've
//    already emitted as many as were on the right, don't emit.
type unionNode struct {
	// right and left are the data source operands.
	// right is read first, to populate the `emit` field.
	right, left planNode

	// columns contains the metadata for the results of this node.
	columns sqlbase.ResultColumns
	// inverted, when true, indicates that the right plan corresponds to
	// the left operand in the input SQL syntax, and vice-versa.
	inverted bool
	// emitAll is a performance optimization for UNION ALL. When set
	// the union logic avoids the `emit` logic entirely.
	emitAll bool

	// unionType is the type of operation (UNION, INTERSECT, EXCEPT)
	unionType tree.UnionType
	// all indicates if the operation is the ALL or DISTINCT version
	all bool

	storeEngine cat.DataStoreEngine
}

// Union constructs a planNode from a UNION/INTERSECT/EXCEPT expression.
func (p *planner) Union(
	ctx context.Context, n *tree.UnionClause, desiredTypes []types.T,
) (planNode, error) {
	left, err := p.newPlan(ctx, n.Left, desiredTypes)
	if err != nil {
		return nil, err
	}
	right, err := p.newPlan(ctx, n.Right, desiredTypes)
	if err != nil {
		return nil, err
	}

	return p.newUnionNode(n.Type, n.All, left, right)
}

func (p *planner) newUnionNode(
	typ tree.UnionType, all bool, left, right planNode,
) (planNode, error) {
	emitAll := false
	switch typ {
	case tree.UnionOp:
		if all {
			emitAll = true
		}
	case tree.IntersectOp:
	case tree.ExceptOp:
	default:
		return nil, errors.Errorf("%v is not supported", typ)
	}

	leftColumns := planColumns(left)
	rightColumns := planColumns(right)
	if len(leftColumns) != len(rightColumns) {
		return nil, pgerror.NewErrorf(
			pgcode.Syntax,
			"each %v query must have the same number of columns: %d vs %d",
			typ, len(leftColumns), len(rightColumns),
		)
	}
	unionColumns := append(sqlbase.ResultColumns(nil), leftColumns...)
	for i := 0; i < len(unionColumns); i++ {
		l := leftColumns[i]
		r := rightColumns[i]

		canConvert := false
		var convertType types.T
		convertType, canConvert = tree.GetUnionType(l.Typ, r.Typ)

		// TODO(dan): This currently checks whether the types are exactly the same,
		// but Postgres is more lenient:
		// http://www.postgresql.org/docs/9.5/static/typeconv-union-case.html.
		if !(l.Typ.Equivalent(r.Typ) || l.Typ == types.Unknown || r.Typ == types.Unknown || canConvert) {
			return nil, pgerror.NewErrorf(pgcode.DatatypeMismatch,
				"%v types %s and %s cannot be matched", typ, l.Typ, r.Typ)
		}
		if l.Hidden != r.Hidden {
			return nil, fmt.Errorf("%v types cannot be matched", typ)
		}
		if l.Typ == types.Unknown {
			unionColumns[i].Typ = r.Typ
		}
		if (!l.Typ.Equivalent(r.Typ)) && canConvert {
			if lRenderNode, ok := left.(*renderNode); ok {
				lRenderNode.columns[i].Typ = convertType
				//reTyped,_ := tree.TypeCheck(lRenderNode.render[i],nil, convertType)
				//lRenderNode.render[i] = reTyped
				if lIndexVar, ok := lRenderNode.render[i].(*tree.IndexedVar); ok {
					lIndexVar.Typ = convertType
				}
				cexpr := tree.ChangeToCastExpr(lRenderNode.render[i], convertType, 0)
				lRenderNode.render[i] = &cexpr
			}
			if rRenderNode, ok := right.(*renderNode); ok {
				rRenderNode.columns[i].Typ = convertType
				//reTyped,_ := tree.TypeCheck(rRenderNode.render[i],nil, convertType)
				//rRenderNode.render[i] = reTyped
				if rIndexVar, ok := rRenderNode.render[i].(*tree.IndexedVar); ok {
					rIndexVar.Typ = convertType
				}
				cexpr := tree.ChangeToCastExpr(rRenderNode.render[i], convertType, 0)
				rRenderNode.render[i] = &cexpr
			}
			unionColumns[i].Typ = convertType
		}
	}

	inverted := false
	if typ != tree.ExceptOp {
		// The logic below reads the rows from the right operand first,
		// because for EXCEPT in particular this is what we need to match.
		// However for the other operators (UNION, INTERSECT) it is
		// actually confusing to see the right values come up first in the
		// results. So invert this here, to reduce surprise by users.
		left, right = right, left
		inverted = true
	}

	node := &unionNode{
		right:     right,
		left:      left,
		columns:   unionColumns,
		inverted:  inverted,
		emitAll:   emitAll,
		unionType: typ,
		all:       all,
	}
	return node, nil
}

func (n *unionNode) startExec(params runParams) error {
	panic("unionNode cannot be run in local mode")
}

func (n *unionNode) Next(params runParams) (bool, error) {
	panic("unionNode cannot be run in local mode")
}

func (n *unionNode) Values() tree.Datums {
	panic("unionNode cannot be run in local mode")
}

func (n *unionNode) Close(ctx context.Context) {
	n.right.Close(ctx)
	n.left.Close(ctx)
}

func (n *unionNode) SetDataStoreEngine(ds cat.DataStoreEngine) {
	n.storeEngine = ds
}

func (n *unionNode) GetDataStoreEngine() cat.DataStoreEngine {
	return n.storeEngine
}
