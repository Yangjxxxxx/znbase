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

package tree

import (
	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/sql/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
)

// VarName occurs inside scalar expressions.
//
// Immediately after parsing, the following types can occur:
//
// - UnqualifiedStar: a naked star as argument to a function, e.g. count(*),
//   or at the top level of a SELECT clause.
//   See also uses of StarExpr() and StarSelectExpr() in the grammar.
//
// - UnresolvedName: other names of the form `a.b....e` or `a.b...e.*`.
//
// Consumers of variable names do not like UnresolvedNames and instead
// expect either AllColumnsSelector or ColumnItem. Use
// NormalizeVarName() for this.
//
// After a ColumnItem is available, it should be further resolved, for this
// the Resolve() method should be used; see name_resolution.go.
type VarName interface {
	TypedExpr

	// NormalizeVarName() guarantees to return a variable name
	// that is not an UnresolvedName. This converts the UnresolvedName
	// to an AllColumnsSelector or ColumnItem as necessary.
	NormalizeVarName() (VarName, error)
}

var _ VarName = &UnresolvedName{}
var _ VarName = UnqualifiedStar{}
var _ VarName = &AllColumnsSelector{}
var _ VarName = &TupleStar{}
var _ VarName = &ColumnItem{}
var _ VarName = &PlpgsqlVar{}

// UnqualifiedStar corresponds to a standalone '*' in a scalar
// expression.
type UnqualifiedStar struct{}

// Format implements the NodeFormatter interface.
func (UnqualifiedStar) Format(ctx *FmtCtx) { ctx.WriteByte('*') }
func (u UnqualifiedStar) String() string   { return AsString(u) }

// NormalizeVarName implements the VarName interface.
func (u UnqualifiedStar) NormalizeVarName() (VarName, error) { return u, nil }

var singletonStarName VarName = UnqualifiedStar{}

// StarExpr is a convenience function that represents an unqualified "*".
func StarExpr() VarName { return singletonStarName }

// ResolvedType implements the TypedExpr interface.
func (UnqualifiedStar) ResolvedType() types.T {
	panic("unqualified stars ought to be replaced before this point")
}

// Variable implements the VariableExpr interface.
func (UnqualifiedStar) Variable() {}

// UnresolvedName is defined in name_part.go. It also implements the
// VarName interface, and thus TypedExpr too.

// ResolvedType implements the TypedExpr interface.
func (*UnresolvedName) ResolvedType() types.T {
	panic("unresolved names ought to be replaced before this point")
}

// Variable implements the VariableExpr interface.  Although, the
// UnresolvedName ought to be replaced to an IndexedVar before the points the
// VariableExpr interface is used.
func (*UnresolvedName) Variable() {}

// NormalizeVarName implements the VarName interface.
func (n *UnresolvedName) NormalizeVarName() (VarName, error) {
	return classifyColumnItem(n)
}

// AllColumnsSelector corresponds to a selection of all
// columns in a table when used in a SELECT clause.
// (e.g. `table.*`).
type AllColumnsSelector struct {
	// TableName corresponds to the table prefix, before the star.
	TableName *UnresolvedObjectName
}

// Format implements the NodeFormatter interface.
func (a *AllColumnsSelector) Format(ctx *FmtCtx) {
	ctx.FormatNode(a.TableName)
	ctx.WriteString(".*")
}
func (a *AllColumnsSelector) String() string { return AsString(a) }

// NormalizeVarName implements the VarName interface.
func (a *AllColumnsSelector) NormalizeVarName() (VarName, error) { return a, nil }

// Variable implements the VariableExpr interface.  Although, the
// AllColumnsSelector ought to be replaced to an IndexedVar before the points the
// VariableExpr interface is used.
func (a *AllColumnsSelector) Variable() {}

// ResolvedType implements the TypedExpr interface.
func (*AllColumnsSelector) ResolvedType() types.T {
	panic("all-columns selectors ought to be replaced before this point")
}

// ColumnItem corresponds to the name of a column in an expression.
type ColumnItem struct {
	// TableName holds the table prefix, if the name refers to a column. It is
	// optional.
	//
	// This uses UnresolvedObjectName because we need to preserve the
	// information about which parts were initially specified in the SQL
	// text. ColumnItems are intermediate data structures anyway, that
	// still need to undergo name resolution.
	TableName *UnresolvedObjectName
	// ColumnName names the designated column.
	ColumnName Name

	// This column is a selector column expression used in a SELECT
	// for an UPDATE/DELETE.
	// TODO(vivek): Do not artificially create such expressions
	// when scanning columns for an UPDATE/DELETE.
	ForUpdateOrDelete bool
}

// Format implements the NodeFormatter interface.
func (c *ColumnItem) Format(ctx *FmtCtx) {
	if c.TableName != nil {
		c.TableName.Format(ctx)
		ctx.WriteByte('.')
	}
	ctx.FormatNode(&c.ColumnName)
}
func (c *ColumnItem) String() string { return AsString(c) }

// NormalizeVarName implements the VarName interface.
func (c *ColumnItem) NormalizeVarName() (VarName, error) { return c, nil }

// Column retrieves the unqualified column name.
func (c *ColumnItem) Column() string {
	return string(c.ColumnName)
}

// Variable implements the VariableExpr interface.
//
// Note that in common uses, ColumnItem ought to be replaced to an
// IndexedVar prior to evaluation.
func (c *ColumnItem) Variable() {}

// ResolvedType implements the TypedExpr interface.
func (c *ColumnItem) ResolvedType() types.T {
	if presetTypesForTesting == nil {
		return nil
	}
	return presetTypesForTesting[c.String()]
}

// NewColumnItem constructs a column item from an already valid
// TableName. This can be used for e.g. pretty-printing.
func NewColumnItem(tn *TableName, colName Name) *ColumnItem {
	c := MakeColumnItem(tn, colName)
	return &c
}

// MakeColumnItem constructs a column item from an already valid
// TableName. This can be used for e.g. pretty-printing.
func MakeColumnItem(tn *TableName, colName Name) ColumnItem {
	c := ColumnItem{ColumnName: colName}
	if tn.Table() != "" {
		numParts := 1
		if tn.ExplicitCatalog {
			numParts = 3
		} else if tn.ExplicitSchema {
			numParts = 2
		}

		c.TableName = &UnresolvedObjectName{
			NumParts: numParts,
			Parts:    [3]string{tn.Table(), tn.Schema(), tn.Catalog()},
		}
	}
	return c
}

// PlpgsqlVar use to resolve plsql variable, store name, type and index information
type PlpgsqlVar struct {
	VarName       Name
	VarType       types.T
	Index         int
	DesiredTyp    types.T
	IsPlaceHolder bool
	/*NumParts int
	Star bool
	Parts NameParts*/
}

func (p *PlpgsqlVar) String() string { return string(p.VarName) }

// Format implements the NodeFormatter interface.
func (p *PlpgsqlVar) Format(ctx *FmtCtx) {}

// Walk implements the Expr interface.
func (p *PlpgsqlVar) Walk(visitor Visitor) Expr { return p }

// TypeCheck implements the Expr interface.
func (p *PlpgsqlVar) TypeCheck(s *SemaContext, desired types.T, useOrigin bool) (TypedExpr, error) {
	return p, nil
}

// ResolvedType implements the TypedExpr interface.
func (p *PlpgsqlVar) ResolvedType() types.T { return types.UnwrapType(p.VarType) }

// NormalizeVarName implements the VarName interface.
func (p *PlpgsqlVar) NormalizeVarName() (VarName, error) { return p, nil }

// Eval implements the TypedExpr interface.
func (p *PlpgsqlVar) Eval(ctx *EvalContext) (Datum, error) {
	varName := string(p.VarName)
	if p.Index != -1 {
		if p.Index < len(ctx.Params) {
			retDatum, err := plpgsqlVarGetReturn(ctx, ctx.Params[p.Index], p.VarType, p.DesiredTyp)
			if err != nil {
				return nil, err
			}
			return retDatum, nil
		}
		return nil, errors.New("UDR variable search index out of range")
	}

	i := len(ctx.Params) - 1
	for ; i >= 0; i-- {
		if (ctx.Params[i].VarName == varName || FindNameInNames(varName, ctx.Params[i].AliasNames)) && !ctx.Params[i].IsCursorArg {
			retDatum, err := plpgsqlVarGetReturn(ctx, ctx.Params[i], p.VarType, p.DesiredTyp)
			if err != nil {
				return nil, err
			}
			return retDatum, nil
		}
	}
	return nil, errors.Errorf("cannot find an UDR variable with name %s", varName)
}

func plpgsqlVarGetReturn(
	ctx *EvalContext, v UDRVar, originTyp types.T, desired types.T,
) (Datum, error) {
	if v.IsNull {
		return DNull, nil
	}
	fromDatum := v.VarDatum
	if desired != nil && desired != types.Any && originTyp != desired {
		desiredType, err := coltypes.DatumTypeToColumnType(desired)
		if err != nil {
			return nil, err
		}
		toDatum, err := PerformCast(ctx, fromDatum, desiredType)
		if err != nil {
			return nil, err
		}
		return toDatum, nil
	}
	return v.VarDatum, nil
}

// CursorReplaceVar represent cursor param.
type CursorReplaceVar struct {
	CurVarName  Name
	CurVarType  types.T
	CurVarDatum Datum
}

func (p *CursorReplaceVar) String() string { return string(p.CurVarName) }

// Format implements the NodeFormatter interface.
func (p *CursorReplaceVar) Format(ctx *FmtCtx) {}

// Walk implements the Expr interface.
func (p *CursorReplaceVar) Walk(visitor Visitor) Expr { return p }

// TypeCheck implements the TypedExpr interface.
func (p *CursorReplaceVar) TypeCheck(
	ctx *SemaContext, desired types.T, useOrigin bool,
) (TypedExpr, error) {
	return p, nil
}

// ResolvedType implements the TypedExpr interface.
func (p *CursorReplaceVar) ResolvedType() types.T { return p.CurVarType }

// NormalizeVarName implements the VarName interface.
func (p *CursorReplaceVar) NormalizeVarName() (VarName, error) { return p, nil }

// Eval implements the TypedExpr interface.
func (p *CursorReplaceVar) Eval(ctx *EvalContext) (Datum, error) {
	return p.CurVarDatum, nil
}
