// Copyright 2020  The Cockroach Authors.

package gcjob

import (
	"context"
	"time"

	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/kv"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

// gcTables drops the table data and descriptor of tables that have an expired
// deadline and updates the job details to mark the work it did.
// The job progress is updated in place, but needs to be persisted to the job.
func gcTables(
	ctx context.Context, execCfg *sql.ExecutorConfig, progress *jobspb.SchemaChangeGCProgress,
) error {
	if log.V(2) {
		log.Infof(ctx, "GC is being considered for tables: %+v", progress.Tables)
	}
	for _, droppedTable := range progress.Tables {
		if droppedTable.Status != jobspb.SchemaChangeGCProgress_DELETING {
			// Table is not ready to be dropped, or has already been dropped.
			continue
		}

		var table *sqlbase.TableDescriptor
		if err := execCfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
			var err error
			table, err = sqlbase.GetTableDescFromID(ctx, txn, droppedTable.ID)
			return err
		}); err != nil {
			return errors.Wrapf(err, "fetching table %d", droppedTable.ID)
		}

		if !table.Dropped() {
			// We shouldn't drop this table yet.
			continue
		}

		// First, delete all the table data.
		if err := clearTableData(ctx, execCfg.DB, execCfg.DistSender, table); err != nil {
			return errors.Wrapf(err, "clearing data for table %d", table.ID)
		}

		// Finished deleting all the table data, now delete the table meta data.
		if err := dropTableDesc(ctx, execCfg.DB, table); err != nil {
			return errors.Wrapf(err, "dropping table descriptor for table %d", table.ID)
		}

		// Update the details payload to indicate that the table was dropped.
		markTableGCed(ctx, table.ID, progress)
	}
	return nil
}

// clearTableData deletes all of the data in the specified table.
func clearTableData(
	ctx context.Context, db *client.DB, distSender *kv.DistSender, table *sqlbase.TableDescriptor,
) error {
	// If DropTime isn't set, assume this drop request is from a version
	// 1.1 server and invoke legacy code that uses DeleteRange and range GC.
	// TODO(pbardea): Note that we never set the drop time for interleaved tables,
	// but this check was added to be more explicit about it. This should get
	// cleaned up.
	if table.DropTime == 0 || table.IsInterleaved() {
		log.Infof(ctx, "clearing data in chunks for table %d", table.ID)
		return sql.ClearTableDataInChunks(ctx, table, db, false /* traceKV */)
	}
	log.Infof(ctx, "clearing data for table %d", table.ID)

	tableKey := roachpb.RKey(keys.MakeTablePrefix(uint32(table.ID)))
	tableSpan := roachpb.RSpan{Key: tableKey, EndKey: tableKey.PrefixEnd()}

	// ClearRange requests lays down RocksDB range deletion tombstones that have
	// serious performance implications (#24029). The logic below attempts to
	// bound the number of tombstones in one store by sending the ClearRange
	// requests to each range in the table in small, sequential batches rather
	// than letting DistSender send them all in parallel, to hopefully give the
	// compaction queue time to compact the range tombstones away in between
	// requests.
	//
	// As written, this approach has several deficiencies. It does not actually
	// wait for the compaction queue to compact the tombstones away before
	// sending the next request. It is likely insufficient if multiple DROP
	// TABLEs are in flight at once. It does not save its progress in case the
	// coordinator goes down. These deficiencies could be addressed, but this code
	// was originally a stopgap to avoid the range tombstone performance hit. The
	// RocksDB range tombstone implementation has since been improved and the
	// performance implications of many range tombstones has been reduced
	// dramatically making this simplistic throttling sufficient.

	// These numbers were chosen empirically for the clearrange roachtest and
	// could certainly use more tuning.
	const batchSize = 100
	const waitTime = 500 * time.Millisecond

	var n int
	lastKey := tableSpan.Key
	ri := kv.NewRangeIterator(distSender)
	timer := timeutil.NewTimer()
	defer timer.Stop()

	for ri.Seek(ctx, tableSpan.Key, kv.Ascending); ; ri.Next(ctx) {
		if !ri.Valid() {
			return ri.Error().GoError()
		}

		if n++; n >= batchSize || !ri.NeedAnother(tableSpan) {
			endKey := ri.Desc().EndKey
			if tableSpan.EndKey.Less(endKey) {
				endKey = tableSpan.EndKey
			}
			var b client.Batch
			b.AddRawRequest(&roachpb.ClearRangeRequest{
				RequestHeader: roachpb.RequestHeader{
					Key:    lastKey.AsRawKey(),
					EndKey: endKey.AsRawKey(),
				},
			})
			log.VEventf(ctx, 2, "ClearRange %s - %s", lastKey, endKey)
			if err := db.Run(ctx, &b); err != nil {
				return err
			}
			n = 0
			lastKey = endKey
			timer.Reset(waitTime)
			select {
			case <-timer.C:
				timer.Read = true
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if !ri.NeedAnother(tableSpan) {
			break
		}
	}

	return nil
}
