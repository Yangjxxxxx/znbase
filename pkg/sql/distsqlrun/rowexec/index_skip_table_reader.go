// Copyright 2019  The Cockroach Authors.

package rowexec

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/row"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

// indexSkipTableReader is a processor that retrieves distinct rows from
// a table using the prefix of an index to skip reading some rows in the
// table. Specifically, given a prefix of an index to distinct over,
// the indexSkipTableReader returns all distinct rows where that prefix
// of the index is distinct. It uses the index to seek to distinct values
// of the prefix instead of doing a full table scan.
type indexSkipTableReader struct {
	runbase.ProcessorBase

	spans roachpb.Spans

	// currentSpan maintains which span we are currently scanning.
	currentSpan int

	// keyPrefixLen holds the length of the prefix of the index
	// that we are performing a distinct over.
	keyPrefixLen int
	// indexLen holds the number of columns in the index that
	// is being considered.
	indexLen int

	reverse bool

	ignoreMisplannedRanges bool
	misplannedRanges       []roachpb.RangeInfo

	fetcher row.Fetcher
	alloc   sqlbase.DatumAlloc
}

const indexSkipTableReaderProcName = "index skip table reader"

var istrPool = sync.Pool{
	New: func() interface{} {
		return &indexSkipTableReader{}
	},
}

var _ runbase.Processor = &indexSkipTableReader{}
var _ runbase.RowSource = &indexSkipTableReader{}
var _ distsqlpb.MetadataSource = &indexSkipTableReader{}

func newIndexSkipTableReader(
	flowCtx *runbase.FlowCtx,
	processorID int32,
	spec *distsqlpb.IndexSkipTableReaderSpec,
	post *distsqlpb.PostProcessSpec,
	output runbase.RowReceiver,
) (*indexSkipTableReader, error) {
	if flowCtx.NodeID == 0 {
		return nil, errors.Errorf("attempting to create a tableReader with uninitialized NodeID")
	}

	t := istrPool.Get().(*indexSkipTableReader)

	returnMutations := spec.Visibility == distsqlpb.ScanVisibility_PUBLIC_AND_NOT_PUBLIC
	types := spec.Table.ColumnTypesWithMutations(returnMutations)
	t.ignoreMisplannedRanges = flowCtx.Local
	t.reverse = spec.Reverse

	if err := t.Init(
		t,
		post,
		types,
		flowCtx,
		processorID,
		output,
		nil, /* memMonitor */
		runbase.ProcStateOpts{
			InputsToDrain:        nil,
			TrailingMetaCallback: t.generateTrailingMeta,
		},
	); err != nil {
		return nil, err
	}

	neededColumns := t.Out.NeededColumns()
	t.keyPrefixLen = neededColumns.Len()

	columnIdxMap := spec.Table.ColumnIdxMapWithMutations(returnMutations)

	immutDesc := sqlbase.NewImmutableTableDescriptor(spec.Table)
	index, isSecondaryIndex, err := immutDesc.FindIndexByIndexIdx(int(spec.IndexIdx))
	if err != nil {
		return nil, err
	}
	t.indexLen = len(index.ColumnIDs)

	cols := immutDesc.Columns
	if returnMutations {
		cols = immutDesc.ReadableColumns
	}

	tableArgs := row.FetcherTableArgs{
		Desc:             immutDesc,
		Index:            index,
		ColIdxMap:        columnIdxMap,
		IsSecondaryIndex: isSecondaryIndex,
		Cols:             cols,
		ValNeededForCol:  neededColumns,
	}

	if err := t.fetcher.Init(
		t.reverse, true, /* returnRangeInfo */
		false /* isCheck */, &t.alloc,
		sqlbase.ScanLockingStrength_FOR_NONE, sqlbase.ScanLockingWaitPolicy{LockLevel: sqlbase.ScanLockingWaitLevel_BLOCK}, tableArgs); err != nil {
		return nil, err
	}

	// Make a copy of the spans for this reader, as we will modify them.
	nSpans := len(spec.Spans)
	if cap(t.spans) >= nSpans {
		t.spans = t.spans[:nSpans]
	} else {
		t.spans = make(roachpb.Spans, nSpans)
	}

	// If we are scanning in reverse, then copy the spans in backwards.
	if t.reverse {
		for i, s := range spec.Spans {
			t.spans[len(spec.Spans)-i-1] = s.Span
		}
	} else {
		for i, s := range spec.Spans {
			t.spans[i] = s.Span
		}
	}

	return t, nil
}

func (t *indexSkipTableReader) Start(ctx context.Context) context.Context {
	t.StartInternal(ctx, indexSkipTableReaderProcName)
	return ctx
}

func (t *indexSkipTableReader) Next() (sqlbase.EncDatumRow, *distsqlpb.ProducerMetadata) {
	for t.State == runbase.StateRunning {
		if t.currentSpan >= len(t.spans) {
			t.MoveToDraining(nil)
			return nil, t.DrainHelper()
		}

		// Start a scan to get the smallest value within this span.
		err := t.fetcher.StartScan(
			t.Ctx, t.FlowCtx.Txn, t.spans[t.currentSpan:t.currentSpan+1],
			true, 1 /* batch size limit */, t.FlowCtx.TraceKV,
		)
		if err != nil {
			t.MoveToDraining(err)
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}

		// Range info resets once a scan begins, so we need to maintain
		// the range info we get after each scan.
		if !t.ignoreMisplannedRanges {
			ranges := runbase.MisplannedRanges(t.Ctx, t.fetcher.GetRangesInfo(), t.FlowCtx.NodeID)
			for _, r := range ranges {
				t.misplannedRanges = roachpb.InsertRangeInfo(t.misplannedRanges, r)
			}
		}

		// This key *must not* be modified, as this will cause the fetcher
		// to begin acting incorrectly. This is because modifications
		// will corrupt the row internal to the fetcher.
		key, err := t.fetcher.PartialKey(t.keyPrefixLen)
		if err != nil {
			t.MoveToDraining(err)
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}

		row, _, _, err := t.fetcher.NextRow(t.Ctx)
		if err != nil {
			t.MoveToDraining(err)
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}
		if row == nil {
			// No more rows in this span, so move to the next one.
			t.currentSpan++
			continue
		}

		if !t.reverse {
			// We set the new key to be the largest key with the prefix that we have
			// so that we skip all values with the same prefix, and "skip" to the
			// next distinct value.
			t.spans[t.currentSpan].Key = key.PrefixEnd()
		} else {
			// In the case of reverse, this is much easier. The reverse fetcher
			// returns the key retrieved, in this case the first key smaller
			// than EndKey in the current span. Since EndKey is exclusive, we
			// just set the retrieved key as EndKey for the next scan.
			t.spans[t.currentSpan].EndKey = key
		}

		// If the changes we made turned our current span invalid, mark that
		// we should move on to the next span before returning the row.
		if !t.spans[t.currentSpan].Valid() {
			t.currentSpan++
		}

		if outRow := t.ProcessRowHelper(row); outRow != nil {
			return outRow, nil
		}
	}
	return nil, t.DrainHelper()
}

func (t *indexSkipTableReader) Release() {
	t.ProcessorBase.Reset()
	t.fetcher.Reset()
	*t = indexSkipTableReader{
		ProcessorBase:    t.ProcessorBase,
		fetcher:          t.fetcher,
		spans:            t.spans[:0],
		misplannedRanges: t.misplannedRanges[:0],
		currentSpan:      0,
	}
	istrPool.Put(t)
}

func (t *indexSkipTableReader) ConsumerClosed() {
	t.InternalClose()
}

func (t *indexSkipTableReader) generateTrailingMeta(
	ctx context.Context,
) []distsqlpb.ProducerMetadata {
	trailingMeta := t.generateMeta(ctx)
	t.InternalClose()
	return trailingMeta
}

func (t *indexSkipTableReader) generateMeta(ctx context.Context) []distsqlpb.ProducerMetadata {
	var trailingMeta []distsqlpb.ProducerMetadata
	if !t.ignoreMisplannedRanges {
		if len(t.misplannedRanges) != 0 {
			trailingMeta = append(trailingMeta, distsqlpb.ProducerMetadata{Ranges: t.misplannedRanges})
		}
	}
	if meta := runbase.GetTxnCoordMeta(ctx, t.FlowCtx.Txn); meta != nil {
		trailingMeta = append(trailingMeta, distsqlpb.ProducerMetadata{TxnCoordMeta: meta})
	}
	return trailingMeta
}

func (t *indexSkipTableReader) DrainMeta(ctx context.Context) []distsqlpb.ProducerMetadata {
	return t.generateMeta(ctx)
}

//GetBatch is part of the RowSource interface.
func (t *indexSkipTableReader) GetBatch(
	ctx context.Context, respons chan roachpb.BlockStruct,
) context.Context {
	return ctx
}

//SetChan is part of the RowSource interface.
func (t *indexSkipTableReader) SetChan() {}
