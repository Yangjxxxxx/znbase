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

package memo

import (
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/sql/opt"
	"github.com/znbasedb/znbase/pkg/sql/opt/props"
	"github.com/znbasedb/znbase/pkg/sql/opt/props/physical"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// CheckExpr does sanity checking on an Expr. This code is called in testrace
// builds (which gives us test/CI coverage but elides this code in regular
// builds).
//
// This function does not assume that the expression has been fully normalized.
func (m *Memo) checkExpr(e opt.Expr) {
	// RaceEnabled ensures that checks are run on every PR (as part of make
	// testrace) while keeping the check code out of non-test builds.
	if !util.RaceEnabled {
		return
	}

	// Check properties.
	switch t := e.(type) {
	case RelExpr:
		t.Relational().Verify()

		// If the expression was added to an existing group, cross-check its
		// properties against the properties of the group. Skip this check if the
		// operator is known to not have code for building logical props.
		if t != t.FirstExpr() && t.Op() != opt.MergeJoinOp {
			var relProps props.Relational
			// Don't build stats when verifying logical props - unintentionally
			// building stats for non-normalized expressions could add extra colStats
			// to the output in opt_tester in cases where checkExpr runs (i.e. testrace)
			// compared to cases where it doesn't.
			m.logPropsBuilder.disableStats = true
			m.logPropsBuilder.buildProps(t, &relProps)
			m.logPropsBuilder.disableStats = false
			t.Relational().VerifyAgainst(&relProps)
		}

	case ScalarPropsExpr:
		t.ScalarProps(m).Verify()
	}

	// Check operator-specific fields.
	switch t := e.(type) {
	case *ScanExpr:
		if t.Flags.NoIndexJoin && t.Flags.IndexHintType == keys.IndexHintForce {
			panic(pgerror.NewAssertionErrorf("NoIndexJoin and ForceIndex set"))
		}

	case *ProjectExpr:
		for _, item := range t.Projections {
			// Check that list items are not nested.
			if opt.IsListItemOp(item.Element) {
				panic(pgerror.NewAssertionErrorf("projections list item cannot contain another list item"))
			}

			// Check that column id is set.
			if item.Col == 0 {
				panic(pgerror.NewAssertionErrorf("projections column cannot have id of 0"))
			}

			// Check that column is not both passthrough and synthesized.
			if t.Passthrough.Contains(int(item.Col)) {
				panic(pgerror.NewAssertionErrorf("both passthrough and synthesized have column %d", log.Safe(item.Col)))
			}

			// Check that columns aren't passed through in projection expressions.
			if v, ok := item.Element.(*VariableExpr); ok {
				if v.Col == item.Col {
					panic(pgerror.NewAssertionErrorf("projection passes through column %d", log.Safe(item.Col)))
				}
			}
		}

	case *SelectExpr:
		checkFilters(t.Filters)

	case *AggregationsExpr:
		var checkAggs func(scalar opt.ScalarExpr)
		checkAggs = func(scalar opt.ScalarExpr) {
			switch scalar.Op() {
			case opt.AggDistinctOp:
				checkAggs(scalar.Child(0).(opt.ScalarExpr))

			case opt.VariableOp:

			default:
				if !opt.IsAggregateOp(scalar) {
					panic(pgerror.NewAssertionErrorf("aggregate contains illegal op: %s", log.Safe(scalar.Op())))
				}
			}
		}
		for _, item := range *t {
			// Check that aggregations only contain aggregates and variables.
			checkAggs(item.Agg)

			// Check that column id is set.
			if item.Col == 0 {
				panic(pgerror.NewAssertionErrorf("aggregations column cannot have id of 0"))
			}

			// Check that we don't have any bare variables as aggregations.
			if item.Agg.Op() == opt.VariableOp {
				panic(pgerror.NewAssertionErrorf("aggregation contains bare variable"))
			}
		}

	case *DistinctOnExpr:
		// Check that aggregates can be only FirstAgg or ConstAgg.
		for _, item := range t.Aggregations {
			switch item.Agg.Op() {
			case opt.FirstAggOp, opt.ConstAggOp:

			default:
				panic(pgerror.NewAssertionErrorf("distinct-on contains %s", log.Safe(item.Agg.Op())))
			}
		}

	case *GroupByExpr, *ScalarGroupByExpr:
		// Check that aggregates cannot be FirstAgg.
		for _, item := range *t.Child(1).(*AggregationsExpr) {
			switch item.Agg.Op() {
			case opt.FirstAggOp:
				panic(pgerror.NewAssertionErrorf("group-by contains %s", log.Safe(item.Agg.Op())))
			}
		}

	case *IndexJoinExpr:
		if t.Cols.Empty() {
			panic(pgerror.NewAssertionErrorf("index join with no columns"))
		}

	case *LookupJoinExpr:
		if len(t.KeyCols) == 0 {
			panic(pgerror.NewAssertionErrorf("lookup join with no key columns"))
		}
		if t.Cols.Empty() {
			panic(pgerror.NewAssertionErrorf("lookup join with no output columns"))
		}
		if t.Cols.SubsetOf(t.Input.Relational().OutputCols) {
			panic(pgerror.NewAssertionErrorf("lookup join with no lookup columns"))
		}

	case *InsertExpr:
		tab := m.Metadata().Table(t.Table)
		m.checkColListLen(t.InsertCols, tab.DeletableColumnCount(), "InsertCols")
		m.checkColListLen(t.FetchCols, 0, "FetchCols")
		m.checkColListLen(t.UpdateCols, 0, "UpdateCols")

		// Ensure that insert columns include all columns except for delete-only
		// mutation columns (which do not need to be part of INSERT).
		for i, n := 0, tab.WritableColumnCount(); i < n; i++ {
			if t.InsertCols[i] == 0 {
				panic(pgerror.NewAssertionErrorf("insert values not provided for all table columns"))
			}
		}

		m.checkMutationExpr(t, &t.MutationPrivate)

	case *UpdateExpr:
		tab := m.Metadata().Table(t.Table)
		m.checkColListLen(t.InsertCols, 0, "InsertCols")
		m.checkColListLen(t.FetchCols, tab.DeletableColumnCount(), "FetchCols")
		m.checkColListLen(t.UpdateCols, tab.DeletableColumnCount(), "UpdateCols")
		m.checkMutationExpr(t, &t.MutationPrivate)

	case *ZigzagJoinExpr:
		if len(t.LeftEqCols) != len(t.RightEqCols) {
			panic(pgerror.NewAssertionErrorf("zigzag join with mismatching eq columns"))
		}

	case *AggDistinctExpr:
		if t.Input.Op() == opt.AggFilterOp {
			panic(pgerror.NewAssertionErrorf("AggFilter should always be on top of AggDistinct"))
		}

	case *ConstExpr:
		if t.Value == tree.DNull {
			panic(pgerror.NewAssertionErrorf("NULL values should always use NullExpr, not ConstExpr"))
		}

	default:
		if !opt.IsListOp(e) {
			for i := 0; i < e.ChildCount(); i++ {
				child := e.Child(i)
				if opt.IsListItemOp(child) {
					panic(pgerror.NewAssertionErrorf("non-list op contains item op: %s", log.Safe(child.Op())))
				}
			}
		}

		if e.Op() == opt.StringAggOp && !CanExtractConstDatum(e.Child(1)) {
			panic(pgerror.NewAssertionErrorf(
				"second argument to StringAggOp must always be constant, but got %s",
				log.Safe(e.Child(1).Op()),
			))
		}

		if opt.IsJoinOp(e) {
			checkFilters(*e.Child(2).(*FiltersExpr))
		}
	}

	// Check orderings within operators.
	checkExprOrdering(e)
}

func (m *Memo) checkColListLen(colList opt.ColList, expectedLen int, listName string) {
	if len(colList) != expectedLen {
		panic(pgerror.NewAssertionErrorf("column list %s expected length = %d, actual length = %d",
			listName, log.Safe(expectedLen), len(colList)))
	}
}

func (m *Memo) checkMutationExpr(rel RelExpr, private *MutationPrivate) {
	// Output columns should never include mutation columns.
	tab := m.Metadata().Table(private.Table)
	var mutCols opt.ColSet
	for i, n := tab.ColumnCount(), tab.DeletableColumnCount(); i < n; i++ {
		mutCols.Add(int(private.Table.ColumnID(i)))
	}
	if rel.Relational().OutputCols.Intersects(mutCols) {
		panic(pgerror.NewAssertionErrorf("output columns cannot include mutation columns"))
	}
}

// checkExprOrdering runs checks on orderings stored inside operators.
func checkExprOrdering(e opt.Expr) {
	// Verify that orderings stored in operators only refer to columns produced by
	// their input.
	var ordering physical.OrderingChoice
	switch t := e.Private().(type) {
	case *physical.OrderingChoice:
		ordering = *t
	case *OrdinalityPrivate:
		ordering = t.Ordering
	case GroupingPrivate:
		ordering = t.Ordering
	default:
		return
	}
	if outCols := e.(RelExpr).Relational().OutputCols; !ordering.SubsetOfCols(outCols) {
		panic(pgerror.NewAssertionErrorf(
			"invalid ordering %v (op: %s, outcols: %v)",
			log.Safe(ordering), log.Safe(e.Op()), log.Safe(outCols),
		))
	}
}

func checkFilters(filters FiltersExpr) {
	for _, item := range filters {
		if opt.IsListItemOp(item.Condition) {
			panic(pgerror.NewAssertionErrorf("filters list item cannot contain another list item"))
		}
		if item.Condition.Op() == opt.RangeOp {
			if !item.scalar.TightConstraints {
				panic(pgerror.NewAssertionErrorf("Range operator should always have tight constraints"))
			}
			if item.scalar.OuterCols.Len() != 1 {
				panic(pgerror.NewAssertionErrorf("Range operator should have exactly one outer col"))
			}
		}
	}
}

// IsManualCastVariable is to determine if the expr is a castExpr, need to modify scalar.opt
func IsManualCastVariable(expr opt.Expr) (*VariableExpr, bool) {
	if castExpr, ok1 := expr.(*CastExpr); ok1 {
		if _, ok2 := castExpr.Input.(*VariableExpr); ok2 && castExpr.IsManual {
			return castExpr.Input.(*VariableExpr), true
		}
		return IsManualCastVariable(castExpr.Input)
	}
	return nil, false
}
