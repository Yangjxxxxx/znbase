// Copyright 2016 The Cockroach Authors.
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

package rowexec

import (
	"context"

	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/sql/backfill"
	"github.com/znbasedb/znbase/pkg/sql/distsqlpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/row"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage/storagebase"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/util/tracing"
)

// indexBackfiller is a processor that backfills new indexes.
type indexBackfiller struct {
	backfiller

	backfill.IndexBackfiller

	adder storagebase.BulkAdder

	desc *sqlbase.ImmutableTableDescriptor
}

var _ runbase.Processor = &indexBackfiller{}
var _ chunkBackfiller = &indexBackfiller{}

var backfillerBufferSize = settings.RegisterByteSizeSetting(
	"schemachanger.backfiller.buffer_size", "amount to buffer in memory during backfills", 196<<20,
)

var backillerSSTSize = settings.RegisterByteSizeSetting(
	"schemachanger.backfiller.max_sst_size", "target size for ingested files during backfills", 16<<20,
)

func newIndexBackfiller(
	flowCtx *runbase.FlowCtx,
	processorID int32,
	spec distsqlpb.BackfillerSpec,
	post *distsqlpb.PostProcessSpec,
	output runbase.RowReceiver,
) (*indexBackfiller, error) {
	ib := &indexBackfiller{
		desc: sqlbase.NewImmutableTableDescriptor(spec.Table),
		backfiller: backfiller{
			name:        "Index",
			filter:      backfill.IndexMutationFilter,
			flowCtx:     flowCtx,
			processorID: processorID,
			output:      output,
			spec:        spec,
		},
	}
	ib.backfiller.chunks = ib

	if err := ib.IndexBackfiller.Init(ib.desc); err != nil {
		return nil, err
	}
	ib.EvalCtx = flowCtx.NewEvalCtx()
	return ib, nil
}

func (ib *indexBackfiller) prepare(ctx context.Context) error {
	bufferSize := backfillerBufferSize.Get(&ib.flowCtx.Cfg.Settings.SV)
	sstSize := backillerSSTSize.Get(&ib.flowCtx.Cfg.Settings.SV)
	adder, err := ib.flowCtx.Cfg.BulkAdder(ctx, ib.flowCtx.Cfg.DB, bufferSize, sstSize, ib.spec.ReadAsOf, false)

	if err != nil {
		return err
	}
	ib.adder = adder
	ib.adder.SkipLocalDuplicates(ib.ContainsInvertedIndex())
	return nil
}

func (ib indexBackfiller) close(ctx context.Context) {
	ib.adder.Close(ctx)
}

func (ib *indexBackfiller) flush(ctx context.Context) error {
	return ib.wrapDupError(ctx, ib.adder.Flush(ctx))
}

func (ib *indexBackfiller) wrapDupError(ctx context.Context, orig error) error {
	if orig == nil {
		return nil
	}
	typed, ok := orig.(storagebase.DuplicateKeyError)
	if !ok {
		return orig
	}

	desc, err := ib.desc.MakeFirstMutationPublic()
	immutable := sqlbase.NewImmutableTableDescriptor(*desc.TableDesc())
	if err != nil {
		return err
	}
	v := &roachpb.Value{RawBytes: typed.Value}
	return row.NewUniquenessConstraintViolationError(ctx, immutable, typed.Key, v)
}

func (ib *indexBackfiller) runChunk(
	tctx context.Context,
	mutations []sqlbase.DescriptorMutation,
	sp roachpb.Span,
	chunkSize int64,
	readAsOf hlc.Timestamp,
) (roachpb.Key, error) {
	if ib.flowCtx.TestingKnobs().RunBeforeBackfillChunk != nil {
		if err := ib.flowCtx.TestingKnobs().RunBeforeBackfillChunk(sp); err != nil {
			return nil, err
		}
	}
	if ib.flowCtx.TestingKnobs().RunAfterBackfillChunk != nil {
		defer ib.flowCtx.TestingKnobs().RunAfterBackfillChunk()
	}

	ctx, traceSpan := tracing.ChildSpan(tctx, "chunk")
	defer tracing.FinishSpan(traceSpan)

	var key roachpb.Key
	transactionalChunk := func(ctx context.Context) error {
		return ib.flowCtx.Cfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
			// TODO(knz): do KV tracing in DistSQL processors.
			var err error
			key, err = ib.RunIndexBackfillChunk(
				ctx, txn, ib.desc, sp, chunkSize, true /*alsoCommit*/, false /*traceKV*/)
			return err
		})
	}

	// TODO(jordan): enable this once IsMigrated is a real implementation.
	/*
		if !util.IsMigrated() {
			// If we're running a mixed cluster, some of the nodes will have an old
			// implementation of InitPut that doesn't take into account the expected
			// timetsamp. In that case, we have to run our chunk transactionally at the
			// current time.
			err := transactionalChunk(ctx)
			return ib.fetcher.Key(), err
		}
	*/

	start := timeutil.Now()
	var entries []sqlbase.IndexEntry
	if err := ib.flowCtx.Cfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
		txn.SetFixedTimestamp(ctx, readAsOf)

		// TODO(knz): do KV tracing in DistSQL processors.
		var err error
		entries, key, err = ib.BuildIndexEntriesChunk(ctx, txn, ib.desc, sp, chunkSize, false /*traceKV*/)
		return err
	}); err != nil {
		return nil, err
	}
	prepTime := timeutil.Now().Sub(start)

	enabled := backfill.BulkWriteIndex.Get(&ib.flowCtx.Cfg.Settings.SV)
	if enabled {
		start := timeutil.Now()

		for _, i := range entries {
			if err := ib.adder.Add(ctx, i.Key, i.Value.RawBytes); err != nil {
				return nil, ib.wrapDupError(ctx, err)
			}
		}
		if ib.flowCtx.TestingKnobs().RunAfterBackfillChunk != nil {
			if err := ib.adder.Flush(ctx); err != nil {
				return nil, ib.wrapDupError(ctx, err)
			}
		}
		addTime := timeutil.Now().Sub(start)

		// Don't log perf stats in tests with small indexes.
		if len(entries) > 1000 {
			log.Infof(ctx, "index backfill stats: entries %d, prepare %+v, add-sst %+v",
				len(entries), prepTime, addTime)
		}
		return key, nil
	}
	retried := false
	// Write the new index values.
	if err := ib.flowCtx.Cfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
		batch := txn.NewBatch()

		for _, entry := range entries {
			// Since we're not regenerating the index entries here, if the
			// transaction restarts the values might already have their checksums
			// set which is invalid - clear them.
			if retried {
				// Reset the value slice. This is necessary because gRPC may still be
				// holding onto the underlying slice here. See #17348 for more details.
				// We only need to reset RawBytes because neither entry nor entry.Value
				// are pointer types.
				rawBytes := entry.Value.RawBytes
				entry.Value.RawBytes = make([]byte, len(rawBytes))
				copy(entry.Value.RawBytes, rawBytes)
				entry.Value.ClearChecksum()
			}
			batch.InitPut(entry.Key, &entry.Value, true /* failOnTombstones */)
		}
		retried = true
		if err := txn.CommitInBatch(ctx, batch); err != nil {
			if _, ok := batch.MustPErr().GetDetail().(*roachpb.ConditionFailedError); ok {
				return pgerror.NewError(pgcode.UniqueViolation, "")
			}
			return err
		}
		return nil
	}); err != nil {
		if sqlbase.IsUniquenessConstraintViolationError(err) {
			log.VEventf(ctx, 2, "failed write. retrying transactionally: %v", err)
			// Someone wrote a value above one of our new index entries. Since we did
			// a historical read, we didn't have the most up-to-date value for the
			// row we were backfilling so we can't just blindly write it to the
			// index. Instead, we retry the transaction at the present timestamp.
			if err := transactionalChunk(ctx); err != nil {
				log.VEventf(ctx, 2, "failed transactional write: %v", err)
				return nil, err
			}
		} else {
			log.VEventf(ctx, 2, "failed write due to other error, not retrying: %v", err)
			return nil, err
		}
	}

	return key, nil
}
