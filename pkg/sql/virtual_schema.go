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
	"sort"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/security"
	"github.com/znbasedb/znbase/pkg/security/privilege"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/parser"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/log"
)

//
// Programmer interface to define virtual schemasMap.
//

// virtualSchema represents a database with a set of virtual tables. Virtual
// tables differ from standard tables in that they are not persisted to storage,
// and instead their contents are populated whenever they are queried.
//
// The virtual database and its virtual tables also differ from standard databases
// and tables in that their descriptors are not distributed, but instead live statically
// in code. This means that they are accessed separately from standard descriptors.
type virtualSchema struct {
	name           string
	allTableNames  map[string]struct{}
	tableDefs      map[sqlbase.ID]virtualSchemaDef
	tableValidator func(*sqlbase.TableDescriptor) error // optional
	// Some virtual tables can be used if there is no current database set; others can't.
	validWithNoDatabaseContext bool
}

// virtualSchemaDef represents the interface of a table definition within a virtualSchema.
type virtualSchemaDef interface {
	getSchema() string
	initVirtualTableDesc(
		ctx context.Context, st *cluster.Settings, id sqlbase.ID,
	) (sqlbase.TableDescriptor, error)
	getComment() string
}

// virtualSchemaTable represents a table within a virtualSchema.
type virtualSchemaTable struct {
	// Exactly one of the populate and generator fields should be defined for
	// each virtualSchemaTable.
	schema string

	// comment represents comment of virtual schema table.
	comment string

	// populate, if non-nil, is a function that is used when creating a
	// valuesNode. This function eagerly loads every row of the virtual table
	// during initialization of the valuesNode.
	populate func(ctx context.Context, p *planner, db *DatabaseDescriptor, addRow func(...tree.Datum) error) error

	// generator, if non-nil, is a function that is used when creating a
	// virtualTableNode. This function returns a virtualTableGenerator function
	// which generates the next row of the virtual table when called.
	generator func(ctx context.Context, p *planner, db *DatabaseDescriptor) (virtualTableGenerator, error)

	// delegate, if non-nil, uses delegateQuery to reroute a query on this virtual
	// table into another more efficient query.
	delegate func(ctx context.Context, p *planner, db *DatabaseDescriptor) (planNode, error)
}

var _ virtualSchemaDef = virtualSchemaView{}

// virtualSchemaView represents a view within a virtualSchema
type virtualSchemaView struct {
	schema        string
	resultColumns sqlbase.ResultColumns
}

// getSchema is part of the virtualSchemaDef interface.
func (t virtualSchemaTable) getSchema() string {
	return t.schema
}

// initVirtualTableDesc is part of the virtualSchemaDef interface.
func (t virtualSchemaTable) initVirtualTableDesc(
	ctx context.Context, st *cluster.Settings, id sqlbase.ID,
) (sqlbase.TableDescriptor, error) {
	stmt, err := parser.ParseOne(t.schema, false)
	if err != nil {
		return sqlbase.TableDescriptor{}, err
	}

	create := stmt.AST.(*tree.CreateTable)
	for _, def := range create.Defs {
		if d, ok := def.(*tree.ColumnTableDef); ok && d.HasDefaultExpr() {
			return sqlbase.TableDescriptor{},
				errors.Errorf("virtual tables are not allowed to use default exprs "+
					"because bootstrapping: %s:%s", &create.Table, d.Name)
		}
	}

	// Virtual tables never use SERIAL so we need not process SERIAL
	// types here.
	mutDesc, err := MakeTableDesc(
		ctx,
		nil, /* txn */
		nil, /* vt */
		st,
		create,
		0, /* parentID */
		id,
		hlc.Timestamp{}, /* creationTime */
		publicSelectPrivileges,
		nil, /* affected */
		nil, /* semaCtx */
		nil, /* evalCtx */
		nil,
		nil,
	)
	return mutDesc.TableDescriptor, err
}

// getComment is part of the virtualSchemaDef interface.
func (t virtualSchemaTable) getComment() string {
	return t.comment
}

// getSchema is part of the virtualSchemaDef interface.
func (v virtualSchemaView) getSchema() string {
	return v.schema
}

// initVirtualTableDesc is part of the virtualSchemaDef interface.
func (v virtualSchemaView) initVirtualTableDesc(
	ctx context.Context, st *cluster.Settings, id sqlbase.ID,
) (sqlbase.TableDescriptor, error) {
	stmt, err := parser.ParseOne(v.schema, false)
	if err != nil {
		return sqlbase.TableDescriptor{}, err
	}

	create := stmt.AST.(*tree.CreateView)

	mutDesc, err := MakeViewTableDesc(
		create,
		v.resultColumns,
		0,
		id,
		hlc.Timestamp{},
		publicSelectPrivileges,
		nil, // semaCtx
		nil, // evalCtx
	)
	return mutDesc.TableDescriptor, err
}

// getComment is part of the virtualSchemaDef interface.
func (v virtualSchemaView) getComment() string {
	return ""
}

// virtualSchemas holds a slice of statically registered virtualSchema objects.
//
// When adding a new virtualSchema, define a virtualSchema in a separate file, and
// add that object to this slice.
var virtualSchemas = map[sqlbase.ID]virtualSchema{
	sqlbase.InformationSchemaID: informationSchema,
	sqlbase.PgCatalogID:         pgCatalog,
	sqlbase.CrdbInternalID:      znbaseInternal,
}

//
// SQL-layer interface to work with virtual schemasMap.
//

// VirtualSchemaHolder is a type used to provide convenient access to virtual
// database and table descriptors. VirtualSchemaHolder, virtualSchemaEntry,
// and virtualDefEntry make up the generated data structure which the
// virtualSchemas slice is mapped to. Because of this, they should not be
// created directly, but instead will be populated in a post-startup hook
// on an Executor.
type VirtualSchemaHolder struct {
	entries      map[string]virtualSchemaEntry
	orderedNames []string
}

type virtualSchemaEntry struct {
	desc            *sqlbase.DatabaseDescriptor
	defs            map[string]virtualDefEntry
	orderedDefNames []string
	allTableNames   map[string]struct{}
}

type virtualDefEntry struct {
	virtualDef                 virtualSchemaDef
	desc                       *sqlbase.TableDescriptor
	comment                    string
	validWithNoDatabaseContext bool
}

type virtualTableConstructor func(context.Context, *planner, string) (planNode, error)

var errInvalidDbPrefix = pgerror.NewError(pgcode.UndefinedObject,
	"cannot access virtual schema in anonymous database",
).SetHintf("verify that the current database is set")

func newInvalidVirtualSchemaError() error {
	return pgerror.NewAssertionErrorf("virtualSchema cannot have both the populate and generator functions defined")
}

func newInvalidVirtualDefEntryError() error {
	return pgerror.NewAssertionErrorf("virtualDefEntry.virtualDef must be a virtualSchemaTable")
}

// getPlanInfo returns the column metadata and a constructor for a new
// valuesNode for the virtual table. We use deferred construction here
// so as to avoid populating a RowContainer during query preparation,
// where we can't guarantee it will be Close()d in case of error.
func (e virtualDefEntry) getPlanInfo() (sqlbase.ResultColumns, virtualTableConstructor) {
	var columns sqlbase.ResultColumns
	for _, col := range e.desc.Columns {
		columns = append(columns, sqlbase.ResultColumn{
			Name: col.Name,
			Typ:  col.Type.ToDatumType(),
		})
	}

	constructor := func(ctx context.Context, p *planner, dbName string) (planNode, error) {
		var dbDesc *DatabaseDescriptor
		if dbName != "" {
			var err error
			dbDesc, err = p.LogicalSchemaAccessor().GetDatabaseDesc(ctx, p.txn, dbName,
				DatabaseLookupFlags{required: true})
			if err != nil {
				return nil, err
			}
		} else {
			if !e.validWithNoDatabaseContext {
				return nil, errInvalidDbPrefix
			}
		}

		switch def := e.virtualDef.(type) {
		case virtualSchemaTable:
			if def.delegate != nil {
				return def.delegate(ctx, p, dbDesc)
			}

			if def.generator != nil && def.populate != nil {
				return nil, newInvalidVirtualSchemaError()
			}

			if def.generator != nil {
				next, err := def.generator(ctx, p, dbDesc)
				if err != nil {
					return nil, err
				}
				return p.newContainerVirtualTableNode(columns, 0, next), nil
			}
			v := p.newContainerValuesNode(columns, 0)

			if err := def.populate(ctx, p, dbDesc, func(datums ...tree.Datum) error {
				if r, c := len(datums), len(v.columns); r != c {
					log.Fatalf(ctx, "datum row count and column count differ: %d vs %d", r, c)
				}
				for i, col := range v.columns {
					datum := datums[i]
					if datum == tree.DNull {
						if !e.desc.Columns[i].Nullable {
							log.Fatalf(ctx, "column %s.%s not nullable, but found NULL value",
								e.desc.Name, col.Name)
						}
					} else if !datum.ResolvedType().Equivalent(col.Typ) {
						log.Fatalf(ctx, "datum column %q expected to be type %s; found type %s",
							col.Name, col.Typ, datum.ResolvedType())
					}
				}
				_, err := v.rows.AddRow(ctx, datums)
				return err
			}); err != nil {
				v.Close(ctx)
				return nil, err
			}

			return v, nil
		default:
			return nil, newInvalidVirtualDefEntryError()
		}
	}

	return columns, constructor
}

// NewVirtualSchemaHolder creates a new VirtualSchemaHolder.
func NewVirtualSchemaHolder(
	ctx context.Context, st *cluster.Settings,
) (*VirtualSchemaHolder, error) {
	vs := &VirtualSchemaHolder{
		entries:      make(map[string]virtualSchemaEntry, len(virtualSchemas)),
		orderedNames: make([]string, len(virtualSchemas)),
	}

	order := 0
	for schemaID, schema := range virtualSchemas {
		dbName := schema.name
		dbDesc := initVirtualDatabaseDesc(schemaID, dbName)
		defs := make(map[string]virtualDefEntry, len(schema.tableDefs))
		orderedDefNames := make([]string, 0, len(schema.tableDefs))

		for id, def := range schema.tableDefs {
			tableDesc, err := def.initVirtualTableDesc(ctx, st, id)

			if err != nil {
				return nil, errors.Wrapf(err, "failed to initialize %s", def.getSchema())
			}

			if schema.tableValidator != nil {
				if err := schema.tableValidator(&tableDesc); err != nil {
					return nil, errors.Wrap(err, "programmer error")
				}
			}

			defs[tableDesc.Name] = virtualDefEntry{
				virtualDef:                 def,
				desc:                       &tableDesc,
				validWithNoDatabaseContext: schema.validWithNoDatabaseContext,
				comment:                    def.getComment(),
			}
			orderedDefNames = append(orderedDefNames, tableDesc.Name)
		}

		sort.Strings(orderedDefNames)

		vs.entries[dbName] = virtualSchemaEntry{
			desc:            dbDesc,
			defs:            defs,
			orderedDefNames: orderedDefNames,
			allTableNames:   schema.allTableNames,
		}
		vs.orderedNames[order] = dbName
		order++
	}
	sort.Strings(vs.orderedNames)
	return vs, nil
}

// Virtual databases and tables each have SELECT privileges for "public", which includes
// all users. However, virtual schemasMap have more fine-grained access control.
// For instance, information_schema will only expose rows to a given user which that
// user has access to.
var publicSelectPrivileges = sqlbase.NewPrivilegeDescriptor(sqlbase.PublicRole, privilege.List{privilege.SELECT}, security.NodeUser)

func initVirtualDatabaseDesc(id sqlbase.ID, name string) *sqlbase.DatabaseDescriptor {
	return &sqlbase.DatabaseDescriptor{
		Name:       name,
		ID:         id,
		Privileges: publicSelectPrivileges,
	}
}

// getEntries is part of the VirtualTabler interface.
func (vs *VirtualSchemaHolder) getEntries() map[string]virtualSchemaEntry {
	return vs.entries
}

// getSchemaNames is part of the VirtualTabler interface.
func (vs *VirtualSchemaHolder) getSchemaNames() []string {
	return vs.orderedNames
}

// getVirtualSchemaEntry retrieves a virtual schema entry given a database name.
// getVirtualSchemaEntry is part of the VirtualTabler interface.
func (vs *VirtualSchemaHolder) getVirtualSchemaEntry(name string) (virtualSchemaEntry, bool) {
	e, ok := vs.entries[name]
	return e, ok
}

// getVirtualTableEntry checks if the provided name matches a virtual database/table
// pair. The function will return the table's virtual table entry if the name matches
// a specific table. It will return an error if the name references a virtual database
// but the table is non-existent.
// getVirtualTableEntry is part of the VirtualTabler interface.
func (vs *VirtualSchemaHolder) getVirtualTableEntry(tn *tree.TableName) (virtualDefEntry, error) {
	if db, ok := vs.getVirtualSchemaEntry(tn.Schema()); ok {
		tableName := tn.Table()
		if t, ok := db.defs[tableName]; ok {
			return t, nil
		}
		if _, ok := db.allTableNames[tableName]; ok {
			return virtualDefEntry{}, pgerror.Unimplemented(tn.Schema()+"."+tableName,
				"virtual schema table not implemented: %s.%s", tn.Schema(), tableName)
		}
		return virtualDefEntry{}, sqlbase.NewUndefinedRelationError(tn)
	}
	return virtualDefEntry{}, nil
}

// VirtualTabler is used to fetch descriptors for virtual tables and databases.
type VirtualTabler interface {
	getVirtualTableDesc(tn *tree.TableName) (*sqlbase.TableDescriptor, error)
	getVirtualSchemaEntry(name string) (virtualSchemaEntry, bool)
	getVirtualTableEntry(tn *tree.TableName) (virtualDefEntry, error)
	getEntries() map[string]virtualSchemaEntry
	getSchemaNames() []string
}

// getVirtualTableDesc checks if the provided name matches a virtual database/table
// pair, and returns its descriptor if it does.
// getVirtualTableDesc is part of the VirtualTabler interface.
func (vs *VirtualSchemaHolder) getVirtualTableDesc(
	tn *tree.TableName,
) (*sqlbase.TableDescriptor, error) {
	t, err := vs.getVirtualTableEntry(tn)
	if err != nil {
		return nil, err
	}
	return t.desc, nil
}
