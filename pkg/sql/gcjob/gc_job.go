// Copyright 2020  The Cockroach Authors.

package gcjob

import (
	"context"
	"time"

	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

var (
	// MaxSQLGCInterval is the longest the polling interval between checking if
	// elements should be GC'd.
	MaxSQLGCInterval = 5 * time.Minute
)

type schemaChangeGCResumer struct {
	jobID int64
}

func schemaChangeGCResumeHook(typ jobspb.Type, settings *cluster.Settings) jobs.Resumer {
	if typ != jobspb.TypeSchemaChangeGC {
		return nil
	}

	return &schemaChangeGCResumer{}
}

func performGC(
	ctx context.Context,
	execCfg *sql.ExecutorConfig,
	jobID int64,
	details *jobspb.SchemaChangeGCDetails,
	progress *jobspb.SchemaChangeGCProgress,
) error {
	if details.Indexes != nil {
		if err := gcIndexes(ctx, execCfg, details.ParentID, progress); err != nil {
			return errors.Wrap(err, "attempting to GC indexes")
		}
	} else if details.Tables != nil {
		if err := gcTables(ctx, execCfg, progress); err != nil {
			return errors.Wrap(err, "attempting to GC tables")
		}

		// Drop database zone config when all the tables have been GCed.
		if details.ParentID != sqlbase.InvalidID && isDoneGC(progress) {
			if err := deleteDatabaseZoneConfig(ctx, execCfg.DB, details.ParentID); err != nil {
				return errors.Wrap(err, "deleting database zone config")
			}
		}
	}

	persistProgress(ctx, execCfg, jobID, progress)
	return nil
}

// Resume is part of the jobs.Resumer interface.
func (r schemaChangeGCResumer) Resume(
	ctx context.Context, job *jobs.Job, phs interface{}, resultsCh chan<- tree.Datums,
) error {
	r.jobID = *job.ID()
	p := phs.(sql.PlanHookState)
	// TODO(pbardea): Wait for no versions.
	execCfg := p.ExecCfg()
	details, progress, err := initDetailsAndProgress(ctx, execCfg, r.jobID)
	if err != nil {
		return err
	}
	zoneCfgFilter, descTableFilter, gossipUpdateC := setupConfigWatchers(execCfg)
	tableDropTimes, indexDropTimes := getDropTimes(details)

	allTables := getAllTablesWaitingForGC(details, progress)
	expired, earliestDeadline := refreshTables(ctx, execCfg, allTables, tableDropTimes, indexDropTimes, r.jobID, progress)
	timerDuration := time.Until(earliestDeadline)
	if expired {
		timerDuration = 0
	} else if timerDuration > MaxSQLGCInterval {
		timerDuration = MaxSQLGCInterval
	}
	timer := timeutil.NewTimer()
	defer timer.Stop()
	timer.Reset(timerDuration)

	for {
		select {
		case <-gossipUpdateC:
			// Upon notification of a gossip update, update the status of the relevant schema elements.
			if log.V(2) {
				log.Info(ctx, "received a new system config")
			}
			updatedTables := getUpdatedTables(ctx, execCfg.Gossip.GetSystemConfig(), zoneCfgFilter, descTableFilter, details, progress)
			if log.V(2) {
				log.Infof(ctx, "updating status for tables %+v", updatedTables)
			}
			expired, earliestDeadline = refreshTables(ctx, execCfg, updatedTables, tableDropTimes, indexDropTimes, r.jobID, progress)
			timerDuration := time.Until(earliestDeadline)
			if expired {
				timerDuration = 0
			} else if timerDuration > MaxSQLGCInterval {
				timerDuration = MaxSQLGCInterval
			}

			timer.Reset(timerDuration)
		case <-timer.C:
			timer.Read = true
			if log.V(2) {
				log.Info(ctx, "SchemaChangeGC timer triggered")
			}
			// Refresh the status of all tables in case any GC TTLs have changed.
			remainingTables := getAllTablesWaitingForGC(details, progress)
			_, earliestDeadline = refreshTables(ctx, execCfg, remainingTables, tableDropTimes, indexDropTimes, r.jobID, progress)

			if err := performGC(ctx, execCfg, r.jobID, details, progress); err != nil {
				return err
			}
			if isDoneGC(progress) {
				return nil
			}

			// Schedule the next check for GC.
			timerDuration := time.Until(earliestDeadline)
			if timerDuration > MaxSQLGCInterval {
				timerDuration = MaxSQLGCInterval
			}
			timer.Reset(timerDuration)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r schemaChangeGCResumer) OnSuccess(context.Context, *client.Txn, *jobs.Job) error {
	return nil
}

func (r schemaChangeGCResumer) OnTerminal(
	ctx context.Context, job *jobs.Job, _ jobs.Status, _ chan<- tree.Datums,
) {
	log.Errorf(ctx, "schemaChangeGCResumer don't expect OnTerminal %d", job.ID())
}

// OnFailOrCancel is part of the jobs.Resumer interface.
func (r schemaChangeGCResumer) OnFailOrCancel(context.Context, *client.Txn, *jobs.Job) error {
	return nil
}

func init() {
	jobs.AddResumeHook(schemaChangeGCResumeHook)
}
