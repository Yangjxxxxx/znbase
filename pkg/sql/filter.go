// Copyright 2016  The Cockroach Authors.
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

	"github.com/znbasedb/znbase/pkg/sql/opt/cat"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
)

// filterNode implements a filtering stage. It is intended to be used
// during plan optimizations in order to avoid instantiating a fully
// blown selectTopNode/renderNode pair.
type filterNode struct {
	source           planDataSource
	filter           tree.TypedExpr
	ivarHelper       tree.IndexedVarHelper
	props            physicalProps
	storeEngine      cat.DataStoreEngine
	IsInsertOrUpdate bool
}

// filterNode implements tree.IndexedVarContainer
var _ tree.IndexedVarContainer = &filterNode{}

// IndexedVarEval implements the tree.IndexedVarContainer interface.
func (f *filterNode) IndexedVarEval(idx int, ctx *tree.EvalContext) (tree.Datum, error) {
	return f.source.plan.Values()[idx].Eval(ctx)
}

// IndexedVarResolvedType implements the tree.IndexedVarContainer interface.
func (f *filterNode) IndexedVarResolvedType(idx int) types.T {
	return f.source.info.SourceColumns[idx].Typ
}

// GetVisibleType implements the parser.IndexedVarContainer interface.
func (f *filterNode) GetVisibleType(idx int) string {
	return ""
}

// IndexedVarNodeFormatter implements the tree.IndexedVarContainer interface.
func (f *filterNode) IndexedVarNodeFormatter(idx int) tree.NodeFormatter {
	return f.source.info.NodeFormatter(idx)
}

// SetForInsertOrUpdate implements the parser.IndexedVarContainer interface.
func (f *filterNode) SetForInsertOrUpdate(b bool) {
	f.IsInsertOrUpdate = b
}

// GetForInsertOrUpdate implements the parser.IndexedVarContainer interface.
func (f *filterNode) GetForInsertOrUpdate() bool {
	return f.IsInsertOrUpdate
}

func (f *filterNode) startExec(runParams) error {
	return nil
}

// Next implements the planNode interface.
func (f *filterNode) Next(params runParams) (bool, error) {
	panic("filterNode cannot be run in local mode")
}

func (f *filterNode) Values() tree.Datums {
	panic("filterNode cannot be run in local mode")
}

func (f *filterNode) Close(ctx context.Context) { f.source.plan.Close(ctx) }

func (f *filterNode) computePhysicalProps(evalCtx *tree.EvalContext) {
	f.props = planPhysicalProps(f.source.plan)
	f.props.applyExpr(evalCtx, f.filter)
}

func (f *filterNode) SetDataStoreEngine(ds cat.DataStoreEngine) {
	f.storeEngine = ds
}

func (f *filterNode) GetDataStoreEngine() cat.DataStoreEngine {
	return f.storeEngine
}
