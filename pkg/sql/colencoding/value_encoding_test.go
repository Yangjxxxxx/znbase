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

package colencoding

import (
	"fmt"
	"testing"

	"github.com/znbasedb/znbase/pkg/col/coldata"
	"github.com/znbasedb/znbase/pkg/sql/sem"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types1"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/encoding"
	"github.com/znbasedb/znbase/pkg/util/randutil"
)

func TestDecodeTableValueToCol(t *testing.T) {
	rng, _ := randutil.NewPseudoRand()
	var buf []byte
	var scratch []byte
	nCols := 1000
	datums := make([]tree.Datum, nCols)
	colTyps := make([]sqlbase.ColumnType, nCols)
	typs := make([]types1.T, nCols)
	for i := 0; i < nCols; i++ {
		ct := sqlbase.RandColumnType(rng)
		et := sqlbase.FromColumnType(ct)
		if et.Family() == types1.Unknown.Family() {
			i--
			continue
		}
		datum := sqlbase.RandDatum(rng, ct, false /* nullOk */)
		colTyps[i] = ct
		typs[i] = et
		datums[i] = datum
		var err error
		fmt.Println(datum)
		buf, err = sqlbase.EncodeTableValue(buf, sqlbase.ColumnID(encoding.NoColumnID), datum, scratch)
		if err != nil {
			t.Fatal(err)
		}
	}
	coltyps, err := sem.FromColumnTypes(typs)
	if err != nil {
		t.Fatal(err)
	}
	batch := coldata.NewMemBatchWithSize(coltyps, 1)
	for i := 0; i < nCols; i++ {
		typeOffset, dataOffset, _, typ, err := encoding.DecodeValueTag(buf)
		fmt.Println(typ)
		if err != nil {
			t.Fatal(err)
		}
		buf, err = DecodeTableValueToCol(batch.ColVec(i), 0 /* rowIdx */, typ,
			dataOffset, &colTyps[i], buf[typeOffset:])
		if err != nil {
			t.Fatal(err)
		}

		// TODO(jordan): should actually compare the outputs as well, but this is a
		// decent enough smoke test.
	}

	if len(buf) != 0 {
		t.Fatalf("leftover bytes %s", buf)
	}
}
