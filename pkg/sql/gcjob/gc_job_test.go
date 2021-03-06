// Copyright 2020  The Cockroach Authors.

package gcjob

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/testutils/jobutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

// TODO(pbardea): Add more testing around the timer calculations.
func TestSchemaChangeGCJob(t *testing.T) {
	defer leaktest.AfterTest(t)()

	defer func(oldAdoptInterval, oldGCInterval time.Duration) {
		jobs.DefaultAdoptInterval = oldAdoptInterval
		MaxSQLGCInterval = oldGCInterval
	}(jobs.DefaultAdoptInterval, MaxSQLGCInterval)
	jobs.DefaultAdoptInterval = 100 * time.Millisecond
	MaxSQLGCInterval = 100 * time.Millisecond

	type DropItem int
	const (
		INDEX = iota
		TABLE
		DATABASE
	)

	for _, dropItem := range []DropItem{INDEX, TABLE, DATABASE} {
		for _, lowerTTL := range []bool{true, false} {
			s, db, kvDB := serverutils.StartServer(t, base.TestServerArgs{})
			ctx := context.Background()
			defer s.Stopper().Stop(ctx)
			sqlDB := sqlutils.MakeSQLRunner(db)

			jobRegistry := s.JobRegistry().(*jobs.Registry)

			sqlDB.Exec(t, "CREATE DATABASE my_db")
			sqlDB.Exec(t, "USE my_db")
			sqlDB.Exec(t, "CREATE TABLE my_table (a int primary key, b int, index (b))")
			sqlDB.Exec(t, "CREATE TABLE my_other_table (a int primary key, b int, index (b))")
			if lowerTTL {
				sqlDB.Exec(t, "ALTER TABLE my_table CONFIGURE ZONE USING gc.ttlseconds = 1")
				sqlDB.Exec(t, "ALTER TABLE my_other_table CONFIGURE ZONE USING gc.ttlseconds = 3")
			}
			myDBID := sqlbase.ID(keys.MinNonPredefinedUserDescID + 1)
			myTableID := sqlbase.ID(keys.MinNonPredefinedUserDescID + 2)
			myOtherTableID := sqlbase.ID(keys.MinNonPredefinedUserDescID + 3)

			var myTableDesc *sqlbase.TableDescriptor
			var myOtherTableDesc *sqlbase.TableDescriptor
			if err := kvDB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
				var err error
				myTableDesc, err = sqlbase.GetTableDescFromID(ctx, txn, myTableID)
				if err != nil {
					return err
				}
				myOtherTableDesc, err = sqlbase.GetTableDescFromID(ctx, txn, myOtherTableID)
				return err
			}); err != nil {
				t.Fatal(err)
			}

			// Start the job that drops an index.
			dropTime := timeutil.Now().UnixNano()
			var details jobspb.SchemaChangeGCDetails
			switch dropItem {
			case INDEX:
				details = jobspb.SchemaChangeGCDetails{
					Indexes: []jobspb.SchemaChangeGCDetails_DroppedIndex{
						{
							IndexID:  sqlbase.IndexID(2),
							DropTime: timeutil.Now().UnixNano(),
						},
					},
					ParentID: myTableID,
				}
			case TABLE:
				details = jobspb.SchemaChangeGCDetails{
					Tables: []jobspb.SchemaChangeGCDetails_DroppedID{
						{
							ID:       myTableID,
							DropTime: dropTime,
						},
					},
				}
			case DATABASE:
				details = jobspb.SchemaChangeGCDetails{
					Tables: []jobspb.SchemaChangeGCDetails_DroppedID{
						{
							ID:       myTableID,
							DropTime: dropTime,
						},
						{
							ID:       myOtherTableID,
							DropTime: dropTime,
						},
					},
					ParentID: myDBID,
				}
			}

			jobRecord := jobs.Record{
				Description:   fmt.Sprintf("GC test"),
				Username:      "user",
				DescriptorIDs: sqlbase.IDs{myTableID},
				Details:       details,
				Progress:      jobspb.SchemaChangeGCProgress{},
				NonCancelable: true,
			}

			// The job record that will be used to lookup this job.
			lookupJR := jobs.Record{
				Description:   fmt.Sprintf("GC test"),
				Username:      "user",
				DescriptorIDs: sqlbase.IDs{myTableID},
				Details:       details,
			}

			resultsCh := make(chan tree.Datums)
			job, _, err := jobRegistry.StartJob(ctx, resultsCh, jobRecord)
			if err != nil {
				t.Fatal(err)
			}

			switch dropItem {
			case INDEX:
				myTableDesc.Indexes = myTableDesc.Indexes[:0]
				myTableDesc.GCMutations = append(myTableDesc.GCMutations, sqlbase.TableDescriptor_GCDescriptorMutation{
					IndexID:  sqlbase.IndexID(2),
					DropTime: timeutil.Now().UnixNano(),
					JobID:    *job.ID(),
				})
			case DATABASE:
				myOtherTableDesc.State = sqlbase.TableDescriptor_DROP
				myOtherTableDesc.DropTime = dropTime
				fallthrough
			case TABLE:
				myTableDesc.State = sqlbase.TableDescriptor_DROP
				myTableDesc.DropTime = dropTime
			}

			if err := kvDB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
				b := txn.NewBatch()
				descKey := sqlbase.MakeDescMetadataKey(myTableID)
				descDesc := sqlbase.WrapDescriptor(myTableDesc)
				b.Put(descKey, descDesc)
				descKey2 := sqlbase.MakeDescMetadataKey(myOtherTableID)
				descDesc2 := sqlbase.WrapDescriptor(myOtherTableDesc)
				b.Put(descKey2, descDesc2)
				return txn.Run(ctx, b)
			}); err != nil {
				t.Fatal(err)
			}

			// Check that the job started.
			jobIDStr := strconv.Itoa(int(*job.ID()))
			if err := jobutils.VerifySystemJob(t, sqlDB, 0, jobspb.TypeSchemaChangeGC, jobs.StatusRunning, lookupJR); err != nil {
				t.Fatal(err)
			}

			if lowerTTL {
				sqlDB.CheckQueryResultsRetry(t, fmt.Sprintf("SELECT status FROM [SHOW JOBS] WHERE job_id = %s", jobIDStr), [][]string{{"succeeded"}})
				if err := jobutils.VerifySystemJob(t, sqlDB, 0, jobspb.TypeSchemaChangeGC, jobs.StatusSucceeded, lookupJR); err != nil {
					t.Fatal(err)
				}
			} else {
				time.Sleep(500 * time.Millisecond)
			}

			if err := kvDB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
				var err error
				myTableDesc, err = sqlbase.GetTableDescFromID(ctx, txn, myTableID)
				if lowerTTL && (dropItem == TABLE || dropItem == DATABASE) {
					// We dropped the table, so expect it to not be found.
					require.EqualError(t, err, "descriptor not found")
					return nil
				}
				myOtherTableDesc, err = sqlbase.GetTableDescFromID(ctx, txn, myOtherTableID)
				if lowerTTL && dropItem == DATABASE {
					// We dropped the entire database, so expect none of the tables to be found.
					require.EqualError(t, err, "descriptor not found")
					return nil
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}

			switch dropItem {
			case INDEX:
				if lowerTTL {
					require.Equal(t, 0, len(myTableDesc.GCMutations))
				} else {
					require.Equal(t, 1, len(myTableDesc.GCMutations))
				}
			case TABLE:
			case DATABASE:
				// Already handled the case where the TTL was lowered, since we expect
				// to not find the descriptor.
				// If the TTL was not lowered, we just expect to have not found an error
				// when fetching the TTL.
			}
		}
	}
}
