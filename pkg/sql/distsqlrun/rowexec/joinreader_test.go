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
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/distsqlpb"
	"github.com/znbasedb/znbase/pkg/sql/distsqlrun/runbase"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/mon"
	"github.com/znbasedb/znbase/pkg/util/tracing"
)

func TestJoinReader(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	s, sqlDB, kvDB := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)

	// Create a table where each row is:
	//
	//  |     a    |     b    |         sum         |         s           |
	//  |-----------------------------------------------------------------|
	//  | rowId/10 | rowId%10 | rowId/10 + rowId%10 | IntToEnglish(rowId) |

	aFn := func(row int) tree.Datum {
		return tree.NewDInt(tree.DInt(row / 10))
	}
	bFn := func(row int) tree.Datum {
		return tree.NewDInt(tree.DInt(row % 10))
	}
	sumFn := func(row int) tree.Datum {
		return tree.NewDInt(tree.DInt(row/10 + row%10))
	}

	sqlutils.CreateTable(t, sqlDB, "t",
		"a INT, b INT, sum INT, s STRING, PRIMARY KEY (a,b), INDEX bs (b,s)",
		99,
		sqlutils.ToRowFn(aFn, bFn, sumFn, sqlutils.RowEnglishFn))

	// Insert a row for NULL testing.
	if _, err := sqlDB.Exec("INSERT INTO test.t VALUES (10, 0, NULL, NULL)"); err != nil {
		t.Fatal(err)
	}

	tdSecondary := sqlbase.GetTableDescriptor(kvDB, "test", "public", "t")

	sqlutils.CreateTable(t, sqlDB, "t2",
		"a INT, b INT, sum INT, s STRING, PRIMARY KEY (a,b), FAMILY f1 (a, b), FAMILY f2 (s), FAMILY f3 (sum), INDEX bs (b,s)",
		99,
		sqlutils.ToRowFn(aFn, bFn, sumFn, sqlutils.RowEnglishFn))

	tdFamily := sqlbase.GetTableDescriptor(kvDB, "test", "public", "t2")

	sqlutils.CreateTable(t, sqlDB, "t3parent",
		"a INT PRIMARY KEY",
		0,
		sqlutils.ToRowFn(aFn))

	sqlutils.CreateTableInterleaved(t, sqlDB, "t3",
		"a INT, b INT, sum INT, s STRING, PRIMARY KEY (a,b), INDEX bs (b,s)",
		"t3parent(a)",
		99,
		sqlutils.ToRowFn(aFn, bFn, sumFn, sqlutils.RowEnglishFn))
	tdInterleaved := sqlbase.GetTableDescriptor(kvDB, "test", "public", "t3")

	testCases := []struct {
		description string
		indexIdx    uint32
		post        distsqlpb.PostProcessSpec
		onExpr      string
		input       [][]tree.Datum
		lookupCols  []uint32
		joinType    sqlbase.JoinType
		inputTypes  []sqlbase.ColumnType
		outputTypes []sqlbase.ColumnType
		expected    string
	}{
		{
			description: "Test selecting columns from second table",
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1, 4},
			},
			input: [][]tree.Datum{
				{aFn(2), bFn(2)},
				{aFn(5), bFn(5)},
				{aFn(10), bFn(10)},
				{aFn(15), bFn(15)},
			},
			lookupCols:  []uint32{0, 1},
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.ThreeIntCols,
			expected:    "[[0 2 2] [0 5 5] [1 0 1] [1 5 6]]",
		},
		{
			description: "Test duplicates in the input of lookup joins",
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1, 3},
			},
			input: [][]tree.Datum{
				{aFn(2), bFn(2)},
				{aFn(2), bFn(2)},
				{aFn(5), bFn(5)},
				{aFn(10), bFn(10)},
				{aFn(15), bFn(15)},
			},
			lookupCols:  []uint32{0, 1},
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.ThreeIntCols,
			expected:    "[[0 2 2] [0 2 2] [0 5 5] [1 0 0] [1 5 5]]",
		},
		{
			description: "Test lookup join queries with separate families",
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1, 3, 4},
			},
			input: [][]tree.Datum{
				{aFn(2), bFn(2)},
				{aFn(5), bFn(5)},
				{aFn(10), bFn(10)},
				{aFn(15), bFn(15)},
			},
			lookupCols:  []uint32{0, 1},
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.FourIntCols,
			expected:    "[[0 2 2 2] [0 5 5 5] [1 0 0 1] [1 5 5 6]]",
		},
		{
			description: "Test lookup joins preserve order of left input",
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1, 3},
			},
			input: [][]tree.Datum{
				{aFn(2), bFn(2)},
				{aFn(5), bFn(5)},
				{aFn(2), bFn(2)},
				{aFn(10), bFn(10)},
				{aFn(15), bFn(15)},
			},
			lookupCols:  []uint32{0, 1},
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.ThreeIntCols,
			expected:    "[[0 2 2] [0 5 5] [0 2 2] [1 0 0] [1 5 5]]",
		},
		{
			description: "Test lookup join with onExpr",
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1, 4},
			},
			input: [][]tree.Datum{
				{aFn(2), bFn(2)},
				{aFn(5), bFn(5)},
				{aFn(10), bFn(10)},
				{aFn(15), bFn(15)},
			},
			lookupCols:  []uint32{0, 1},
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.ThreeIntCols,
			onExpr:      "@2 < @5",
			expected:    "[[1 0 1] [1 5 6]]",
		},
		{
			description: "Test left outer lookup join on primary index",
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1, 4},
			},
			input: [][]tree.Datum{
				{aFn(100), bFn(100)},
				{aFn(2), bFn(2)},
			},
			lookupCols:  []uint32{0, 1},
			joinType:    sqlbase.LeftOuterJoin,
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.ThreeIntCols,
			expected:    "[[10 0 NULL] [0 2 2]]",
		},
		{
			description: "Test lookup join on secondary index with NULL lookup value",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(0), tree.DNull},
			},
			lookupCols:  []uint32{0, 1},
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.OneIntCol,
			expected:    "[]",
		},
		{
			description: "Test left outer lookup join on secondary index with NULL lookup value",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 2},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(0), tree.DNull},
			},
			lookupCols:  []uint32{0, 1},
			joinType:    sqlbase.LeftOuterJoin,
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.TwoIntCols,
			expected:    "[[0 NULL]]",
		},
		{
			description: "Test lookup join on secondary index with an implicit key column",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{2},
			},
			input: [][]tree.Datum{
				{aFn(2), bFn(2), sqlutils.RowEnglishFn(2)},
			},
			lookupCols:  []uint32{1, 2, 0},
			inputTypes:  []sqlbase.ColumnType{sqlbase.IntType, sqlbase.IntType, sqlbase.StrType},
			outputTypes: sqlbase.OneIntCol,
			expected:    "[['two']]",
		},
		{
			description: "Test left semi lookup join",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(tree.DInt(1)), sqlutils.RowEnglishFn(2)},
				{tree.NewDInt(tree.DInt(1)), sqlutils.RowEnglishFn(2)},
				{tree.NewDInt(tree.DInt(1234)), sqlutils.RowEnglishFn(2)},
				{tree.NewDInt(tree.DInt(6)), sqlutils.RowEnglishFn(2)},
				{tree.NewDInt(tree.DInt(7)), sqlutils.RowEnglishFn(2)},
				{tree.NewDInt(tree.DInt(1)), sqlutils.RowEnglishFn(2)},
			},
			lookupCols:  []uint32{0},
			joinType:    sqlbase.LeftSemiJoin,
			inputTypes:  []sqlbase.ColumnType{sqlbase.IntType, sqlbase.StrType},
			outputTypes: sqlbase.TwoIntCols,
			expected:    "[[1 'two'] [1 'two'] [6 'two'] [7 'two'] [1 'two']]",
		},
		{
			description: "Test left semi lookup join on secondary index with NULL lookup value",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(0), tree.DNull},
			},
			lookupCols:  []uint32{0, 1},
			joinType:    sqlbase.LeftSemiJoin,
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.OneIntCol,
			expected:    "[]",
		},
		{
			description: "Test left semi lookup join with onExpr",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(tree.DInt(1)), bFn(3)},
				{tree.NewDInt(tree.DInt(1)), bFn(2)},
				{tree.NewDInt(tree.DInt(1234)), bFn(2)},
				{tree.NewDInt(tree.DInt(6)), bFn(2)},
				{tree.NewDInt(tree.DInt(7)), bFn(3)},
				{tree.NewDInt(tree.DInt(1)), bFn(2)},
			},
			lookupCols:  []uint32{0},
			joinType:    sqlbase.LeftSemiJoin,
			onExpr:      "@2 > 2",
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.TwoIntCols,
			expected:    "[[1 3] [7 3]]",
		},
		{
			description: "Test left anti lookup join",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(tree.DInt(1234)), tree.NewDInt(tree.DInt(1234))},
			},
			lookupCols:  []uint32{0},
			joinType:    sqlbase.LeftAntiJoin,
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.TwoIntCols,
			expected:    "[[1234 1234]]",
		},
		{
			description: "Test left anti lookup join with onExpr",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(tree.DInt(1)), bFn(3)},
				{tree.NewDInt(tree.DInt(1)), bFn(2)},
				{tree.NewDInt(tree.DInt(6)), bFn(2)},
				{tree.NewDInt(tree.DInt(7)), bFn(3)},
				{tree.NewDInt(tree.DInt(1)), bFn(2)},
			},
			lookupCols:  []uint32{0},
			joinType:    sqlbase.LeftAntiJoin,
			onExpr:      "@2 > 2",
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.TwoIntCols,
			expected:    "[[1 2] [6 2] [1 2]]",
		},
		{
			description: "Test left anti lookup join with match",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1},
			},
			input: [][]tree.Datum{
				{aFn(10), tree.NewDInt(tree.DInt(1234))},
			},
			lookupCols:  []uint32{0},
			joinType:    sqlbase.LeftAntiJoin,
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.OneIntCol,
			expected:    "[]",
		},
		{
			description: "Test left anti lookup join on secondary index with NULL lookup value",
			indexIdx:    1,
			post: distsqlpb.PostProcessSpec{
				Projection:    true,
				OutputColumns: []uint32{0, 1},
			},
			input: [][]tree.Datum{
				{tree.NewDInt(0), tree.DNull},
			},
			lookupCols:  []uint32{0, 1},
			joinType:    sqlbase.LeftAntiJoin,
			inputTypes:  sqlbase.TwoIntCols,
			outputTypes: sqlbase.TwoIntCols,
			expected:    "[[0 NULL]]",
		},
	}
	st := cluster.MakeTestingClusterSettings()
	tempEngine, err := engine.NewTempEngine(ctx, engine.DefaultStorageEngine, base.DefaultTestTempStorageConfig(st), base.DefaultTestStoreSpec)
	if err != nil {
		t.Fatal(err)
	}
	defer tempEngine.Close()
	diskMonitor := mon.MakeMonitor(
		"test-disk",
		mon.DiskResource,
		nil, /* curCount */
		nil, /* maxHist */
		-1,  /* increment: use default block size */
		math.MaxInt64,
		st,
	)
	diskMonitor.Start(ctx, nil /* pool */, mon.MakeStandaloneBudget(math.MaxInt64))
	defer diskMonitor.Stop(ctx)
	for i, td := range []*sqlbase.TableDescriptor{tdSecondary, tdFamily, tdInterleaved} {
		for _, c := range testCases {
			for _, reqOrdering := range []bool{true, false} {
				t.Run(fmt.Sprintf("%d/reqOrdering=%t/%s", i, reqOrdering, c.description), func(t *testing.T) {
					evalCtx := tree.MakeTestingEvalContext(st)
					defer evalCtx.Stop(ctx)
					flowCtx := runbase.FlowCtx{
						EvalCtx: &evalCtx,
						Cfg: &runbase.ServerConfig{
							Settings:    st,
							TempStorage: tempEngine,
							DiskMonitor: &diskMonitor,
						},
						Txn: client.NewTxn(ctx, s.DB(), s.NodeID(), client.RootTxn),
					}
					encRows := make(sqlbase.EncDatumRows, len(c.input))
					for rowIdx, row := range c.input {
						encRow := make(sqlbase.EncDatumRow, len(row))
						for i, d := range row {
							encRow[i] = sqlbase.DatumToEncDatum(c.inputTypes[i], d)
						}
						encRows[rowIdx] = encRow
					}
					in := runbase.NewRowBuffer(c.inputTypes, encRows, runbase.RowBufferArgs{})

					out := &runbase.RowBuffer{}
					jr, err := newJoinReader(
						&flowCtx,
						0, /* processorID */
						&distsqlpb.JoinReaderSpec{
							Table:            *td,
							IndexIdx:         c.indexIdx,
							LookupColumns:    c.lookupCols,
							OnExpr:           distsqlpb.Expression{Expr: c.onExpr},
							Type:             c.joinType,
							MaintainOrdering: reqOrdering,
						},
						in,
						&c.post,
						out,
						lookupJoinReaderType,
					)
					if err != nil {
						t.Fatal(err)
					}

					// Set a lower batch size to force multiple batches.
					jr.(*joinReader).SetBatchSizeBytes(int64(encRows[0].Size() * 3))

					jr.Run(ctx)

					if !in.Done {
						t.Fatal("joinReader didn't consume all the rows")
					}
					if !out.ProducerClosed() {
						t.Fatalf("output RowReceiver not closed")
					}

					var res sqlbase.EncDatumRows
					for {
						row := out.NextNoMeta(t)
						if row == nil {
							break
						}
						res = append(res, row)
					}

					// processOutputRows is a helper function that takes a stringified
					// EncDatumRows output (e.g. [[1 2] [3 1]]) and returns a slice of
					// stringified rows without brackets (e.g. []string{"1 2", "3 1"}).
					processOutputRows := func(output string) []string {
						// Comma-separate the rows.
						output = strings.ReplaceAll(output, "] [", ",")
						// Remove leading and trailing bracket.
						output = strings.Trim(output, "[]")
						// Split on the commas that were introduced and return that.
						return strings.Split(output, ",")
					}

					result := processOutputRows(res.String(c.outputTypes))
					expected := processOutputRows(c.expected)

					if !reqOrdering {
						// An ordering was not required, so sort both the result and
						// expected slice to reuse equality comparison.
						sort.Strings(result)
						sort.Strings(expected)
					}

					require.Equal(t, expected, result)
				})
			}
		}
	}
}

// TestJoinReaderDrain tests various scenarios in which a joinReader's consumer
// is closed.
func TestJoinReaderDrain(t *testing.T) {
	defer leaktest.AfterTest(t)()

	s, sqlDB, kvDB := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(context.TODO())

	sqlutils.CreateTable(
		t,
		sqlDB,
		"t",
		"a INT, PRIMARY KEY (a)",
		1, /* numRows */
		sqlutils.ToRowFn(sqlutils.RowIdxFn),
	)
	td := sqlbase.GetTableDescriptor(kvDB, "test", "public", "t")

	evalCtx := tree.MakeTestingEvalContext(s.ClusterSettings())
	defer evalCtx.Stop(context.Background())

	// Run the flow in a snowball trace so that we can test for tracing info.
	tracer := tracing.NewTracer()
	ctx, sp, err := tracing.StartSnowballTrace(context.Background(), tracer, "test flow ctx")
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Finish()

	flowCtx := runbase.FlowCtx{
		EvalCtx: &evalCtx,
		Cfg:     &runbase.ServerConfig{Settings: s.ClusterSettings()},
		Txn:     client.NewTxn(ctx, s.DB(), s.NodeID(), client.LeafTxn),
	}

	encRow := make(sqlbase.EncDatumRow, 1)
	encRow[0] = sqlbase.DatumToEncDatum(sqlbase.IntType, tree.NewDInt(1))

	// ConsumerClosed verifies that when a joinReader's consumer is closed, the
	// joinReader finishes gracefully.
	t.Run("ConsumerClosed", func(t *testing.T) {
		in := runbase.NewRowBuffer(sqlbase.OneIntCol, sqlbase.EncDatumRows{encRow}, runbase.RowBufferArgs{})

		out := &runbase.RowBuffer{}
		out.ConsumerClosed()
		jr, err := newJoinReader(
			&flowCtx, 0 /* processorID */, &distsqlpb.JoinReaderSpec{Table: *td}, in, &distsqlpb.PostProcessSpec{}, out, lookupJoinReaderType,
		)
		if err != nil {
			t.Fatal(err)
		}
		jr.Run(ctx)
	})

	// ConsumerDone verifies that the producer drains properly by checking that
	// metadata coming from the producer is still read when ConsumerDone is
	// called on the consumer.
	t.Run("ConsumerDone", func(t *testing.T) {
		expectedMetaErr := errors.New("dummy")
		in := runbase.NewRowBuffer(sqlbase.OneIntCol, nil /* rows */, runbase.RowBufferArgs{})
		if status := in.Push(encRow, &distsqlpb.ProducerMetadata{Err: expectedMetaErr}); status != runbase.NeedMoreRows {
			t.Fatalf("unexpected response: %d", status)
		}

		out := &runbase.RowBuffer{}
		out.ConsumerDone()
		jr, err := newJoinReader(
			&flowCtx, 0 /* processorID */, &distsqlpb.JoinReaderSpec{Table: *td}, in, &distsqlpb.PostProcessSpec{}, out, lookupJoinReaderType,
		)
		if err != nil {
			t.Fatal(err)
		}
		jr.Run(ctx)
		row, meta := out.Next()
		if row != nil {
			t.Fatalf("row was pushed unexpectedly: %s", row.String(sqlbase.OneIntCol))
		}
		if meta.Err != expectedMetaErr {
			t.Fatalf("unexpected error in metadata: %v", meta.Err)
		}

		// Check for trailing metadata.
		var traceSeen, txnCoordMetaSeen bool
		for {
			row, meta = out.Next()
			if row != nil {
				t.Fatalf("row was pushed unexpectedly: %s", row.String(sqlbase.OneIntCol))
			}
			if meta == nil {
				break
			}
			if meta.TraceData != nil {
				traceSeen = true
			}
			if meta.TxnCoordMeta != nil {
				txnCoordMetaSeen = true
			}
		}
		if !traceSeen {
			t.Fatal("missing tracing trailing metadata")
		}
		if !txnCoordMetaSeen {
			t.Fatal("missing txn trailing metadata")
		}
	})
}

// BenchmarkJoinReader benchmarks an index join where there is a 1:1
// relationship between the two sides.
func BenchmarkJoinReader(b *testing.B) {
	logScope := log.Scope(b)
	defer logScope.Close(b)
	ctx := context.Background()

	s, sqlDB, kvDB := serverutils.StartServer(b, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)

	evalCtx := tree.MakeTestingEvalContext(s.ClusterSettings())
	defer evalCtx.Stop(ctx)

	flowCtx := runbase.FlowCtx{
		EvalCtx: &evalCtx,
		Cfg:     &runbase.ServerConfig{Settings: s.ClusterSettings()},
		Txn:     client.NewTxn(ctx, s.DB(), s.NodeID(), client.RootTxn),
	}

	const numCols = 2
	const numInputCols = 1
	for _, numRows := range []int{1 << 4, 1 << 8, 1 << 12, 1 << 16} {
		tableName := fmt.Sprintf("t%d", numRows)
		sqlutils.CreateTable(
			b, sqlDB, tableName, "k INT PRIMARY KEY, v INT", numRows,
			sqlutils.ToRowFn(sqlutils.RowIdxFn, sqlutils.RowIdxFn),
		)
		tableDesc := sqlbase.GetTableDescriptor(kvDB, "test", "public", tableName)

		spec := distsqlpb.JoinReaderSpec{Table: *tableDesc}
		input := runbase.NewRepeatableRowSource(sqlbase.ColumnTypesToDatumTypes(sqlbase.OneIntCol), sqlbase.MakeIntRows(numRows, numInputCols))
		post := distsqlpb.PostProcessSpec{}
		output := runbase.RowDisposer{}

		b.Run(fmt.Sprintf("rows=%d", numRows), func(b *testing.B) {
			b.SetBytes(int64(numRows * (numCols + numInputCols) * 8))
			for i := 0; i < b.N; i++ {
				jr, err := newJoinReader(&flowCtx, 0 /* processorID */, &spec, input, &post, &output, lookupJoinReaderType)
				if err != nil {
					b.Fatal(err)
				}
				jr.Run(ctx)
				input.Reset()
			}
		})
	}
}
