package jobs

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gorhill/cronexpr"
	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/scheduledjobs"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/sql/sqlutil"
	"github.com/znbasedb/znbase/pkg/util/protoutil"
)

// scheduledJobRecord is a reflective representation of a row in
// a system.scheduled_job table.
// Each field in this struct has a tag specifying the column
// name in the system.scheduled_job table containing the data for the field.
// Do not manipulate these fields directly, use methods in the ScheduledJob.
type scheduledJobRecord struct {
	ScheduleID      int64                     `col:"schedule_id"`
	ScheduleName    string                    `col:"schedule_name"`
	Owner           string                    `col:"owner"`
	NextRun         time.Time                 `col:"next_run"`
	ScheduleExpr    string                    `col:"schedule_expr"`
	ScheduleDetails jobspb.ScheduleDetails    `col:"schedule_details"`
	ExecutorType    string                    `col:"executor_type"`
	ExecutionArgs   jobspb.ExecutionArguments `col:"execution_args"`
	ScheduleChanges jobspb.ScheduleChangeInfo `col:"schedule_changes"`
	// It is unused now.
	// CreatedTime       time.Time                 `col:"created"`
	DefaultDatabase   string `col:"default_database"`
	DefaultSearchPath string `col:"default_searchpath"`
}

// ScheduledJob  is a representation of the scheduled job.
// This struct can marshal/unmarshal changes made to the underlying system.scheduled_job table.
type ScheduledJob struct {
	env scheduledjobs.JobSchedulerEnv

	// The "record" for this schedule job.  Do not access this field
	// directly (except in tests); Use Get/Set methods on ScheduledJob instead.
	rec scheduledJobRecord

	// The time this scheduled job was supposed to run.
	// This field is initialized to rec.NextRun when the scheduled job record
	// is loaded from the table.
	scheduledTime time.Time

	// Set of changes to this job that need to be persisted.
	dirty map[string]struct{}
}

// NewScheduledJob creates and initializes ScheduledJob.
func NewScheduledJob(env scheduledjobs.JobSchedulerEnv) *ScheduledJob {
	return &ScheduledJob{
		env:   env,
		dirty: make(map[string]struct{}),
	}
}

// ScheduleID returns schedule ID.
func (j *ScheduledJob) ScheduleID() int64 {
	return j.rec.ScheduleID
}

// ScheduleName returns schedule name.
// It is unused now.
//func (j *ScheduledJob) ScheduleName() string {
//	return j.rec.ScheduleName
//}

// SetScheduleName updates schedule name.
func (j *ScheduledJob) SetScheduleName(name string) {
	j.rec.ScheduleName = name
	j.markDirty("schedule_name")
}

// NextRun returns the next time this schedule supposed to execute.
// A sentinel value of time.Time{} indicates this schedule is paused.
func (j *ScheduledJob) NextRun() time.Time {
	return j.rec.NextRun
}

// ScheduledRunTime returns the time this schedule was supposed to execute.
// This value reflects the 'next_run' value loaded from the system.scheduled_jobs table,
// prior to any mutations to the 'next_run' value.
func (j *ScheduledJob) ScheduledRunTime() time.Time {
	return j.scheduledTime
}

// IsPaused returns true if this schedule is paused.
func (j *ScheduledJob) IsPaused() bool {
	return j.rec.NextRun == time.Time{}
}

// ExecutorType returns executor type for this schedule.
func (j *ScheduledJob) ExecutorType() string {
	return j.rec.ExecutorType
}

// ExecutionArgs returns ExecutionArgs set for this schedule.
func (j *ScheduledJob) ExecutionArgs() *jobspb.ExecutionArguments {
	return &j.rec.ExecutionArgs
}

// SetExecutionArgs returns ExecutionArgs set for this schedule.
func (j *ScheduledJob) SetExecutionArgs(args jobspb.ExecutionArguments) {
	j.rec.ExecutionArgs = args
	j.markDirty("execution_args")
}

// SetSchedule updates periodicity of this schedule, and updates this schedules
// next run time.
func (j *ScheduledJob) SetSchedule(scheduleExpr string) error {
	j.rec.ScheduleExpr = scheduleExpr
	j.markDirty("schedule_expr")
	return j.ScheduleNextRun()
}

// HasRecurringSchedule returns true if this schedule job runs periodically.
func (j *ScheduledJob) HasRecurringSchedule() bool {
	return len(j.rec.ScheduleExpr) > 0
}

// Frequency returns how often this schedule executes.
// It is unused now.
//func (j *ScheduledJob) Frequency() (time.Duration, error) {
//	if !j.HasRecurringSchedule() {
//		return 0, errors.Newf(
//			"schedule %d is not periodic", j.rec.ScheduleID)
//	}
//	expr, err := cronexpr.Parse(j.rec.ScheduleExpr)
//	if err != nil {
//		return 0, errors.Wrapf(err, "parsing schedule expression: %q", j.rec.ScheduleExpr)
//	}
//	next := expr.Next(j.env.Now())
//	nextNext := expr.Next(next)
//	return nextNext.Sub(next), nil
//}

// ScheduleNextRun updates next run based on job schedule.
func (j *ScheduledJob) ScheduleNextRun() error {
	expr, err := cronexpr.Parse(j.rec.ScheduleExpr)
	if err != nil {
		return errors.Wrapf(err, "parsing schedule expression: %q", j.rec.ScheduleExpr)
	}
	j.SetNextRun(expr.Next(j.env.Now()))
	return nil
}

// SetNextRun updates next run time for this schedule.
func (j *ScheduledJob) SetNextRun(t time.Time) {
	j.rec.NextRun = t
	j.markDirty("next_run")
}

// ScheduleDefaultDatabase return current scheduleJob's defaultdb.
func (j *ScheduledJob) ScheduleDefaultDatabase() string {
	return j.rec.DefaultDatabase
}

// SetDefaultDatabase updates defaultdb for this schedule.
func (j *ScheduledJob) SetDefaultDatabase(database string) {
	j.rec.DefaultDatabase = database
	j.markDirty("default_database")
}

// ScheduleDefaultSearchPath returns current scheduleJob's defaultdb.
func (j *ScheduledJob) ScheduleDefaultSearchPath() string {
	return j.rec.DefaultSearchPath
}

// SetDefaultSearchPath updates defaultdb  for this schedule.
func (j *ScheduledJob) SetDefaultSearchPath(searchPath string) {
	j.rec.DefaultSearchPath = searchPath
	j.markDirty("default_searchpath")
}

// ScheduleDefaultDatabase return current scheduleJob's defaultdb.
// It is unused now.
//func (j *ScheduledJob) ScheduleCreatedTime() time.Time {
//	return j.rec.CreatedTime
//}

// SetDefaultDatabase updates defaultdb  for this schedule.
// It is unused now.
//func (j *ScheduledJob) SetCreatedTime(time time.Time) {
//	j.rec.CreatedTime = time
//	j.markDirty("created")
//}

// ScheduleDetails returns schedule configuration information.
func (j *ScheduledJob) ScheduleDetails() *jobspb.ScheduleDetails {
	return &j.rec.ScheduleDetails
}

// SetScheduleDetails updates schedule configuration.
func (j *ScheduledJob) SetScheduleDetails(details jobspb.ScheduleDetails) {
	j.rec.ScheduleDetails = details
	j.markDirty("schedule_details")
}

// AddScheduleChangeReason adds change information to this job.
// Arguments are interpreted same as printf.
// If there are too many changes already recorded, trims older changes.
func (j *ScheduledJob) AddScheduleChangeReason(reasonFmt string, args ...interface{}) {
	if len(j.rec.ScheduleChanges.Changes) > 10 {
		j.rec.ScheduleChanges.Changes = j.rec.ScheduleChanges.Changes[1:]
	}

	j.rec.ScheduleChanges.Changes = append(
		j.rec.ScheduleChanges.Changes,
		jobspb.ScheduleChangeInfo_Change{
			Time:   j.env.Now().UnixNano(),
			Reason: fmt.Sprintf(reasonFmt, args...),
		})
	j.markDirty("schedule_changes")
}

// Pause pauses this schedule.
func (j *ScheduledJob) Pause(reason string) {
	j.rec.NextRun = time.Time{}
	j.markDirty("next_run")
	j.AddScheduleChangeReason(reason)
}

// Unpause resumes running this schedule.
// It is unused now.
//func (j *ScheduledJob) Unpause(reason string) error {
//	if err := j.SetSchedule(j.rec.ScheduleExpr); err != nil {
//		return err
//	}
//	j.AddScheduleChangeReason(reason)
//	return nil
//}

// SetExecutionDetails sets execution specific fields for this schedule.
func (j *ScheduledJob) SetExecutionDetails(executor string, args jobspb.ExecutionArguments) {
	j.rec.ExecutorType = executor
	j.rec.ExecutionArgs = args
	j.markDirty("executor_type", "execution_args")
}

// InitFromDatums initializes this ScheduledJob object based on datums and column names.
func (j *ScheduledJob) InitFromDatums(datums []tree.Datum, cols []sqlbase.ResultColumn) error {
	if len(datums) != len(cols) {
		return errors.Errorf(
			"datums length != columns length: %d != %d", len(datums), len(cols))
	}

	record := reflect.ValueOf(&j.rec).Elem()

	for i, col := range cols {
		native, err := datumToNative(datums[i])
		if err != nil {
			return err
		}

		if native == nil {
			continue
		}

		fieldNum, ok := columnNameToField[col.Name]
		if !ok {
			// Table contains columns we don't care about (e.g. created)
			continue
		}

		field := record.Field(fieldNum)

		if data, ok := native.([]byte); ok {
			// []byte == protocol message.
			if pb, ok := field.Addr().Interface().(protoutil.Message); ok {
				if err := protoutil.Unmarshal(data, pb); err != nil {
					return err
				}
			} else {
				return errors.Newf(
					"field %s with value of type %T is does not appear to be a protocol message",
					field.String(), field.Addr().Interface())
			}
		} else {
			// We ought to be able to assign native directly to our field.
			// But, be paranoid and double check.
			rv := reflect.ValueOf(native)
			if !rv.Type().AssignableTo(field.Type()) {
				return errors.Newf("value of type %T cannot be assigned to %s",
					native, field.Type().String())
			}
			field.Set(rv)
		}
	}

	j.scheduledTime = j.rec.NextRun
	return nil
}

// Create persists this schedule in the system.scheduled_jobs table.
// Sets j.scheduleID to the ID of the newly created schedule.
// Only the values initialized in this schedule are written to the specified transaction.
// If an error is returned, it is callers responsibility to handle it (e.g. rollback transaction).
func (j *ScheduledJob) Create(
	ctx context.Context, ex sqlutil.InternalExecutor, txn *client.Txn,
) error {
	if j.rec.ScheduleID != 0 {
		return errors.New("cannot specify schedule id when creating new cron job")
	}

	if !j.isDirty() {
		return errors.New("no settings specified for scheduled job")
	}

	cols, qargs, err := j.marshalChanges()
	if err != nil {
		return err
	}

	rows, retCols, err := ex.QueryWithCols(ctx, "sched-create", txn,
		fmt.Sprintf("INSERT INTO %s (%s) VALUES(%s) RETURNING schedule_id",
			j.env.ScheduledJobsTableName(), strings.Join(cols, ","), generatePlaceholders(len(qargs))),
		qargs...,
	)

	if err != nil {
		return err
	}

	if len(rows) != 1 {
		return errors.New("failed to create new schedule")
	}

	return j.InitFromDatums(rows[0], retCols)
}

// Update saves changes made to this schedule.
// If an error is returned, it is callers responsibility to handle it (e.g. rollback transaction).
func (j *ScheduledJob) Update(
	ctx context.Context, ex sqlutil.InternalExecutor, txn *client.Txn,
) error {
	if !j.isDirty() {
		return nil
	}

	if j.rec.ScheduleID == 0 {
		return errors.New("cannot update job: missing schedule id")
	}

	cols, qargs, err := j.marshalChanges()
	if err != nil {
		return err
	}

	if len(qargs) == 0 {
		return nil // Nothing changed.
	}

	n, err := ex.Exec(ctx, "sched-update", txn,
		fmt.Sprintf("UPDATE %s SET (%s) = (%s) WHERE schedule_id = %d",
			j.env.ScheduledJobsTableName(), strings.Join(cols, ","),
			generatePlaceholders(len(qargs)), j.ScheduleID()),
		qargs...,
	)

	if err != nil {
		return err
	}

	if n != 1 {
		return fmt.Errorf("expected to update 1 schedule, updated %d instead", n)
	}

	return nil
}

// marshalChanges marshals all changes in the in-memory representation and returns
// the names of the columns and marshaled values.
// If no error is returned, the job is not considered to be modified anymore.
// If the error is returned, this job object should no longer be used.
func (j *ScheduledJob) marshalChanges() ([]string, []interface{}, error) {
	var cols []string
	var qargs []interface{}

	for col := range j.dirty {
		var arg tree.Datum
		var err error

		switch col {
		case `schedule_name`:
			arg = tree.NewDString(j.rec.ScheduleName)
		case `owner`:
			arg = tree.NewDString(j.rec.Owner)
		case `next_run`:
			if (j.rec.NextRun == time.Time{}) {
				arg = tree.DNull
			} else {
				arg = tree.MakeDTimestampTZ(j.rec.NextRun, time.Microsecond)
			}
		case `schedule_expr`:
			arg = tree.NewDString(j.rec.ScheduleExpr)
		case `schedule_details`:
			arg, err = marshalProto(&j.rec.ScheduleDetails)
		case `executor_type`:
			arg = tree.NewDString(j.rec.ExecutorType)
		case `execution_args`:
			arg, err = marshalProto(&j.rec.ExecutionArgs)
		case `schedule_changes`:
			arg, err = marshalProto(&j.rec.ScheduleChanges)

		case `default_database`:
			arg = tree.NewDString(j.rec.DefaultDatabase)
		case `default_searchpath`:
			arg = tree.NewDString(j.rec.DefaultSearchPath)
		default:
			return nil, nil, errors.Newf("cannot marshal column %q", col)
		}

		if err != nil {
			return nil, nil, err
		}
		cols = append(cols, col)
		qargs = append(qargs, arg)
	}

	j.dirty = make(map[string]struct{})
	return cols, qargs, nil
}

// markDirty marks specified columns as dirty.
func (j *ScheduledJob) markDirty(cols ...string) {
	for _, col := range cols {
		j.dirty[col] = struct{}{}
	}
}

func (j *ScheduledJob) isDirty() bool {
	return len(j.dirty) > 0
}

// generates "$1,$2,..." placeholders for the specified 'n' number of arguments.
func generatePlaceholders(n int) string {
	placeholders := strings.Builder{}
	for i := 1; i <= n; i++ {
		if i > 1 {
			placeholders.WriteByte(',')
		}
		placeholders.WriteString(fmt.Sprintf("$%d", i))
	}
	return placeholders.String()
}

// marshalProto is a helper to serialize protocol message.
func marshalProto(message protoutil.Message) (tree.Datum, error) {
	data := make([]byte, message.Size())
	if _, err := message.MarshalTo(data); err != nil {
		return nil, err
	}
	return tree.NewDBytes(tree.DBytes(data)), nil
}

// datumToNative is a helper to convert tree.Datum into Go native
// types.  We only care about types stored in the system.scheduled_jobs table.
func datumToNative(datum tree.Datum) (interface{}, error) {
	datum = tree.UnwrapDatum(nil, datum)
	if datum == tree.DNull {
		return nil, nil
	}
	switch d := datum.(type) {
	case *tree.DString:
		return string(*d), nil
	case *tree.DInt:
		return int64(*d), nil
	case *tree.DTimestampTZ:
		return d.Time, nil
	case *tree.DBytes:
		return []byte(*d), nil
	}
	return nil, errors.Newf("cannot handle type %T", datum)
}

var columnNameToField = make(map[string]int)

func init() {
	// Initialize columnNameToField map, mapping system.schedule_job columns
	// to the appropriate fields int he scheduledJobRecord.
	j := reflect.TypeOf(scheduledJobRecord{})

	for f := 0; f < j.NumField(); f++ {
		field := j.Field(f)
		col := field.Tag.Get("col")
		if col != "" {
			columnNameToField[col] = f
		}
	}
}
