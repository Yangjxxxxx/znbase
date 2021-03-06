// Copyright 2018  The Cockroach Authors.
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

package workload_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/workload"
	"github.com/znbasedb/znbase/pkg/workload/bank"
	"github.com/znbasedb/znbase/pkg/workload/tpcc"
)

func TestHandleCSV(t *testing.T) {
	defer leaktest.AfterTest(t)()

	tests := []struct {
		params, expected string
	}{
		{
			`?rows=1`, `
0,0,initial-dTqnRurXztAPkykhZWvsCmeJkMwRNcJAvTlNbgUEYfagEQJaHmfPsquKZUBOGwpAjPtATpGXFJkrtQCEJODSlmQctvyh`,
		},
		{
			`?rows=5&row-start=1&row-end=3`, `
1,0,initial-vOpikzTTWxvMqnkpfEIVXgGyhZNDqvpVqpNnHawruAcIVltgbnIEIGmCDJcnkVkfVmAcutkMvRACFuUBPsZTemTDSfZT
2,0,initial-qMvoPeRiOBXvdVQxhZUfdmehETKPXyBaVWxzMqwiStIkxfoDFygYxIDyXiaVEarcwMboFhBlCAapvKijKAyjEAhRBNZz`,
		},
	}

	meta := bank.FromRows(0).Meta()
	for _, test := range tests {
		t.Run(test.params, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := workload.HandleCSV(w, r, `/bank/`, meta); err != nil {
					panic(err)
				}
			}))
			defer ts.Close()

			res, err := http.Get(ts.URL + `/bank/bank` + test.params)
			if err != nil {
				t.Fatal(err)
			}
			data, err := ioutil.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}

			if d, e := strings.TrimSpace(string(data)), strings.TrimSpace(test.expected); d != e {
				t.Errorf("got [\n%s\n] expected [\n%s\n]", d, e)
			}
		})
	}
}

func BenchmarkWriteCSVRows(b *testing.B) {
	ctx := context.Background()

	var rows [][][]interface{}
	for _, table := range tpcc.FromWarehouses(1).Tables() {
		rows = append(rows, table.InitialRows.Batch(0))
	}
	table := workload.Table{
		InitialRows: workload.BatchedTuples{
			Batch: func(rowIdx int) [][]interface{} { return rows[rowIdx] },
		},
	}

	var buf bytes.Buffer
	fn := func() {
		const limit = -1
		if _, err := workload.WriteCSVRows(ctx, &buf, table, 0, len(rows), limit); err != nil {
			b.Fatalf(`%+v`, err)
		}
	}

	// Run fn once to pre-size buf.
	fn()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		fn()
	}
	b.StopTimer()
	b.SetBytes(int64(buf.Len()))
}

func TestCSVRowsReader(t *testing.T) {
	defer leaktest.AfterTest(t)()

	table := bank.FromRows(10).Tables()[0]
	r := workload.NewCSVRowsReader(table, 1, 3)
	b, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	expected := `
1,0,initial-vOpikzTTWxvMqnkpfEIVXgGyhZNDqvpVqpNnHawruAcIVltgbnIEIGmCDJcnkVkfVmAcutkMvRACFuUBPsZTemTDSfZT
2,0,initial-qMvoPeRiOBXvdVQxhZUfdmehETKPXyBaVWxzMqwiStIkxfoDFygYxIDyXiaVEarcwMboFhBlCAapvKijKAyjEAhRBNZz
`
	require.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(b)))
}

func BenchmarkCSVRowsReader(b *testing.B) {
	var rows [][][]interface{}
	for _, table := range tpcc.FromWarehouses(1).Tables() {
		rows = append(rows, table.InitialRows.Batch(0))
	}
	table := workload.Table{
		InitialRows: workload.BatchedTuples{
			Batch:      func(rowIdx int) [][]interface{} { return rows[rowIdx] },
			NumBatches: len(rows),
		},
	}

	var buf bytes.Buffer
	fn := func() {
		r := workload.NewCSVRowsReader(table, 0, 0)
		_, err := io.Copy(&buf, r)
		require.NoError(b, err)
	}

	// Run fn once to pre-size buf.
	fn()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		fn()
	}
	b.StopTimer()
	b.SetBytes(int64(buf.Len()))
}
