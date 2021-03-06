// Copyright 2018 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/CCL.txt

package workloadccl

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/znbasedb/znbase/pkg/base"
	_ "github.com/znbasedb/znbase/pkg/icl"
	"github.com/znbasedb/znbase/pkg/sql/stats"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/workload"
	"github.com/znbasedb/znbase/pkg/workload/tpcc"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

const fixtureTestGenRows = 10

type fixtureTestGen struct {
	flags workload.Flags
	val   string
	empty string
}

func makeTestWorkload() workload.Flagser {
	g := &fixtureTestGen{}
	g.flags.FlagSet = pflag.NewFlagSet(`fx`, pflag.ContinueOnError)
	g.flags.StringVar(&g.val, `val`, `default`, `The value for each row`)
	g.flags.StringVar(&g.empty, `empty`, ``, `An empty flag`)
	return g
}

var fixtureTestMeta = workload.Meta{
	Name: `fixture`,
	New: func() workload.Generator {
		return makeTestWorkload()
	},
}

func init() {
	workload.Register(fixtureTestMeta)
}

func (fixtureTestGen) Meta() workload.Meta     { return fixtureTestMeta }
func (g fixtureTestGen) Flags() workload.Flags { return g.flags }
func (g fixtureTestGen) Tables() []workload.Table {
	return []workload.Table{{
		Name:   `fx`,
		Schema: `(key INT PRIMARY KEY, value INT)`,
		InitialRows: workload.Tuples(
			fixtureTestGenRows,
			func(rowIdx int) []interface{} {
				return []interface{}{rowIdx, g.val}
			},
		),
		Stats: []workload.JSONStatistic{
			// Use stats that *don't* match reality, so we can test that these
			// stats were injected and not calculated by CREATE STATISTICS.
			workload.MakeStat([]string{"key"}, 100, 100, 0),
			workload.MakeStat([]string{"value"}, 100, 1, 5),
		},
	}}
}

func TestFixture(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	gcsBucket := os.Getenv(`GS_BUCKET`)
	gcsKey := os.Getenv(`GS_JSONKEY`)
	if gcsBucket == "" || gcsKey == "" {
		t.Skip("GS_BUCKET and GS_JSONKEY env vars must be set")
	}

	source, err := google.JWTConfigFromJSON([]byte(gcsKey), storage.ScopeReadWrite)
	if err != nil {
		t.Fatalf(`%+v`, err)
	}
	gcs, err := storage.NewClient(ctx,
		option.WithScopes(storage.ScopeReadWrite),
		option.WithTokenSource(source.TokenSource(ctx)))
	if err != nil {
		t.Fatalf(`%+v`, err)
	}
	defer func() { _ = gcs.Close() }()

	s, db, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)
	sqlDB := sqlutils.MakeSQLRunner(db)
	sqlDB.Exec(t, `SET CLUSTER SETTING cloudstorage.gs.default.key = $1`, gcsKey)

	gen := makeTestWorkload()
	flag := fmt.Sprintf(`val=%d`, timeutil.Now().UnixNano())
	if err := gen.Flags().Parse([]string{"--" + flag}); err != nil {
		t.Fatalf(`%+v`, err)
	}

	config := FixtureConfig{
		GCSBucket: gcsBucket,
		GCSPrefix: fmt.Sprintf(`TestFixture-%d`, timeutil.Now().UnixNano()),
	}

	if _, err := GetFixture(ctx, gcs, config, gen); !testutils.IsError(err, `fixture not found`) {
		t.Fatalf(`expected "fixture not found" error but got: %+v`, err)
	}

	fixtures, err := ListFixtures(ctx, gcs, config)
	if err != nil {
		t.Fatalf(`%+v`, err)
	}
	if len(fixtures) != 0 {
		t.Errorf(`expected no fixtures but got: %+v`, fixtures)
	}

	const filesPerNode = 1
	fixture, err := MakeFixture(ctx, db, gcs, config, gen, filesPerNode)
	if err != nil {
		t.Fatalf(`%+v`, err)
	}

	_, err = MakeFixture(ctx, db, gcs, config, gen, filesPerNode)
	if !testutils.IsError(err, `already exists`) {
		t.Fatalf(`expected 'already exists' error got: %+v`, err)
	}

	fixtures, err = ListFixtures(ctx, gcs, config)
	if err != nil {
		t.Fatalf(`%+v`, err)
	}
	if len(fixtures) != 1 || !strings.Contains(fixtures[0], flag) {
		t.Errorf(`expected exactly one %s fixture but got: %+v`, flag, fixtures)
	}

	sqlDB.Exec(t, `CREATE DATABASE test`)
	if _, err := RestoreFixture(ctx, db, fixture, `test`); err != nil {
		t.Fatalf(`%+v`, err)
	}
	sqlDB.CheckQueryResults(t,
		`SELECT count(*) FROM test.fx`, [][]string{{strconv.Itoa(fixtureTestGenRows)}})
}

func TestImportFixture(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	defer func(oldRefreshInterval, oldAsOf time.Duration) {
		stats.DefaultRefreshInterval = oldRefreshInterval
		stats.DefaultAsOfTime = oldAsOf
	}(stats.DefaultRefreshInterval, stats.DefaultAsOfTime)
	stats.DefaultRefreshInterval = time.Millisecond
	stats.DefaultAsOfTime = 10 * time.Millisecond

	s, db, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)
	sqlDB := sqlutils.MakeSQLRunner(db)

	sqlDB.Exec(t, `SET CLUSTER SETTING sql.stats.automatic_collection.enabled=true`)

	gen := makeTestWorkload()
	flag := fmt.Sprintf(`val=%d`, timeutil.Now().UnixNano())
	if err := gen.Flags().Parse([]string{"--" + flag}); err != nil {
		t.Fatalf(`%+v`, err)
	}

	const filesPerNode = 1
	sqlDB.Exec(t, `CREATE DATABASE distsort`)
	_, err := ImportFixture(
		ctx, db, gen, `distsort`, false /* directIngestion */, filesPerNode, true, /* injectStats */
	)
	require.NoError(t, err)
	sqlDB.CheckQueryResults(t,
		`SELECT count(*) FROM distsort.fx`, [][]string{{strconv.Itoa(fixtureTestGenRows)}})

	sqlDB.CheckQueryResults(t,
		`SELECT statistics_name, column_names, row_count, distinct_count, null_count
           FROM [SHOW STATISTICS FOR TABLE distsort.fx]`,
		[][]string{
			{"__auto__", "{key}", "100", "100", "0"},
			{"__auto__", "{value}", "100", "1", "5"},
		})

	sqlDB.Exec(t, `CREATE DATABASE direct`)
	_, err = ImportFixture(
		ctx, db, gen, `direct`, true /* directIngestion */, filesPerNode, false, /* injectStats */
	)
	require.NoError(t, err)
	sqlDB.CheckQueryResults(t,
		`SELECT count(*) FROM direct.fx`, [][]string{{strconv.Itoa(fixtureTestGenRows)}})

	fingerprints := sqlDB.QueryStr(t, `SHOW EXPERIMENTAL_FINGERPRINTS FROM TABLE distsort.fx`)
	sqlDB.CheckQueryResults(t, `SHOW EXPERIMENTAL_FINGERPRINTS FROM TABLE direct.fx`, fingerprints)

	// Since we did not inject stats, the IMPORT should have triggered
	// automatic stats collection.
	sqlDB.CheckQueryResultsRetry(t,
		`SELECT statistics_name, column_names, row_count, distinct_count, null_count
           FROM [SHOW STATISTICS FOR TABLE direct.fx]`,
		[][]string{
			{"__auto__", "{key}", "10", "10", "0"},
			{"__auto__", "{value}", "10", "1", "0"},
		})

}

func BenchmarkImportFixtureTPCC(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping long benchmark")
	}
	ctx := context.Background()
	gen := tpcc.FromWarehouses(1)

	var bytes int64
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		s, db, _ := serverutils.StartServer(b, base.TestServerArgs{})
		sqlDB := sqlutils.MakeSQLRunner(db)
		sqlDB.Exec(b, `CREATE DATABASE d`)

		b.StartTimer()
		const filesPerNode = 1
		importBytes, err := ImportFixture(
			ctx, db, gen, `d`, true /* directIngestion */, filesPerNode, true, /* injectStats */
		)
		require.NoError(b, err)
		bytes += importBytes
		b.StopTimer()

		s.Stopper().Stop(ctx)
	}
	b.SetBytes(bytes / int64(b.N))
}
