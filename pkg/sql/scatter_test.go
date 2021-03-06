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
	"reflect"
	"testing"

	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestScatterRandomizeLeases(t *testing.T) {
	defer leaktest.AfterTest(t)()

	if testutils.NightlyStress() && util.RaceEnabled {
		t.Skip("uses too many resources for stressrace")
	}

	const numHosts = 3

	// Prevent the merge queue from immediately discarding our splits. This is
	// more foolproof than changing the cluster setting because the cluster
	// setting change has to be propagated to all nodes via gossip so there's
	// still a small chance that a non-gateway node will try a merge after we
	// change the setting.
	var testClusterArgs base.TestClusterArgs
	testClusterArgs.ServerArgs.Knobs.Store = &storage.StoreTestingKnobs{
		DisableMergeQueue: true,
	}
	tc := serverutils.StartTestCluster(t, numHosts, testClusterArgs)
	defer tc.Stopper().Stop(context.TODO())

	sqlutils.CreateTable(
		t, tc.ServerConn(0), "t",
		"k INT PRIMARY KEY, v INT",
		1000,
		sqlutils.ToRowFn(sqlutils.RowIdxFn, sqlutils.RowModuloFn(10)),
	)

	r := sqlutils.MakeSQLRunner(tc.ServerConn(0))

	// Even though we disabled merges via the store testing knob, we must also
	// disable the setting in order for manual splits to be allowed.
	r.Exec(t, "SET CLUSTER SETTING kv.range_merge.queue_enabled = false")

	// Introduce 99 splits to get 100 ranges.
	r.Exec(t, "ALTER TABLE test.t SPLIT AT (SELECT i*10 FROM generate_series(1, 99) AS g(i))")

	getLeaseholders := func() (map[int]int, error) {
		rows := r.Query(t, `SELECT range_id, lease_holder FROM [SHOW EXPERIMENTAL_RANGES FROM TABLE test.t]`)
		leaseholders := make(map[int]int)
		numRows := 0
		for ; rows.Next(); numRows++ {
			var rangeID, leaseholder int
			if err := rows.Scan(&rangeID, &leaseholder); err != nil {
				return nil, err
			}
			if rangeID < 1 {
				t.Fatalf("invalid rangeID: %d", rangeID)
			}
			if leaseholder < 1 || leaseholder > numHosts {
				return nil, fmt.Errorf("invalid lease_holder value: %d", leaseholder)
			}
			leaseholders[rangeID] = leaseholder
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if numRows != 100 {
			return nil, fmt.Errorf("expected 100 ranges, got %d", numRows)
		}
		return leaseholders, nil
	}

	oldLeaseholders, err := getLeaseholders()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		// Ensure that scattering changes the leaseholders, which is really all
		// that randomizing the lease placements can probabilistically guarantee -
		// it doesn't guarantee a uniform distribution.
		r.Exec(t, "ALTER TABLE test.t SCATTER")
		newLeaseholders, err := getLeaseholders()
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(oldLeaseholders, newLeaseholders) {
			t.Errorf("expected scatter to change lease distribution, but got no change: %v", newLeaseholders)
		}
		oldLeaseholders = newLeaseholders
	}
}

// TestScatterResponse ensures that ALTER TABLE... SCATTER includes one row of
// output per range in the table. It does *not* test that scatter properly
// distributes replicas and leases; see TestScatter for that.
//
// TODO(benesch): consider folding this test into TestScatter once TestScatter
// is unskipped.
func TestScatterResponse(t *testing.T) {
	defer leaktest.AfterTest(t)()

	s, sqlDB, kvDB := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(context.Background())

	sqlutils.CreateTable(
		t, sqlDB, "t",
		"k INT PRIMARY KEY, v INT",
		1000,
		sqlutils.ToRowFn(sqlutils.RowIdxFn, sqlutils.RowModuloFn(10)),
	)
	tableDesc := sqlbase.GetTableDescriptor(kvDB, "test", "public", "t")

	r := sqlutils.MakeSQLRunner(sqlDB)
	// Prevent the merge queue from immediately discarding our splits.
	r.Exec(t, "SET CLUSTER SETTING kv.range_merge.queue_enabled = false")
	r.Exec(t, "ALTER TABLE test.t SPLIT AT (SELECT i*10 FROM generate_series(1, 99) AS g(i))")
	rows := r.Query(t, "ALTER TABLE test.t SCATTER")

	i := 0
	for ; rows.Next(); i++ {
		var actualKey []byte
		var pretty string
		if err := rows.Scan(&actualKey, &pretty); err != nil {
			t.Fatal(err)
		}
		var expectedKey roachpb.Key
		if i == 0 {
			expectedKey = keys.MakeTablePrefix(uint32(tableDesc.ID))
		} else {
			var err error
			expectedKey, err = sqlbase.TestingMakePrimaryIndexKey(tableDesc, i*10)
			if err != nil {
				t.Fatal(err)
			}
		}
		if e, a := expectedKey, roachpb.Key(actualKey); !e.Equal(a) {
			t.Errorf("%d: expected split key %s, but got %s", i, e, a)
		}
		if e, a := expectedKey.String(), pretty; e != a {
			t.Errorf("%d: expected pretty split key %s, but got %s", i, e, a)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if e, a := 100, i; e != a {
		t.Fatalf("expected %d rows, but got %d", e, a)
	}
}
