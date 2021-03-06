// Copyright 2018 The Cockroach Authors.
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
	"fmt"
	"testing"

	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/distsqlpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

func BenchmarkNoop(b *testing.B) {
	const numRows = 1 << 16

	ctx := context.Background()
	st := cluster.MakeTestingClusterSettings()
	evalCtx := tree.MakeTestingEvalContext(st)
	defer evalCtx.Stop(ctx)

	flowCtx := &runbase.FlowCtx{
		Cfg:     &runbase.ServerConfig{Settings: st},
		EvalCtx: &evalCtx,
	}
	post := &distsqlpb.PostProcessSpec{}
	disposer := &runbase.RowDisposer{}
	for _, numCols := range []int{1, 1 << 1, 1 << 2, 1 << 4, 1 << 8} {
		b.Run(fmt.Sprintf("cols=%d", numCols), func(b *testing.B) {
			cols := make([]sqlbase.ColumnType, numCols)
			for i := range cols {
				cols[i] = sqlbase.IntType
			}
			input := runbase.NewRepeatableRowSource(sqlbase.ColumnTypesToDatumTypes(cols), sqlbase.MakeIntRows(numRows, numCols))

			b.SetBytes(int64(8 * numRows * numCols))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d, err := newNoopProcessor(flowCtx, 0 /* processorID */, input, post, disposer)
				if err != nil {
					b.Fatal(err)
				}
				d.Run(context.Background())
				input.Reset()
			}
		})
	}
}
