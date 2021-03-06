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

package jobs_test

import (
	"context"
	gosql "database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/jobs"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/testutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/protoutil"
	"github.com/znbasedb/znbase/pkg/util/retry"
	"github.com/znbasedb/znbase/pkg/util/syncutil"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

// expectation defines the information necessary to determine the validity of
// a job in the system.jobs table.
type expectation struct {
	DB                *gosql.DB
	Record            jobs.Record
	Type              jobspb.Type
	Before            time.Time
	FractionCompleted float32
	Error             string
}

func (expected *expectation) verify(id *int64, expectedStatus jobs.Status) error {
	var statusString string
	var created time.Time
	var payloadBytes []byte
	var progressBytes []byte
	if err := expected.DB.QueryRow(
		`SELECT status, created, payload, progress FROM system.jobs WHERE id = $1`, id,
	).Scan(
		&statusString, &created, &payloadBytes, &progressBytes,
	); err != nil {
		return err
	}

	var payload jobspb.Payload
	if err := protoutil.Unmarshal(payloadBytes, &payload); err != nil {
		return err
	}
	var progress jobspb.Progress
	if err := protoutil.Unmarshal(progressBytes, &progress); err != nil {
		return err
	}

	// Verify the upstream-provided fields.
	details := payload.UnwrapDetails()
	progressDetail := progress.UnwrapDetails()

	if e, a := expected.Record, (jobs.Record{
		Description:   payload.Description,
		Details:       details,
		DescriptorIDs: payload.DescriptorIDs,
		Username:      payload.Username,
		Progress:      progressDetail,
	}); !reflect.DeepEqual(e, a) {
		diff := strings.Join(pretty.Diff(e, a), "\n")
		return errors.Errorf("Records do not match:\n%s", diff)
	}

	// Verify internally-managed fields.
	status := jobs.Status(statusString)
	if e, a := expectedStatus, status; e != a {
		return errors.Errorf("expected status %v, got %v", e, a)
	}
	if e, a := expected.Type, payload.Type(); e != a {
		return errors.Errorf("expected type %v, got type %v", e, a)
	}
	if e, a := expected.FractionCompleted, progress.GetFractionCompleted(); e != a {
		return errors.Errorf("expected fraction completed %f, got %f", e, a)
	}

	if status == jobs.StatusPending {
		return nil
	}
	started := timeutil.FromUnixMicros(payload.StartedMicros)
	if started.Equal(timeutil.UnixEpoch) && status == jobs.StatusSucceeded {
		return errors.Errorf("started time is empty but job claims to be successful")
	}
	if status == jobs.StatusRunning || status == jobs.StatusPaused {
		return nil
	}

	if e, a := expected.Error, payload.Error; e != a {
		return errors.Errorf("expected error %q, got %q", e, a)
	}
	return nil
}

func TestJobsTableProgressFamily(t *testing.T) {
	defer leaktest.AfterTest(t)()

	ctx := context.TODO()
	s, db, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)

	var table, schema string
	sqlutils.MakeSQLRunner(db).QueryRow(t, `SHOW CREATE system.jobs`).Scan(&table, &schema)
	if !strings.Contains(schema, `FAMILY progress (progress)`) {
		t.Fatalf("expected progress family, got %q", schema)
	}
}

func TestRegistryLifecycle(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer jobs.ResetResumeHooks()()

	defer func(oldInterval time.Duration) {
		jobs.DefaultAdoptInterval = oldInterval
	}(jobs.DefaultAdoptInterval)
	jobs.DefaultAdoptInterval = 100 * time.Millisecond

	ctx := context.TODO()

	s, outerDB, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)
	sqlDB := sqlutils.MakeSQLRunner(outerDB)

	registry := s.JobRegistry().(*jobs.Registry)

	done := make(chan struct{})
	defer close(done)

	type Counters struct {
		resume, resumeExit, terminal int
		// These sometimes retry, so just use bools.
		fail, success bool
	}

	var lock syncutil.Mutex
	var e, a Counters

	mockJob := jobs.Record{Details: jobspb.ImportDetails{}, Progress: jobspb.ImportProgress{}}

	check := func(t *testing.T) {
		t.Helper()
		if err := retry.ForDuration(time.Second*5, func() error {
			lock.Lock()
			defer lock.Unlock()
			if e != a {
				return errors.Errorf("expected %v, got %v", e, a)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	clear := func() {
		lock.Lock()
		a = Counters{}
		e = Counters{}
		lock.Unlock()
	}

	resumeCh := make(chan error)
	progressCh := make(chan struct{})
	// resumeCheckCh is used to wait for the resume check loop to start. This is
	// useful to prevent race conditions where progressCh checking jobs.Progressed
	// can race with a PAUSE or CANCEL transaction.
	resumeCheckCh := make(chan struct{})
	termCh := make(chan struct{})

	// Instead of a ch for success and fail, use a variable because they can
	// retry since they are in a transaction.
	var successErr, failErr error

	dummy := jobs.FakeResumer{
		OnResume: func(job *jobs.Job) error {
			lock.Lock()
			a.resume++
			lock.Unlock()
			defer func() {
				lock.Lock()
				a.resumeExit++
				lock.Unlock()
			}()
			for {
				<-resumeCheckCh
				select {
				case err := <-resumeCh:
					return err
				case <-progressCh:
					err := job.FractionProgressed(ctx, jobs.FractionUpdater(0))
					if err != nil {
						return err
					}
					// continue
				}
			}
		},
		Fail: func(*jobs.Job) error {
			lock.Lock()
			defer lock.Unlock()
			a.fail = true
			return failErr
		},
		Success: func(*jobs.Job) error {
			lock.Lock()
			defer lock.Unlock()
			a.success = true
			return successErr
		},
		Terminal: func(*jobs.Job) {
			lock.Lock()
			a.terminal++
			lock.Unlock()
			termCh <- struct{}{}
		},
	}

	jobs.AddResumeHook(func(typ jobspb.Type, _ *cluster.Settings) jobs.Resumer {
		return dummy
	})

	var jobErr = errors.New("error")

	t.Run("normal success", func(t *testing.T) {
		clear()
		_, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		resumeCheckCh <- struct{}{}
		resumeCh <- nil
		e.resumeExit++
		e.success = true
		e.terminal++
		<-termCh
		check(t)
	})

	t.Run("pause", func(t *testing.T) {
		clear()
		job, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		sqlDB.Exec(t, "PAUSE JOB $1", *job.ID())
		resumeCheckCh <- struct{}{}
		progressCh <- struct{}{}
		e.resumeExit++
		check(t)
		sqlDB.Exec(t, "PAUSE JOB $1", *job.ID())
		check(t)
		sqlDB.Exec(t, "RESUME JOB $1", *job.ID())
		resumeCheckCh <- struct{}{}
		resumeCh <- nil
		e.resume++
		e.resumeExit++
		e.success = true
		e.terminal++
		<-termCh
		check(t)
	})

	t.Run("cancel", func(t *testing.T) {
		clear()
		job, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		sqlDB.Exec(t, "CANCEL JOB $1", *job.ID())
		// Test for a canceled error message.
		if err := job.CheckStatus(ctx); !testutils.IsError(err, "cannot update progress on canceled job") {
			t.Fatalf("unexpected %v", err)
		}
		resumeCheckCh <- struct{}{}
		progressCh <- struct{}{}
		e.resumeExit++
		e.fail = true
		e.terminal++
		<-termCh
		check(t)
	})

	// Verify that pause and cancel in a rollback do nothing.
	t.Run("rollback", func(t *testing.T) {
		clear()
		job, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		resumeCheckCh <- struct{}{}
		check(t)
		// Rollback a CANCEL.
		{
			txn, err := outerDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			// OnFailOrCancel is called before the txn fails, so this should be set.
			e.fail = true
			if _, err := txn.Exec("CANCEL JOB $1", *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := txn.Rollback(); err != nil {
				t.Fatal(err)
			}
			progressCh <- struct{}{}
			resumeCheckCh <- struct{}{}
			check(t)
		}
		// Rollback a PAUSE.
		{
			txn, err := outerDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := txn.Exec("PAUSE JOB $1", *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := txn.Rollback(); err != nil {
				t.Fatal(err)
			}
			progressCh <- struct{}{}
			resumeCheckCh <- struct{}{}
			check(t)
		}
		// Now pause it for reals.
		{
			txn, err := outerDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := txn.Exec("PAUSE JOB $1", *job.ID()); err != nil {
				t.Fatal(err)
			}
			// Not committed yet, so state shouldn't have changed.
			check(t)
			if err := txn.Commit(); err != nil {
				t.Fatal(err)
			}
			// Test for a paused error message.
			if err := job.CheckStatus(ctx); !testutils.IsError(err, "cannot update progress on paused job") {
				t.Fatalf("unexpected %v", err)
			}
		}
		progressCh <- struct{}{}
		e.resumeExit++
		check(t)
		// Rollback a RESUME.
		{
			txn, err := outerDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := txn.Exec("RESUME JOB $1", *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := txn.Rollback(); err != nil {
				t.Fatal(err)
			}
			check(t)
		}
		// Commit a RESUME.
		{
			txn, err := outerDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := txn.Exec("RESUME JOB $1", *job.ID()); err != nil {
				t.Fatal(err)
			}
			// Not committed yet, so state shouldn't have changed.
			check(t)
			if err := txn.Commit(); err != nil {
				t.Fatal(err)
			}
		}
		e.resume++
		check(t)
		resumeCheckCh <- struct{}{}
		resumeCh <- nil
		e.resumeExit++
		e.success = true
		e.terminal++
		<-termCh
		check(t)
	})

	t.Run("failed running", func(t *testing.T) {
		clear()
		_, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		resumeCheckCh <- struct{}{}
		resumeCh <- jobErr
		e.resumeExit++
		e.fail = true
		e.terminal++
		<-termCh
		check(t)
	})

	// Attempt to mark success, but fail.
	t.Run("fail marking success", func(t *testing.T) {
		clear()
		successErr = jobErr
		defer func() { successErr = nil }()
		_, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		resumeCheckCh <- struct{}{}
		resumeCh <- nil
		e.resumeExit++
		e.success = true
		e.fail = true
		e.terminal++
		<-termCh
		check(t)
	})

	// Fail the job, so expected it to attempt to mark failed, but fail that
	// also. Thus it should not trigger OnTerminal.
	t.Run("fail marking success and failed", func(t *testing.T) {
		clear()
		successErr = jobErr
		failErr = jobErr
		defer func() { failErr = nil }()
		_, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		resumeCheckCh <- struct{}{}
		resumeCh <- nil
		e.resumeExit++
		e.success = true
		e.fail = true
		// It should restart.
		e.resume++
		check(t)
		// But let it succeed.
		successErr = nil
		resumeCheckCh <- struct{}{}
		resumeCh <- nil
		e.resumeExit++
		e.terminal++
		<-termCh
		check(t)
	})

	// Fail the job, but also fail to mark it failed. No OnTerminal.
	t.Run("fail marking failed", func(t *testing.T) {
		clear()
		failErr = jobErr
		_, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		resumeCheckCh <- struct{}{}
		resumeCh <- jobErr
		e.resumeExit++
		e.fail = true
		// It should restart.
		e.resume++
		check(t)
		// But let it fail.
		failErr = nil
		resumeCheckCh <- struct{}{}
		resumeCh <- jobErr
		e.resumeExit++
		e.terminal++
		<-termCh
		check(t)
	})

	t.Run("fail 2.0 jobs with no progress", func(t *testing.T) {
		clear()
		job, _, err := registry.StartJob(ctx, nil, mockJob)
		if err != nil {
			t.Fatal(err)
		}
		e.resume++
		check(t)
		sqlDB.Exec(t, "PAUSE JOB $1", *job.ID())
		resumeCheckCh <- struct{}{}
		progressCh <- struct{}{}
		e.resumeExit++
		check(t)
		sqlDB.Exec(t, `UPDATE system.jobs SET progress = NULL, status = $2 WHERE id = $1`, *job.ID(), jobs.StatusRunning)
		testutils.SucceedsSoon(t, func() error {
			var status jobs.Status
			var err string
			sqlDB.QueryRow(t, `SELECT error, status FROM [SHOW JOBS] WHERE job_id = $1`, *job.ID()).Scan(&err, &status)
			if status != jobs.StatusFailed {
				return errors.Errorf("unexpected status: %s", status)
			}
			if err != "job predates cluster upgrade and must be re-run" {
				return errors.Errorf("unexpected error: %s", err)
			}
			return nil
		})
		check(t)
	})
}

func TestJobLifecycle(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer jobs.ResetResumeHooks()()

	ctx := context.TODO()

	s, sqlDB, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)

	registry := s.JobRegistry().(*jobs.Registry)

	createJob := func(record jobs.Record) (*jobs.Job, expectation) {
		beforeTime := timeutil.Now()
		job := registry.NewJob(record)
		if err := job.Created(ctx); err != nil {
			t.Fatal(err)
		}
		payload := job.Payload()
		return job, expectation{
			DB:     sqlDB,
			Record: record,
			Type:   payload.Type(),
			Before: beforeTime,
		}
	}

	defaultRecord := jobs.Record{
		// Job does not accept an empty Details field, so arbitrarily provide
		// ImportDetails.
		Details:  jobspb.ImportDetails{},
		Progress: jobspb.ImportProgress{},
	}

	createDefaultJob := func() (*jobs.Job, expectation) {
		return createJob(defaultRecord)
	}

	done := make(chan struct{})
	defer close(done)

	dummy := jobs.FakeResumer{OnResume: func(*jobs.Job) error {
		<-done
		return nil
	}}

	jobs.AddResumeHook(func(typ jobspb.Type, _ *cluster.Settings) jobs.Resumer {
		switch typ {
		case jobspb.TypeImport:
			return dummy
		}
		return nil
	})

	startLeasedJob := func(t *testing.T, record jobs.Record) (*jobs.Job, expectation) {
		beforeTime := timeutil.Now()
		job, _, err := registry.StartJob(ctx, nil, record)
		if err != nil {
			t.Fatal(err)
		}
		payload := job.Payload()
		return job, expectation{
			DB:     sqlDB,
			Record: record,
			Type:   payload.Type(),
			Before: beforeTime,
		}
	}

	t.Run("valid job lifecycles succeed", func(t *testing.T) {
		// Woody is a successful job.
		woodyJob, woodyExp := createJob(jobs.Record{
			Description:   "There's a snake in my boot!",
			Username:      "Woody Pride",
			DescriptorIDs: []sqlbase.ID{1, 2, 3},
			Details:       jobspb.RestoreDetails{},
			Progress:      jobspb.RestoreProgress{},
		})

		if err := woodyJob.Created(ctx); err != nil {
			t.Fatal(err)
		}
		if err := woodyExp.verify(woodyJob.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}

		if err := woodyJob.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := woodyExp.verify(woodyJob.ID(), jobs.StatusRunning); err != nil {
			t.Fatal(err)
		}

		// This fraction completed progression tests that calling Progressed with a
		// fractionCompleted that is less than the last-recorded fractionCompleted
		// is observed.
		progresses := []struct {
			actual   float32
			expected float32
		}{
			{0.0, 0.0}, {0.5, 0.5}, {0.5, 0.5}, {0.4, 0.4}, {0.8, 0.8}, {1.0, 1.0},
		}
		for _, f := range progresses {
			if err := woodyJob.FractionProgressed(ctx, jobs.FractionUpdater(f.actual)); err != nil {
				t.Fatal(err)
			}
			woodyExp.FractionCompleted = f.expected
			if err := woodyExp.verify(woodyJob.ID(), jobs.StatusRunning); err != nil {
				t.Fatal(err)
			}
		}

		// Test Progressed callbacks.
		if err := woodyJob.FractionProgressed(ctx, func(_ context.Context, details jobspb.ProgressDetails) float32 {
			details.(*jobspb.Progress_Restore).Restore.HighWater = roachpb.Key("mariana")
			return 1.0
		}); err != nil {
			t.Fatal(err)
		}
		woodyExp.Record.Progress = jobspb.RestoreProgress{HighWater: roachpb.Key("mariana")}
		if err := woodyExp.verify(woodyJob.ID(), jobs.StatusRunning); err != nil {
			t.Fatal(err)
		}

		if err := woodyJob.Succeeded(ctx, jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		if err := woodyExp.verify(woodyJob.ID(), jobs.StatusSucceeded); err != nil {
			t.Fatal(err)
		}

		// Buzz fails after it starts running.
		buzzRecord := jobs.Record{
			Description:   "To infinity and beyond!",
			Username:      "Buzz Lightyear",
			DescriptorIDs: []sqlbase.ID{3, 2, 1},
			Details:       jobspb.BackupDetails{},
			Progress:      jobspb.BackupProgress{},
		}
		buzzExp := expectation{
			DB:     sqlDB,
			Record: buzzRecord,
			Type:   jobspb.TypeBackup,
			Before: timeutil.Now(),
			Error:  "Buzz Lightyear can't fly",
		}
		buzzJob := registry.NewJob(buzzRecord)

		if err := buzzJob.Created(ctx); err != nil {
			t.Fatal(err)
		}
		if err := buzzExp.verify(buzzJob.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}

		if err := buzzJob.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := buzzExp.verify(buzzJob.ID(), jobs.StatusRunning); err != nil {
			t.Fatal(err)
		}

		if err := buzzJob.FractionProgressed(ctx, jobs.FractionUpdater(.42)); err != nil {
			t.Fatal(err)
		}
		buzzExp.FractionCompleted = .42
		if err := buzzExp.verify(buzzJob.ID(), jobs.StatusRunning); err != nil {
			t.Fatal(err)
		}

		if err := buzzJob.Failed(ctx, errors.New("Buzz Lightyear can't fly"), jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		if err := buzzExp.verify(buzzJob.ID(), jobs.StatusFailed); err != nil {
			t.Fatal(err)
		}

		// Ensure that logging Buzz didn't corrupt Woody.
		if err := woodyExp.verify(woodyJob.ID(), jobs.StatusSucceeded); err != nil {
			t.Fatal(err)
		}

		// Sid fails before it starts running.
		sidJob, sidExp := createJob(jobs.Record{
			Description:   "The toys! The toys are alive!",
			Username:      "Sid Phillips",
			DescriptorIDs: []sqlbase.ID{6, 6, 6},
			Details:       jobspb.RestoreDetails{},
			Progress:      jobspb.RestoreProgress{},
		})

		if err := sidJob.Created(ctx); err != nil {
			t.Fatal(err)
		}
		if err := sidExp.verify(sidJob.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}

		if err := sidJob.Failed(ctx, errors.New("Sid is a total failure"), jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		sidExp.Error = "Sid is a total failure"
		if err := sidExp.verify(sidJob.ID(), jobs.StatusFailed); err != nil {
			t.Fatal(err)
		}

		// Ensure that logging Sid didn't corrupt Woody or Buzz.
		if err := woodyExp.verify(woodyJob.ID(), jobs.StatusSucceeded); err != nil {
			t.Fatal(err)
		}
		if err := buzzExp.verify(buzzJob.ID(), jobs.StatusFailed); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("FinishedWith", func(t *testing.T) {
		t.Run("nil error marks job as successful", func(t *testing.T) {
			job, exp := createDefaultJob()
			exp.FractionCompleted = 1.0
			if err := job.Started(ctx); err != nil {
				t.Fatal(err)
			}
			if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
				t.Fatal(err)
			}
			if err := exp.verify(job.ID(), jobs.StatusSucceeded); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("non-nil error marks job as failed", func(t *testing.T) {
			job, exp := createDefaultJob()
			exp.Error = "boom"
			if err := job.Failed(ctx, errors.New(exp.Error), jobs.NoopFn); err != nil {
				t.Fatal(err)
			}
			if err := exp.verify(job.ID(), jobs.StatusFailed); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("internal errors are not swallowed if marking job as successful", func(t *testing.T) {
			job, _ := createDefaultJob()
			if _, err := sqlDB.Exec(
				`UPDATE system.jobs SET payload = 'garbage' WHERE id = $1`, *job.ID(),
			); err != nil {
				t.Fatal(err)
			}
			if err := job.Succeeded(ctx, jobs.NoopFn); !testutils.IsError(err, "wrong wireType") {
				t.Fatalf("unexpected: %v", err)
			}
		})

		t.Run("internal errors are not swallowed if marking job as failed", func(t *testing.T) {
			job, _ := createDefaultJob()
			if _, err := sqlDB.Exec(
				`UPDATE system.jobs SET payload = 'garbage' WHERE id = $1`, *job.ID(),
			); err != nil {
				t.Fatal(err)
			}
			if err := job.Failed(ctx, errors.New("boom"), jobs.NoopFn); !testutils.IsError(err, "wrong wireType") {
				t.Fatalf("unexpected: %v", err)
			}
		})
	})

	t.Run("cancelable jobs can be paused until finished", func(t *testing.T) {
		job, exp := startLeasedJob(t, defaultRecord)

		if err := registry.Pause(ctx, nil, *job.ID()); err != nil {
			t.Fatal(err)
		}
		if err := registry.Pause(ctx, nil, *job.ID()); err != nil {
			t.Fatal(err)
		}
		if err := exp.verify(job.ID(), jobs.StatusPaused); err != nil {
			t.Fatal(err)
		}
		if err := registry.Resume(ctx, nil, *job.ID()); err != nil {
			t.Fatal(err)
		}
		if err := registry.Resume(ctx, nil, *job.ID()); err != nil {
			t.Fatal(err)
		}
		if err := exp.verify(job.ID(), jobs.StatusRunning); err != nil {
			t.Fatal(err)
		}

		// Pause fails after job is successful.
		if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		if err := registry.Pause(ctx, nil, *job.ID()); !testutils.IsError(err, "cannot pause succeeded job") {
			t.Fatalf("expected 'cannot pause succeeded job', but got '%s'", err)
		}
	})

	t.Run("cancelable jobs can be canceled until finished", func(t *testing.T) {
		{
			job, exp := startLeasedJob(t, defaultRecord)
			if err := registry.Cancel(ctx, nil, *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := exp.verify(job.ID(), jobs.StatusCanceled); err != nil {
				t.Fatal(err)
			}
		}

		{
			job, exp := startLeasedJob(t, defaultRecord)
			if err := job.Started(ctx); err != nil {
				t.Fatal(err)
			}
			if err := registry.Cancel(ctx, nil, *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := exp.verify(job.ID(), jobs.StatusCanceled); err != nil {
				t.Fatal(err)
			}
		}

		{
			job, exp := startLeasedJob(t, defaultRecord)
			if err := registry.Pause(ctx, nil, *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := registry.Cancel(ctx, nil, *job.ID()); err != nil {
				t.Fatal(err)
			}
			if err := exp.verify(job.ID(), jobs.StatusCanceled); err != nil {
				t.Fatal(err)
			}
		}

		{
			job, _ := startLeasedJob(t, defaultRecord)
			if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
				t.Fatal(err)
			}
			expectedErr := "job with status succeeded cannot be canceled"
			if err := registry.Cancel(ctx, nil, *job.ID()); !testutils.IsError(err, expectedErr) {
				t.Fatalf("expected '%s', but got '%s'", expectedErr, err)
			}
		}
	})

	t.Run("unpaused jobs cannot be resumed", func(t *testing.T) {
		checkResumeFails := func(job *jobs.Job, status jobs.Status) {
			expectedErr := fmt.Sprintf("job with status %s cannot be resumed", status)
			if err := registry.Resume(ctx, nil, *job.ID()); !testutils.IsError(err, expectedErr) {
				t.Errorf("expected '%s', but got '%v'", expectedErr, err)
			}
		}

		{
			job, _ := createDefaultJob()
			checkResumeFails(job, jobs.StatusPending)
		}

		{
			job, _ := startLeasedJob(t, defaultRecord)
			if err := registry.Cancel(ctx, nil, *job.ID()); err != nil {
				t.Fatal(err)
			}
			checkResumeFails(job, jobs.StatusCanceled)
		}

		{
			job, _ := startLeasedJob(t, defaultRecord)
			if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
				t.Fatal(err)
			}
			checkResumeFails(job, jobs.StatusSucceeded)
		}
	})

	t.Run("bad job details fail", func(t *testing.T) {
		defer func() {
			if r, ok := recover().(string); !ok || !strings.Contains(r, "unknown details type int") {
				t.Fatalf("expected 'unknown details type int', but got: %v", r)
			}
		}()

		job := registry.NewJob(jobs.Record{
			Details: 42,
		})
		_ = job.Created(ctx)
	})

	t.Run("update before create fails", func(t *testing.T) {
		job := registry.NewJob(jobs.Record{
			Details:  jobspb.RestoreDetails{},
			Progress: jobspb.RestoreProgress{},
		})
		if err := job.Started(ctx); !testutils.IsError(err, "job not created") {
			t.Fatalf("expected 'job not created' error, but got %v", err)
		}
	})

	t.Run("same state transition twice succeeds silently", func(t *testing.T) {
		job, _ := createDefaultJob()
		if err := job.Created(ctx); err != nil {
			t.Fatal(err)
		}
		if err := job.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := job.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("high-water progress works", func(t *testing.T) {
		job, _ := createDefaultJob()
		if err := job.Started(ctx); err != nil {
			t.Fatal(err)
		}
		highWaters := []hlc.Timestamp{
			{WallTime: 1, Logical: 1},
			{WallTime: 2, Logical: 0},
		}
		for _, ts := range highWaters {
			if err := job.HighWaterProgressed(
				ctx, func(context.Context, jobspb.ProgressDetails) hlc.Timestamp { return ts },
			); err != nil {
				t.Fatal(err)
			}
			p := job.Progress()
			if actual := *p.GetHighWater(); actual != ts {
				t.Fatalf(`got %s expected %s`, actual, ts)
			}
		}
	})

	t.Run("out of bounds progress fails", func(t *testing.T) {
		job, _ := createDefaultJob()
		if err := job.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(-0.1)); !testutils.IsError(err, "outside allowable range") {
			t.Fatalf("expected 'outside allowable range' error, but got %v", err)
		}
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(1.1)); !testutils.IsError(err, "outside allowable range") {
			t.Fatalf("expected 'outside allowable range' error, but got %v", err)
		}
		if err := job.HighWaterProgressed(
			ctx, func(context.Context, jobspb.ProgressDetails) hlc.Timestamp {
				return hlc.Timestamp{WallTime: -1}
			},
		); !testutils.IsError(err, "outside allowable range") {
			t.Fatalf("expected 'outside allowable range' error, but got %v", err)
		}
	})

	t.Run("progress on non-started job fails", func(t *testing.T) {
		job, _ := createDefaultJob()
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(0.5)); !testutils.IsError(
			err, `cannot update progress on pending job \(id \d+\)`,
		) {
			t.Fatalf("expected 'cannot update progress' error, but got %v", err)
		}
	})

	t.Run("progress on finished job fails", func(t *testing.T) {
		job, _ := createDefaultJob()
		if err := job.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(0.5)); !testutils.IsError(
			err, `cannot update progress on succeeded job \(id \d+\)`,
		) {
			t.Fatalf("expected 'cannot update progress' error, but got %v", err)
		}
	})

	t.Run("progress on paused job fails", func(t *testing.T) {
		job, _ := startLeasedJob(t, defaultRecord)
		if err := registry.Pause(ctx, nil, *job.ID()); err != nil {
			t.Fatal(err)
		}
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(0.5)); !testutils.IsError(
			err, `cannot update progress on paused job \(id \d+\)`,
		) {
			t.Fatalf("expected progress error, but got %v", err)
		}
	})

	t.Run("progress on canceled job fails", func(t *testing.T) {
		job, _ := startLeasedJob(t, defaultRecord)
		if err := registry.Cancel(ctx, nil, *job.ID()); err != nil {
			t.Fatal(err)
		}
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(0.5)); !testutils.IsError(
			err, `cannot update progress on canceled job \(id \d+\)`,
		) {
			t.Fatalf("expected progress error, but got %v", err)
		}
	})

	t.Run("succeeded forces fraction completed to 1.0", func(t *testing.T) {
		job, exp := createDefaultJob()
		if err := job.Started(ctx); err != nil {
			t.Fatal(err)
		}
		if err := job.FractionProgressed(ctx, jobs.FractionUpdater(0.2)); err != nil {
			t.Fatal(err)
		}
		if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
			t.Fatal(err)
		}
		exp.FractionCompleted = 1.0
		if err := exp.verify(job.ID(), jobs.StatusSucceeded); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("set details works", func(t *testing.T) {
		job, exp := createJob(jobs.Record{
			Details:  jobspb.RestoreDetails{},
			Progress: jobspb.RestoreProgress{},
		})
		if err := exp.verify(job.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}
		newDetails := jobspb.RestoreDetails{URIs: []string{"new"}}
		exp.Record.Details = newDetails
		if err := job.SetDetails(ctx, newDetails); err != nil {
			t.Fatal(err)
		}
		if err := exp.verify(job.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("set progress works", func(t *testing.T) {
		job, exp := createJob(jobs.Record{
			Details:  jobspb.RestoreDetails{},
			Progress: jobspb.RestoreProgress{},
		})
		if err := exp.verify(job.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}
		newDetails := jobspb.RestoreProgress{HighWater: []byte{42}}
		exp.Record.Progress = newDetails
		if err := job.SetProgress(ctx, newDetails); err != nil {
			t.Fatal(err)
		}
		if err := exp.verify(job.ID(), jobs.StatusPending); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cannot pause or resume schema changes", func(t *testing.T) {
		job, _ := createJob(jobs.Record{
			Details:  jobspb.SchemaChangeDetails{},
			Progress: jobspb.SchemaChangeProgress{},
		})
		if err := registry.Pause(ctx, nil, *job.ID()); !testutils.IsError(err, "is not controllable") {
			t.Fatalf("unexpected %v", err)
		}
		if err := registry.Resume(ctx, nil, *job.ID()); !testutils.IsError(err, "is not controllable") {
			t.Fatalf("unexpected %v", err)
		}
		if err := registry.Cancel(ctx, nil, *job.ID()); err != nil {
			t.Fatalf("unexpected %v", err)
		}
	})
}

func TestRunAndWaitForTerminalState(t *testing.T) {
	defer leaktest.AfterTest(t)()

	// Intentionally share the server between subtests, so job records
	// accumulate over time.
	ctx := context.Background()
	s, sqlDB, _ := serverutils.StartServer(t, base.TestServerArgs{})
	defer s.Stopper().Stop(ctx)

	mockJob := jobs.Record{Details: jobspb.BackupDetails{}, Progress: jobspb.BackupProgress{}}
	tests := []struct {
		name   string
		status jobs.Status
		err    string
		execFn func(context.Context) error
	}{
		{
			"non-job execFn",
			"", "no jobs found",
			func(_ context.Context) error { return nil },
		},
		{
			"pre-job error",
			"", "exec failed before job was created.*pre-job error",
			func(_ context.Context) error { return errors.New("pre-job error") },
		},
		{
			"job succeeded",
			jobs.StatusSucceeded, "",
			func(_ context.Context) error {
				registry := s.JobRegistry().(*jobs.Registry)
				job := registry.NewJob(mockJob)
				if err := job.Created(ctx); err != nil {
					return err
				}
				return job.Succeeded(ctx, jobs.NoopFn)
			},
		},
		{
			"job failed",
			jobs.StatusFailed, "in-job error",
			func(_ context.Context) error {
				registry := s.JobRegistry().(*jobs.Registry)
				job := registry.NewJob(mockJob)
				if err := job.Created(ctx); err != nil {
					return err
				}
				err := errors.New("in-job error")
				if err := job.Failed(ctx, err, jobs.NoopFn); err != nil {
					return err
				}
				return err
			},
		},
		{
			"job lease transfer then succeeded",
			jobs.StatusSucceeded, "",
			func(ctx context.Context) error {
				registry := s.JobRegistry().(*jobs.Registry)
				job := registry.NewJob(mockJob)
				if err := job.Created(ctx); err != nil {
					return err
				}
				if err := job.Succeeded(ctx, jobs.NoopFn); err != nil {
					return err
				}
				return errors.New("lease transferred")
			},
		},
		{
			"job lease transfer then failed",
			jobs.StatusFailed, "in-job error",
			func(ctx context.Context) error {
				registry := s.JobRegistry().(*jobs.Registry)
				job := registry.NewJob(mockJob)
				if err := job.Created(ctx); err != nil {
					return err
				}
				if err := job.Failed(ctx, errors.New("in-job error"), jobs.NoopFn); err != nil {
					return err
				}
				return errors.New("lease transferred")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, status, err := jobs.RunAndWaitForTerminalState(ctx, sqlDB, test.execFn)
			if !testutils.IsError(err, test.err) {
				t.Fatalf("got %v expected %v", err, test.err)
			}
			if status != test.status {
				t.Fatalf("got [%s] expected [%s]", status, test.status)
			}
		})
	}
}
