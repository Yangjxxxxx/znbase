// Copyright 2020 The Bidb Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package sql_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/sql/tests"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/syncutil"
)

// TestMaterializedViewClearedAfterRefresh ensures that the old state of the
// view is cleaned up after it is refreshed.
func TestMaterializedViewClearedAfterRefresh(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	ctx := context.Background()
	params, _ := tests.CreateTestServerParams()

	s, sqlDB, kvDB := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(ctx)

	if _, err := sqlDB.Exec(`
CREATE DATABASE t;
CREATE TABLE t.t (x INT);
INSERT INTO t.t VALUES (1), (2);
CREATE MATERIALIZED VIEW t.v AS SELECT x FROM t.t;
`); err != nil {
		t.Fatal(err)
	}

	descBeforeRefresh := sqlbase.GetImmutableTableDescriptor(kvDB, "t", "public", "v")

	// Update the view and refresh it.
	if _, err := sqlDB.Exec(`
INSERT INTO t.t VALUES (3);
REFRESH MATERIALIZED VIEW t.v;
`); err != nil {
		t.Fatal(err)
	}

	// Add a zone config to delete all table data.
	_, err := addImmediateGCZoneConfig(sqlDB, descBeforeRefresh.ID)
	if err != nil {
		t.Fatal(err)
	}

	// The data should be deleted.
	testutils.SucceedsSoon(t, func() error {
		indexPrefix := sqlbase.MakeIndexKeyPrefix(&descBeforeRefresh.TableDescriptor, descBeforeRefresh.PrimaryIndex.ID)
		indexEnd := roachpb.Key(indexPrefix).PrefixEnd()
		if kvs, err := kvDB.Scan(ctx, indexPrefix, indexEnd, 0); err != nil {
			t.Fatal(err)
		} else if len(kvs) != 0 {
			return errors.Newf("expected 0 kvs, found %d", len(kvs))
		}
		return nil
	})
}

// TestMaterializedViewRefreshVisibility ensures that intermediate results written
// as part of the refresh backfill process aren't visibile until the refresh is done.
//func TestMaterializedViewRefreshVisibility(t *testing.T) {
//	defer leaktest.AfterTest(t)()
//	defer log.Scope(t).Close(t)
//
//	ctx := context.Background()
//	params, _ := tests.CreateTestServerParams()
//
//	waitForCommit, waitToProceed, refreshDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
//	params.Knobs = base.TestingKnobs{
//		SQLSchemaChanger: &sql.SchemaChangerTestingKnobs{
//			RunBeforeMaterializedViewRefreshCommit: func() error {
//				close(waitForCommit)
//				<-waitToProceed
//				return nil
//			},
//		},
//	}
//
//	s, sqlDB, _ := serverutils.StartServer(t, params)
//	defer s.Stopper().Stop(ctx)
//
//	// Make a materialized view and update the data behind it.
//	if _, err := sqlDB.Exec(`
//CREATE DATABASE t;
//CREATE TABLE t.t (x INT);
//INSERT INTO t.t VALUES (1), (2);
//CREATE MATERIALIZED VIEW t.v AS SELECT x FROM t.t;
//INSERT INTO t.t VALUES (3);
//`); err != nil {
//		t.Fatal(err)
//	}
//
//	// Start a refresh.
//	go func() {
//		if _, err := sqlDB.Exec(`REFRESH MATERIALIZED VIEW t.v`); err != nil {
//			t.Error(err)
//		}
//		close(refreshDone)
//	}()
//
//	<-waitForCommit
//
//	// Before the refresh commits, we shouldn't see any updated data.
//	runner := sqlutils.MakeSQLRunner(sqlDB)
//	runner.CheckQueryResults(t, "SELECT * FROM t.v ORDER BY x", [][]string{{"1"}, {"2"}})
//
//	// Let the refresh commit.
//	close(waitToProceed)
//	<-refreshDone
//	runner.CheckQueryResults(t, "SELECT * FROM t.v ORDER BY x", [][]string{{"1"}, {"2"}, {"3"}})
//}

func TestMaterializedViewCleansUpOnRefreshFailure(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	ctx := context.Background()
	params, _ := tests.CreateTestServerParams()

	// Protects shouldError
	var mu syncutil.Mutex
	shouldError := true

	params.Knobs = base.TestingKnobs{
		SQLSchemaChanger: &sql.SchemaChangerTestingKnobs{
			RunBeforeMaterializedViewRefreshCommit: func() error {
				mu.Lock()
				defer mu.Unlock()
				if shouldError {
					shouldError = false
					return errors.New("boom")
				}
				return nil
			},
		},
	}

	s, sqlDB, kvDB := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(ctx)

	if _, err := sqlDB.Exec(`
CREATE DATABASE t;
CREATE TABLE t.t (x INT);
INSERT INTO t.t VALUES (1), (2);
CREATE MATERIALIZED VIEW t.v AS SELECT x FROM t.t;
`); err != nil {
		t.Fatal(err)
	}

	descBeforeRefresh := sqlbase.GetImmutableTableDescriptor(kvDB, "t", "public", "v")

	// Add a zone config to delete all table data.
	_, err := addImmediateGCZoneConfig(sqlDB, descBeforeRefresh.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt (and fail) to refresh the view.
	if _, err := sqlDB.Exec(`REFRESH MATERIALIZED VIEW t.v`); err == nil {
		t.Fatal("expected error, but found nil")
	}

	testutils.SucceedsSoon(t, func() error {
		tableStart := keys.MakeTablePrefix(uint32(descBeforeRefresh.ID))
		tableEnd := roachpb.Key(tableStart).PrefixEnd()
		if kvs, err := kvDB.Scan(ctx, tableStart, tableEnd, 0); err != nil {
			t.Fatal(err)
		} else if len(kvs) != 2 {
			return errors.Newf("expected to find only 2 KVs, but found %d", len(kvs))
		}
		return nil
	})
}

func TestDropMaterializedView(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	ctx := context.Background()
	params, _ := tests.CreateTestServerParams()
	s, sqlRaw, kvDB := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(ctx)

	// Disable strict GC TTL enforcement because we're going to shove a zero-value
	// TTL into the system with AddImmediateGCZoneConfig.
	//defer disableGCTTLStrictEnforcement(t, sqlRaw)()

	sqlDB := sqlutils.SQLRunner{DB: sqlRaw}

	// Create a view with some data.
	sqlDB.Exec(t, `
CREATE DATABASE t;
CREATE TABLE t.t (x INT);
INSERT INTO t.t VALUES (1), (2);
CREATE MATERIALIZED VIEW t.v AS SELECT x FROM t.t;
`)
	desc := sqlbase.GetImmutableTableDescriptor(kvDB, "t", "public", "v")
	// Add a zone config to delete all table data.
	_, err := addImmediateGCZoneConfig(sqlRaw, desc.ID)
	require.NoError(t, err)

	// Now drop the view.
	sqlDB.Exec(t, `DROP MATERIALIZED VIEW t.v`)
	require.NoError(t, err)

	// All of the table data should be cleaned up.
	testutils.SucceedsSoon(t, func() error {
		tableStart := keys.MakeTablePrefix(uint32(desc.ID))
		tableEnd := roachpb.Key(tableStart).PrefixEnd()
		if kvs, err := kvDB.Scan(ctx, tableStart, tableEnd, 0); err != nil {
			t.Fatal(err)
		} else if len(kvs) != 0 {
			return errors.Newf("expected to find 0 KVs, but found %d", len(kvs))
		}
		return nil
	})
}
