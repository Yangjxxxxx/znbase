// Copyright 2018  The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package changefeedicl

import (
	"context"
	gosql "database/sql"
	gojson "encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/znbasedb/apd"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/icl/changefeedicl/cdctest"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/kv"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/security"
	"github.com/znbasedb/znbase/pkg/sql/distsql"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/log"
)

func waitForSchemaChange(
	t testing.TB, sqlDB *sqlutils.SQLRunner, stmt string, arguments ...interface{},
) {
	sqlDB.Exec(t, stmt, arguments...)
	row := sqlDB.QueryRow(t, "SELECT job_id FROM [SHOW JOBS] ORDER BY created DESC LIMIT 1")
	var jobID string
	row.Scan(&jobID)

	testutils.SucceedsSoon(t, func() error {
		row := sqlDB.QueryRow(t, "SELECT status FROM [SHOW JOBS] WHERE job_id = $1", jobID)
		var status string
		row.Scan(&status)
		if status != "succeeded" {
			return fmt.Errorf("Job %s had status %s, wanted 'succeeded'", jobID, status)
		}
		return nil
	})
}

func assertPayloads(t testing.TB, f cdctest.TestFeed, expected []string) {
	t.Helper()

	var actual []string
	for len(actual) < len(expected) {
		m, err := f.Next()
		if log.V(1) {
			log.Infof(context.TODO(), `%v %s: %s->%s`, err, m.Topic, m.Key, m.Value)
		}
		if err != nil {
			t.Fatal(err)
		} else if m == nil {
			t.Fatal(`expected message`)
		} else if len(m.Key) > 0 || len(m.Value) > 0 {
			actual = append(actual, fmt.Sprintf(`%s: %s->%s`, m.Topic, m.Key, m.Value))
		}
	}

	// The tests that use this aren't concerned with order, just that these are
	// the next len(expected) messages.
	sort.Strings(expected)
	sort.Strings(actual)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected\n  %s\ngot\n  %s",
			strings.Join(expected, "\n  "), strings.Join(actual, "\n  "))
	}
}

func avroToJSON(t testing.TB, reg *testSchemaRegistry, avroBytes []byte) []byte {
	if len(avroBytes) == 0 {
		return nil
	}
	native, err := reg.encodedAvroToNative(avroBytes)
	if err != nil {
		t.Fatal(err)
	}
	// The avro textual format is a more natural fit, but it's non-deterministic
	// because of go's randomized map ordering. Instead, we use gojson.Marshal,
	// which sorts its object keys and so is deterministic.
	json, err := gojson.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	return json
}

func assertPayloadsAvro(
	t testing.TB, reg *testSchemaRegistry, f cdctest.TestFeed, expected []string,
) {
	t.Helper()

	var actual []string
	for len(actual) < len(expected) {
		m, err := f.Next()
		if err != nil {
			t.Fatal(err)
		} else if m == nil {
			t.Fatal(`expected message`)
		} else if m.Key != nil {
			key, value := avroToJSON(t, reg, m.Key), avroToJSON(t, reg, m.Value)
			actual = append(actual, fmt.Sprintf(`%s: %s->%s`, m.Topic, key, value))
		}
	}

	// The tests that use this aren't concerned with order, just that these are
	// the next len(expected) messages.
	sort.Strings(expected)
	sort.Strings(actual)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected\n  %s\ngot\n  %s",
			strings.Join(expected, "\n  "), strings.Join(actual, "\n  "))
	}
}

func skipResolvedTimestamps(t *testing.T, f cdctest.TestFeed) {
	t.Helper()
	for {
		m, err := f.Next()
		if err != nil {
			t.Fatal(err)
		} else if m == nil {
			t.Fatal(`expected message`)
		} else if m.Key != nil {
			t.Errorf(`unexpected row %s: %s->%s`, m.Topic, m.Key, m.Value)
		}
	}
}

func parseTimeToHLC(t testing.TB, s string) hlc.Timestamp {
	t.Helper()
	d, _, err := apd.NewFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tree.DecimalToHLC(d)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func expectResolvedTimestamp(t testing.TB, f cdctest.TestFeed) hlc.Timestamp {
	t.Helper()
	m, err := f.Next()
	if err != nil {
		t.Fatal(err)
	} else if m == nil {
		t.Fatal(`expected message`)
	}
	if m.Key != nil {
		t.Fatalf(`unexpected row %s: %s -> %s`, m.Topic, m.Key, m.Value)
	}
	if m.Resolved == nil {
		t.Fatal(`expected a resolved timestamp notification`)
	}

	var resolvedRaw struct {
		Resolved string `json:"resolved"`
	}
	if err := gojson.Unmarshal(m.Resolved, &resolvedRaw); err != nil {
		t.Fatal(err)
	}

	return parseTimeToHLC(t, resolvedRaw.Resolved)
}

func expectResolvedTimestampAvro(
	t testing.TB, reg *testSchemaRegistry, f cdctest.TestFeed,
) hlc.Timestamp {
	t.Helper()
	m, err := f.Next()
	if err != nil {
		t.Fatal(err)
	} else if m == nil {
		t.Fatal(`expected message`)
	}
	if m.Key != nil {
		key, value := avroToJSON(t, reg, m.Key), avroToJSON(t, reg, m.Value)
		t.Fatalf(`unexpected row %s: %s -> %s`, m.Topic, key, value)
	}
	if m.Resolved == nil {
		t.Fatal(`expected a resolved timestamp notification`)
	}
	resolvedNative, err := reg.encodedAvroToNative(m.Resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedNative.(map[string]interface{})[`resolved`]
	return parseTimeToHLC(t, resolved.(map[string]interface{})[`string`].(string))
}

func sinklessTest(testFn func(*testing.T, *gosql.DB, cdctest.TestFeedFactory)) func(*testing.T) {
	return func(t *testing.T) {
		ctx := context.Background()
		knobs := base.TestingKnobs{DistSQL: &distsql.TestingKnobs{CDC: &TestingKnobs{
			// Disable the safety net for errors that should be marked as terminal by
			// weren't. We want to catch these in tests.
			ConsecutiveIdenticalErrorBailoutCount: math.MaxInt32,
		}}}
		s, db, _ := serverutils.StartServer(t, base.TestServerArgs{
			Knobs:       knobs,
			UseDatabase: `d`,
		})
		defer s.Stopper().Stop(ctx)
		sqlDB := sqlutils.MakeSQLRunner(db)
		sqlDB.Exec(t, `SET CLUSTER SETTING kv.rangefeed.enabled = true`)
		// TODO(dan): We currently have to set this to an extremely conservative
		// value because otherwise schema changes become flaky (they don't commit
		// their txn in time, get pushed by closed timestamps, and retry forever).
		// This is more likely when the tests run slower (race builds or inside
		// docker). The conservative value makes our tests take a lot longer,
		// though. Figure out some way to speed this up.
		sqlDB.Exec(t, `SET CLUSTER SETTING kv.closed_timestamp.target_duration = '1s'`)
		// TODO(dan): This is still needed to speed up table_history, that should be
		// moved to RangeFeed as well.
		sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.experimental_poll_interval = '10ms'`)
		sqlDB.Exec(t, `CREATE DATABASE d`)

		sink, cleanup := sqlutils.PGUrl(t, s.ServingAddr(), t.Name(), url.User(security.RootUser))
		defer cleanup()
		f := cdctest.MakeSinklessFeedFactory(s, sink)
		testFn(t, db, f)
	}
}

func enterpriseTest(testFn func(*testing.T, *gosql.DB, cdctest.TestFeedFactory)) func(*testing.T) {
	return func(t *testing.T) {
		ctx := context.Background()

		flushCh := make(chan struct{}, 1)
		defer close(flushCh)
		knobs := base.TestingKnobs{DistSQL: &distsql.TestingKnobs{CDC: &TestingKnobs{
			AfterSinkFlush: func() error {
				select {
				case flushCh <- struct{}{}:
				default:
				}
				return nil
			},
			// Disable the safety net for errors that should be marked as terminal by
			// weren't. We want to catch these in tests.
			ConsecutiveIdenticalErrorBailoutCount: math.MaxInt32,
		}}}

		s, db, _ := serverutils.StartServer(t, base.TestServerArgs{
			UseDatabase: "d",
			Knobs:       knobs,
		})
		defer s.Stopper().Stop(ctx)
		sqlDB := sqlutils.MakeSQLRunner(db)
		// TODO(dan): Switch this to RangeFeed, too. It seems wasteful right now
		// because the RangeFeed version of the tests take longer due to
		// closed_timestamp.target_duration's interaction with schema changes.
		//sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.push.enabled = false`)
		//sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.experimental_poll_interval = '10ms'`)
		//sqlDB.Exec(t, `CREATE DATABASE d`)
		sqlDB.Exec(t, `SET CLUSTER SETTING kv.rangefeed.enabled = true`)
		sqlDB.Exec(t, `SET CLUSTER SETTING kv.closed_timestamp.target_duration = '1s'`)
		sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.experimental_poll_interval = '10ms'`)
		sqlDB.Exec(t, `CREATE DATABASE d`)
		sink, cleanup := sqlutils.PGUrl(t, s.ServingAddr(), t.Name(), url.User(security.RootUser))
		defer cleanup()
		f := cdctest.MakeTableFeedFactory(s, db, flushCh, sink)

		testFn(t, db, f)
	}
}

func pollerTest(
	metaTestFn func(func(*testing.T, *gosql.DB, cdctest.TestFeedFactory)) func(*testing.T),
	testFn func(*testing.T, *gosql.DB, cdctest.TestFeedFactory),
) func(*testing.T) {
	return func(t *testing.T) {
		metaTestFn(func(t *testing.T, db *gosql.DB, f cdctest.TestFeedFactory) {
			sqlDB := sqlutils.MakeSQLRunner(db)
			sqlDB.Exec(t, `SET CLUSTER SETTING kv.rangefeed.enabled = true`)
			sqlDB.Exec(t, `SET CLUSTER SETTING kv.closed_timestamp.target_duration = '1s'`)
			sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.experimental_poll_interval = '10ms'`)
			//sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.push.enabled = false`)
			//sqlDB.Exec(t, `SET CLUSTER SETTING changefeed.experimental_poll_interval = '10ms'`)
			testFn(t, db, f)
		})(t)
	}
}

func cloudStorageTest(
	testFn func(*testing.T, *gosql.DB, cdctest.TestFeedFactory),
) func(*testing.T) {
	return func(t *testing.T) {
		ctx := context.Background()

		dir, dirCleanupFn := testutils.TempDir(t)
		defer dirCleanupFn()

		flushCh := make(chan struct{}, 1)
		defer close(flushCh)
		knobs := base.TestingKnobs{DistSQL: &distsql.TestingKnobs{CDC: &TestingKnobs{
			AfterSinkFlush: func() error {
				select {
				case flushCh <- struct{}{}:
				default:
				}
				return nil
			},
		}}}

		s, db, _ := serverutils.StartServer(t, base.TestServerArgs{
			UseDatabase:   "d",
			ExternalIODir: dir,
			Knobs:         knobs,
		})
		defer s.Stopper().Stop(ctx)
		sqlDB := sqlutils.MakeSQLRunner(db)
		sqlDB.Exec(t, `SET CLUSTER SETTING kv.rangefeed.enabled = true`)
		sqlDB.Exec(t, `SET CLUSTER SETTING kv.closed_timestamp.target_duration = '1s'`)
		sqlDB.Exec(t, `CREATE DATABASE d`)

		f := cdctest.MakeCloudFeedFactory(s, db, dir, flushCh)
		testFn(t, db, f)
	}
}

func feed(
	t testing.TB, f cdctest.TestFeedFactory, create string, args ...interface{},
) cdctest.TestFeed {
	t.Helper()
	feed, err := f.Feed(create, args...)
	if err != nil {
		t.Fatal(err)
	}
	return feed
}

func closeFeed(t testing.TB, f cdctest.TestFeed) {
	t.Helper()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func forceTableGC(
	t testing.TB,
	tsi serverutils.TestServerInterface,
	sqlDB *sqlutils.SQLRunner,
	database, table string,
) {
	t.Helper()
	tblID := sqlutils.QueryTableID(t, sqlDB.DB, database, table)

	tblKey := roachpb.Key(keys.MakeTablePrefix(tblID))
	gcr := roachpb.GCRequest{
		RequestHeader: roachpb.RequestHeader{
			Key:    tblKey,
			EndKey: tblKey.PrefixEnd(),
		},
		Threshold: tsi.Clock().Now(),
	}
	if _, err := client.SendWrapped(context.Background(), tsi.DistSender().(*kv.DistSender), &gcr); err != nil {
		t.Fatal(err)
	}
}
