// Copyright 2018  The Cockroach Authors.
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

package optbuilder

import (
	"fmt"

	"github.com/znbasedb/znbase/pkg/server/telemetry"
	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

// buildValuesClause builds a set of memo groups that represent the given values
// clause.
//
// See Builder.buildStmt for a description of the remaining input and
// return values.
func (b *Builder) buildValuesClause(
	values *tree.ValuesClause, desiredTypes []types.T, inScope *scope, mb *mutationBuilder,
) (outScope *scope) {
	var numCols int
	if len(values.Rows) > 0 {
		numCols = len(values.Rows[0])
	}

	colTypes := make([]types.T, numCols)
	for i := range colTypes {
		colTypes[i] = types.Unknown
	}
	rows := make(memo.ScalarListExpr, 0, len(values.Rows))

	// We need to save and restore the previous value of the field in
	// semaCtx in case we are recursively called within a subquery
	// context.
	defer b.semaCtx.Properties.Restore(b.semaCtx.Properties)

	// Ensure there are no special functions in the clause.
	b.semaCtx.Properties.Require("VALUES", tree.RejectSpecial)
	inScope.context = "VALUES"

	var tableDesc *sqlbase.TableDescriptor
	_, isInsert := b.stmt.(*tree.Insert)
	if isInsert {
		if b.catalog != nil && mb != nil && mb.tab != nil {
			tb, _ := b.catalog.ResolveTableDesc(mb.tab.Name())
			if tb != nil {
				tabledesc, ok := tb.(*sqlbase.TableDescriptor)
				if !ok {
					panic(pgerror.NewAssertionErrorf("table descriptor does not exist"))
				}
				tableDesc = tabledesc
			}
		}
	}

	for _, tuple := range values.Rows {
		if numCols != len(tuple) {
			reportValuesLenError(numCols, len(tuple))
		}

		elems := make(memo.ScalarListExpr, numCols)
		for i, expr := range tuple {
			desired := types.Any
			if i < len(desiredTypes) {
				desired = desiredTypes[i]
			}
			texpr := inScope.resolveType(expr, desired)
			typ := texpr.ResolvedType()
			if desired != types.Any && typ != desired && !tree.IsNullExpr(texpr) {
				if ok, c := tree.IsCastDeepValid(typ, desired); ok {
					telemetry.Inc(c)
					width := 0
					if isInsert {
						width, _ = getTargetColWidthAndPrecesion(tableDesc, mb, i)
					}
					CastExpr := tree.ChangeToCastExpr(texpr, desired, width)
					texpr = &CastExpr
					typ = desired
				}

			}
			elems[i] = b.buildScalar(texpr, inScope, nil, nil, nil)

			// Verify that types of each tuple match one another.
			if colTypes[i] == types.Unknown {
				colTypes[i] = typ
			} else if typ != types.Unknown && !typ.Equivalent(colTypes[i]) {
				panic(pgerror.NewErrorf(pgcode.DatatypeMismatch,
					"VALUES types %s and %s cannot be matched", typ, colTypes[i]))
			}
		}

		rows = append(rows, b.factory.ConstructTuple(elems, types.TTuple{Types: colTypes}))
	}

	outScope = inScope.push()
	for i := 0; i < numCols; i++ {
		// The column names for VALUES are column1, column2, etc.
		alias := fmt.Sprintf("column%d", i+1)
		b.synthesizeColumn(outScope, alias, colTypes[i], nil, nil /* scalar */)
	}

	colList := colsToColList(outScope.cols)
	outScope.expr = b.factory.ConstructValues(rows, &memo.ValuesPrivate{
		Cols: colList,
		ID:   b.factory.Metadata().NextValuesID(),
	})
	return outScope
}

func reportValuesLenError(expected, actual int) {
	panic(pgerror.NewErrorf(
		pgcode.Syntax,
		"VALUES lists must all be the same length, expected %d columns, found %d",
		expected, actual))
}

func getTargetColWidthAndPrecesion(
	tableDesc *sqlbase.TableDescriptor, mb *mutationBuilder, targetCol int,
) (int, int) {
	if tableDesc == nil {
		return 0, 0
	}
	width := 0
	precision := 0
	if tableDesc != nil {
		count := 0
		if len(mb.targetColList) != 0 {
			for _, colID := range mb.targetColList {
				if count == targetCol {
					width = tableDesc.Columns[int(colID)-(firstColID(uint64(mb.tabID)))].ColTypeWidth()
					precision = tableDesc.Columns[int(colID)-(firstColID(uint64(mb.tabID)))].ColTypePrecision()
					break
				}
				count++
			}
		} else {
			// Do not target mutation columns.
			for j, n := 0, mb.tab.ColumnCount(); j < n; j++ {
				tabCol := mb.tab.Column(j)
				if !tabCol.IsHidden() {
					if count == targetCol {
						width = tabCol.ColTypeWidth()
						precision = tabCol.ColTypePrecision()
						break
					}
					count++
				}
			}
		}
	}
	return width, precision
}

func firstColID(tableID uint64) int {
	return int(tableID & 0xffffffff)
}
