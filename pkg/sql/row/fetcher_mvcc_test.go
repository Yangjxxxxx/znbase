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

package row_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/row"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/log"
)

func slurpUserDataKVs(t testing.TB, e engine.Engine) []roachpb.KeyValue {
	t.Helper()

	// Scan meta keys directly from engine. We put this in a retry loop
	// because the application of all of a transactions committed writes
	// is not always synchronous with it committing.
	var kvs []roachpb.KeyValue
	testutils.SucceedsSoon(t, func() error {
		kvs = nil
		it := e.NewIterator(engine.IterOptions{UpperBound: roachpb.KeyMax})
		defer it.Close()
		for it.Seek(engine.MVCCKey{Key: keys.UserTableDataMin}); ; it.NextKey() {
			ok, err := it.Valid()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if !it.UnsafeKey().IsValue() {
				return errors.Errorf("found intent key %v", it.UnsafeKey())
			}
			kvs = append(kvs, roachpb.KeyValue{
				Key:   it.Key().Key,
				Value: roachpb.Value{RawBytes: it.Value(), Timestamp: it.UnsafeKey().Timestamp},
			})
		}
		return nil
	})
	return kvs
}

func TestRowFetcherMVCCMetadata(t *testing.T) {
	defer leaktest.AfterTest(t)()

	ctx := context.Background()
	s, db, kvDB := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)
	store, _ := s.GetStores().(*storage.Stores).GetStore(s.GetFirstStoreID())
	sqlDB := sqlutils.MakeSQLRunner(db)

	sqlDB.Exec(t, `CREATE DATABASE d`)
	sqlDB.Exec(t, `USE d`)
	sqlDB.Exec(t, `CREATE TABLE parent (
		a STRING PRIMARY KEY, b STRING, c STRING, d STRING,
		FAMILY (a, b, c), FAMILY (d)
	)`)
	sqlDB.Exec(t, `CREATE TABLE child (
		e STRING, f STRING, PRIMARY KEY (e, f)
	) INTERLEAVE IN PARENT parent (e)`)

	parentDesc := sqlbase.GetImmutableTableDescriptor(kvDB, `d`, "public", `parent`)
	childDesc := sqlbase.GetImmutableTableDescriptor(kvDB, `d`, "public", `child`)
	var args []row.FetcherTableArgs
	for _, desc := range []*sqlbase.ImmutableTableDescriptor{parentDesc, childDesc} {
		colIdxMap := make(map[sqlbase.ColumnID]int)
		var valNeededForCol util.FastIntSet
		for colIdx, col := range desc.Columns {
			colIdxMap[col.ID] = colIdx
			valNeededForCol.Add(colIdx)
		}
		args = append(args, row.FetcherTableArgs{
			Spans:            desc.AllIndexSpans(),
			Desc:             desc,
			Index:            &desc.PrimaryIndex,
			ColIdxMap:        colIdxMap,
			IsSecondaryIndex: false,
			Cols:             desc.Columns,
			ValNeededForCol:  valNeededForCol,
		})
	}
	var rf row.Fetcher
	if err := rf.Init(
		false, /* reverse */
		false, /* returnRangeInfo */
		true,  /* isCheck */
		&sqlbase.DatumAlloc{},
		sqlbase.ScanLockingStrength_FOR_NONE,
		sqlbase.ScanLockingWaitPolicy{LockLevel: sqlbase.ScanLockingWaitLevel_BLOCK},
		args...,
	); err != nil {
		t.Fatal(err)
	}
	type rowWithMVCCMetadata struct {
		PrimaryKey      []string
		RowIsDeleted    bool
		RowLastModified string
	}
	kvsToRows := func(kvs []roachpb.KeyValue) []rowWithMVCCMetadata {
		t.Helper()
		for _, kv := range kvs {
			log.Info(ctx, kv.Key, kv.Value.Timestamp, kv.Value.PrettyPrint())
		}

		if err := rf.StartScanFrom(ctx, &row.SpanKVFetcher{KVs: kvs}); err != nil {
			t.Fatal(err)
		}
		var rows []rowWithMVCCMetadata
		for {
			datums, _, _, err := rf.NextRowDecoded(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if datums == nil {
				break
			}
			row := rowWithMVCCMetadata{
				RowIsDeleted:    rf.RowIsDeleted(),
				RowLastModified: tree.TimestampToDecimal(rf.RowLastModified()).String(),
			}
			for _, datum := range datums {
				if datum == tree.DNull {
					row.PrimaryKey = append(row.PrimaryKey, `NULL`)
				} else {
					row.PrimaryKey = append(row.PrimaryKey, string(*datum.(*tree.DString)))
				}
			}
			rows = append(rows, row)
		}
		return rows
	}

	var ts1 string
	sqlDB.QueryRow(t, `BEGIN;
		INSERT INTO parent VALUES ('1', 'a', 'a', 'a'), ('2', 'b', 'b', 'b');
		INSERT INTO child VALUES ('1', '10'), ('2', '20');
		SELECT cluster_logical_timestamp();
	END;`).Scan(&ts1)
	fmt.Println(sqlDB.QueryStr(t, `SELECT * FROM parent`))

	if actual, expected := kvsToRows(slurpUserDataKVs(t, store.Engine())), []rowWithMVCCMetadata{
		{[]string{`1`, `a`, `a`, `a`}, false, ts1},
		{[]string{`1`, `10`}, false, ts1},
		{[]string{`2`, `b`, `b`, `b`}, false, ts1},
		{[]string{`2`, `20`}, false, ts1},
	}; !reflect.DeepEqual(expected, actual) {
		t.Errorf(`expected %v got %v`, expected, actual)
	}

	var ts2 string
	sqlDB.QueryRow(t, `BEGIN;
		UPDATE parent SET b = NULL, c = NULL, d = NULL WHERE a = '1';
		UPDATE parent SET d = NULL WHERE a = '2';
		UPDATE child SET f = '21' WHERE e = '2';
		SELECT cluster_logical_timestamp();
	END;`).Scan(&ts2)
	fmt.Println(sqlDB.QueryStr(t, `SELECT * FROM parent`))

	if actual, expected := kvsToRows(slurpUserDataKVs(t, store.Engine())), []rowWithMVCCMetadata{
		{[]string{`1`, `NULL`, `NULL`, `NULL`}, false, ts2},
		{[]string{`1`, `10`}, false, ts1},
		{[]string{`2`, `b`, `b`, `NULL`}, false, ts2},
		{[]string{`2`, `20`}, true, ts2},
		{[]string{`2`, `21`}, false, ts2},
	}; !reflect.DeepEqual(expected, actual) {
		t.Errorf(`expected %v got %v`, expected, actual)
	}

	var ts3 string
	sqlDB.QueryRow(t, `BEGIN;
		DELETE FROM parent WHERE a = '1';
		DELETE FROM child WHERE e = '2';
		SELECT cluster_logical_timestamp();
	END;`).Scan(&ts3)
	if actual, expected := kvsToRows(slurpUserDataKVs(t, store.Engine())), []rowWithMVCCMetadata{
		{[]string{`1`, `NULL`, `NULL`, `NULL`}, true, ts3},
		{[]string{`1`, `10`}, false, ts1},
		{[]string{`2`, `b`, `b`, `NULL`}, false, ts2},
		{[]string{`2`, `20`}, true, ts2}, // ignore me: artifact of how the test is written
		{[]string{`2`, `21`}, true, ts3},
	}; !reflect.DeepEqual(expected, actual) {
		t.Errorf(`expected %v got %v`, expected, actual)
	}
}
