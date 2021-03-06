// Copyright 2017 The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package sampledataicl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/icl/dump"
	"github.com/znbasedb/znbase/pkg/icl/load"
	"github.com/znbasedb/znbase/pkg/icl/storageicl"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/workload"
)

// ToBackup creates an enterprise backup in `dir`.
func ToBackup(t testing.TB, data workload.Table, dir string) (*Backup, error) {
	return toBackup(t, data, dir, 0)
}

func toBackup(t testing.TB, data workload.Table, dir string, chunkBytes int64) (*Backup, error) {
	tempDir, dirCleanupFn := testutils.TempDir(t)
	defer dirCleanupFn()

	// TODO(dan): Get rid of the `t testing.TB` parameter and this `TestServer`.
	ctx := context.Background()
	s, db, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)
	if _, err := db.Exec(`CREATE DATABASE data`); err != nil {
		return nil, err
	}

	var stmts bytes.Buffer
	_, _ = fmt.Fprintf(&stmts, "CREATE TABLE %s %s;\n", data.Name, data.Schema)
	for rowIdx := 0; rowIdx < data.InitialRows.NumBatches; rowIdx++ {
		for _, row := range data.InitialRows.Batch(rowIdx) {
			rowBatch := strings.Join(workload.StringTuple(row), `,`)
			_, _ = fmt.Fprintf(&stmts, "INSERT INTO %s VALUES (%s);\n", data.Name, rowBatch)
		}
	}

	// TODO(dan): The csv load will be less overhead, use it when we have it.
	ts := hlc.Timestamp{WallTime: hlc.UnixNano()}
	desc, err := load.Load(ctx, db, &stmts, `data`, `nodelocal:///`, ts, chunkBytes, tempDir, dir)
	if err != nil {
		return nil, err
	}
	return &Backup{BaseDir: dir, Desc: desc}, nil
}

// Backup is a representation of an enterprise BACKUP.
type Backup struct {
	// BaseDir can be used for a RESTORE. All paths in the descriptor are
	// relative to this.
	BaseDir string
	Desc    dump.DumpDescriptor

	fileIdx int
	iterIdx int
}

// ResetKeyValueIteration resets the NextKeyValues iteration to the first kv.
func (b *Backup) ResetKeyValueIteration() {
	b.fileIdx = 0
	b.iterIdx = 0
}

// NextKeyValues iterates and returns every *user table data* key-value in the
// backup. At least `count` kvs will be returned, but rows are not broken up, so
// slightly more than `count` may come back. If fewer than `count` are
// available, err will be `io.EOF` and kvs may be partially filled with the
// remainer.
func (b *Backup) NextKeyValues(
	count int, newTableID sqlbase.ID,
) ([]engine.MVCCKeyValue, roachpb.Span, error) {
	var userTables []*sqlbase.TableDescriptor
	for _, d := range b.Desc.Descriptors {
		//if t := d.GetTable(); t != nil && t.ParentID != keys.SystemDatabaseID {
		if t := d.Table(hlc.Timestamp{}); t != nil && t.ParentID != keys.SystemDatabaseID {
			userTables = append(userTables, t)
		}
	}
	if len(userTables) != 1 {
		return nil, roachpb.Span{}, errors.Errorf(
			"only backups of one table are currently supported, got %d", len(userTables))
	}
	tableDesc := userTables[0]

	newDesc := *tableDesc
	newDesc.ID = newTableID
	kr, err := storageicl.MakeKeyRewriter(sqlbase.TablesByID{tableDesc.ID: &newDesc})
	if err != nil {
		return nil, roachpb.Span{}, err
	}

	var kvs []engine.MVCCKeyValue
	span := roachpb.Span{Key: keys.MaxKey}
	for ; b.fileIdx < len(b.Desc.Files); b.fileIdx++ {
		file := b.Desc.Files[b.fileIdx]

		sst := engine.MakeRocksDBSstFileReader()
		defer sst.Close()
		fileContents, err := ioutil.ReadFile(filepath.Join(b.BaseDir, file.Path))
		if err != nil {
			return nil, roachpb.Span{}, err
		}
		if err := sst.IngestExternalFile(fileContents); err != nil {
			return nil, roachpb.Span{}, err
		}

		it := sst.NewIterator(engine.IterOptions{UpperBound: roachpb.KeyMax})
		defer it.Close()
		it.Seek(engine.MVCCKey{Key: file.Span.Key})

		iterIdx := 0
		for ; ; it.Next() {
			if len(kvs) >= count {
				break
			}
			if iterIdx < b.iterIdx {
				iterIdx++
				continue
			}

			ok, err := it.Valid()
			if err != nil {
				return nil, roachpb.Span{}, err
			}
			if !ok || it.UnsafeKey().Key.Compare(file.Span.EndKey) >= 0 {
				break
			}

			if iterIdx < b.iterIdx {
				break
			}
			b.iterIdx = iterIdx

			key := it.Key()
			key.Key, ok, err = kr.RewriteKey(key.Key)
			if err != nil {
				return nil, roachpb.Span{}, err
			}
			if !ok {
				return nil, roachpb.Span{}, errors.Errorf("rewriter did not match key: %s", key.Key)
			}
			v := roachpb.Value{RawBytes: it.Value()}
			v.ClearChecksum()
			v.InitChecksum(key.Key)
			kvs = append(kvs, engine.MVCCKeyValue{Key: key, Value: v.RawBytes})

			if key.Key.Compare(span.Key) < 0 {
				span.Key = append(span.Key[:0], key.Key...)
			}
			if key.Key.Compare(span.EndKey) > 0 {
				span.EndKey = append(span.EndKey[:0], key.Key...)
			}
			iterIdx++
		}
		b.iterIdx = iterIdx

		if len(kvs) >= count {
			break
		}
		if ok, _ := it.Valid(); !ok {
			b.iterIdx = 0
		}
	}

	span.EndKey = span.EndKey.Next()
	if len(kvs) < count {
		return kvs, span, io.EOF
	}
	return kvs, span, nil
}
