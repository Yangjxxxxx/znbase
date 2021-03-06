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

package ordering

import (
	"fmt"
	"testing"

	"github.com/znbasedb/znbase/pkg/sql/opt"
	"github.com/znbasedb/znbase/pkg/sql/opt/cat"
	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/opt/norm"
	"github.com/znbasedb/znbase/pkg/sql/opt/props"
	"github.com/znbasedb/znbase/pkg/sql/opt/props/physical"
	"github.com/znbasedb/znbase/pkg/sql/opt/testutils/testcat"
	"github.com/znbasedb/znbase/pkg/sql/opt/testutils/testexpr"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/util"
)

func TestLookupJoinProvided(t *testing.T) {
	tc := testcat.New()
	if _, err := tc.ExecuteDDL(
		"CREATE TABLE t (c1 INT, c2 INT, c3 INT, c4 INT, PRIMARY KEY(c1, c2))",
	); err != nil {
		t.Fatal(err)
	}
	evalCtx := tree.NewTestingEvalContext(nil /* st */)
	var f norm.Factory
	f.Init(evalCtx)
	md := f.Metadata()
	tab := md.AddTable(tc.Table(tree.NewUnqualifiedTableName("t")))

	if c1 := tab.ColumnID(0); c1 != 1 {
		t.Fatalf("unexpected ID for column c1: %d\n", c1)
	}

	c := func(cols ...int) opt.ColSet {
		return util.MakeFastIntSet(cols...)
	}

	testCases := []struct {
		keyCols  opt.ColList
		outCols  opt.ColSet
		required string
		input    string
		provided string
	}{
		// In these tests, the input (left side of the join) has columns 5,6 and the
		// table (right side) has columns 1,2,3,4 and the join has condition
		// (c5, c6) = (c1, c2).
		//
		{ // case 1: the lookup join adds columns 3,4 from the table and retains the
			// input columns.
			keyCols:  opt.ColList{5, 6},
			outCols:  c(3, 4, 5, 6),
			required: "+5,+6",
			input:    "+5,+6",
			provided: "+5,+6",
		},
		{ // case 2: the lookup join produces all columns. The provided ordering
			// on 5,6 is equivalent to an ordering on 1,2.
			keyCols:  opt.ColList{5, 6},
			outCols:  c(1, 2, 3, 4, 5, 6),
			required: "-1,+2",
			input:    "-5,+6",
			provided: "-5,+6",
		},
		{ // case 3: the lookup join does not produce input columns 5,6; we must
			// remap the input ordering to refer to output columns 1,2 instead.
			keyCols:  opt.ColList{5, 6},
			outCols:  c(1, 2, 3, 4),
			required: "+1,-2",
			input:    "+5,-6",
			provided: "+1,-2",
		},
		{ // case 4: a hybrid of the two cases above (we need to remap column 6).
			keyCols:  opt.ColList{5, 6},
			outCols:  c(1, 2, 3, 4, 5),
			required: "-1,-2",
			input:    "-5,-6",
			provided: "-5,-2",
		},
	}

	for tcIdx, tc := range testCases {
		t.Run(fmt.Sprintf("case%d", tcIdx+1), func(t *testing.T) {
			input := &testexpr.Instance{
				Rel: &props.Relational{},
				Provided: &physical.Provided{
					Ordering: physical.ParseOrdering(tc.input),
				},
			}
			lookupJoin := f.Memo().MemoizeLookupJoin(
				input,
				nil, /* FiltersExpr */
				&memo.LookupJoinPrivate{
					Table:   tab,
					Index:   cat.PrimaryIndex,
					KeyCols: tc.keyCols,
					Cols:    tc.outCols,
				},
			)
			req := physical.ParseOrderingChoice(tc.required)
			res := lookupJoinBuildProvided(lookupJoin, &req).String()
			if res != tc.provided {
				t.Errorf("expected '%s', got '%s'", tc.provided, res)
			}
		})
	}
}
