// Copyright 2020  The Cockroach Authors.

package gcjob

import (
	"context"

	"github.com/znbasedb/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/log"
)

// gcIndexes find the indexes that need to be GC'd, GC's them, and then updates
// the cleans up the table descriptor, zone configs and job payload to indicate
// the work that it did.
func gcIndexes(
	ctx context.Context,
	execCfg *sql.ExecutorConfig,
	parentID sqlbase.ID,
	progress *jobspb.SchemaChangeGCProgress,
) error {
	droppedIndexes := progress.Indexes
	if log.V(2) {
		log.Infof(ctx, "GC is being considered on table %d for indexes indexes: %+v", parentID, droppedIndexes)
	}

	var parentTable *sqlbase.TableDescriptor
	if err := execCfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
		var err error
		parentTable, err = sqlbase.GetTableDescFromID(ctx, txn, parentID)
		return err
	}); err != nil {
		return errors.Wrapf(err, "fetching parent table %d", parentID)
	}

	for _, index := range droppedIndexes {
		if index.Status != jobspb.SchemaChangeGCProgress_DELETING {
			continue
		}

		indexDesc := sqlbase.IndexDescriptor{ID: index.IndexID}
		if err := clearIndex(ctx, execCfg.DB, parentTable, indexDesc); err != nil {
			return errors.Wrapf(err, "clearing index %d", indexDesc.ID)
		}

		// All the data chunks have been removed. Now also removed the
		// zone configs for the dropped indexes, if any.
		if err := execCfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
			return sql.RemoveIndexZoneConfigs(ctx, txn, execCfg, parentTable.GetID(), []sqlbase.IndexDescriptor{indexDesc})
		}); err != nil {
			return errors.Wrapf(err, "removing index %d zone configs", indexDesc.ID)
		}

		if err := completeDroppedIndex(ctx, execCfg, parentTable, index.IndexID, progress); err != nil {
			return err
		}
	}

	return nil
}

// clearIndexes issues Clear Range requests over all specified indexes.
func clearIndex(
	ctx context.Context,
	db *client.DB,
	tableDesc *sqlbase.TableDescriptor,
	index sqlbase.IndexDescriptor,
) error {
	log.Infof(ctx, "clearing index %d from table %d", index.ID, tableDesc.ID)
	if index.IsInterleaved() {
		return errors.Errorf("unexpected interleaved index %d", index.ID)
	}

	sp := tableDesc.IndexSpan(index.ID)

	// ClearRange cannot be run in a transaction, so create a
	// non-transactional batch to send the request.
	b := &client.Batch{}
	b.AddRawRequest(&roachpb.ClearRangeRequest{
		RequestHeader: roachpb.RequestHeader{
			Key:    sp.Key,
			EndKey: sp.EndKey,
		},
	})
	return db.Run(ctx, b)
}

// completeDroppedIndexes updates the mutations of the table descriptor to
// indicate that the index was dropped, as well as the job detail payload.
func completeDroppedIndex(
	ctx context.Context,
	execCfg *sql.ExecutorConfig,
	table *sqlbase.TableDescriptor,
	indexID sqlbase.IndexID,
	progress *jobspb.SchemaChangeGCProgress,
) error {
	if err := updateDescriptorGCMutations(ctx, execCfg, table, indexID); err != nil {
		return errors.Wrapf(err, "updating GC mutations")
	}

	markIndexGCed(ctx, indexID, progress)

	return nil
}
