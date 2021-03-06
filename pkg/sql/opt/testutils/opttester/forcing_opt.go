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

package opttester

import (
	"github.com/znbasedb/znbase/pkg/sql/opt"
	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/opt/props/physical"
	"github.com/znbasedb/znbase/pkg/sql/opt/xform"
)

// forcingOptimizer is a wrapper around an Optimizer which adds low-level
// control, like restricting rule application or the expressions that can be
// part of the final expression.
type forcingOptimizer struct {
	o xform.Optimizer

	coster forcingCoster

	// remaining is the number of "unused" steps remaining.
	remaining int

	// lastMatched records the name of the rule that was most recently matched
	// by the optimizer.
	lastMatched opt.RuleName

	// lastApplied records the name of the rule that was most recently applied by
	// the optimizer. This is not necessarily the same with lastMatched because
	// normalization rules can run in-between the match and the application of an
	// exploration rule.
	lastApplied opt.RuleName

	// lastAppliedSource is the expression matched by an exploration rule, or is
	// nil for a normalization rule.
	lastAppliedSource opt.Expr

	// lastAppliedTarget is the new expression constructed by a normalization or
	// exploration rule. For an exploration rule, it can be nil if no expressions
	// were constructed, or can have additional expressions beyond the first that
	// are accessible via NextExpr links.
	lastAppliedTarget opt.Expr
}

// newForcingOptimizer creates a forcing optimizer that stops applying any rules
// after <steps> rules are matched. If ignoreNormRules is true, normalization
// rules don't count against this limit.
func newForcingOptimizer(
	tester *OptTester, steps int, ignoreNormRules bool,
) (*forcingOptimizer, error) {
	fo := &forcingOptimizer{
		remaining:   steps,
		lastMatched: opt.InvalidRuleName,
	}
	fo.o.Init(&tester.evalCtx)
	fo.coster.Init(&fo.o)
	fo.o.SetCoster(&fo.coster)

	fo.o.NotifyOnMatchedRule(func(ruleName opt.RuleName) bool {
		if ignoreNormRules && ruleName.IsNormalize() {
			return true
		}
		if fo.remaining == 0 {
			return false
		}
		fo.remaining--
		fo.lastMatched = ruleName
		return true
	})

	// Hook the AppliedRule notification in order to track the portion of the
	// expression tree affected by each transformation rule.
	fo.o.NotifyOnAppliedRule(
		func(ruleName opt.RuleName, source, target opt.Expr) {
			if ignoreNormRules && ruleName.IsNormalize() {
				return
			}
			fo.lastApplied = ruleName
			fo.lastAppliedSource = source
			fo.lastAppliedTarget = target
		},
	)

	if err := tester.buildExpr(fo.o.Factory()); err != nil {
		return nil, err
	}
	return fo, nil
}

func (fo *forcingOptimizer) Optimize() opt.Expr {
	expr, err := fo.o.Optimize()
	if err != nil {
		panic(err)
	}
	return expr
}

// LookupPath returns the path of the given node.
func (fo *forcingOptimizer) LookupPath(target opt.Expr) exprPath {
	return fo.coster.cache.lookupPath(fo.o.Memo().RootExpr(), target)
}

// RestrictToExpr sets up the optimizer to restrict the result to only those
// expression trees which include the given expression path.
func (fo *forcingOptimizer) RestrictToExpr(path exprPath) {
	fo.coster.AddAllowedPath(path)
}

// RestrictToGroup sets up the optimizer to restrict the result to only those
// expression trees which include the given expression path, or other expression
// paths in the same group.
func (fo *forcingOptimizer) RestrictToGroup(path exprPath) {
	// Since the path points to an expression, remove the last path in order to
	// allow any children from the expression's group.
	fo.coster.AddAllowedPath(path.truncateLastStep())
}

// forcingCoster implements the xform.Coster interface so that it can suppress
// expressions in the memo that can't be part of the output tree.
type forcingCoster struct {
	o       *xform.Optimizer
	inner   xform.Coster
	allowed []exprPath
	cache   pathCache
}

func (fc *forcingCoster) Init(o *xform.Optimizer) {
	fc.o = o
	fc.inner = o.Coster()
}

// AddAllowedPath adds an allowed path to the coster. Any expressions which do
// not fall along an allowed path are suppressed by the coster.
func (fc *forcingCoster) AddAllowedPath(path exprPath) {
	fc.allowed = append(fc.allowed, path)
}

// ComputeCost is part of the xform.Coster interface.
func (fc *forcingCoster) ComputeCost(e memo.RelExpr, required *physical.Required) memo.Cost {
	// If no allowed paths have been added, allow all expressions.
	if len(fc.allowed) != 0 {
		// Derive the path of the expression in the tree.
		path := fc.cache.lookupPath(fc.o.Memo().RootExpr(), e)

		// If none of the paths allow the expression, suppress it.
		suppress := true
		for _, restricted := range fc.allowed {
			if !path.isSuppressedBy(restricted) {
				suppress = false
				break
			}
		}
		if suppress {
			// Suppressed expressions get assigned MaxCost so that they never have
			// the lowest cost.
			return memo.MaxCost
		}
	}

	return fc.inner.ComputeCost(e, required)
}
