// Copyright 2015  The Cockroach Authors.
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

package sql

import (
	"context"
	"reflect"
	"testing"

	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/encoding"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestDesiredAggregateOrder(t *testing.T) {
	defer leaktest.AfterTest(t)()

	testData := []struct {
		expr     string
		ordering sqlbase.ColumnOrdering
	}{
		{`min(a)`, sqlbase.ColumnOrdering{{ColIdx: 0, Direction: encoding.Ascending}}},
		{`max(a)`, sqlbase.ColumnOrdering{{ColIdx: 0, Direction: encoding.Descending}}},
		{`min(a+1)`, sqlbase.ColumnOrdering{{ColIdx: 0, Direction: encoding.Ascending}}},
		{`(min(a), max(a))`, nil},
		{`(min(a), avg(a))`, nil},
		{`(min(a), count(a))`, nil},
		{`(min(a), sum(a))`, nil},
		{`(min(a), min(a))`, sqlbase.ColumnOrdering{{ColIdx: 0, Direction: encoding.Ascending}}},
		{`(min(a+1), min(a))`, nil},
		{`(count(a), min(a))`, nil},
	}
	p := makeTestPlanner()
	for _, d := range testData {
		t.Run(d.expr, func(t *testing.T) {
			p.extendedEvalCtx = makeTestingExtendedEvalContext(cluster.MakeTestingClusterSettings())
			defer p.extendedEvalCtx.Stop(context.Background())
			sel := makeSelectNode(t, p)
			expr := parseAndNormalizeExpr(t, p, d.expr, sel)
			group := &groupNode{}
			render := &renderNode{}
			postRender := &renderNode{}
			postRender.ivarHelper = tree.MakeIndexedVarHelper(postRender, len(group.funcs))
			v := extractAggregatesVisitor{
				ctx:        context.TODO(),
				groupNode:  group,
				preRender:  render,
				ivarHelper: &postRender.ivarHelper,
				planner:    p,
			}
			if _, err := v.extract(expr); err != nil {
				t.Fatal(err)
			}
			ordering := group.desiredAggregateOrdering(p.EvalContext())
			if !reflect.DeepEqual(d.ordering, ordering) {
				t.Fatalf("%s: expected %v, but found %v", d.expr, d.ordering, ordering)
			}
			// Verify we never have a desired ordering if there is a GROUP BY.
			group.groupCols = []int{0}
			ordering = group.desiredAggregateOrdering(p.EvalContext())
			if len(ordering) > 0 {
				t.Fatalf("%s: expected no ordering when there is a GROUP BY, found %v", d.expr, ordering)
			}
		})
	}
}
