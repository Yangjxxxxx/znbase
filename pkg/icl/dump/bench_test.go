// Copyright 2016  The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package dump_test

import (
	"bytes"
	gosql "database/sql"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/znbasedb/znbase/pkg/icl/dump"
	"github.com/znbasedb/znbase/pkg/icl/load"
	"github.com/znbasedb/znbase/pkg/icl/utilicl/sampledataicl"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/workload"
	"github.com/znbasedb/znbase/pkg/workload/bank"
)

func bankBuf(numAccounts int) *bytes.Buffer {
	bankData := bank.FromRows(numAccounts).Tables()[0]
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "CREATE TABLE %s %s;\n", bankData.Name, bankData.Schema)
	for rowIdx := 0; rowIdx < bankData.InitialRows.NumBatches; rowIdx++ {
		for _, row := range bankData.InitialRows.Batch(rowIdx) {
			rowBatch := strings.Join(workload.StringTuple(row), `,`)
			fmt.Fprintf(&buf, "INSERT INTO %s VALUES (%s);\n", bankData.Name, rowBatch)
		}
	}
	return &buf
}

func BenchmarkClusterBackup(b *testing.B) {
	if testing.Short() {
		b.Skip("TODO: fix benchmark")
	}
	// NB: This benchmark takes liberties in how b.N is used compared to the go
	// documentation's description. We're getting useful information out of it,
	// but this is not a pattern to cargo-cult.

	_, _, sqlDB, dir, cleanupFn := dumpLoadTestSetup(b, multiNode, 0, initNone)
	defer cleanupFn()
	sqlDB.Exec(b, `DROP TABLE data.bank`)

	bankData := bank.FromRows(b.N).Tables()[0]
	loadDir := filepath.Join(dir, "load")
	if _, err := sampledataicl.ToBackup(b, bankData, loadDir); err != nil {
		b.Fatalf("%+v", err)
	}
	sqlDB.Exec(b, fmt.Sprintf(`LOAD TABLE data.bank FROM '%s'`, loadDir))

	// TODO(dan): Ideally, this would split and rebalance the ranges in a more
	// controlled way. A previous version of this code did it manually with
	// `SPLIT AT` and TestCluster's TransferRangeLease, but it seemed to still
	// be doing work after returning, which threw off the timing and the results
	// of the benchmark. DistSQL is working on improving this infrastructure, so
	// use what they build.

	b.ResetTimer()
	var unused string
	var dataSize int64
	sqlDB.QueryRow(b, fmt.Sprintf(`DUMP DATABASE data TO SST '%s'`, dir)).Scan(
		&unused, &unused, &unused, &unused, &unused, &unused, &dataSize,
	)
	b.StopTimer()
	b.SetBytes(dataSize / int64(b.N))
}

func BenchmarkClusterRestore(b *testing.B) {
	// NB: This benchmark takes liberties in how b.N is used compared to the go
	// documentation's description. We're getting useful information out of it,
	// but this is not a pattern to cargo-cult.

	_, _, sqlDB, dir, cleanup := dumpLoadTestSetup(b, multiNode, 0, initNone)
	defer cleanup()
	sqlDB.Exec(b, `DROP TABLE data.bank`)

	bankData := bank.FromRows(b.N).Tables()[0]
	_, err := sampledataicl.ToBackup(b, bankData, filepath.Join(dir, "foo"))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	dumpDesc := dump.DumpDescriptor{}
	b.SetBytes(dumpDesc.EntryCounts.DataSize / int64(b.N))

	b.ResetTimer()
	sqlDB.Exec(b, `LOAD TABLE data.bank FROM 'nodelocal:///foo'`)
	b.StopTimer()
}

func BenchmarkLoadRestore(b *testing.B) {
	if testing.Short() {
		b.Skip("TODO: fix benchmark")
	}
	// NB: This benchmark takes liberties in how b.N is used compared to the go
	// documentation's description. We're getting useful information out of it,
	// but this is not a pattern to cargo-cult.

	ctx, _, sqlDB, dir, cleanup := dumpLoadTestSetup(b, multiNode, 0, initNone)
	defer cleanup()
	sqlDB.Exec(b, `DROP TABLE data.bank`)

	buf := bankBuf(b.N)
	b.SetBytes(int64(buf.Len() / b.N))
	ts := hlc.Timestamp{WallTime: hlc.UnixNano()}
	b.ResetTimer()
	if _, err := load.Load(ctx, sqlDB.DB.(*gosql.DB), buf, "data", dir, ts, 0, dir, dir); err != nil {
		b.Fatalf("%+v", err)
	}
	sqlDB.Exec(b, fmt.Sprintf(`LOAD TABLE data.bank FROM '%s'`, dir))
	b.StopTimer()
}

func BenchmarkLoadSQL(b *testing.B) {
	// NB: This benchmark takes liberties in how b.N is used compared to the go
	// documentation's description. We're getting useful information out of it,
	// but this is not a pattern to cargo-cult.
	_, _, sqlDB, _, cleanup := dumpLoadTestSetup(b, multiNode, 0, initNone)
	defer cleanup()
	sqlDB.Exec(b, `DROP TABLE data.bank`)

	buf := bankBuf(b.N)
	b.SetBytes(int64(buf.Len() / b.N))
	lines := make([]string, 0, b.N)
	for {
		line, err := buf.ReadString(';')
		if err == io.EOF {
			break
		} else if err != nil {
			b.Fatalf("%+v", err)
		}
		lines = append(lines, line)
	}

	b.ResetTimer()
	for _, line := range lines {
		sqlDB.Exec(b, line)
	}
	b.StopTimer()
}

func BenchmarkClusterEmptyIncrementalBackup(b *testing.B) {
	if testing.Short() {
		b.Skip("TODO: fix benchmark")
	}
	const numStatements = 100000

	_, _, sqlDB, _, cleanupFn := dumpLoadTestSetup(b, multiNode, 0, initNone)
	defer cleanupFn()

	restoreDir := filepath.Join(localFoo, "restore")
	fullDir := filepath.Join(localFoo, "full")

	bankData := bank.FromRows(numStatements).Tables()[0]
	_, err := sampledataicl.ToBackup(b, bankData, restoreDir)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	sqlDB.Exec(b, `DROP TABLE data.bank`)
	sqlDB.Exec(b, `LOAD TABLE data.bank FROM $1`, restoreDir)

	var unused string
	var dataSize int64
	sqlDB.QueryRow(b, `DUMP DATABASE data TO SST $1`, fullDir).Scan(
		&unused, &unused, &unused, &unused, &unused, &unused, &dataSize,
	)

	// We intentionally don't write anything to the database between the full and
	// incremental backup.

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		incrementalDir := filepath.Join(localFoo, fmt.Sprintf("incremental%d", i))
		sqlDB.Exec(b, `DUMP DATABASE data TO SST $1 INCREMENTAL FROM $2`, incrementalDir, fullDir)
	}
	b.StopTimer()

	// We report the number of bytes that incremental backup was able to
	// *skip*--i.e., the number of bytes in the full backup.
	b.SetBytes(int64(b.N) * dataSize)
}
