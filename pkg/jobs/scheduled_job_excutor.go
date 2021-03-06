package jobs

import (
	"context"
	"fmt"

	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/scheduledjobs"
)

// ScheduledJobExecutor is an interface describing execution of the scheduled job.
type ScheduledJobExecutor interface {
	// Executes scheduled job;  Implementation may use provided transaction.
	// Modifications to the ScheduledJob object will be persisted.
	ExecuteJob(
		ctx context.Context,
		cfg *scheduledjobs.JobExecutionConfig,
		env scheduledjobs.JobSchedulerEnv,
		schedule *ScheduledJob,
		txn *client.Txn,
	) error

	// Notifies that the system.job started by the ScheduledJob completed.
	// Implementation may use provided transaction to perform any additional mutations.
	// Modifications to the ScheduledJob object will be persisted.
	NotifyJobTermination(
		ctx context.Context,
		cfg *scheduledjobs.JobExecutionConfig,
		env scheduledjobs.JobSchedulerEnv,
		md *JobMetadata,
		schedule *ScheduledJob,
		txn *client.Txn,
	) error
}

// ScheduledJobExecutorFactory is a callback to create a ScheduledJobExecutor.
type ScheduledJobExecutorFactory = func() (ScheduledJobExecutor, error)

var registeredExecutorFactories = make(map[string]ScheduledJobExecutorFactory)

// RegisterScheduledJobExecutorFactory registers callback for creating ScheduledJobExecutor
// with the specified name.
func RegisterScheduledJobExecutorFactory(name string, factory ScheduledJobExecutorFactory) {
	if _, ok := registeredExecutorFactories[name]; ok {
		panic("executor " + name + " already registered")
	}
	registeredExecutorFactories[name] = factory
}

// NewScheduledJobExecutor creates new ScheduledJobExecutor.
func NewScheduledJobExecutor(name string) (ScheduledJobExecutor, error) {
	if factory, ok := registeredExecutorFactories[name]; ok {
		return factory()
	}
	return nil, errors.Newf("executor %q is not registered", name)
}

// DefaultHandleFailedRun is a default implementation for handling failed run
// (either system.job failure, or perhaps error processing the schedule itself).
func DefaultHandleFailedRun(schedule *ScheduledJob, jobID int64, err error) {
	switch schedule.ScheduleDetails().OnError {
	case jobspb.ScheduleDetails_RETRY_SOON:
		schedule.AddScheduleChangeReason("retrying job %d due to failure: %v", jobID, err)
		schedule.SetNextRun(schedule.env.Now().Add(retryFailedJobAfter)) // TODO(yevgeniy): backoff
	case jobspb.ScheduleDetails_PAUSE_SCHED:
		schedule.Pause(fmt.Sprintf("schedule paused due job %d failure: %v", jobID, err))
	default:
		// Nothing: ScheduleDetails_RETRY_SCHED already handled since
		// the next run was set when we started running scheduled job.
	}
}

// NotifyJobTermination is invoked when the job triggered by specified schedule
// completes
//
// The 'txn' transaction argument is the transaction the job will use to update its
// state (e.g. status, etc).  If any changes need to be made to the scheduled job record,
// those changes are applied to the same transaction -- that is, they are applied atomically
// with the job status changes.
// It is unused now.
//func NotifyJobTermination(
//	ctx context.Context,
//	cfg *scheduledjobs.JobExecutionConfig,
//	env scheduledjobs.JobSchedulerEnv,
//	md *JobMetadata,
//	scheduleID int64,
//	txn *client.Txn,
//) error {
//	if !md.Status.Terminal() {
//		return errors.Newf(
//			"job completion expects terminal state, found %s instead for job %d", md.Status, md.ID)
//	}
//
//	if env == nil {
//		env = scheduledjobs.ProdJobSchedulerEnv
//	}
//
//	// Get the executor for this schedule.
//	schedule, executor, err := lookupScheduleAndExecutor(
//		ctx, env, cfg.InternalExecutor, scheduleID, txn)
//	if err != nil {
//		return err
//	}
//
//	// Delegate handling of the job termination to the executor.
//	err = executor.NotifyJobTermination(ctx, cfg, env, md, schedule, txn)
//	if err != nil {
//		return err
//	}
//
//	// Update this schedule in case executor made changes to it.
//	return schedule.Update(ctx, cfg.InternalExecutor, txn)
//}

// It is unused now.
//func lookupScheduleAndExecutor(
//	ctx context.Context,
//	env scheduledjobs.JobSchedulerEnv,
//	ex sqlutil.InternalExecutor,
//	scheduleID int64,
//	txn *client.Txn,
//) (*ScheduledJob, ScheduledJobExecutor, error) {
//	rows, cols, err := ex.QueryWithCols(ctx, "lookup-schedule", txn,
//		fmt.Sprintf(
//			"SELECT schedule_id, schedule_details, executor_type FROM %s WHERE schedule_id = %d",
//			env.ScheduledJobsTableName(), scheduleID))
//
//	if err != nil {
//		return nil, nil, err
//	}
//
//	if len(rows) != 1 {
//		return nil, nil, errors.Newf(
//			"expected to find 1 schedule, found %d with schedule_id=%d",
//			len(rows), scheduleID)
//	}
//
//	j := NewScheduledJob(env)
//	if err := j.InitFromDatums(rows[0], cols); err != nil {
//		return nil, nil, err
//	}
//	executor, err := NewScheduledJobExecutor(j.ExecutorType())
//	if err == nil {
//		return j, executor, nil
//	}
//	return nil, nil, err
//}
