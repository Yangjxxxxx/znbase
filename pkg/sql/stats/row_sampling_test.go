// Copyright 2017 The Cockroach Authors.
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

package stats

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/randutil"
)

// runSampleTest feeds rows with the given ranks through a reservoir
// of a given size and verifies the results are correct.
func runSampleTest(t *testing.T, evalCtx *tree.EvalContext, numSamples int, ranks []int) {
	emptyLocale := ""
	typeInt := sqlbase.ColumnType{SemanticType: sqlbase.ColumnType_INT, Width: 64, Locale: &emptyLocale}
	ctx := context.Background()
	var sr SampleReservoir
	sr.Init(numSamples, []sqlbase.ColumnType{typeInt}, nil /* memAcc */, util.MakeFastIntSet(0))
	for _, r := range ranks {
		d := sqlbase.DatumToEncDatum(typeInt, tree.NewDInt(tree.DInt(r)))
		if err := sr.SampleRow(ctx, evalCtx, sqlbase.EncDatumRow{d}, uint64(r)); err != nil {
			t.Errorf("%v", err)
		}
	}
	samples := sr.Get()
	sampledRanks := make([]int, len(samples))

	// Verify that the row and the ranks weren't mishandled.
	for i, s := range samples {
		if *s.Row[0].Datum.(*tree.DInt) != tree.DInt(s.Rank) {
			t.Fatalf(
				"mismatch between row %s and rank %d", s.Row.String([]sqlbase.ColumnType{typeInt}), s.Rank,
			)
		}
		sampledRanks[i] = int(s.Rank)
	}

	// Verify the top ranks made it.
	sort.Ints(sampledRanks)
	expected := append([]int(nil), ranks...)
	sort.Ints(expected)
	if len(expected) > numSamples {
		expected = expected[:numSamples]
	}
	if !reflect.DeepEqual(expected, sampledRanks) {
		t.Errorf("invalid ranks: %v vs %v", sampledRanks, expected)
	}
}

func TestSampleReservoir(t *testing.T) {
	evalCtx := tree.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())
	for _, n := range []int{10, 100, 1000, 10000} {
		rng, _ := randutil.NewPseudoRand()
		ranks := make([]int, n)
		for i := range ranks {
			ranks[i] = rng.Int()
		}
		for _, k := range []int{1, 5, 10, 100} {
			t.Run(fmt.Sprintf("%d/%d", n, k), func(t *testing.T) {
				runSampleTest(t, &evalCtx, k, ranks)
			})
		}
	}
}
