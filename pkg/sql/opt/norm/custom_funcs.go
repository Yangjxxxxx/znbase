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

package norm

import (
	"math"
	"reflect"
	"sort"

	"github.com/znbasedb/apd"
	"github.com/znbasedb/znbase/pkg/sql/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/opt"
	"github.com/znbasedb/znbase/pkg/sql/opt/cat"
	"github.com/znbasedb/znbase/pkg/sql/opt/constraint"
	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/opt/props"
	"github.com/znbasedb/znbase/pkg/sql/opt/props/physical"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/json"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// CustomFuncs contains all the custom match and replace functions used by
// the normalization rules. These are also imported and used by the explorer.
type CustomFuncs struct {
	f   *Factory
	mem *memo.Memo
}

// Init initializes a new CustomFuncs with the given factory.
func (c *CustomFuncs) Init(f *Factory) {
	c.f = f
	c.mem = f.Memo()
}

// Succeeded returns true if a result expression is not nil.
func (c *CustomFuncs) Succeeded(result opt.Expr) bool {
	return result != nil
}

// ----------------------------------------------------------------------
//
// ScalarList functions
//   General custom match and replace functions used to test and construct
//   scalar lists.
//
// ----------------------------------------------------------------------

// NeedSortedUniqueList returns true if the given list is composed entirely of
// constant values that are either not in sorted order or have duplicates. If
// true, then ConstructSortedUniqueList needs to be called on the list to
// normalize it.
func (c *CustomFuncs) NeedSortedUniqueList(list memo.ScalarListExpr) bool {
	if len(list) <= 1 {
		return false
	}
	ls := listSorter{cf: c, list: list}
	var needSortedUniqueList bool
	for i, item := range list {
		if !opt.IsConstValueOp(item) {
			return false
		}
		if i != 0 && !ls.less(i-1, i) {
			needSortedUniqueList = true
		}
	}
	return needSortedUniqueList
}

// ConstructSortedUniqueList sorts the given list and removes duplicates, and
// returns the resulting list. See the comment for listSorter.compare for
// comparison rule details.
func (c *CustomFuncs) ConstructSortedUniqueList(
	list memo.ScalarListExpr,
) (memo.ScalarListExpr, types.T) {
	// Make a copy of the list, since it needs to stay immutable.
	newList := make(memo.ScalarListExpr, len(list))
	copy(newList, list)
	ls := listSorter{cf: c, list: newList}

	// Sort the list.
	sort.Slice(ls.list, ls.less)

	// Remove duplicates from the list.
	n := 0
	for i := range newList {
		if i == 0 || ls.compare(i-1, i) < 0 {
			newList[n] = newList[i]
			n++
		}
	}
	newList = newList[:n]

	// Construct the type of the tuple.
	typ := types.TTuple{Types: make([]types.T, n)}
	for i := range newList {
		typ.Types[i] = newList[i].DataType()
	}

	return newList, typ
}

// ----------------------------------------------------------------------
//
// Typing functions
//   General custom match and replace functions used to test and construct
//   expression data types.
//
// ----------------------------------------------------------------------

// HasColType returns true if the given scalar expression has a static type
// that's equivalent to the requested coltype.
func (c *CustomFuncs) HasColType(scalar opt.ScalarExpr, dstTyp coltypes.T) bool {
	srcTyp, _ := coltypes.DatumTypeToColumnType(scalar.DataType())
	if reflect.TypeOf(srcTyp) != reflect.TypeOf(dstTyp) {
		return false
	}
	return coltypes.ColTypeAsString(srcTyp) == coltypes.ColTypeAsString(dstTyp)
}

// IsString returns true if the given scalar expression is of type String.
func (c *CustomFuncs) IsString(scalar opt.ScalarExpr) bool {
	return scalar.DataType() == types.String
}

// ColTypeToDatumType maps the given column type to a datum type.
func (c *CustomFuncs) ColTypeToDatumType(colTyp coltypes.T) types.T {
	return coltypes.CastTargetToDatumType(colTyp)
}

// BoolType returns the boolean SQL type.
func (c *CustomFuncs) BoolType() types.T {
	return types.Bool
}

// AnyType returns the wildcard Any type.
func (c *CustomFuncs) AnyType() types.T {
	return types.Any
}

// CanConstructBinary returns true if (op left right) has a valid binary op
// overload and is therefore legal to construct. For example, while
// (Minus <date> <int>) is valid, (Minus <int> <date>) is not.
func (c *CustomFuncs) CanConstructBinary(op opt.Operator, left, right opt.ScalarExpr) bool {
	return memo.BinaryOverloadExists(op, left.DataType(), right.DataType())
}

// ArrayType returns the type of the first output column wrapped
// in an array.
func (c *CustomFuncs) ArrayType(in memo.RelExpr) types.T {
	inCol, _ := c.OutputCols(in).Next(0)
	inTyp := c.mem.Metadata().ColumnMeta(opt.ColumnID(inCol)).Type
	return types.TArray{Typ: inTyp}
}

// BinaryColType returns the column type of the binary overload for the
// given operator and operands.
func (c *CustomFuncs) BinaryColType(op opt.Operator, left, right opt.ScalarExpr) coltypes.T {
	o, _ := memo.FindBinaryOverload(op, left.DataType(), right.DataType())
	colType, _ := coltypes.DatumTypeToColumnType(o.ReturnType)
	return colType
}

// ----------------------------------------------------------------------
//
// Property functions
//   General custom match and replace functions used to test expression
//   logical properties.
//
// ----------------------------------------------------------------------

// OutputCols returns the set of columns returned by the input expression.
func (c *CustomFuncs) OutputCols(input memo.RelExpr) opt.ColSet {
	return input.Relational().OutputCols
}

// OutputCols2 returns the union of columns returned by the left and right
// expressions.
func (c *CustomFuncs) OutputCols2(left, right memo.RelExpr) opt.ColSet {
	return left.Relational().OutputCols.Union(right.Relational().OutputCols)
}

// CandidateKey returns the candidate key columns from the given input
// expression. If there is no candidate key, CandidateKey returns ok=false.
func (c *CustomFuncs) CandidateKey(input memo.RelExpr) (key opt.ColSet, ok bool) {
	return input.Relational().FuncDeps.StrictKey()
}

// IsColNotNull returns true if the given input column is never null.
func (c *CustomFuncs) IsColNotNull(col opt.ColumnID, input memo.RelExpr) bool {
	return input.Relational().NotNullCols.Contains(int(col))
}

// IsColNotNull2 returns true if the given column is part of the left or right
// expressions' set of not-null columns.
func (c *CustomFuncs) IsColNotNull2(col opt.ColumnID, left, right memo.RelExpr) bool {
	return left.Relational().NotNullCols.Contains(int(col)) ||
		right.Relational().NotNullCols.Contains(int(col))
}

// OuterCols returns the set of outer columns associated with the given
// expression, whether it be a relational or scalar operator.
func (c *CustomFuncs) OuterCols(e opt.Expr) opt.ColSet {
	return c.sharedProps(e).OuterCols
}

// HasOuterCols returns true if the input expression has at least one outer
// column, or in other words, a reference to a variable that is not bound within
// its own scope. For example:
//
//   SELECT * FROM a WHERE EXISTS(SELECT * FROM b WHERE b.x = a.x)
//
// The a.x variable in the EXISTS subquery references a column outside the scope
// of the subquery. It is an "outer column" for the subquery (see the comment on
// RelationalProps.OuterCols for more details).
func (c *CustomFuncs) HasOuterCols(input opt.Expr) bool {
	return !c.OuterCols(input).Empty()
}

// IsBoundBy returns true if all outer references in the source expression are
// bound by the given columns. For example:
//
//   (InnerJoin
//     (Scan a)
//     (Scan b)
//     [ ... $item:(FiltersItem (Eq (Variable a.x) (Const 1))) ... ]
//   )
//
// The $item expression is fully bound by the output columns of the (Scan a)
// expression because all of its outer references are satisfied by the columns
// produced by the Scan.
func (c *CustomFuncs) IsBoundBy(src opt.Expr, cols opt.ColSet) bool {
	return c.OuterCols(src).SubsetOf(cols)
}

// IsCorrelated returns true if any variable in the source expression references
// a column from the destination expression. For example:
//   (InnerJoin
//     (Scan a)
//     (Scan b)
//     [ ... (FiltersItem $item:(Eq (Variable a.x) (Const 1))) ... ]
//   )
//
// The $item expression is correlated with the (Scan a) expression because it
// references one of its columns. But the $item expression is not correlated
// with the (Scan b) expression.
func (c *CustomFuncs) IsCorrelated(src, dst memo.RelExpr) bool {
	return src.Relational().OuterCols.Intersects(dst.Relational().OutputCols)
}

// HasNoCols returns true if the input expression has zero output columns.
func (c *CustomFuncs) HasNoCols(input memo.RelExpr) bool {
	return input.Relational().OutputCols.Empty()
}

// HasZeroRows returns true if the input expression never returns any rows.
func (c *CustomFuncs) HasZeroRows(input memo.RelExpr) bool {
	return input.Relational().Cardinality.IsZero()
}

// HasOneRow returns true if the input expression always returns exactly one
// row.
func (c *CustomFuncs) HasOneRow(input memo.RelExpr) bool {
	return input.Relational().Cardinality.IsOne()
}

// HasZeroOrOneRow returns true if the input expression returns at most one row.
func (c *CustomFuncs) HasZeroOrOneRow(input memo.RelExpr) bool {
	return input.Relational().Cardinality.IsZeroOrOne()
}

// CanHaveZeroRows returns true if the input expression might return zero rows.
func (c *CustomFuncs) CanHaveZeroRows(input memo.RelExpr) bool {
	return input.Relational().Cardinality.CanBeZero()
}

// ColsAreEmpty returns true if the column set is empty.
func (c *CustomFuncs) ColsAreEmpty(cols opt.ColSet) bool {
	return cols.Empty()
}

// ColsAreSubset returns true if the left columns are a subset of the right
// columns.
func (c *CustomFuncs) ColsAreSubset(left, right opt.ColSet) bool {
	return left.SubsetOf(right)
}

// ColsAreEqual returns true if left and right contain the same set of columns.
func (c *CustomFuncs) ColsAreEqual(left, right opt.ColSet) bool {
	return left.Equals(right)
}

// ColsIntersect returns true if at least one column appears in both the left
// and right sets.
func (c *CustomFuncs) ColsIntersect(left, right opt.ColSet) bool {
	return left.Intersects(right)
}

// UnionCols returns the union of the left and right column sets.
func (c *CustomFuncs) UnionCols(left, right opt.ColSet) opt.ColSet {
	return left.Union(right)
}

// UnionCols3 returns the union of the three column sets.
func (c *CustomFuncs) UnionCols3(cols1, cols2, cols3 opt.ColSet) opt.ColSet {
	cols := cols1.Union(cols2)
	cols.UnionWith(cols3)
	return cols
}

// UnionCols4 returns the union of the four column sets.
func (c *CustomFuncs) UnionCols4(cols1, cols2, cols3, cols4 opt.ColSet) opt.ColSet {
	cols := cols1.Union(cols2)
	cols.UnionWith(cols3)
	cols.UnionWith(cols4)
	return cols
}

// DifferenceCols returns the difference of the left and right column sets.
func (c *CustomFuncs) DifferenceCols(left, right opt.ColSet) opt.ColSet {
	return left.Difference(right)
}

// sharedProps returns the shared logical properties for the given expression.
// Only relational expressions and certain scalar list items (e.g. FiltersItem,
// ProjectionsItem, AggregationsItem) have shared properties.
func (c *CustomFuncs) sharedProps(e opt.Expr) *props.Shared {
	switch t := e.(type) {
	case memo.RelExpr:
		return &t.Relational().Shared
	case memo.ScalarPropsExpr:
		return &t.ScalarProps(c.mem).Shared
	}
	panic(pgerror.NewAssertionErrorf("no logical properties available for node: %v", e))
}

// ----------------------------------------------------------------------
//
// Ordering functions
//   General custom match and replace functions related to orderings.
//
// ----------------------------------------------------------------------

// HasColsInOrdering returns true if all columns that appear in an ordering are
// output columns of the input expression.
func (c *CustomFuncs) HasColsInOrdering(input memo.RelExpr, ordering physical.OrderingChoice) bool {
	return ordering.CanProjectCols(input.Relational().OutputCols)
}

// OrderingCols returns all non-optional columns that are part of the given
// OrderingChoice.
func (c *CustomFuncs) OrderingCols(ordering physical.OrderingChoice) opt.ColSet {
	return ordering.ColSet()
}

// PruneOrdering removes any columns referenced by an OrderingChoice that are
// not part of the needed column set. Should only be called if HasColsInOrdering
// is true.
func (c *CustomFuncs) PruneOrdering(
	ordering physical.OrderingChoice, needed opt.ColSet,
) physical.OrderingChoice {
	if ordering.SubsetOfCols(needed) {
		return ordering
	}
	ordCopy := ordering.Copy()
	ordCopy.ProjectCols(needed)
	return ordCopy
}

// EmptyOrdering returns a pseudo-choice that does not require any
// ordering.
func (c *CustomFuncs) EmptyOrdering() physical.OrderingChoice {
	return physical.OrderingChoice{}
}

// -----------------------------------------------------------------------
//
// Filter functions
//   General custom match and replace functions used to test and construct
//   filters in Select and Join rules.
//
// -----------------------------------------------------------------------

// FilterOuterCols returns the union of all outer columns from the given filter
// conditions.
func (c *CustomFuncs) FilterOuterCols(filters memo.FiltersExpr) opt.ColSet {
	var colSet opt.ColSet
	for i := range filters {
		colSet.UnionWith(filters[i].ScalarProps(c.mem).OuterCols)
	}
	return colSet
}

// FilterHasCorrelatedSubquery returns true if any of the filter conditions
// contain a correlated subquery.
func (c *CustomFuncs) FilterHasCorrelatedSubquery(filters memo.FiltersExpr) bool {
	for i := range filters {
		if filters[i].ScalarProps(c.mem).HasCorrelatedSubquery {
			return true
		}
	}
	return false
}

// IsFilterFalse returns true if the filters always evaluate to false. The only
// case that's checked is the fully normalized case, when the list contains a
// single False condition.
func (c *CustomFuncs) IsFilterFalse(filters memo.FiltersExpr) bool {
	return filters.IsFalse()
}

// IsContradiction returns true if the given filter item contains a
// contradiction constraint.
func (c *CustomFuncs) IsContradiction(item *memo.FiltersItem) bool {
	return item.ScalarProps(c.mem).Constraints == constraint.Contradiction
}

// ConcatFilters creates a new Filters operator that contains conditions from
// both the left and right boolean filter expressions.
func (c *CustomFuncs) ConcatFilters(left, right memo.FiltersExpr) memo.FiltersExpr {
	// No need to recompute properties on the new filters, since they should
	// still be valid.
	newFilters := make(memo.FiltersExpr, len(left)+len(right))
	copy(newFilters, left)
	copy(newFilters[len(left):], right)
	return newFilters
}

// RemoveFiltersItem returns a new list that is a copy of the given list, except
// that it does not contain the given search item. If the list contains the item
// multiple times, then only the first instance is removed. If the list does not
// contain the item, then the method panics.
func (c *CustomFuncs) RemoveFiltersItem(
	filters memo.FiltersExpr, search *memo.FiltersItem,
) memo.FiltersExpr {
	newFilters := make(memo.FiltersExpr, len(filters)-1)
	for i := range filters {
		if search == &filters[i] {
			copy(newFilters, filters[:i])
			copy(newFilters[i:], filters[i+1:])
			return newFilters
		}
	}
	panic(pgerror.NewAssertionErrorf("item to remove is not in the list: %v", search))
}

// ReplaceFiltersItem returns a new list that is a copy of the given list,
// except that the given search item has been replaced by the given replace
// item. If the list contains the search item multiple times, then only the
// first instance is replaced. If the list does not contain the item, then the
// method panics.
func (c *CustomFuncs) ReplaceFiltersItem(
	filters memo.FiltersExpr, search *memo.FiltersItem, replace opt.ScalarExpr,
) memo.FiltersExpr {
	newFilters := make([]memo.FiltersItem, len(filters))
	for i := range filters {
		if search == &filters[i] {
			copy(newFilters, filters[:i])
			newFilters[i].Condition = replace
			copy(newFilters[i+1:], filters[i+1:])
			return newFilters
		}
	}
	panic(pgerror.NewAssertionErrorf("item to replace is not in the list: %v", search))
}

// FiltersBoundBy returns true if all outer references in any of the filter
// conditions are bound by the given columns. For example:
//
//   (InnerJoin
//     (Scan a)
//     (Scan b)
//     $filters:[ (FiltersItem (Eq (Variable a.x) (Const 1))) ]
//   )
//
// The $filters expression is fully bound by the output columns of the (Scan a)
// expression because all of its outer references are satisfied by the columns
// produced by the Scan.
func (c *CustomFuncs) FiltersBoundBy(filters memo.FiltersExpr, cols opt.ColSet) bool {
	for i := range filters {
		if !filters[i].ScalarProps(c.mem).OuterCols.SubsetOf(cols) {
			return false
		}
	}
	return true
}

// ExtractBoundConditions returns a new list containing only those expressions
// from the given list that are fully bound by the given columns (i.e. all
// outer references are to one of these columns). For example:
//
//   (InnerJoin
//     (Scan a)
//     (Scan b)
//     (Filters [
//       (Eq (Variable a.x) (Variable b.x))
//       (Gt (Variable a.x) (Const 1))
//     ])
//   )
//
// Calling ExtractBoundConditions with the filter conditions list and the output
// columns of (Scan a) would extract the (Gt) expression, since its outer
// references only reference columns from a.
func (c *CustomFuncs) ExtractBoundConditions(
	filters memo.FiltersExpr, cols opt.ColSet,
) memo.FiltersExpr {
	newFilters := make(memo.FiltersExpr, 0, len(filters))
	for i := range filters {
		if c.IsBoundBy(&filters[i], cols) {
			newFilters = append(newFilters, filters[i])
		}
	}
	return newFilters
}

// ExtractUnboundConditions is the opposite of ExtractBoundConditions. Instead of
// extracting expressions that are bound by the given columns, it extracts
// list expressions that have at least one outer reference that is *not* bound
// by the given columns (i.e. it has a "free" variable).
func (c *CustomFuncs) ExtractUnboundConditions(
	filters memo.FiltersExpr, cols opt.ColSet,
) memo.FiltersExpr {
	newFilters := make(memo.FiltersExpr, 0, len(filters))
	for i := range filters {
		if !c.IsBoundBy(&filters[i], cols) {
			newFilters = append(newFilters, filters[i])
		}
	}
	return newFilters
}

// CanConsolidateFilters returns true if there are at least two different
// filter conditions that contain the same variable, where the conditions
// have tight constraints and contain a single variable. For example,
// CanConsolidateFilters returns true with filters {x > 5, x < 10}, but false
// with {x > 5, y < 10} and {x > 5, x = y}.
func (c *CustomFuncs) CanConsolidateFilters(filters memo.FiltersExpr) bool {
	var seen opt.ColSet
	for i := range filters {
		if col, ok := c.canConsolidateFilter(&filters[i]); ok {
			if seen.Contains(col) {
				return true
			}
			seen.Add(col)
		}
	}
	return false
}

// canConsolidateFilter determines whether a filter condition can be
// consolidated. Filters can be consolidated if they have tight constraints
// and contain a single variable. Examples of such filters include x < 5 and
// x IS NULL. If the filter can be consolidated, canConsolidateFilter returns
// the column ID of the variable and ok=true. Otherwise, canConsolidateFilter
// returns ok=false.
func (c *CustomFuncs) canConsolidateFilter(filter *memo.FiltersItem) (col int, ok bool) {
	if !filter.ScalarProps(c.mem).TightConstraints {
		return 0, false
	}

	outerCols := c.OuterCols(filter)
	if outerCols.Len() != 1 {
		return 0, false
	}

	col, _ = outerCols.Next(0)
	return col, true
}

// ConsolidateFilters consolidates filter conditions that contain the same
// variable, where the conditions have tight constraints and contain a single
// variable. The consolidated filters are combined with a tree of nested
// And operations, and wrapped with a Range expression.
//
// See the ConsolidateSelectFilters rule for more details about why this is
// necessary.
func (c *CustomFuncs) ConsolidateFilters(filters memo.FiltersExpr) memo.FiltersExpr {
	// First find the columns that have filter conditions that can be
	// consolidated.
	var seen, seenTwice opt.ColSet
	for i := range filters {
		if col, ok := c.canConsolidateFilter(&filters[i]); ok {
			if seen.Contains(col) {
				seenTwice.Add(col)
			} else {
				seen.Add(col)
			}
		}
	}

	newFilters := make(memo.FiltersExpr, seenTwice.Len(), len(filters)-seenTwice.Len())

	// newFilters contains an empty item for each of the new Range expressions
	// that will be created below. Fill in rangeMap to track which column
	// corresponds to each item.
	var rangeMap util.FastIntMap
	i := 0
	for col, ok := seenTwice.Next(0); ok; col, ok = seenTwice.Next(col + 1) {
		rangeMap.Set(col, i)
		i++
	}

	// Iterate through each existing filter condition, and either consolidate it
	// into one of the new Range expressions or add it unchanged to the new
	// filters.
	for i := range filters {
		if col, ok := c.canConsolidateFilter(&filters[i]); ok && seenTwice.Contains(col) {
			// This is one of the filter conditions that can be consolidated into a
			// Range.
			cond := filters[i].Condition
			switch t := cond.(type) {
			case *memo.RangeExpr:
				// If it is already a range expression, unwrap it.
				cond = t.And
			}
			rangeIdx, _ := rangeMap.Get(col)
			rangeItem := &newFilters[rangeIdx]
			if rangeItem.Condition == nil {
				// This is the first condition.
				rangeItem.Condition = cond
			} else {
				// Build a left-deep tree of ANDs.
				rangeItem.Condition = c.f.ConstructAnd(rangeItem.Condition, cond)
			}
		} else {
			newFilters = append(newFilters, filters[i])
		}
	}

	// Construct each of the new Range operators now that we have built the
	// conjunctions.
	for i, n := 0, seenTwice.Len(); i < n; i++ {
		newFilters[i].Condition = c.f.ConstructRange(newFilters[i].Condition)
	}

	return newFilters
}

// ----------------------------------------------------------------------
//
// Project functions
//   General custom match and replace functions used to test and construct
//   Project and Projections operators.
//
// ----------------------------------------------------------------------

// CanMergeProjections returns true if the outer Projections operator never
// references any of the inner Projections columns. If true, then the outer does
// not depend on the inner, and the two can be merged into a single set.
func (c *CustomFuncs) CanMergeProjections(outer, inner memo.ProjectionsExpr) bool {
	innerCols := c.ProjectionCols(inner)
	for i := range outer {
		if outer[i].ScalarProps(c.mem).OuterCols.Intersects(innerCols) {
			return false
		}
	}
	return true
}

// MergeProjections concatenates the synthesized columns from the outer
// Projections operator, and the synthesized columns from the inner Projections
// operator that are passed through by the outer. Note that the outer
// synthesized columns must never contain references to the inner synthesized
// columns; this can be verified by first calling CanMergeProjections.
func (c *CustomFuncs) MergeProjections(
	outer, inner memo.ProjectionsExpr, passthrough opt.ColSet,
) memo.ProjectionsExpr {
	// No need to recompute properties on the new projections, since they should
	// still be valid.
	newProjections := make(memo.ProjectionsExpr, len(outer), len(outer)+len(inner))
	copy(newProjections, outer)
	for i := range inner {
		item := &inner[i]
		if passthrough.Contains(int(item.Col)) {
			newProjections = append(newProjections, *item)
		}
	}
	return newProjections
}

// MergeProjectWithValues merges a Project operator with its input Values
// operator. This is only possible in certain circumstances, which are described
// in the MergeProjectWithValues rule comment.
//
// Values columns that are part of the Project passthrough columns are retained
// in the final Values operator, and Project synthesized columns are added to
// it. Any unreferenced Values columns are discarded. For example:
//
//   SELECT column1, 3 FROM (VALUES (1, 2))
//   =>
//   (VALUES (1, 3))
//
func (c *CustomFuncs) MergeProjectWithValues(
	projections memo.ProjectionsExpr, passthrough opt.ColSet, input memo.RelExpr,
) memo.RelExpr {
	newExprs := make(memo.ScalarListExpr, 0, len(projections)+passthrough.Len())
	newTypes := make([]types.T, 0, len(newExprs))
	newCols := make(opt.ColList, 0, len(newExprs))

	values := input.(*memo.ValuesExpr)
	tuple := values.Rows[0].(*memo.TupleExpr)
	for i, colID := range values.Cols {
		if passthrough.Contains(int(colID)) {
			newExprs = append(newExprs, tuple.Elems[i])
			newTypes = append(newTypes, tuple.Elems[i].DataType())
			newCols = append(newCols, colID)
		}
	}

	for i := range projections {
		item := &projections[i]
		newExprs = append(newExprs, item.Element)
		newTypes = append(newTypes, item.Element.DataType())
		newCols = append(newCols, item.Col)
	}

	rows := memo.ScalarListExpr{c.f.ConstructTuple(newExprs, types.TTuple{Types: newTypes})}
	return c.f.ConstructValues(rows, &memo.ValuesPrivate{
		Cols: newCols,
		ID:   values.ID,
	})
}

// ProjectionCols returns the ids of the columns synthesized by the given
// Projections operator.
func (c *CustomFuncs) ProjectionCols(projections memo.ProjectionsExpr) opt.ColSet {
	var colSet opt.ColSet
	for i := range projections {
		colSet.Add(int(projections[i].Col))
	}
	return colSet
}

// ProjectionOuterCols returns the union of all outer columns from the given
// projection expressions.
func (c *CustomFuncs) ProjectionOuterCols(projections memo.ProjectionsExpr) opt.ColSet {
	var colSet opt.ColSet
	for i := range projections {
		colSet.UnionWith(projections[i].ScalarProps(c.mem).OuterCols)
	}
	return colSet
}

// AreProjectionsCorrelated returns true if any element in the projections
// references any of the given columns.
func (c *CustomFuncs) AreProjectionsCorrelated(
	projections memo.ProjectionsExpr, cols opt.ColSet,
) bool {
	for i := range projections {
		if projections[i].ScalarProps(c.mem).OuterCols.Intersects(cols) {
			return true
		}
	}
	return false
}

// MakeEmptyColSet returns a column set with no columns in it.
func (c *CustomFuncs) MakeEmptyColSet() opt.ColSet {
	return opt.ColSet{}
}

// ProjectExtraCol constructs a new Project operator that passes through all
// columns in the given "in" expression, and then adds the given "extra"
// expression as an additional column.
func (c *CustomFuncs) ProjectExtraCol(
	in memo.RelExpr, extra opt.ScalarExpr, extraID opt.ColumnID,
) memo.RelExpr {
	projections := memo.ProjectionsExpr{{
		Element:    extra,
		ColPrivate: memo.ColPrivate{Col: extraID}},
	}
	return c.f.ConstructProject(in, projections, in.Relational().OutputCols)
}

// ----------------------------------------------------------------------
//
// Select Rules
//   Custom match and replace functions used with select.opt rules.
//
// ----------------------------------------------------------------------

// SimplifyFilters removes True operands from a FiltersExpr, and normalizes any
// False or Null condition to a single False condition. Null values map to False
// because FiltersExpr are only used by Select and Join, both of which treat a
// Null filter conjunct exactly as if it were false.
//
// SimplifyFilters also "flattens" any And operator child by merging its
// conditions into a new FiltersExpr list. If, after simplification, no operands
// remain, then SimplifyFilters returns an empty FiltersExpr.
//
// This method assumes that the NormalizeNestedAnds rule has already run and
// ensured a left deep And tree. If not (maybe because it's a testing scenario),
// then this rule may rematch, but it should still make forward progress).
func (c *CustomFuncs) SimplifyFilters(filters memo.FiltersExpr) memo.FiltersExpr {
	// Start by counting the number of conjuncts that will be flattened so that
	// the capacity of the FiltersExpr list can be determined.
	cnt := 0
	for _, item := range filters {
		cnt++
		condition := item.Condition
		for condition.Op() == opt.AndOp {
			cnt++
			condition = condition.(*memo.AndExpr).Left
		}
	}

	// Construct new filter list.
	newFilters := make(memo.FiltersExpr, 0, cnt)
	for _, item := range filters {
		var ok bool
		if newFilters, ok = c.addConjuncts(item.Condition, newFilters); !ok {
			return memo.FalseFilter
		}
	}

	return newFilters
}

// addConjuncts recursively walks a scalar expression as long as it continues to
// find nested And operators. It adds any conjuncts (ignoring True operators) to
// the given FiltersExpr and returns true. If it finds a False or Null operator,
// it propagates a false return value all the up the call stack, and
// SimplifyFilters maps that to a FiltersExpr that is always false.
func (c *CustomFuncs) addConjuncts(
	scalar opt.ScalarExpr, filters memo.FiltersExpr,
) (_ memo.FiltersExpr, ok bool) {
	switch t := scalar.(type) {
	case *memo.AndExpr:
		var ok bool
		if filters, ok = c.addConjuncts(t.Left, filters); !ok {
			return nil, false
		}
		return c.addConjuncts(t.Right, filters)

	case *memo.FalseExpr, *memo.NullExpr:
		// Filters expression evaluates to False if any operand is False or Null.
		return nil, false

	case *memo.TrueExpr:
		// Filters operator skips True operands.

	default:
		filters = append(filters, memo.FiltersItem{Condition: t})
	}
	return filters, true
}

// ConstructEmptyValues constructs a Values expression with no rows.
func (c *CustomFuncs) ConstructEmptyValues(cols opt.ColSet) memo.RelExpr {
	colList := make(opt.ColList, 0, cols.Len())
	for i, ok := cols.Next(0); ok; i, ok = cols.Next(i + 1) {
		colList = append(colList, opt.ColumnID(i))
	}
	return c.f.ConstructValues(memo.EmptyScalarListExpr, &memo.ValuesPrivate{
		Cols: colList,
		ID:   c.mem.Metadata().NextValuesID(),
	})
}

// ----------------------------------------------------------------------
//
// GroupBy Rules
//   Custom match and replace functions used with groupby.opt rules.
//
// ----------------------------------------------------------------------

// AggregationOuterCols returns the union of all outer columns from the given
// aggregation expressions.
func (c *CustomFuncs) AggregationOuterCols(aggregations memo.AggregationsExpr) opt.ColSet {
	var colSet opt.ColSet
	for i := range aggregations {
		colSet.UnionWith(aggregations[i].ScalarProps(c.mem).OuterCols)
	}
	return colSet
}

// GroupingAndConstCols returns the grouping columns and ConstAgg columns (for
// which the input and output column IDs match). A filter on these columns can
// be pushed through a GroupBy.
func (c *CustomFuncs) GroupingAndConstCols(
	grouping *memo.GroupingPrivate, aggs memo.AggregationsExpr,
) opt.ColSet {
	result := grouping.GroupingCols.Copy()

	// Add any ConstAgg columns.
	for i := range aggs {
		item := &aggs[i]
		if constAgg, ok := item.Agg.(*memo.ConstAggExpr); ok {
			// Verify that the input and output column IDs match.
			if item.Col == constAgg.Input.(*memo.VariableExpr).Col {
				result.Add(int(item.Col))
			}
		}
	}
	return result
}

// GroupingColsAreKey returns true if the input expression's grouping columns
// form a strict key for its output rows. A strict key means that any two rows
// will have unique key column values. Nulls are treated as equal to one another
// (i.e. no duplicate nulls allowed). Having a strict key means that the set of
// key column values uniquely determine the values of all other columns in the
// relation.
func (c *CustomFuncs) GroupingColsAreKey(grouping *memo.GroupingPrivate, input memo.RelExpr) bool {
	colSet := grouping.GroupingCols
	return input.Relational().FuncDeps.ColsAreStrictKey(colSet)
}

// IsUnorderedGrouping returns true if the given grouping ordering is not
// specified.
func (c *CustomFuncs) IsUnorderedGrouping(grouping *memo.GroupingPrivate) bool {
	return grouping.Ordering.Any()
}

// ----------------------------------------------------------------------
//
// Limit Rules
//   Custom match and replace functions used with limit.opt rules.
//
// ----------------------------------------------------------------------

// LimitGeMaxRows returns true if the given constant limit value is greater than
// or equal to the max number of rows returned by the input expression.
func (c *CustomFuncs) LimitGeMaxRows(limit tree.Datum, input memo.RelExpr) bool {
	limitVal := int64(*limit.(*tree.DInt))
	maxRows := input.Relational().Cardinality.Max
	return limitVal >= 0 && maxRows < math.MaxUint32 && limitVal >= int64(maxRows)
}

// ----------------------------------------------------------------------
//
// ProjectSet Rules
//   Custom match and replace functions used with ProjectSet rules.
//
// ----------------------------------------------------------------------

// IsZipCorrelated returns true if any element in the zip references
// any of the given columns.
func (c *CustomFuncs) IsZipCorrelated(zip memo.ZipExpr, cols opt.ColSet) bool {
	for i := range zip {
		if zip[i].ScalarProps(c.mem).OuterCols.Intersects(cols) {
			return true
		}
	}
	return false
}

// ZipOuterCols returns the union of all outer columns from the given
// zip expressions.
func (c *CustomFuncs) ZipOuterCols(zip memo.ZipExpr) opt.ColSet {
	var colSet opt.ColSet
	for i := range zip {
		colSet.UnionWith(zip[i].ScalarProps(c.mem).OuterCols)
	}
	return colSet
}

// ----------------------------------------------------------------------
//
// Set Rules
//   Custom match and replace functions used with set.opt rules.
//
// ----------------------------------------------------------------------

// ProjectColMapLeft returns a Projections operator that maps the left side
// columns in a SetPrivate to the output columns in it. Useful for replacing set
// operations with simpler constructs.
func (c *CustomFuncs) ProjectColMapLeft(set *memo.SetPrivate) memo.ProjectionsExpr {
	return c.projectColMapSide(set.OutCols, set.LeftCols)
}

// ProjectColMapRight returns a Project operator that maps the right side
// columns in a SetPrivate to the output columns in it. Useful for replacing set
// operations with simpler constructs.
func (c *CustomFuncs) ProjectColMapRight(set *memo.SetPrivate) memo.ProjectionsExpr {
	return c.projectColMapSide(set.OutCols, set.RightCols)
}

// projectColMapSide implements the side-agnostic logic from ProjectColMapLeft
// and ProjectColMapRight.
func (c *CustomFuncs) projectColMapSide(toList, fromList opt.ColList) memo.ProjectionsExpr {
	items := make(memo.ProjectionsExpr, len(toList))
	for idx, fromCol := range fromList {
		toCol := toList[idx]
		items[idx].Element = c.f.ConstructVariable(fromCol)
		items[idx].Col = toCol
	}
	return items
}

// ----------------------------------------------------------------------
//
// Window Rules
//   Custom match and replace functions used with window.opt rules.
//
// ----------------------------------------------------------------------

// ColsAreDeterminedBy returns true if the given columns are functionally
// determined by the "in" ColSet according to the functional dependencies of the
// input expression.
func (c *CustomFuncs) ColsAreDeterminedBy(cols, in opt.ColSet, input memo.RelExpr) bool {
	return input.Relational().FuncDeps.InClosureOf(cols, in)
}

// ExtractDeterminedConditions returns a new list of filters containing only
// those expressions from the given list which are bound by columns which
// are functionally determined by the given columns.
func (c *CustomFuncs) ExtractDeterminedConditions(
	filters memo.FiltersExpr, cols opt.ColSet, input memo.RelExpr,
) memo.FiltersExpr {
	newFilters := make(memo.FiltersExpr, 0, len(filters))
	for i := range filters {
		if c.ColsAreDeterminedBy(filters[i].ScalarProps(c.mem).OuterCols, cols, input) {
			newFilters = append(newFilters, filters[i])
		}
	}
	return newFilters
}

// ExtractUndeterminedConditions is the opposite of
// ExtractDeterminedConditions.
func (c *CustomFuncs) ExtractUndeterminedConditions(
	filters memo.FiltersExpr, cols opt.ColSet, input memo.RelExpr,
) memo.FiltersExpr {
	newFilters := make(memo.FiltersExpr, 0, len(filters))
	for i := range filters {
		if !c.ColsAreDeterminedBy(filters[i].ScalarProps(c.mem).OuterCols, cols, input) {
			newFilters = append(newFilters, filters[i])
		}
	}
	return newFilters
}

// AllArePrefixSafe returns whether every window function in the list satisfies
// the "prefix-safe" property.
//
// Being prefix-safe means that the computation of a window function on a given
// row does not depend on any of the rows that come after it. It's also
// precisely the property that lets us push limit operators below window
// functions:
//
//		(Limit (Window $input) n) = (Window (Limit $input n))
//
// Note that the frame affects whether a given window function is prefix-safe or not.
// rank() is prefix-safe under any frame, but avg():
//  * is not prefix-safe under RANGE BETWEEN UNBOUNDED PRECEDING TO CURRENT ROW
//    (the default), because we might cut off mid-peer group. If we can
//    guarantee that the ordering is over a key, then this becomes safe.
//  * is not prefix-safe under ROWS BETWEEN UNBOUNDED PRECEDING TO UNBOUNDED
//    FOLLOWING, because it needs to look at the entire partition.
//  * is prefix-safe under ROWS BETWEEN UNBOUNDED PRECEDING TO CURRENT ROW,
//    because it only needs to look at the rows up to any given row.
// (We don't currently handle this case).
//
// This function is best-effort. It's OK to report a function not as
// prefix-safe, even if it is.
func (c *CustomFuncs) AllArePrefixSafe(fns memo.WindowsExpr) bool {
	for i := range fns {
		if !c.isPrefixSafe(&fns[i]) {
			return false
		}
	}
	return true
}

// isPrefixSafe returns whether or not the given window function satisfies the
// "prefix-safe" property. See the comment above AllArePrefixSafe for more
// details.
func (c *CustomFuncs) isPrefixSafe(fn *memo.WindowsItem) bool {
	switch fn.Function.Op() {
	case opt.RankOp, opt.RowNumberOp, opt.DenseRankOp:
		return true
	}
	// TODO(justin): Add other cases. I think aggregates are valid here if the
	// upper bound is CURRENT ROW, and either:
	// * the mode is ROWS, or
	// * the mode is RANGE and the ordering is over a key.
	return false
}

// MakeSegmentedOrdering returns an ordering choice which satisfies both
// limitOrdering and the ordering required by a window function. Returns nil if
// no such ordering exists. See OrderingChoice.PrefixIntersection for more
// details.
func (c *CustomFuncs) MakeSegmentedOrdering(
	input memo.RelExpr,
	prefix opt.ColSet,
	ordering physical.OrderingChoice,
	limitOrdering physical.OrderingChoice,
) *physical.OrderingChoice {

	// The columns in the closure of the prefix may be included in it. It's
	// beneficial to do so for a given column iff that column appears in the
	// limit's ordering.
	cl := input.Relational().FuncDeps.ComputeClosure(prefix)
	cl.IntersectionWith(limitOrdering.ColSet())
	cl.UnionWith(prefix)
	prefix = cl
	oc, ok := limitOrdering.PrefixIntersection(prefix, ordering.Columns)
	if !ok {
		return nil
	}
	return &oc
}

// DerefOrderingChoice returns an OrderingChoice from a pointer.
func (c *CustomFuncs) DerefOrderingChoice(result *physical.OrderingChoice) physical.OrderingChoice {
	return *result
}

// RedundantCols returns the subset of the given columns that are functionally
// determined by the remaining columns. In many contexts (such as if they are
// grouping columns), these columns can be dropped. The input expression's
// functional dependencies are used to make the decision.
func (c *CustomFuncs) RedundantCols(input memo.RelExpr, cols opt.ColSet) opt.ColSet {
	reducedCols := input.Relational().FuncDeps.ReduceCols(cols)
	if reducedCols.Equals(cols) {
		return opt.ColSet{}
	}
	return cols.Difference(reducedCols)
}

// HasRangeFrameWithOffset returns true if w contains a WindowsItem Frame that
// has a mode of RANGE and has a specific offset, such as OffsetPreceding or
// OffsetFollowing.
func (c *CustomFuncs) HasRangeFrameWithOffset(w memo.WindowsExpr) bool {
	for i := range w {
		if w[i].Frame.Mode == tree.RANGE && w[i].Frame.HasOffset() {
			return true
		}
	}
	return false
}

// RemoveWindowPartitionCols returns a new window private struct with the given
// columns removed from the window partition column set.
func (c *CustomFuncs) RemoveWindowPartitionCols(
	private *memo.WindowPrivate, cols opt.ColSet,
) *memo.WindowPrivate {
	p := *private
	p.Partition = p.Partition.Difference(cols)
	return &p
}

// OrderingSucceeded returns true if an OrderingChoice is not nil.
func (c *CustomFuncs) OrderingSucceeded(result *physical.OrderingChoice) bool {
	return result != nil
}

// WindowPartition returns the set of columns that the window function uses to
// partition.
func (c *CustomFuncs) WindowPartition(priv *memo.WindowPrivate) opt.ColSet {
	return priv.Partition
}

// WindowOrdering returns the ordering used by the window function.
func (c *CustomFuncs) WindowOrdering(private *memo.WindowPrivate) physical.OrderingChoice {
	return private.Ordering
}

// ----------------------------------------------------------------------
//
// Boolean Rules
//   Custom match and replace functions used with bool.opt rules.
//
// ----------------------------------------------------------------------

// ConcatLeftDeepAnds concatenates any left-deep And expressions in the right
// expression with any left-deep And expressions in the left expression. The
// result is a combined left-deep And expression. Note that NormalizeNestedAnds
// has already guaranteed that both inputs will already be left-deep.
func (c *CustomFuncs) ConcatLeftDeepAnds(left, right opt.ScalarExpr) opt.ScalarExpr {
	if and, ok := right.(*memo.AndExpr); ok {
		return c.f.ConstructAnd(c.ConcatLeftDeepAnds(left, and.Left), and.Right)
	}
	return c.f.ConstructAnd(left, right)
}

// NegateComparison negates a comparison op like:
//   a.x = 5
// to:
//   a.x <> 5
func (c *CustomFuncs) NegateComparison(
	cmp opt.Operator, left, right opt.ScalarExpr,
) opt.ScalarExpr {
	negate := opt.NegateOpMap[cmp]
	return c.f.DynamicConstruct(negate, left, right).(opt.ScalarExpr)
}

// CommuteInequality swaps the operands of an inequality comparison expression,
// changing the operator to compensate:
//   5 < x
// to:
//   x > 5
func (c *CustomFuncs) CommuteInequality(
	op opt.Operator, left, right opt.ScalarExpr,
) opt.ScalarExpr {
	switch op {
	case opt.GeOp:
		return c.f.ConstructLe(right, left)
	case opt.GtOp:
		return c.f.ConstructLt(right, left)
	case opt.LeOp:
		return c.f.ConstructGe(right, left)
	case opt.LtOp:
		return c.f.ConstructGt(right, left)
	}
	panic(pgerror.NewAssertionErrorf("called commuteInequality with operator %s", log.Safe(op)))
}

// FindRedundantConjunct takes the left and right operands of an Or operator as
// input. It examines each conjunct from the left expression and determines
// whether it appears as a conjunct in the right expression. If so, it returns
// the matching conjunct. Otherwise, it returns nil. For example:
//
//   A OR A                               =>  A
//   B OR A                               =>  nil
//   A OR (A AND B)                       =>  A
//   (A AND B) OR (A AND C)               =>  A
//   (A AND B AND C) OR (A AND (D OR E))  =>  A
//
// Once a redundant conjunct has been found, it is extracted via a call to the
// ExtractRedundantConjunct function. Redundant conjuncts are extracted from
// multiple nested Or operators by repeated application of these functions.
func (c *CustomFuncs) FindRedundantConjunct(left, right opt.ScalarExpr) opt.ScalarExpr {
	// Recurse over each conjunct from the left expression and determine whether
	// it's redundant.
	for {
		// Assume a left-deep And expression tree normalized by NormalizeNestedAnds.
		if and, ok := left.(*memo.AndExpr); ok {
			if c.isConjunct(and.Right, right) {
				return and.Right
			}
			left = and.Left
		} else {
			if c.isConjunct(left, right) {
				return left
			}
			return nil
		}
	}
}

// isConjunct returns true if the candidate expression is a conjunct within the
// given conjunction. The conjunction is assumed to be left-deep (normalized by
// the NormalizeNestedAnds rule).
func (c *CustomFuncs) isConjunct(candidate, conjunction opt.ScalarExpr) bool {
	for {
		if and, ok := conjunction.(*memo.AndExpr); ok {
			if and.Right == candidate {
				return true
			}
			conjunction = and.Left
		} else {
			return conjunction == candidate
		}
	}
}

// ExtractRedundantConjunct extracts a redundant conjunct from an Or expression,
// and returns an And of the conjunct with the remaining Or expression (a
// logically equivalent expression). For example:
//
//   (A AND B) OR (A AND C)  =>  A AND (B OR C)
//
// If extracting the conjunct from one of the OR conditions would result in an
// empty condition, the conjunct itself is returned (a logically equivalent
// expression). For example:
//
//   A OR (A AND B)  =>  A
//
// These transformations are useful for finding a conjunct that can be pushed
// down in the query tree. For example, if the redundant conjunct A is fully
// bound by one side of a join, it can be pushed through the join, even if B and
// C cannot.
func (c *CustomFuncs) ExtractRedundantConjunct(
	conjunct, left, right opt.ScalarExpr,
) opt.ScalarExpr {
	if conjunct == left || conjunct == right {
		return conjunct
	}

	return c.f.ConstructAnd(
		conjunct,
		c.f.ConstructOr(
			c.extractConjunct(conjunct, left.(*memo.AndExpr)),
			c.extractConjunct(conjunct, right.(*memo.AndExpr)),
		),
	)
}

// extractConjunct traverses the And subtree looking for the given conjunct,
// which must be present. Once it's located, it's removed from the tree, and
// the remaining expression is returned.
func (c *CustomFuncs) extractConjunct(conjunct opt.ScalarExpr, and *memo.AndExpr) opt.ScalarExpr {
	if and.Right == conjunct {
		return and.Left
	}
	if and.Left == conjunct {
		return and.Right
	}
	return c.f.ConstructAnd(c.extractConjunct(conjunct, and.Left.(*memo.AndExpr)), and.Right)
}

// ----------------------------------------------------------------------
//
// Comparison Rules
//   Custom match and replace functions used with comp.opt rules.
//
// ----------------------------------------------------------------------

// NormalizeTupleEquality remaps the elements of two tuples compared for
// equality, like this:
//   (a, b, c) = (x, y, z)
// into this:
//   (a = x) AND (b = y) AND (c = z)
func (c *CustomFuncs) NormalizeTupleEquality(left, right memo.ScalarListExpr) opt.ScalarExpr {
	if len(left) != len(right) {
		panic(pgerror.NewAssertionErrorf("tuple length mismatch"))
	}
	if len(left) == 0 {
		// () = (), which is always true.
		return memo.TrueSingleton
	}

	var result opt.ScalarExpr
	for i := range left {
		eq := c.f.ConstructEq(left[i], right[i])
		if result == nil {
			result = eq
		} else {
			result = c.f.ConstructAnd(result, eq)
		}
	}
	return result
}

// ----------------------------------------------------------------------
//
// Scalar Rules
//   Custom match and replace functions used with scalar.opt rules.
//
// ----------------------------------------------------------------------

// SimplifyCoalesce discards any leading null operands, and then if the next
// operand is a constant, replaces with that constant.
func (c *CustomFuncs) SimplifyCoalesce(args memo.ScalarListExpr) opt.ScalarExpr {
	for i := 0; i < len(args)-1; i++ {
		item := args[i]

		// If item is not a constant value, then its value may turn out to be
		// null, so no more folding. Return operands from then on.
		if !c.IsConstValueOrTuple(item) {
			return c.f.ConstructCoalesce(args[i:])
		}

		if item.Op() != opt.NullOp {
			return item
		}
	}

	// All operands up to the last were null (or the last is the only operand),
	// so return the last operand without the wrapping COALESCE function.
	return args[len(args)-1]
}

// SimplifyNvl discards any leading null operands, and then if the next
// operand is a constant, replaces with that constant.
func (c *CustomFuncs) SimplifyNvl(args memo.ScalarListExpr) opt.ScalarExpr {
	for i := 0; i < len(args)-1; i++ {
		item := args[i]

		// If item is not a constant value, then its value may turn out to be
		// null, so no more folding. Return operands from then on.
		if !c.IsConstValueOrTuple(item) {
			return c.f.ConstructNvl(args[i:])
		}

		if item.Op() != opt.NullOp {
			return item
		}
	}

	// All operands up to the last were null (or the last is the only operand),
	// so return the last operand without the wrapping COALESCE function.
	return args[len(args)-1]
}

// AllowNullArgs returns true if the binary operator with the given inputs
// allows one of those inputs to be null. If not, then the binary operator will
// simply be replaced by null.
func (c *CustomFuncs) AllowNullArgs(op opt.Operator, left, right opt.ScalarExpr) bool {
	return memo.BinaryAllowsNullArgs(op, left.DataType(), right.DataType())
}

// FoldNullUnary replaces the unary operator with a typed null value having the
// same type as the unary operator would have.
func (c *CustomFuncs) FoldNullUnary(op opt.Operator, input opt.ScalarExpr) opt.ScalarExpr {
	return c.f.ConstructNull(memo.InferUnaryType(op, input.DataType()))
}

// FoldNullBinary replaces the binary operator with a typed null value having
// the same type as the binary operator would have.
func (c *CustomFuncs) FoldNullBinary(op opt.Operator, left, right opt.ScalarExpr) opt.ScalarExpr {
	return c.f.ConstructNull(memo.InferBinaryType(op, left.DataType(), right.DataType()))
}

// IsJSONScalar returns if the JSON value is a number, string, true, false, or null.
func (c *CustomFuncs) IsJSONScalar(value opt.ScalarExpr) bool {
	v := value.(*memo.ConstExpr).Value.(*tree.DJSON)
	return v.JSON.Type() != json.ObjectJSONType && v.JSON.Type() != json.ArrayJSONType
}

// MakeSingleKeyJSONObject returns a JSON object with one entry, mapping key to value.
func (c *CustomFuncs) MakeSingleKeyJSONObject(key, value opt.ScalarExpr) opt.ScalarExpr {
	k := key.(*memo.ConstExpr).Value.(*tree.DString)
	v := value.(*memo.ConstExpr).Value.(*tree.DJSON)

	builder := json.NewObjectBuilder(1)
	builder.Add(string(*k), v.JSON)
	j := builder.Build()

	return c.f.ConstructConst(&tree.DJSON{JSON: j})
}

// IsConstValueEqual returns whether const1 and const2 are equal.
func (c *CustomFuncs) IsConstValueEqual(const1, const2 opt.ScalarExpr) bool {
	op1 := const1.Op()
	op2 := const2.Op()
	if op1 != op2 || op1 == opt.NullOp {
		return false
	}
	switch op1 {
	case opt.TrueOp, opt.FalseOp:
		return true
	case opt.ConstOp:
		datum1 := const1.(*memo.ConstExpr).Value
		datum2 := const2.(*memo.ConstExpr).Value
		return datum1.Compare(c.f.evalCtx, datum2) == 0
	default:
		panic(pgerror.NewAssertionErrorf("unexpected Op type: %v", log.Safe(op1)))
	}
}

// SimplifyWhens removes known unreachable WHEN cases and constructs a new CASE
// statement. Any known true condition is converted to the ELSE. If only the
// ELSE remains, its expression is returned. condition must be a ConstValue.
func (c *CustomFuncs) SimplifyWhens(
	condition opt.ScalarExpr, whens memo.ScalarListExpr, orElse opt.ScalarExpr,
) opt.ScalarExpr {
	newWhens := make(memo.ScalarListExpr, 0, len(whens))
	for _, item := range whens {
		when := item.(*memo.WhenExpr)
		if opt.IsConstValueOp(when.Condition) {
			if !c.IsConstValueEqual(condition, when.Condition) {
				// Ignore known unmatching conditions.
				continue
			}

			// If this is true, we won't ever match anything else, so convert this to
			// the ELSE (or just return it if there are no earlier items).
			if len(newWhens) == 0 {
				return c.ensureTyped(when.Value, memo.InferWhensType(whens, orElse))
			}
			return c.f.ConstructCase(condition, newWhens, when.Value)
		}

		newWhens = append(newWhens, when)
	}

	// The ELSE value.
	if len(newWhens) == 0 {
		// ELSE is the only clause (there are no WHENs), remove the CASE.
		// NULLs in this position will not be typed, so we tag them with
		// a type we observed earlier.
		// typ will never be nil here because the definition of
		// SimplifyCaseWhenConstValue ensures that whens is nonempty.
		return c.ensureTyped(orElse, memo.InferWhensType(whens, orElse))
	}

	return c.f.ConstructCase(condition, newWhens, orElse)
}

// ensureTyped makes sure that any NULL passing through gets tagged with an
// appropriate type.
func (c *CustomFuncs) ensureTyped(d opt.ScalarExpr, typ types.T) opt.ScalarExpr {
	if d.DataType() == types.Unknown {
		return c.f.ConstructNull(typ)
	}
	return d
}

// OpsAreSame returns true if the two operators are the same.
func (c *CustomFuncs) OpsAreSame(left, right opt.Operator) bool {
	return left == right
}

// IsConstArray returns true if the expression is a constant array.
func (c *CustomFuncs) IsConstArray(scalar opt.ScalarExpr) bool {
	if cnst, ok := scalar.(*memo.ConstExpr); ok {
		if _, ok := cnst.Value.(*tree.DArray); ok {
			return true
		}
	}
	return false
}

// ConvertConstArrayToTuple converts a constant ARRAY datum to the equivalent
// homogeneous tuple, so ARRAY[1, 2, 3] becomes (1, 2, 3).
func (c *CustomFuncs) ConvertConstArrayToTuple(scalar opt.ScalarExpr) opt.ScalarExpr {
	darr := scalar.(*memo.ConstExpr).Value.(*tree.DArray)
	elems := make(memo.ScalarListExpr, len(darr.Array))
	ts := make([]types.T, len(darr.Array))
	for i, delem := range darr.Array {
		elems[i] = c.f.ConstructConstVal(delem, delem.ResolvedType())
		ts[i] = darr.ParamTyp
	}
	return c.f.ConstructTuple(elems, types.TTuple{Types: ts})
}

// CastToCollatedString returns the given string or collated string as a
// collated string constant with the given locale.
func (c *CustomFuncs) CastToCollatedString(str opt.ScalarExpr, locale string) opt.ScalarExpr {
	var value string
	switch t := str.(*memo.ConstExpr).Value.(type) {
	case *tree.DString:
		value = string(*t)
	case *tree.DCollatedString:
		value = t.Contents
	default:
		panic(pgerror.NewAssertionErrorf("unexpected type for COLLATE: %T", log.Safe(str.(*memo.ConstExpr).Value)))
	}

	return c.f.ConstructConst(tree.NewDCollatedString(value, locale, &c.f.evalCtx.CollationEnv))
}

// MakeUnorderedSubquery returns a SubqueryPrivate that specifies no ordering.
func (c *CustomFuncs) MakeUnorderedSubquery() *memo.SubqueryPrivate {
	return &memo.SubqueryPrivate{}
}

// SubqueryOrdering returns the ordering property on a SubqueryPrivate.
func (c *CustomFuncs) SubqueryOrdering(sub *memo.SubqueryPrivate) physical.OrderingChoice {
	var oc physical.OrderingChoice
	oc.FromOrdering(sub.Ordering)
	return oc
}

// FirstCol returns the first column in the input expression.
func (c *CustomFuncs) FirstCol(in memo.RelExpr) opt.ColumnID {
	inCol, _ := c.OutputCols(in).Next(0)
	return opt.ColumnID(inCol)
}

// MakeArrayAggCol returns a ColPrivate with the given type and an "array_agg" label.
func (c *CustomFuncs) MakeArrayAggCol(typ types.T) *memo.ColPrivate {
	return &memo.ColPrivate{Col: c.mem.Metadata().AddColumn("array_agg", typ, nil)}
}

// MakeOrderedGrouping constructs a new GroupingPrivate using the given
// grouping columns and OrderingChoice private.
func (c *CustomFuncs) MakeOrderedGrouping(
	groupingCols opt.ColSet, ordering physical.OrderingChoice,
) *memo.GroupingPrivate {
	return &memo.GroupingPrivate{GroupingCols: groupingCols, Ordering: ordering}
}

// IsLimited indicates whether a limit was pushed under the subquery
// already. See e.g. the rule IntroduceExistsLimit.
func (c *CustomFuncs) IsLimited(sub *memo.SubqueryPrivate) bool {
	return sub.WasLimited
}

// MakeLimited specifies that the subquery has a limit set
// already. This prevents e.g. the rule IntroduceExistsLimit from
// applying twice.
func (c *CustomFuncs) MakeLimited(sub *memo.SubqueryPrivate) *memo.SubqueryPrivate {
	newSub := *sub
	newSub.WasLimited = true
	return &newSub
}

// ----------------------------------------------------------------------
//
// Numeric Rules
//   Custom match and replace functions used with numeric.opt rules.
//
// ----------------------------------------------------------------------

// IsAdditive returns true if the type of the expression supports addition and
// subtraction in the natural way. This differs from "has a +/- Numeric
// implementation" because JSON has an implementation for "- INT" which doesn't
// obey x - 0 = x. Additive types include all numeric types as well as
// timestamps and dates.
func (c *CustomFuncs) IsAdditive(e opt.ScalarExpr) bool {
	return types.IsAdditiveType(e.DataType())
}

// EqualsNumber returns true if the given numeric value (decimal, float, or
// integer) is equal to the given integer value.
func (c *CustomFuncs) EqualsNumber(datum tree.Datum, value int64) bool {
	switch t := datum.(type) {
	case *tree.DDecimal:
		if value == 0 {
			return t.Decimal.IsZero()
		} else if value == 1 {
			return t.Decimal.Cmp(&tree.DecimalOne.Decimal) == 0
		}
		var dec apd.Decimal
		dec.SetInt64(value)
		return t.Decimal.Cmp(&dec) == 0

	case *tree.DFloat:
		return *t == tree.DFloat(value)

	case *tree.DInt:
		return *t == tree.DInt(value)
	}
	return false
}

// ----------------------------------------------------------------------
//
// Constant Folding Rules
//   Custom match and replace functions used with fold_constants.opt
//   rules.
//
// ----------------------------------------------------------------------

// IsListOfConstants returns true if elems is a list of constant values or
// tuples.
func (c *CustomFuncs) IsListOfConstants(elems memo.ScalarListExpr) bool {
	for _, elem := range elems {
		if !c.IsConstValueOrTuple(elem) {
			return false
		}
	}
	return true
}

// FoldArray evaluates an Array expression with constant inputs. It returns the
// array as a Const datum with type TArray.
func (c *CustomFuncs) FoldArray(elems memo.ScalarListExpr, typ types.T) opt.ScalarExpr {
	elemType := typ.(types.TArray).Typ
	a := tree.NewDArray(elemType)
	a.Array = make(tree.Datums, len(elems))
	for i := range a.Array {
		a.Array[i] = memo.ExtractConstDatum(elems[i])
		if a.Array[i] == tree.DNull {
			a.HasNulls = true
		}
	}
	return c.f.ConstructConst(a)
}

// IsConstValueOrTuple returns true if the input is a constant or a tuple of
// constants.
func (c *CustomFuncs) IsConstValueOrTuple(input opt.ScalarExpr) bool {
	return memo.CanExtractConstDatum(input)
}

// FoldBinary evaluates a binary expression with constant inputs. It returns
// a constant expression as long as it finds an appropriate overload function
// for the given operator and input types, and the evaluation causes no error.
func (c *CustomFuncs) FoldBinary(op opt.Operator, left, right opt.ScalarExpr) opt.ScalarExpr {
	lDatum, rDatum := memo.ExtractConstDatum(left), memo.ExtractConstDatum(right)

	o, ok := memo.FindBinaryOverload(op, left.DataType(), right.DataType())
	if !ok {
		return nil
	}

	result, err := o.Fn(c.f.evalCtx, lDatum, rDatum)
	if err != nil {
		return nil
	}
	return c.f.ConstructConstVal(result, o.ReturnType)
}

// FoldUnary evaluates a unary expression with a constant input. It returns
// a constant expression as long as it finds an appropriate overload function
// for the given operator and input type, and the evaluation causes no error.
func (c *CustomFuncs) FoldUnary(op opt.Operator, input opt.ScalarExpr) opt.ScalarExpr {
	datum := memo.ExtractConstDatum(input)

	o, ok := memo.FindUnaryOverload(op, input.DataType())
	if !ok {
		return nil
	}

	result, err := o.Fn(c.f.evalCtx, datum)
	if err != nil {
		return nil
	}
	return c.f.ConstructConstVal(result, o.ReturnType)
}

// FoldCast evaluates a cast expression with a constant input. It returns
// a constant expression as long as the evaluation causes no error.
func (c *CustomFuncs) FoldCast(input opt.ScalarExpr, colType coltypes.T) opt.ScalarExpr {
	switch colType.(type) {
	case *coltypes.TOid:
		// Save this cast for the execbuilder.
		return nil
	}

	datum := memo.ExtractConstDatum(input)
	texpr, err := tree.NewTypedCastExpr(datum, colType)
	if err != nil {
		return nil
	}

	result, err := texpr.Eval(c.f.evalCtx)
	if err != nil {
		return nil
	}

	return c.f.ConstructConstVal(result, c.ColTypeToDatumType(colType))
}

// isMonotonicConversion returns true if conversion of a value from FROM to
// TO is monotonic.
// That is, if a and b are values of type FROM, then
//
//   1. a = b implies a::TO = b::TO and
//   2. a < b implies a::TO <= b::TO
//
// Property (1) can be violated by cases like:
//
//   '-0'::FLOAT = '0'::FLOAT, but '-0'::FLOAT::STRING != '0'::FLOAT::STRING
//
// Property (2) can be violated by cases like:
//
//   2 < 10, but  2::STRING > 10::STRING.
//
// Note that the stronger version of (2),
//
//   a < b implies a::TO < b::TO
//
// is not required, for instance this is not generally true of conversion from
// a TIMESTAMP to a DATE, but certain such conversions can still generate spans
// in some cases where values under FROM and TO are "the same" (such as where a
// TIMESTAMP precisely falls on a date boundary).  We don't need this property
// because we will subsequently check that the values can round-trip to ensure
// that we don't lose any information by doing the conversion.
// TODO(justin): fill this out with the complete set of such conversions.
func isMonotonicConversion(from, to coltypes.T) bool {
	if from == coltypes.Timestamp ||
		from == coltypes.TimestampWithTZ ||
		from == coltypes.Date {
		return to == coltypes.Timestamp ||
			to == coltypes.TimestampWithTZ ||
			to == coltypes.Date
	}

	if from == coltypes.Int8 ||
		from == coltypes.Float8 ||
		from == coltypes.Decimal {
		return to == coltypes.Int8 ||
			to == coltypes.Float8 ||
			to == coltypes.Decimal
	}

	return false
}

// UnifyComparison attempts to convert a constant expression to the type of the
// variable expression, if that conversion can round-trip and is monotonic.
func (c *CustomFuncs) UnifyComparison(left, right opt.ScalarExpr) opt.ScalarExpr {
	v := left.(*memo.VariableExpr)
	cnst := right.(*memo.ConstExpr)

	desiredType := v.DataType()
	originalType := cnst.DataType()

	// Don't bother if they're already the same.
	if desiredType.Equivalent(originalType) {
		return nil
	}

	desiredColType, err := coltypes.DatumTypeToColumnType(desiredType)
	if err != nil {
		return nil
	}

	originalColType, err := coltypes.DatumTypeToColumnType(originalType)
	if err != nil {
		return nil
	}

	if !isMonotonicConversion(originalColType, desiredColType) {
		return nil
	}

	// Check that the datum can round-trip between the types. If this is true, it
	// means we don't lose any information needed to generate spans, and combined
	// with monotonicity means that it's safe to convert the RHS to the type of
	// the LHS.
	convertedDatum, err := tree.PerformCast(c.f.evalCtx, cnst.Value, desiredColType)
	if err != nil {
		return nil
	}

	convertedBack, err := tree.PerformCast(c.f.evalCtx, convertedDatum, originalColType)
	if err != nil {
		return nil
	}

	if convertedBack.Compare(c.f.evalCtx, cnst.Value) != 0 {
		return nil
	}

	return c.f.ConstructConst(convertedDatum)
}

// FoldComparison evaluates a comparison expression with constant inputs. It
// returns a constant expression as long as it finds an appropriate overload
// function for the given operator and input types, and the evaluation causes
// no error.
func (c *CustomFuncs) FoldComparison(op opt.Operator, left, right opt.ScalarExpr) opt.ScalarExpr {
	lDatum, rDatum := memo.ExtractConstDatum(left), memo.ExtractConstDatum(right)

	var flipped, not bool
	o, flipped, not, ok := memo.FindComparisonOverload(op, left.DataType(), right.DataType())
	if !ok {
		return nil
	}

	if flipped {
		lDatum, rDatum = rDatum, lDatum
	}

	result, err := o.Fn(c.f.evalCtx, lDatum, rDatum)
	if err != nil {
		return nil
	}
	if b, ok := result.(*tree.DBool); ok && not {
		result = tree.MakeDBool(!*b)
	}
	return c.f.ConstructConstVal(result, types.Bool)
}

// AreFiltersSorted determines whether the expressions in a FiltersExpr are
// ordered by their expression IDs.
func (c *CustomFuncs) AreFiltersSorted(f memo.FiltersExpr) bool {
	for i, n := 0, f.ChildCount(); i < n-1; i++ {
		if f.Child(i).Child(0).(opt.ScalarExpr).ID() > f.Child(i+1).Child(0).(opt.ScalarExpr).ID() {
			return false
		}
	}
	return true
}

// SortFilters sorts a filter list by the IDs of the expressions. This has the
// effect of canonicalizing FiltersExprs which may have the same filters, but
// in a different order.
func (c *CustomFuncs) SortFilters(f memo.FiltersExpr) memo.FiltersExpr {
	result := make(memo.FiltersExpr, len(f))
	for i, n := 0, f.ChildCount(); i < n; i++ {
		fi := f.Child(i).(*memo.FiltersItem)
		result[i] = *fi
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Child(0).(opt.ScalarExpr).ID() < result[j].Child(0).(opt.ScalarExpr).ID()
	})
	return result
}

// IsTableDSEngineValid check if data store engine of table (defined in scanPrivate) meet rule match pattern.
func (c *CustomFuncs) IsTableDSEngineValid(
	private *memo.ScanPrivate, _ opt.Operator, name *tree.DString,
) bool {
	var eType cat.EngineTypeSet
	eType = private.EngineTypeSet.ETypeSet & c.stringToEngineTypeSet(*name)
	if eType > 0 {
		return true
	}
	return false
}

// IsInputsDSEngineValid check if data store engine of all inputs meet rule match pattern.
func (c *CustomFuncs) IsInputsDSEngineValid(
	_ opt.Operator, name *tree.DString, args ...memo.RelExpr,
) bool {
	var matchEType cat.EngineTypeSet
	if len(args) > 0 {
		matchEType = c.stringToEngineTypeSet(*name)
	}
	for _, arg := range args {
		var eType cat.EngineTypeSet
		eType = arg.GetDataStoreEngine().ETypeSet & matchEType
		if eType == 0 {
			return false
		}
	}
	return true
}

func (c *CustomFuncs) stringToEngineTypeSet(str tree.DString) cat.EngineTypeSet {
	switch str {
	case "EngineColumnOnly":
		return cat.EngineColumnOnly
	case "EngineAll":
		return cat.EngineAll
	default:
		return cat.EngineKVOnly
	}
}

// NoJoinHints returns true if no hints were specified for this join.
func (c *CustomFuncs) NoJoinHints(p *memo.JoinPrivate) bool {
	return p.Flags.Empty()
}
