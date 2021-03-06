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

// Package cat contains interfaces that are used by the query optimizer to avoid
// including specifics of sqlbase structures in the opt code.
package cat

import (
	"context"

	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/security/privilege"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
)

// StableID permanently and uniquely identifies a catalog object (table, view,
// index, column, etc.) within its scope:
//
//   data source StableID: unique within database
//   index StableID: unique within table
//   column StableID: unique within table
//
// If a new catalog object is created, it will always be assigned a new StableID
// that has never, and will never, be reused by a different object in the same
// scope. This uniqueness guarantee is true even if the new object has the same
// name and schema as an old (and possibly dropped) object. The StableID will
// never change as long as the object exists.
//
// Note that while two instances of the same catalog object will always have the
// same StableID, they can have different schema if the schema has changed over
// time. See the Version type comments for more details.
//
// For most sqlbase objects, the StableID is the 32-bit descriptor ID. However,
// this is not always the case. For example, the StableID for virtual tables
// prepends the database ID, since the same descriptor ID is reused across
// databases.
type StableID uint64

// SchemaName is an alias for tree.TableNamePrefix, since it consists of the
// catalog + schema name.
type SchemaName = tree.TableNamePrefix

// Flags allows controlling aspects of some Catalog operations.
type Flags struct {
	// AvoidDescriptorCaches avoids using any cached descriptors (for tables,
	// views, schemas, etc). This is useful in cases where we are running a
	// statement like SHOW and we don't want to get table leases or otherwise
	// pollute the caches.
	AvoidDescriptorCaches bool

	// NoTableStats doesn't retrieve table statistics. This should be used in all
	// cases where we don't need them (like SHOW variants), to avoid polluting the
	// stats cache.
	NoTableStats bool
}

// Catalog is an interface to a database catalog, exposing only the information
// needed by the query optimizer.
//
// NOTE: Catalog implementations need not be thread-safe. However, the objects
// returned by the Resolve methods (schemas and data sources) *must* be
// immutable after construction, and therefore also thread-safe.
type Catalog interface {
	// ResolveSchema locates a schema with the given name and returns it along
	// with the resolved SchemaName (which has all components filled in).
	//
	// The resolved SchemaName is the same with the resulting Schema.Name() except
	// that it has the ExplicitCatalog/ExplicitSchema flags set to correspond to
	// the input name. Its use is mainly for cosmetic purposes.
	//
	// If no such schema exists, then ResolveSchema returns an error.
	//
	// NOTE: The returned schema must be immutable after construction, and so can
	// be safely copied or used across goroutines.
	ResolveSchema(ctx context.Context, flags Flags, name *SchemaName) (Schema, SchemaName, error)

	// ResolveAllDataSource returns the list of names for the data sources that the
	// schema contains.
	ResolveAllDataSource(
		ctx context.Context, flags Flags, name *SchemaName,
	) ([]DataSourceName, error)

	// ResolveDataSource locates a data source with the given name and returns it
	// along with the resolved DataSourceName.
	//
	// The resolved DataSourceName is the same with the resulting
	// DataSource.Name() except that it has the ExplicitCatalog/ExplicitSchema
	// flags set to correspond to the input name. Its use is mainly for cosmetic
	// purposes. For example: the input name might be "t". The fully qualified
	// DataSource.Name() would be "currentdb.public.t"; the returned
	// DataSourceName would have the same fields but would still be formatted as
	// "t".
	//
	// If no such data source exists, then ResolveDataSource returns an error.
	//
	// NOTE: The returned data source must be immutable after construction, and
	// so can be safely copied or used across goroutines.
	ResolveDataSource(ctx context.Context, flags Flags, name *DataSourceName) (DataSource, DataSourceName, error)

	// ResolveDataSourceByID is similar to ResolveDataSource, except that it
	// locates a data source by its StableID. See the comment for StableID for
	// more details.
	//
	// NOTE: The returned data source must be immutable after construction, and
	// so can be safely copied or used across goroutines.
	ResolveDataSourceByID(ctx context.Context, flags Flags, id StableID) (DataSource, error)

	// ResolveUdrFunction will resolve a opt function by name
	ResolveUdrFunction(ctx context.Context, name *DataSourceName) (UdrFunction, error)

	// CheckPrivilege verifies that the current user has the given privilege on
	// the given catalog object. If not, then CheckPrivilege returns an error.
	CheckPrivilege(ctx context.Context, o Object, priv privilege.Kind) error

	// CheckAnyPrivilege verifies that the current user has any privilege on
	// the given catalog object. If not, then CheckAnyPrivilege returns an error.
	CheckAnyPrivilege(ctx context.Context, o Object) error

	// CheckDMLPrivilege verifies that the user has the given privilege on
	// the specified columns of the given catalog object. If not, the function returns an error.
	// If cols is nil, it will be treated as all the columns of object.
	CheckDMLPrivilege(ctx context.Context, o Object, cols tree.NameList, priv privilege.Kind, user string) error

	// CheckAnyColumnPrivilege is used to check whether user can use table which only has columns privilege.
	CheckAnyColumnPrivilege(ctx context.Context, o Object) error

	// ResolveTableDesc returns the table descriptor via table name.
	ResolveTableDesc(name *tree.TableName) (interface{}, error)

	// GetDatabaseDescByName gets the database descriptor by dbName
	GetDatabaseDescByName(ctx context.Context, txn *client.Txn, dbName string) (interface{}, error)

	// GetSchemaDescByName gets the schema descriptor by scName
	GetSchemaDescByName(ctx context.Context, txn *client.Txn, scName string) (interface{}, error)

	GetCurrentDatabase(ctx context.Context) string
	GetCurrentSchema(ctx context.Context, name tree.Name) (bool, string, error)
	HasRoleOption(ctx context.Context, action string) error

	// ResolveUDRFunctionDesc return the function descriptors via function name.
	ResolveUDRFunctionDesc(name *tree.TableName) (interface{}, UdrFunction, error)

	// CheckFunctionPrivilege checks the privilege for function, return privilege or nil for all the functions same name.
	CheckFunctionPrivilege(ctx context.Context, ds DataSource) []error
}
