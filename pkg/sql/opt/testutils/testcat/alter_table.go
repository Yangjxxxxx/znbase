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

package testcat

import (
	gojson "encoding/json"
	"fmt"
	"sort"

	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/sql/stats"
)

// AlterTable is a partial implementation of the ALTER TABLE statement.
//
// Supported commands:
//  - INJECT STATISTICS: imports table statistics from a JSON object.
//
func (tc *Catalog) AlterTable(stmt *tree.AlterTable) {
	// Update the table name to include catalog and schema if not provided.
	tc.qualifyTableName(&stmt.Table)
	tab := tc.Table(&stmt.Table)

	for _, cmd := range stmt.Cmds {
		switch t := cmd.(type) {
		case *tree.AlterTableInjectStats:
			injectTableStats(tab, t.Stats)

		default:
			panic(fmt.Sprintf("unsupported ALTER TABLE command %T", t))
		}
	}
}

// injectTableStats sets the table statistics as specified by a JSON object.
func injectTableStats(tt *Table, statsExpr tree.Expr) {
	semaCtx := tree.MakeSemaContext()
	evalCtx := tree.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())
	typedExpr, err := tree.TypeCheckAndRequire(statsExpr, &semaCtx, types.JSON, "INJECT STATISTICS", false)
	if err != nil {
		panic(err)
	}
	val, err := typedExpr.Eval(&evalCtx)
	if err != nil {
		panic(err)
	}

	if val == tree.DNull {
		panic("statistics cannot be NULL")
	}
	jsonStr := val.(*tree.DJSON).JSON.String()
	var stats []stats.JSONStatistic
	if err := gojson.Unmarshal([]byte(jsonStr), &stats); err != nil {
		panic(err)
	}
	tt.Stats = make([]*TableStat, len(stats))
	for i := range stats {
		tt.Stats[i] = &TableStat{js: stats[i], tt: tt}
	}
	// Call ColumnOrdinal on all possible columns to assert that
	// the column names are valid.
	for _, ts := range tt.Stats {
		for i := 0; i < ts.ColumnCount(); i++ {
			ts.ColumnOrdinal(i)
		}
	}

	// Finally, sort the stats with most recent first.
	sort.Sort(tt.Stats)
}
