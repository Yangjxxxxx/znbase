// Copyright 2015  The Cockroach Authors.
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

package tests_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/kr/pretty"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/security"
	"github.com/znbasedb/znbase/pkg/security/privilege"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestInitialKeys(t *testing.T) {
	defer leaktest.AfterTest(t)()

	const keysPerDesc = 2
	const nonDescKeys = 7

	ms := sqlbase.MakeMetadataSchema()
	kv, _ /* splits */ := ms.GetInitialValues()
	expected := nonDescKeys + keysPerDesc*ms.SystemDescriptorCount()
	if actual := len(kv); actual != expected {
		t.Fatalf("Wrong number of initial sql kv pairs: %d, wanted %d", actual, expected)
	}

	// Add an additional table.
	sqlbase.SystemAllowedPrivileges[keys.MaxReservedDescID] = privilege.TablePrivileges
	desc, err := sql.CreateTestTableDescriptor(
		context.TODO(),
		keys.SystemDatabaseID,
		keys.MaxReservedDescID,
		"CREATE TABLE system.x (val INTEGER PRIMARY KEY)",
		sqlbase.NewDefaultObjectPrivilegeDescriptor(privilege.Table, security.NodeUser),
	)
	if err != nil {
		t.Fatal(err)
	}
	ms.AddDescriptor(keys.SystemDatabaseID, &desc)
	kv, _ /* splits */ = ms.GetInitialValues()
	expected = nonDescKeys + keysPerDesc*ms.SystemDescriptorCount()
	if actual := len(kv); actual != expected {
		t.Fatalf("Wrong number of initial sql kv pairs: %d, wanted %d", actual, expected)
	}

	// Verify that IDGenerator value is correct.
	found := false
	var idgenkv roachpb.KeyValue
	for _, v := range kv {
		if v.Key.Equal(keys.DescIDGenerator) {
			idgenkv = v
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Could not find descriptor ID generator in initial key set")
	}
	// Expect 2 non-reserved IDs to have been allocated.
	i, err := idgenkv.Value.GetInt()
	if err != nil {
		t.Fatal(err)
	}
	if a, e := i, int64(keys.MinUserDescID); a != e {
		t.Fatalf("Expected next descriptor ID to be %d, was %d", e, a)
	}
}

// TestSystemTableLiterals compares the result of evaluating the `CREATE TABLE`
// statement strings that describe each system table with the TableDescriptor
// literals that are actually used at runtime. This ensures we can use the hand-
// written literals instead of having to evaluate the `CREATE TABLE` statements
// before initialization and with limited SQL machinery bootstraped, while still
// confident that the result is the same as if `CREATE TABLE` had been run.
//
// This test may also be useful when writing a new system table:
// adding the new schema along with a trivial, empty TableDescriptor literal
// will print the expected proto which can then be used to replace the empty
// one (though pruning the explicit zero values may make it more readable).
func TestSystemTableLiterals(t *testing.T) {
	defer leaktest.AfterTest(t)()
	type testcase struct {
		id     sqlbase.ID
		schema string
		pkg    sqlbase.TableDescriptor
	}

	for _, test := range []testcase{
		{keys.NamespaceTableID, sqlbase.NamespaceTableSchema, sqlbase.NamespaceTable},
		{keys.FunctionNamespaceTableID, sqlbase.FunctionNamespaceTableSchema, sqlbase.FunctionNamespaceTable},
		{keys.DescriptorTableID, sqlbase.DescriptorTableSchema, sqlbase.DescriptorTable},
		{keys.UsersTableID, sqlbase.UsersTableSchema, sqlbase.UsersTable},
		{keys.ZonesTableID, sqlbase.ZonesTableSchema, sqlbase.ZonesTable},
		{keys.LocationTableID, sqlbase.LocationTableSchema, sqlbase.LocationTable},
		{keys.LeaseTableID, sqlbase.LeaseTableSchema, sqlbase.LeaseTable},
		{keys.EventLogTableID, sqlbase.EventLogTableSchema, sqlbase.EventLogTable},
		{keys.RangeEventTableID, sqlbase.RangeEventTableSchema, sqlbase.RangeEventTable},
		{keys.UITableID, sqlbase.UITableSchema, sqlbase.UITable},
		{keys.JobsTableID, sqlbase.JobsTableSchema, sqlbase.JobsTable},
		{keys.SettingsTableID, sqlbase.SettingsTableSchema, sqlbase.SettingsTable},
		{keys.WebSessionsTableID, sqlbase.WebSessionsTableSchema, sqlbase.WebSessionsTable},
		{keys.TableStatisticsTableID, sqlbase.TableStatisticsTableSchema, sqlbase.TableStatisticsTable},
		{keys.LocationsTableID, sqlbase.LocationsTableSchema, sqlbase.LocationsTable},
		{keys.RoleMembersTableID, sqlbase.RoleMembersTableSchema, sqlbase.RoleMembersTable},
		{keys.CommentsTableID, sqlbase.CommentsTableSchema, sqlbase.CommentsTable},
		{keys.SnapshotsTableID, sqlbase.SnapshotsTableSchema, sqlbase.SnapshotsTable},
		{keys.AuthenticationTableID, sqlbase.AuthenticationTableSchema, sqlbase.AuthenticationTable},
		{keys.UserOptionsTableID, sqlbase.UserOptionsTableSchema, sqlbase.UserOptionsTable},
		{keys.ScheduledJobsTableID, sqlbase.ScheduledJobsTableSchema, sqlbase.ScheduledJobsTable},
		{keys.FlashbackTableID, sqlbase.FlashbackTableSchema, sqlbase.FlashbackTable},
		{keys.TriggersTableID, sqlbase.TriggersTableSchema, sqlbase.TriggersTable},
		{keys.HintTableID, sqlbase.GlobalHintTableSchema, sqlbase.HintsTable},
	} {
		privs := *test.pkg.Privileges
		gen, err := sql.CreateTestTableDescriptor(
			context.TODO(),
			keys.SystemDatabaseID,
			test.id,
			test.schema,
			&privs,
		)
		if err != nil {
			t.Fatalf("test: %+v, err: %v", test, err)
		}

		if !proto.Equal(&test.pkg, &gen) {
			diff := strings.Join(pretty.Diff(&test.pkg, &gen), "\n")
			t.Errorf("%s table descriptor generated from CREATE TABLE statement does not match "+
				"hardcoded table descriptor:\n%s", test.pkg.Name, diff)
		}
	}
}
