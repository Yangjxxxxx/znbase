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

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/storage"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

const (
	// systemLogGCPeriod is the period for running gc on systemlog tables.
	systemLogGCPeriod = 10 * time.Minute
)

var (
	// rangeLogTTL is the TTL for rows in system.rangelog. If non zero, range log
	// entries are periodically garbage collected.
	rangeLogTTL = settings.RegisterDurationSetting(
		"server.rangelog.ttl",
		fmt.Sprintf(
			"if nonzero, range log entries older than this duration are deleted every %s. "+
				"Should not be lowered below 24 hours.",
			systemLogGCPeriod,
		),
		30*24*time.Hour, // 30 days
	)

	// eventLogTTL is the TTL for rows in system.eventlog. If non zero, event log
	// entries are periodically garbage collected.
	eventLogTTL = settings.RegisterValidatedDurationSetting(
		"server.eventlog.ttl",
		fmt.Sprintf(
			"if nonzero, event log entries older than this duration are deleted every %s. "+
				"Should not be lowered below 24 hours.",
			systemLogGCPeriod,
		),
		90*24*time.Hour, // 90 days
		func(v time.Duration) error {
			if v < 24*time.Hour {
				return errors.Errorf("this duration should not be lowered below 24 hours.")
			}
			return nil
		},
	)
)

// gcSystemLog deletes entries in the given system log table between
// timestampLowerBound and timestampUpperBound if the server is the lease holder
// for range 1.
// Leaseholder constraint is present so that only one node in the cluster
// performs gc.
// The system log table is expected to have a "timestamp" column.
// It returns the timestampLowerBound to be used in the next iteration, number
// of rows affected and error (if any).
func (s *Server) gcSystemLog(
	ctx context.Context, table string, timestampLowerBound, timestampUpperBound time.Time,
) (time.Time, int64, error) {
	var totalRowsAffected int64
	repl, err := s.node.stores.GetReplicaForRangeID(roachpb.RangeID(1))
	if err != nil {
		return timestampLowerBound, 0, nil
	}

	if !repl.IsFirstRange() || !repl.OwnsValidLease(s.clock.Now()) {
		return timestampLowerBound, 0, nil
	}

	deleteStmt := fmt.Sprintf(
		`SELECT count(1), max(timestamp) FROM 
[DELETE FROM system.%s WHERE timestamp >= $1 AND timestamp <= $2 LIMIT 1000 RETURNING timestamp]`,
		table,
	)

	for {
		var rowsAffected int64
		err := s.db.Txn(ctx, func(ctx context.Context, txn *client.Txn) error {
			var err error
			row, err := s.internalExecutor.QueryRow(
				ctx,
				table+"-gc",
				txn,
				deleteStmt,
				timestampLowerBound,
				timestampUpperBound,
			)
			if err != nil {
				return err
			}

			if row != nil {
				rowCount, ok := row[0].(*tree.DInt)
				if !ok {
					return errors.Errorf("row count is of unknown type %T", row[0])
				}
				if rowCount == nil {
					return errors.New("error parsing row count")
				}
				rowsAffected = int64(*rowCount)

				if rowsAffected > 0 {
					maxTimestamp, ok := row[1].(*tree.DTimestamp)
					if !ok {
						return errors.Errorf("timestamp is of unknown type %T", row[1])
					}
					if maxTimestamp == nil {
						return errors.New("error parsing timestamp")
					}
					timestampLowerBound = maxTimestamp.Time
				}
			}
			return nil
		})
		totalRowsAffected += rowsAffected
		if err != nil {
			return timestampLowerBound, totalRowsAffected, err
		}

		if rowsAffected == 0 {
			return timestampUpperBound, totalRowsAffected, nil
		}
	}
}

// systemLogGCConfig has configurations for gc of systemlog.
type systemLogGCConfig struct {
	// ttl is the time to live for rows in systemlog table.
	ttl *settings.DurationSetting
	// timestampLowerBound is the timestamp below which rows are gc'ed.
	// It is maintained to avoid hitting tombstones during gc and is updated
	// after every gc run.
	timestampLowerBound time.Time
}

// startSystemLogsGC starts a worker which periodically GCs system.rangelog
// and system.eventlog.
// The TTLs for each of these logs is retrieved from cluster settings.
func (s *Server) startSystemLogsGC(ctx context.Context) {
	systemLogsToGC := map[string]*systemLogGCConfig{
		"rangelog": {
			ttl:                 rangeLogTTL,
			timestampLowerBound: timeutil.Unix(0, 0),
		},
		"eventlog": {
			ttl:                 eventLogTTL,
			timestampLowerBound: timeutil.Unix(0, 0),
		},
	}

	s.stopper.RunWorker(ctx, func(ctx context.Context) {
		period := systemLogGCPeriod
		if storeKnobs, ok := s.cfg.TestingKnobs.Store.(*storage.StoreTestingKnobs); ok && storeKnobs.SystemLogsGCPeriod != 0 {
			period = storeKnobs.SystemLogsGCPeriod
		}

		t := time.NewTicker(period)
		defer t.Stop()

		for {
			select {
			case <-t.C:
				for table, gcConfig := range systemLogsToGC {
					ttl := gcConfig.ttl.Get(&s.cfg.Settings.SV)
					if ttl > 0 {
						timestampUpperBound := timeutil.Unix(0, s.clock.PhysicalNow()-int64(ttl))
						newTimestampLowerBound, rowsAffected, err := s.gcSystemLog(
							ctx,
							table,
							gcConfig.timestampLowerBound,
							timestampUpperBound,
						)
						if err != nil {
							log.Warningf(
								ctx,
								"error garbage collecting %s: %v",
								table,
								err,
							)
						} else {
							gcConfig.timestampLowerBound = newTimestampLowerBound
							if log.V(1) {
								log.Infof(ctx, "garbage collected %d rows from %s", rowsAffected, table)
							}
						}
					}
				}

				if storeKnobs, ok := s.cfg.TestingKnobs.Store.(*storage.StoreTestingKnobs); ok && storeKnobs.SystemLogsGCGCDone != nil {
					select {
					case storeKnobs.SystemLogsGCGCDone <- struct{}{}:
					case <-s.stopper.ShouldStop():
						// Test has finished.
						return
					}
				}
			case <-s.stopper.ShouldStop():
				return
			}
		}
	})
}
