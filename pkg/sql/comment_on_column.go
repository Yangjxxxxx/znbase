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

package sql

import (
	"context"

	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/security/audit/server"
	"github.com/znbasedb/znbase/pkg/security/privilege"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

type commentOnColumnNode struct {
	n         *tree.CommentOnColumn
	tableDesc *ImmutableTableDescriptor
}

// CommentOnColumn add comment on a column.
// Privileges: CREATE on table.
func (p *planner) CommentOnColumn(ctx context.Context, n *tree.CommentOnColumn) (planNode, error) {
	var tableName tree.TableName
	if n.ColumnItem.TableName != nil {
		tableName = n.ColumnItem.TableName.ToTableName()
	}
	tableDesc, err := p.ResolveUncachedTableDescriptor(ctx, &tableName, true, requireTableDesc)
	if err != nil {
		return nil, err
	}

	if err := p.CheckPrivilege(ctx, tableDesc, privilege.REFERENCES); err != nil {
		return nil, err
	}

	return &commentOnColumnNode{n: n, tableDesc: tableDesc}, nil
}

func (n *commentOnColumnNode) startExec(params runParams) error {
	col, _, err := n.tableDesc.FindColumnByName(n.n.ColumnItem.ColumnName)
	if err != nil {
		return err
	}

	if n.n.Comment != nil {
		_, err := params.p.extendedEvalCtx.ExecCfg.InternalExecutor.Exec(
			params.ctx,
			"set-column-comment",
			params.p.Txn(),
			"UPSERT INTO system.comments VALUES ($1, $2, $3, $4)",
			keys.ColumnCommentType,
			n.tableDesc.ID,
			col.ID,
			*n.n.Comment)
		if err != nil {
			return err
		}
	} else {
		_, err := params.p.extendedEvalCtx.ExecCfg.InternalExecutor.Exec(
			params.ctx,
			"delete-column-comment",
			params.p.Txn(),
			"DELETE FROM system.comments WHERE type=$1 AND object_id=$2 AND sub_id=$3",
			keys.ColumnCommentType,
			n.tableDesc.ID,
			col.ID)
		if err != nil {
			return err
		}
	}

	// some audit data
	params.p.curPlan.auditInfo = &server.AuditInfo{
		EventTime: timeutil.Now(),
		EventType: string(EventLogCommentOnColumn),
		TargetInfo: &server.TargetInfo{
			TargetID: int32(n.tableDesc.ID),
			Desc: struct {
				User       string
				TableName  string
				ColumnName string
			}{
				params.SessionData().User,
				n.tableDesc.Name,
				string(n.n.ColumnItem.ColumnName),
			},
		},
	}
	return nil
}

func (n *commentOnColumnNode) Next(runParams) (bool, error) { return false, nil }
func (n *commentOnColumnNode) Values() tree.Datums          { return tree.Datums{} }
func (n *commentOnColumnNode) Close(context.Context)        {}
