// Copyright 2019  The Cockroach Authors.

package vecexec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/col/coltypes"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestBufferOp(t *testing.T) {
	defer leaktest.AfterTest(t)()

	ctx := context.Background()
	inputTuples := tuples{{int64(1)}, {int64(2)}, {int64(3)}}
	input := newOpTestInput(coldata.BatchSize(), inputTuples, []coltypes.T{coltypes.Int64})
	buffer := NewBufferOp(input).(*bufferOp)
	buffer.Init()

	t.Run("TestBufferReturnsInputCorrectly", func(t *testing.T) {
		buffer.advance(ctx)
		b := buffer.Next(ctx)
		require.Nil(t, b.Selection())
		require.Equal(t, uint16(len(inputTuples)), b.Length())
		for i, val := range inputTuples {
			require.Equal(t, val[0], b.ColVec(0).Int64()[i])
		}

		// We've read over the batch, so we now should get a zero-length batch.
		b = buffer.Next(ctx)
		require.Nil(t, b.Selection())
		require.Equal(t, uint16(0), b.Length())
	})

	t.Run("TestBufferRestoresOriginalBatch", func(t *testing.T) {
		buffer.rewind()
		b := buffer.Next(ctx)
		b.SetSelection(true)
		sel := b.Selection()
		sel[0] = 1
		b.SetLength(1)

		// We have modified the selection batch, but rewinding the buffer should
		// restore the returned batch to the original state.
		buffer.rewind()
		b = buffer.Next(ctx)
		require.Nil(t, b.Selection())
		require.Equal(t, uint16(len(inputTuples)), b.Length())
		b = buffer.Next(ctx)
		require.Nil(t, b.Selection())
		require.Equal(t, uint16(0), b.Length())
	})
}
