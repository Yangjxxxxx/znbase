// Copyright 2016 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package rowexec

import (
	"context"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/sql/distsqlpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/mon"
)

// streamGroupAccumulator groups input rows coming from src into groups dictated
// by equality according to the ordering columns.
type streamGroupAccumulator struct {
	src   runbase.RowSource
	types []sqlbase.ColumnType

	// srcConsumed is set once src has been exhausted.
	srcConsumed bool
	ordering    sqlbase.ColumnOrdering

	// curGroup maintains the rows accumulated in the current group.
	curGroup   []sqlbase.EncDatumRow
	datumAlloc sqlbase.DatumAlloc

	// leftoverRow is the first row of the next group. It's saved in the
	// accumulator after the current group is returned, so the accumulator can
	// resume later.
	leftoverRow sqlbase.EncDatumRow

	rowAlloc sqlbase.EncDatumRowAlloc

	memAcc mon.BoundAccount

	meta *distsqlpb.ProducerMetadata
}

func makeStreamGroupAccumulator(
	src runbase.RowSource, ordering sqlbase.ColumnOrdering, memMonitor *mon.BytesMonitor,
) streamGroupAccumulator {
	return streamGroupAccumulator{
		src:      src,
		types:    src.OutputTypes(),
		ordering: ordering,
		memAcc:   memMonitor.MakeBoundAccount(),
	}
}

func (s *streamGroupAccumulator) start(ctx context.Context) {
	s.src.Start(ctx)
}

// nextGroup returns the next group from the inputs. The returned slice is not safe
// to use after the next call to nextGroup.
func (s *streamGroupAccumulator) nextGroup(
	ctx context.Context, evalCtx *tree.EvalContext,
) ([]sqlbase.EncDatumRow, *distsqlpb.ProducerMetadata) {
	if s.srcConsumed {
		// If src has been exhausted, then we also must have advanced away from the
		// last group.
		return nil, nil
	}

	if s.leftoverRow != nil {
		s.curGroup = append(s.curGroup, s.leftoverRow)
		s.leftoverRow = nil
	}

	for {
		row, meta := s.src.Next()
		if meta != nil {
			return nil, meta
		}
		if row == nil {
			s.srcConsumed = true
			return s.curGroup, nil
		}

		if err := s.memAcc.Grow(ctx, int64(row.Size())); err != nil {
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}
		row = s.rowAlloc.CopyRow(row)

		if len(s.curGroup) == 0 {
			if s.curGroup == nil {
				s.curGroup = make([]sqlbase.EncDatumRow, 0, 64)
			}
			s.curGroup = append(s.curGroup, row)
			continue
		}

		cmp, err := s.curGroup[0].Compare(s.types, &s.datumAlloc, s.ordering, evalCtx, row)
		if err != nil {
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}
		if cmp == 0 {
			s.curGroup = append(s.curGroup, row)
		} else if cmp == 1 {
			return nil, &distsqlpb.ProducerMetadata{
				Err: errors.Errorf(
					"detected badly ordered input: %s > %s, but expected '<'",
					s.curGroup[0].String(s.types), row.String(s.types)),
			}
		} else {
			n := len(s.curGroup)
			ret := s.curGroup[:n:n]
			s.curGroup = s.curGroup[:0]
			s.memAcc.Empty(ctx)
			s.leftoverRow = row
			return ret, nil
		}
	}
}

// nextGroup returns the next group from the inputs. The returned slice is not safe
// to use after the next call to nextGroup.
func (s *streamGroupAccumulator) nextGroup1(
	ctx context.Context, evalCtx *tree.EvalContext,
) ([]sqlbase.EncDatumRow, *distsqlpb.ProducerMetadata) {
	if s.srcConsumed {
		// If src has been exhausted, then we also must have advanced away from the
		// last group.
		return nil, nil
	}

	if s.leftoverRow != nil {
		s.curGroup = append(s.curGroup, s.leftoverRow)
		s.leftoverRow = nil
	}

	for {
		var row sqlbase.EncDatumRow

		if s.meta != nil {
			return nil, s.meta
		}

		row, s.meta = s.src.Next()

		if row == nil {
			s.srcConsumed = true
			return s.curGroup, nil
		}

		if err := s.memAcc.Grow(ctx, int64(row.Size())); err != nil {
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}
		row = s.rowAlloc.CopyRow(row)

		if len(s.curGroup) == 0 {
			if s.curGroup == nil {
				s.curGroup = make([]sqlbase.EncDatumRow, 0, 64)
			}
			s.curGroup = append(s.curGroup, row)
			continue
		}

		cmp, err := s.curGroup[0].Compare(s.types, &s.datumAlloc, s.ordering, evalCtx, row)
		if err != nil {
			return nil, &distsqlpb.ProducerMetadata{Err: err}
		}
		if cmp == 0 {
			s.curGroup = append(s.curGroup, row)
		} else if cmp == 1 {
			return nil, &distsqlpb.ProducerMetadata{
				Err: errors.Errorf(
					"detected badly ordered input: %s > %s, but expected '<'",
					s.curGroup[0].String(s.types), row.String(s.types)),
			}
		} else {
			n := len(s.curGroup)
			ret := s.curGroup[:n:n]
			s.curGroup = s.curGroup[:0]
			s.memAcc.Empty(ctx)
			s.leftoverRow = row
			return ret, nil
		}
	}
}

func (s *streamGroupAccumulator) close(ctx context.Context) {
	s.memAcc.Close(ctx)
}
