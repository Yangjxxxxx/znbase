// Copyright 2020  The Cockroach Authors.

package gcjob

import (
	"context"
	"math"
	"time"

	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/gossip"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/encoding"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

// refreshTables updates the status of tables/indexes that are waiting to be
// GC'd.
// It returns whether or not any index/table has expired and the duration until
// the next index/table expires.
func refreshTables(
	ctx context.Context,
	execCfg *sql.ExecutorConfig,
	tableIDs []sqlbase.ID,
	tableDropTimes map[sqlbase.ID]int64,
	indexDropTimes map[sqlbase.IndexID]int64,
	jobID int64,
	progress *jobspb.SchemaChangeGCProgress,
) (expired bool, earliestDeadline time.Time) {
	earliestDeadline = timeutil.Unix(0, math.MaxInt64)

	for _, tableID := range tableIDs {
		tableHasExpiredElem, deadline := updateStatusForGCElements(
			ctx,
			execCfg,
			tableID,
			tableDropTimes, indexDropTimes,
			progress,
		)
		if tableHasExpiredElem {
			expired = true
		}
		if deadline.Before(earliestDeadline) {
			earliestDeadline = deadline
		}
	}

	if expired {
		persistProgress(ctx, execCfg, jobID, progress)
	}

	return expired, earliestDeadline
}

// updateStatusForGCElements updates the status for indexes on this table if any
// are waiting for GC. If the table is waiting for GC then the status of the table
// will be updated.
// It returns whether any indexes or the table have expired as well as the time
// until the next index expires if there are any more to drop.
func updateStatusForGCElements(
	ctx context.Context,
	execCfg *sql.ExecutorConfig,
	tableID sqlbase.ID,
	tableDropTimes map[sqlbase.ID]int64,
	indexDropTimes map[sqlbase.IndexID]int64,
	progress *jobspb.SchemaChangeGCProgress,
) (expired bool, timeToNextTrigger time.Time) {
	defTTL := config.DefaultZoneConfig().GC.TTLSeconds
	cfg := execCfg.Gossip.GetSystemConfig()

	earliestDeadline := timeutil.Unix(0, int64(math.MaxInt64))

	if err := execCfg.DB.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
		table, err := sqlbase.GetTableDescFromID(ctx, txn, tableID)
		if err != nil {
			return err
		}

		zoneCfg, placeholder, _, err := sql.ZoneConfigHook(cfg, uint32(tableID))
		if err != nil {
			log.Errorf(ctx, "zone config for desc: %d, err = %+v", tableID, err)
			return nil
		}
		tableTTL := getTableTTL(defTTL, zoneCfg)
		if placeholder == nil {
			placeholder = zoneCfg
		}

		// Update the status of the table if the table was dropped.
		if table.Dropped() {
			deadline := updateTableStatus(ctx, int64(tableTTL), table, tableDropTimes, progress)
			if timeutil.Until(deadline) < 0 {
				expired = true
			} else if deadline.Before(earliestDeadline) {
				earliestDeadline = deadline
			}
		}

		// Update the status of any indexes waiting for GC.
		indexesExpired, deadline := updateIndexesStatus(ctx, tableTTL, table, placeholder, indexDropTimes, progress)
		if indexesExpired {
			expired = true
		}
		if deadline.Before(earliestDeadline) {
			earliestDeadline = deadline
		}

		return nil
	}); err != nil {
		log.Warningf(ctx, "error while calculating GC time for table %d, err: %+v", tableID, err)
		return false, earliestDeadline
	}

	return expired, earliestDeadline
}

// updateTableStatus sets the status the table to DELETING if the GC TTL has
// expired.
func updateTableStatus(
	ctx context.Context,
	ttlSeconds int64,
	table *sqlbase.TableDescriptor,
	tableDropTimes map[sqlbase.ID]int64,
	progress *jobspb.SchemaChangeGCProgress,
) time.Time {
	deadline := timeutil.Unix(0, int64(math.MaxInt64))
	lifetime := ttlSeconds * time.Second.Nanoseconds()

	for i, t := range progress.Tables {
		droppedTable := &progress.Tables[i]
		if droppedTable.ID != table.ID || droppedTable.Status != jobspb.SchemaChangeGCProgress_WAITING_FOR_GC {
			continue
		}

		deadlineNanos := tableDropTimes[t.ID] + lifetime
		deadline = timeutil.Unix(0, deadlineNanos)

		lifetime := timeutil.Until(deadline)
		if lifetime < 0 {
			if log.V(2) {
				log.Infof(ctx, "detected expired table %d", t.ID)
			}
			droppedTable.Status = jobspb.SchemaChangeGCProgress_DELETING
		} else {
			if log.V(2) {
				log.Infof(ctx, "table %d still has %+v until GC", t.ID, lifetime)
			}
		}
		break
	}

	return deadline
}

// updateIndexesStatus updates the status on every index that is waiting for GC
// TTL in this table.
// It returns whether any indexes have expired and the timestamp of when another
// index should be GC'd, if any, otherwise MaxInt.
func updateIndexesStatus(
	ctx context.Context,
	tableTTL int32,
	table *sqlbase.TableDescriptor,
	placeholder *config.ZoneConfig,
	indexDropTimes map[sqlbase.IndexID]int64,
	progress *jobspb.SchemaChangeGCProgress,
) (expired bool, soonestDeadline time.Time) {
	// Update the deadline for indexes that are being dropped, if any.
	soonestDeadline = timeutil.Unix(0, int64(math.MaxInt64))
	for i := 0; i < len(progress.Indexes); i++ {
		idxProgress := &progress.Indexes[i]

		ttlSeconds := getIndexTTL(tableTTL, placeholder, idxProgress.IndexID)

		deadlineNanos := indexDropTimes[idxProgress.IndexID] + int64(ttlSeconds)*time.Second.Nanoseconds()
		deadline := timeutil.Unix(0, deadlineNanos)
		lifetime := time.Until(deadline)
		if lifetime > 0 {
			if log.V(2) {
				log.Infof(ctx, "index %d from table %d still has %+v until GC", idxProgress.IndexID, table.ID, lifetime)
			}
		}
		if lifetime < 0 {
			expired = true
			if log.V(2) {
				log.Infof(ctx, "detected expired index %d from table %d", idxProgress.IndexID, table.ID)
			}
			idxProgress.Status = jobspb.SchemaChangeGCProgress_DELETING
		} else if deadline.Before(soonestDeadline) {
			soonestDeadline = deadline
		}
	}
	return expired, soonestDeadline
}

// Helpers.

func getIndexTTL(tableTTL int32, placeholder *config.ZoneConfig, indexID sqlbase.IndexID) int32 {
	ttlSeconds := tableTTL
	if placeholder != nil {
		if subzone := placeholder.GetSubzone(
			uint32(indexID), ""); subzone != nil && subzone.Config.GC != nil {
			ttlSeconds = subzone.Config.GC.TTLSeconds
		}
	}
	return ttlSeconds
}

func getTableTTL(defTTL int32, zoneCfg *config.ZoneConfig) int32 {
	ttlSeconds := defTTL
	if zoneCfg != nil {
		ttlSeconds = zoneCfg.GC.TTLSeconds
	}
	return ttlSeconds
}

// getUpdatedTables returns any tables who's TTL may have changed based on
// gossip changes. The zoneCfgFilter watches for changes in the zone config
// table and the descTableFilter watches for changes in the descriptor table.
func getUpdatedTables(
	ctx context.Context,
	cfg *config.SystemConfig,
	zoneCfgFilter gossip.SystemConfigDeltaFilter,
	descTableFilter gossip.SystemConfigDeltaFilter,
	details *jobspb.SchemaChangeGCDetails,
	progress *jobspb.SchemaChangeGCProgress,
) []sqlbase.ID {
	tablesToGC := make(map[sqlbase.ID]*jobspb.SchemaChangeGCProgress_TableProgress)
	for _, table := range progress.Tables {
		tablesToGC[table.ID] = &table
	}

	// Check to see if the zone cfg or any of the descriptors have been modified.
	var tablesToCheck []sqlbase.ID
	zoneCfgModified := false
	zoneCfgFilter.ForModified(cfg, func(kv roachpb.KeyValue) {
		zoneCfgModified = true
	})
	if zoneCfgModified {
		// If any zone config was modified, check all the remaining tables.
		return getAllTablesWaitingForGC(details, progress)
	}

	descTableFilter.ForModified(cfg, func(kv roachpb.KeyValue) {
		// Attempt to unmarshal config into a table/database descriptor.
		var descriptor sqlbase.Descriptor
		if err := kv.Value.GetProto(&descriptor); err != nil {
			log.Warningf(ctx, "%s: unable to unmarshal descriptor %v", kv.Key, kv.Value)
			return
		}
		switch union := descriptor.Union.(type) {
		case *sqlbase.Descriptor_Table:
			table := union.Table
			if ok := table.MaybeFillInDescriptor(); !ok {
				log.Errorf(ctx, "%s: failed to fill in table descriptor %v", kv.Key, table)
				return
			}
			if err := table.ValidateTable(nil); err != nil {
				log.Errorf(ctx, "%s: received invalid table descriptor: %s. Desc: %v",
					kv.Key, err, table,
				)
				return
			}
			// Check the expiration again for this descriptor if it is waiting for GC.
			if table, ok := tablesToGC[table.ID]; ok {
				if table.Status == jobspb.SchemaChangeGCProgress_WAITING_FOR_GC {
					tablesToCheck = append(tablesToCheck, table.ID)
				}
			}

		case *sqlbase.Descriptor_Database:
			// We don't care if the database descriptor changes as it doesn't have any
			// effect on the TTL of it's tables.
		}
	})

	return tablesToCheck
}

// setupConfigWatchers returns a filter to watch zone config changes, a filter
// to watch descriptor changes and a channel that is notified when there are
// changes.
func setupConfigWatchers(
	execCfg *sql.ExecutorConfig,
) (gossip.SystemConfigDeltaFilter, gossip.SystemConfigDeltaFilter, <-chan struct{}) {
	k := keys.MakeTablePrefix(uint32(keys.ZonesTableID))
	k = encoding.EncodeUvarintAscending(k, uint64(keys.ZonesTablePrimaryIndexID))
	zoneCfgFilter := gossip.MakeSystemConfigDeltaFilter(k)
	descKeyPrefix := keys.MakeTablePrefix(uint32(sqlbase.DescriptorTable.ID))
	descTableFilter := gossip.MakeSystemConfigDeltaFilter(descKeyPrefix)
	gossipUpdateC := execCfg.Gossip.RegisterSystemConfigChannel()
	return zoneCfgFilter, descTableFilter, gossipUpdateC
}
