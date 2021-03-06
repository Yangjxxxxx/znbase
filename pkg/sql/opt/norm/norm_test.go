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

package norm_test

import (
	"testing"

	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/opt"
	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/opt/norm"
	"github.com/znbasedb/znbase/pkg/sql/opt/testutils/opttester"
	"github.com/znbasedb/znbase/pkg/sql/opt/testutils/testcat"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/testutils/datadriven"
)

// TestNormRules tests the various Optgen normalization rules found in the rules
// directory. The tests are data-driven cases of the form:
//   <command>
//   <SQL statement>
//   ----
//   <expected results>
//
// See OptTester.Handle for supported commands.
//
// Rules files can be run separately like this:
//   make test PKG=./pkg/sql/opt/norm TESTS="TestNormRules/bool"
//   make test PKG=./pkg/sql/opt/norm TESTS="TestNormRules/comp"
//   ...
func TestNormRules(t *testing.T) {
	const fmtFlags = memo.ExprFmtHideStats | memo.ExprFmtHideCost | memo.ExprFmtHideRuleProps |
		memo.ExprFmtHideQualifications | memo.ExprFmtHideScalars
	datadriven.Walk(t, "testdata/rules", func(t *testing.T, path string) {
		catalog := testcat.New()
		datadriven.RunTest(t, path, func(d *datadriven.TestData) string {
			tester := opttester.New(catalog, d.Input)
			tester.Flags.ExprFormat = fmtFlags
			return tester.RunCommand(t, d)
		})
	})
}

// Test the FoldNullInEmpty rule. Can't create empty tuple on right side of
// IN/NOT IN in SQL, so do it here.
func TestRuleFoldNullInEmpty(t *testing.T) {
	evalCtx := tree.MakeTestingEvalContext(cluster.MakeTestingClusterSettings())
	var f norm.Factory
	f.Init(&evalCtx)

	in := f.ConstructIn(memo.NullSingleton, memo.EmptyTuple)
	if in.Op() != opt.FalseOp {
		t.Errorf("expected NULL IN () to fold to False")
	}

	notIn := f.ConstructNotIn(memo.NullSingleton, memo.EmptyTuple)
	if notIn.Op() != opt.TrueOp {
		t.Errorf("expected NULL NOT IN () to fold to True")
	}
}

// Ensure that every binary commutative operator overload can have its operands
// switched. Patterns like CommuteConst rely on this being possible.
func TestRuleBinaryAssumption(t *testing.T) {
	fn := func(op opt.Operator) {
		for _, overload := range tree.BinOps[opt.BinaryOpReverseMap[op]] {
			binOp := overload.(*tree.BinOp)
			if op == opt.PlusOp && binOp.LeftType == types.Time && (binOp.RightType == types.Timestamp || binOp.RightType == types.TimestampTZ) {
				continue
			}
			if !memo.BinaryOverloadExists(op, binOp.RightType, binOp.LeftType) {
				t.Errorf("could not find inverse for overload: %+v", op)
			}
		}
	}

	// Only include commutative binary operators.
	fn(opt.PlusOp)
	fn(opt.MultOp)
	fn(opt.BitandOp)
	fn(opt.BitorOp)
	fn(opt.BitxorOp)
}
