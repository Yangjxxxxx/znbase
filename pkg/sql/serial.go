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

package sql

import (
	"context"
	"fmt"

	"github.com/znbasedb/znbase/pkg/server/telemetry"
	"github.com/znbasedb/znbase/pkg/sql/coltypes"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sessiondata"
	"github.com/znbasedb/znbase/pkg/sql/sqltelemetry"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// uniqueRowIDExpr is used as default expression when
// SessionNormalizationMode is SerialUsesRowID.
var uniqueRowIDExpr = &tree.FuncExpr{Func: tree.WrapFunction("unique_rowid")}

// uniqueRowID16Expr same as uniqueRowIDExpr
var uniqueRowID16Expr = &tree.FuncExpr{Func: tree.WrapFunction("unique_rowid16")}

// uniqueRowID32Expr same as uniqueRowIDExpr
var uniqueRowID32Expr = &tree.FuncExpr{Func: tree.WrapFunction("unique_rowid32")}

// realSequenceOpts (nil) is used when SessionNormalizationMode is
// SerialUsesSQLSequences.
var realSequenceOpts tree.SequenceOptions

// virtualSequenceOpts is used when SessionNormalizationMode is
// SerialUsesVirtualSequences.
var virtualSequenceOpts = tree.SequenceOptions{
	tree.SequenceOption{Name: tree.SeqOptVirtual},
}

// processSerialInColumnDef analyzes a column definition and determines
// whether to use a sequence if the requested type is SERIAL-like.
// If a sequence must be created, it returns an ObjectName to use
// to create the new sequence and the DatabaseDescriptor of the
// parent database where it should be created.
// The ColumnTableDef is not mutated in-place; instead a new one is returned.
func (p *planner) processSerialInColumnDef(
	ctx context.Context, d *tree.ColumnTableDef, tableName *ObjectName,
) (*tree.ColumnTableDef, *DatabaseDescriptor, *ObjectName, tree.SequenceOptions, error) {
	t, ok := d.Type.(*coltypes.TSerial)
	if !ok {
		// Column is not SERIAL: nothing to do.
		return d, nil, nil, nil, nil
	}

	if err := assertValidSerialColumnDef(d, tableName); err != nil {
		return nil, nil, nil, nil, err
	}

	newSpec := *d

	// Make the column non-nullable in all cases. PostgreSQL requires
	// this.
	newSpec.Nullable.Nullability = tree.NotNull

	serialNormalizationMode := p.SessionData().SerialNormalizationMode

	// Find the integer type that corresponds to the specification.
	switch serialNormalizationMode {
	case sessiondata.SerialUsesRowID, sessiondata.SerialUsesVirtualSequences:
		// If unique_rowid() or virtual sequences are requested, we have
		// no choice but to use the full-width integer type, no matter
		// which serial size was requested, otherwise the values will not fit.
		//
		// TODO(bob): Follow up with https://github.com/znbasedb/znbase/issues/32534
		// when the default is inverted to determine if we should also
		// switch this behavior around.
		newSpec.Type = d.Type.(*coltypes.TSerial).TInt

	case sessiondata.SerialUsesSQLSequences:
		// With real sequences we can use exactly the requested type.
		newSpec.Type = t.TInt
	}

	telemetry.Inc(sqltelemetry.SerialColumnNormalizationCounter(t.String(), serialNormalizationMode.String()))

	if serialNormalizationMode == sessiondata.SerialUsesRowID {
		// We're not constructing a sequence for this SERIAL column.
		// Use the "old school" ZNBaseDB default.
		switch t.Width {
		case 16:
			newSpec.DefaultExpr.Expr = uniqueRowID16Expr
		case 32:
			newSpec.DefaultExpr.Expr = uniqueRowID32Expr
		default:
			newSpec.DefaultExpr.Expr = uniqueRowIDExpr
		}
		return &newSpec, nil, nil, nil, nil
	}

	log.VEventf(ctx, 2, "creating sequence for new column %q of %q", d, tableName)

	// We want a sequence; for this we need to generate a new sequence name.
	// The constraint on the name is that an object of this name must not exist already.
	seqName := tree.NewUnqualifiedTableName(
		tree.Name(tableName.Table() + "_" + string(d.Name) + "_seq"))
	seqName.SchemaName = tableName.SchemaName
	seqName.ExplicitSchema = tableName.ExplicitSchema
	seqName.CatalogName = tableName.CatalogName
	seqName.ExplicitCatalog = tableName.ExplicitCatalog
	// The first step in the search is to prepare the seqName to fill in
	// the catalog/schema parent. This is what ResolveUncachedDatabase does.
	//
	// Here and below we skip the cache because name resolution using
	// the cache does not work (well) if the txn retries and the
	// descriptor was written already in an early txn attempt.
	dbDesc, err := p.ResolveUncachedDatabase(ctx, seqName)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// Now skip over all names that are already taken.
	nameBase := seqName.TableName
	for i := 0; ; i++ {
		if i > 0 {
			seqName.TableName = tree.Name(fmt.Sprintf("%s%d", nameBase, i))
		}
		res, err := p.ResolveUncachedTableDescriptor(ctx, seqName, false /*required*/, anyDescType)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if res == nil {
			break
		}
	}

	var wrapFunc string
	switch t.Width {
	case 16:
		wrapFunc = "nextval16"
	case 32:
		wrapFunc = "nextval32"
	default:
		wrapFunc = "nextval"
	}

	defaultExpr := &tree.FuncExpr{
		Func:  tree.WrapFunction(wrapFunc),
		Exprs: tree.Exprs{tree.NewStrVal(seqName.Catalog() + "." + seqName.Schema() + "." + seqName.Table())},
	}

	seqType := ""
	seqOpts := realSequenceOpts
	if serialNormalizationMode == sessiondata.SerialUsesVirtualSequences {
		seqType = "virtual "
		seqOpts = virtualSequenceOpts
	}
	log.VEventf(ctx, 2, "new column %q of %q will have %ssequence name %q and default %q",
		d, tableName, seqType, seqName, defaultExpr)

	newSpec.DefaultExpr.Expr = defaultExpr

	return &newSpec, dbDesc, seqName, seqOpts, nil
}

// SimplifySerialInColumnDefWithRowID analyzes a column definition and
// simplifies any use of SERIAL as if SerialNormalizationMode was set
// to SerialUsesRowID. No sequence needs to be created.
//
// This is currently used by bulk I/O import statements which do not
// (yet?) support customization of the SERIAL behavior.
func SimplifySerialInColumnDefWithRowID(
	ctx context.Context, d *tree.ColumnTableDef, tableName *ObjectName,
) error {
	t, ok := d.Type.(*coltypes.TSerial)
	if !ok {
		// Column is not SERIAL: nothing to do.
		return nil
	}

	if err := assertValidSerialColumnDef(d, tableName); err != nil {
		return err
	}

	// Make the column non-nullable in all cases. PostgreSQL requires
	// this.
	d.Nullable.Nullability = tree.NotNull

	// We're not constructing a sequence for this SERIAL column.
	// Use the "old school" ZNBaseDB default.
	d.Type = t.TInt
	switch t.Width {
	case 16:
		d.DefaultExpr.Expr = uniqueRowID16Expr
	case 32:
		d.DefaultExpr.Expr = uniqueRowID32Expr
	default:
		d.DefaultExpr.Expr = uniqueRowIDExpr
	}

	return nil
}

func assertValidSerialColumnDef(d *tree.ColumnTableDef, tableName *ObjectName) error {
	if d.HasDefaultExpr() {
		// SERIAL implies a new default expression, we can't have one to
		// start with. This is the error produced by pg in such case.
		return pgerror.NewErrorf(pgcode.Syntax,
			"multiple default values specified for column %q of table %q",
			tree.ErrString(&d.Name), tree.ErrString(tableName))
	}

	if d.Nullable.Nullability == tree.Null {
		// SERIAL implies a non-NULL column, we can't accept a nullability
		// spec. This is the error produced by pg in such case.
		return pgerror.NewErrorf(pgcode.Syntax,
			"conflicting NULL/NOT NULL declarations for column %q of table %q",
			tree.ErrString(&d.Name), tree.ErrString(tableName))
	}

	if d.Computed.Expr != nil {
		// SERIAL cannot be a computed column.
		return pgerror.NewErrorf(pgcode.Syntax,
			"SERIAL column %q of table %q cannot be computed",
			tree.ErrString(&d.Name), tree.ErrString(tableName))
	}

	return nil
}
