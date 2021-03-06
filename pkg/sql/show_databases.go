// Copyright 2017 The Cockroach Authors.
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

	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
)

// ShowDatabases returns all the databases.
// Privileges: None.
//   Notes: postgres does not have a "show databases"
//          mysql has a "SHOW DATABASES" permission, but we have no system-level permissions.
func (p *planner) ShowDatabases(ctx context.Context, n *tree.ShowDatabases) (planNode, error) {
	//return p.delegateQuery(ctx, "SHOW DATABASES",
	//	`SELECT DISTINCT table_catalog AS database_name
	//   FROM "".information_schema.database_privileges
	//  ORDER BY 1`,
	//	nil, nil)
	queryString := `SELECT NAME as database_name `
	if n.WithComment {
		queryString += `,obj_description(ID) AS comment `
	}
	queryString += `FROM "".zbdb_internal.databases ORDER BY 1`
	return p.delegateQuery(ctx, "SHOW DATABASES", queryString, nil, nil)
}
