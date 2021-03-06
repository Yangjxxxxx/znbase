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

package sql_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/pgtype"
	"github.com/znbasedb/znbase/pkg/gossip"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/security/privilege"
	"github.com/znbasedb/znbase/pkg/server/status/statuspb"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/parser"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/sql/tests"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestGossipAlertsTable(t *testing.T) {
	defer leaktest.AfterTest(t)()
	params, _ := tests.CreateTestServerParams()
	s, _, _ := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(context.TODO())
	ctx := context.TODO()

	if err := s.Gossip().AddInfoProto(gossip.MakeNodeHealthAlertKey(456), &statuspb.HealthCheckResult{
		Alerts: []statuspb.HealthAlert{{
			StoreID:     123,
			Category:    statuspb.HealthAlert_METRICS,
			Description: "foo",
			Value:       100.0,
		}},
	}, time.Hour); err != nil {
		t.Fatal(err)
	}

	ie := s.InternalExecutor().(*sql.InternalExecutor)
	row, err := ie.QueryRow(ctx, "test", nil /* txn */, "SELECT * FROM zbdb_internal.gossip_alerts WHERE store_id = 123")
	if err != nil {
		t.Fatal(err)
	}

	if a, e := len(row), 5; a != e {
		t.Fatalf("got %d rows, wanted %d", a, e)
	}
	a := fmt.Sprintf("%v %v %v %v %v", row[0], row[1], row[2], row[3], row[4])
	e := "456 123 'metrics' 'foo' 100.0"
	if a != e {
		t.Fatalf("got:\n%s\nexpected:\n%s", a, e)
	}
}

// TestOldBitColumnMetadata checks that a pre-2.1 BIT columns
// shows up properly in metadata post-2.1.
func TestOldBitColumnMetadata(t *testing.T) {
	defer leaktest.AfterTest(t)()

	// The descriptor changes made must have an immediate effect
	// so disable leases on tables.
	defer sql.TestDisableTableLeases()()

	params, _ := tests.CreateTestServerParams()
	s, sqlDB, kvDB := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(context.TODO())

	if _, err := sqlDB.Exec(`
CREATE DATABASE t;
CREATE TABLE t.test (k INT);
`); err != nil {
		t.Fatal(err)
	}

	// We now want to create a pre-2.1 table descriptor with an
	// old-style bit column. We're going to edit the table descriptor
	// manually, without going through SQL.
	tableDesc := sqlbase.GetTableDescriptor(kvDB, "t", "public", "test")
	for i := range tableDesc.Columns {
		if tableDesc.Columns[i].Name == "k" {
			tableDesc.Columns[i].Type.VisibleType = 4 // Pre-2.1 BIT.
			tableDesc.Columns[i].Type.Width = 12      // Arbitrary non-std INT size.
			break
		}
	}
	// To make this test future-proof we must ensure that there isn't
	// any logic in an unrelated place which will prevent the table from
	// being committed. To verify this, we add another column and check
	// it appears in introspection afterwards.
	//
	// We also avoid the regular schema change logic entirely, because
	// this may be equipped with code to "fix" the old-style BIT column
	// we defined above.
	alterCmd, err := parser.ParseOne("ALTER TABLE t ADD COLUMN z INT", false)
	if err != nil {
		t.Fatal(err)
	}
	colDef := alterCmd.AST.(*tree.AlterTable).Cmds[0].(*tree.AlterTableAddColumn).ColumnDef
	privs := sqlbase.NewDefaultObjectPrivilegeDescriptor(privilege.Column, sqlbase.AdminRole)
	col, _, _, err := sqlbase.MakeColumnDefDescs(colDef, nil, privs, 0)
	if err != nil {
		t.Fatal(err)
	}
	col.ID = tableDesc.NextColumnID
	tableDesc.NextColumnID++
	tableDesc.Families[0].ColumnNames = append(tableDesc.Families[0].ColumnNames, col.Name)
	tableDesc.Families[0].ColumnIDs = append(tableDesc.Families[0].ColumnIDs, col.ID)
	tableDesc.Columns = append(tableDesc.Columns, *col)

	// Write the modified descriptor.
	if err := kvDB.Txn(context.TODO(), func(ctx context.Context, txn *client.Txn) error {
		if err := txn.SetSystemConfigTrigger(); err != nil {
			return err
		}
		return txn.Put(ctx, sqlbase.MakeDescMetadataKey(tableDesc.ID), sqlbase.WrapDescriptor(tableDesc))
	}); err != nil {
		t.Fatal(err)
	}

	// Read the column metadata from information_schema.
	rows, err := sqlDB.Query(`
SELECT column_name, character_maximum_length, numeric_precision, numeric_precision_radix, znbase_sql_type
  FROM t.information_schema.columns
 WHERE table_catalog = 't' AND table_schema = 'public' AND table_name = 'test'
   AND column_name != 'rowid'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	expected := 0
	for rows.Next() {
		var colName string
		var charMaxLength, numPrec, numPrecRadix pgtype.Int8
		var sqlType string
		if err := rows.Scan(&colName, &charMaxLength, &numPrec, &numPrecRadix, &sqlType); err != nil {
			t.Fatal(err)
		}
		switch colName {
		case "k":
			if charMaxLength.Status != pgtype.Null {
				t.Fatalf("x character_maximum_length: expected null, got %d", charMaxLength.Int)
			}
			if numPrec.Int != 64 {
				t.Fatalf("x numeric_precision: expected 64, got %v", numPrec.Get())
			}
			if numPrecRadix.Int != 2 {
				t.Fatalf("x numeric_precision_radix: expected 64, got %v", numPrecRadix.Get())
			}
			if sqlType != "INT" {
				t.Fatalf("x znbase_sql_type: expected INT, got %q", sqlType)
			}
			expected |= 2
		case "z":
			// This is just a canary to verify that the manually-modified
			// table descriptor is visible to introspection.
			expected |= 1
		default:
			t.Fatalf("unexpected col: %q", colName)
		}
	}
	if expected != 3 {
		t.Fatal("did not find both expected rows")
	}

	// Now test the workaround: using ALTER to "upgrade" the type fully to INT.
	if _, err := sqlDB.Exec(`ALTER TABLE t.test ALTER COLUMN k SET DATA TYPE INT8`); err != nil {
		t.Fatal(err)
	}

	// And verify that this has re-set the fields.
	tableDesc = sqlbase.GetTableDescriptor(kvDB, "t", "public", "test")
	found := false
	for i := range tableDesc.Columns {
		col := tableDesc.Columns[i]
		if col.Name == "k" {
			// TODO(knz): post-2.2, we're removing the visible types for
			// integer types so the expectation will be just 0 here.
			if col.Type.VisibleType != 0 && col.Type.VisibleType != sqlbase.ColumnType_BIGINT {
				t.Errorf("unexpected visible type: got %s, expected NONE or BIGINT", col.Type.VisibleType.String())
			}
			if col.Type.Width != 64 {
				t.Errorf("unexpected width: got %d, expected 64", col.Type.Width)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("column disappeared")
	}

}
