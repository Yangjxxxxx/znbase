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

package optbuilder

import (
	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
)

func (b *Builder) buildShowTrace(
	showTrace *tree.ShowTraceForSession, inScope *scope,
) (outScope *scope) {
	outScope = inScope.push()

	switch showTrace.TraceType {
	case tree.ShowTraceRaw, tree.ShowTraceKV:
		if showTrace.Compact {
			b.synthesizeResultColumns(outScope, sqlbase.ShowCompactTraceColumns)
		} else {
			b.synthesizeResultColumns(outScope, sqlbase.ShowTraceColumns)
		}

	case tree.ShowTraceReplica:
		b.synthesizeResultColumns(outScope, sqlbase.ShowReplicaTraceColumns)

	default:
		panic(pgerror.NewAssertionErrorf("SHOW %s not supported", showTrace.TraceType))
	}

	outScope.expr = b.factory.ConstructShowTraceForSession(&memo.ShowTracePrivate{
		TraceType: showTrace.TraceType,
		Compact:   showTrace.Compact,
		ColList:   colsToColList(outScope.cols),
	})
	return outScope
}
