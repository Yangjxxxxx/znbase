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
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/server"
	"github.com/znbasedb/znbase/pkg/sql/tests"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

// getRangeKeys returns the end keys of all ranges.
func getRangeKeys(db *client.DB) ([]roachpb.Key, error) {
	rows, err := db.Scan(context.TODO(), keys.Meta2Prefix, keys.MetaMax, 0)
	if err != nil {
		return nil, err
	}
	ret := make([]roachpb.Key, len(rows))
	for i := 0; i < len(rows); i++ {
		ret[i] = bytes.TrimPrefix(rows[i].Key, keys.Meta2Prefix)
	}
	return ret, nil
}

func getNumRanges(db *client.DB) (int, error) {
	rows, err := getRangeKeys(db)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func rangesMatchSplits(ranges []roachpb.Key, splits []roachpb.RKey) bool {
	if len(ranges) != len(splits) {
		return false
	}
	for i := 0; i < len(ranges); i++ {
		if !splits[i].Equal(ranges[i]) {
			return false
		}
	}
	return true
}

// TestSplitOnTableBoundaries verifies that ranges get split
// as new tables get created.
func TestSplitOnTableBoundaries(t *testing.T) {
	defer leaktest.AfterTest(t)()

	params, _ := tests.CreateTestServerParams()
	// We want fast scan.
	params.ScanInterval = time.Millisecond
	params.ScanMinIdleTime = time.Millisecond
	params.ScanMaxIdleTime = time.Millisecond
	s, sqlDB, kvDB := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(context.TODO())

	expectedInitialRanges, err := server.ExpectedInitialRangeCount(kvDB)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sqlDB.Exec(`CREATE DATABASE test`); err != nil {
		t.Fatal(err)
	}
	schemaRanges := 1
	// We split up to the largest allocated descriptor ID, if it's a table.
	// Ensure that no split happens if a database is created.
	testutils.SucceedsSoon(t, func() error {
		num, err := getNumRanges(kvDB)
		if err != nil {
			return err
		}
		if e := expectedInitialRanges + 1; num != e {
			return errors.Errorf("expected %d splits, found %d", e, num)
		}
		return nil
	})

	// Verify the actual splits.
	objectID := uint32(keys.MinNonPredefinedUserDescID)
	splits := []roachpb.RKey{roachpb.RKeyMax}
	ranges, err := getRangeKeys(kvDB)
	if err != nil {
		t.Fatal(err)
	}
	if a, e := ranges[expectedInitialRanges:], splits; !rangesMatchSplits(a, e) {
		t.Fatalf("Found ranges: %v\nexpected: %v", a, e)
	}

	// Let's create a table.
	if _, err := sqlDB.Exec(`CREATE TABLE test.test (k INT PRIMARY KEY, v INT)`); err != nil {
		t.Fatal(err)
	}

	testutils.SucceedsSoon(t, func() error {
		num, err := getNumRanges(kvDB)
		if err != nil {
			return err
		}
		if e := expectedInitialRanges + 1 + schemaRanges; num != e {
			return errors.Errorf("expected %d splits, found %d", e, num)
		}
		return nil
	})

	// Verify the actual splits.
	// 0  | defaultdb | 50
	// 50 | public    | 51
	// 0  | postgres  | 52
	// 52 | public    | 53
	// 0  | test      | 54
	// 54 | public    | 55
	// 51 | test      | 56
	// objectID = 49
	databaseRanges := 1
	splits = []roachpb.RKey{keys.MakeTablePrefix(objectID + uint32(databaseRanges) + uint32(schemaRanges)), roachpb.RKeyMax}
	ranges, err = getRangeKeys(kvDB)
	if err != nil {
		t.Fatal(err)
	}
	if a, e := ranges[expectedInitialRanges-1+schemaRanges:], splits; !rangesMatchSplits(a, e) {
		t.Fatalf("Found ranges: %v\nexpected: %v", a, e)
	}
}
