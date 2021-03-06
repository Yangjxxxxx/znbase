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

package sql

import (
	"context"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/jobs"
	"github.com/znbasedb/znbase/pkg/security/audit/server"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

type controlJobsNode struct {
	rows          planNode
	desiredStatus jobs.Status
	numRows       int
}

var jobCommandToDesiredStatus = map[tree.JobCommand]jobs.Status{
	tree.CancelJob: jobs.StatusCanceled,
	tree.ResumeJob: jobs.StatusRunning,
	tree.PauseJob:  jobs.StatusPaused,
}

func (p *planner) ControlJobs(ctx context.Context, n *tree.ControlJobs) (planNode, error) {
	rows, err := p.newPlan(ctx, n.Jobs, []types.T{types.Int})
	if err != nil {
		return nil, err
	}
	cols := planColumns(rows)
	if len(cols) != 1 {
		return nil, errors.Errorf("%s JOBS expects a single column source, got %d columns",
			tree.JobCommandToStatement[n.Command], len(cols))
	}
	if !(cols[0].Typ.Equivalent(types.Int) || cols[0].Typ.Equivalent(types.Int2) || cols[0].Typ.Equivalent(types.Int4)) {
		return nil, errors.Errorf("%s JOBS requires int values, not type %s",
			tree.JobCommandToStatement[n.Command], cols[0].Typ)
	}

	return &controlJobsNode{
		rows:          rows,
		desiredStatus: jobCommandToDesiredStatus[n.Command],
	}, nil
}

// FastPathResults implements the planNodeFastPath inteface.
func (n *controlJobsNode) FastPathResults() (int, bool) {
	return n.numRows, true
}

func (n *controlJobsNode) startExec(params runParams) error {
	reg := params.p.ExecCfg().JobRegistry
	for {
		ok, err := n.rows.Next(params)
		if err != nil {
			return err
		}
		if !ok {
			break
		}

		jobIDDatum := n.rows.Values()[0]
		if jobIDDatum == tree.DNull {
			continue
		}

		jobID, ok := tree.AsDInt(jobIDDatum)
		if !ok {
			return pgerror.NewAssertionErrorf("%q: expected *DInt, found %T", jobIDDatum, jobIDDatum)
		}

		switch n.desiredStatus {
		case jobs.StatusPaused:
			err = reg.Pause(params.ctx, params.p.txn, int64(jobID))
		case jobs.StatusRunning:
			err = reg.Resume(params.ctx, params.p.txn, int64(jobID))
		case jobs.StatusCanceled:
			err = reg.Cancel(params.ctx, params.p.txn, int64(jobID))
		default:
			err = pgerror.NewAssertionErrorf("unhandled status %v", n.desiredStatus)
		}
		if err != nil {
			return err
		}
		n.numRows++
	}
	jobIDDatum := n.rows.Values()[0]
	jobID, _ := tree.AsDInt(jobIDDatum)
	params.p.curPlan.auditInfo = &server.AuditInfo{
		EventTime: timeutil.Now(),
		EventType: string(EventLogControlJobs),
		TargetInfo: &server.TargetInfo{
			Desc: struct {
				JobID int64
				SQL   string
			}{
				JobID: int64(jobID),
				SQL:   params.p.curPlan.AST.String(),
			},
		},
		Info: "User name: " + params.SessionData().User + " SHOW SYNTAX:" + params.p.curPlan.AST.String(),
	}
	return nil
}

func (*controlJobsNode) Next(runParams) (bool, error) { return false, nil }

func (*controlJobsNode) Values() tree.Datums { return nil }

func (n *controlJobsNode) Close(ctx context.Context) {
	n.rows.Close(ctx)
}
