// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in licenses/BSD-vitess.txt.

// Portions of this file are additionally subject to the following
// license and copyright.
//
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

// This code was derived from https://github.com/youtube/vitess.

package tree

// Delete represents a DELETE statement.
type Delete struct {
	With      *With
	Table     TableExpr
	Where     *Where
	OrderBy   OrderBy
	Limit     *Limit
	Returning ReturningClause
}

// Format implements the NodeFormatter interface.
func (n *Delete) Format(ctx *FmtCtx) {
	ctx.FormatNode(n.With)
	ctx.WriteString("DELETE FROM ")
	ctx.FormatNode(n.Table)
	if n.Where != nil {
		ctx.WriteByte(' ')
		ctx.FormatNode(n.Where)
	}
	if len(n.OrderBy) > 0 {
		ctx.WriteByte(' ')
		ctx.FormatNode(&n.OrderBy)
	}
	if n.Limit != nil {
		ctx.WriteByte(' ')
		ctx.FormatNode(n.Limit)
	}
	if HasReturningClause(n.Returning) {
		ctx.WriteByte(' ')
		ctx.FormatNode(n.Returning)
	}
}
