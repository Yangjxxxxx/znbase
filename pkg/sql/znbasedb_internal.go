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
	"bytes"
	"context"
	gojson "encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/build"
	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/gossip"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/jobs"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/security"
	"github.com/znbasedb/znbase/pkg/security/useroption"
	"github.com/znbasedb/znbase/pkg/server/serverpb"
	"github.com/znbasedb/znbase/pkg/server/status/statuspb"
	"github.com/znbasedb/znbase/pkg/server/telemetry"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgerror"
	"github.com/znbasedb/znbase/pkg/sql/sem/builtins"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sem/types"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage/storagepb"
	"github.com/znbasedb/znbase/pkg/util/json"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/protoutil"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

const znbaseInternalName = "zbdb_internal"

//LeaseInfoRequestConcurrentCount 用于控制查询zbdb_internal.ranges时构建LeaseInfoRequest并执行时的并发数量
const LeaseInfoRequestConcurrentCount = 128

// Naming convention:
// - if the response is served from memory, prefix with node_
// - if the response is served via a kv request, prefix with kv_
// - if the response is not from kv requests but is cluster-wide (i.e. the
//    answer isn't specific to the sql connection being used, prefix with cluster_.
//
// Adding something new here will require an update to `pkg/cli` for inclusion in
// a `debug zip`; the unit tests will guide you.
//
// Many existing tables don't follow the conventions above, but please apply
// them to future additions.
var znbaseInternal = virtualSchema{
	name: znbaseInternalName,
	tableDefs: map[sqlbase.ID]virtualSchemaDef{
		sqlbase.CrdbInternalBackwardDependenciesTableID: znbaseInternalBackwardDependenciesTable,
		sqlbase.CrdbInternalBuildInfoTableID:            znbaseInternalBuildInfoTable,
		sqlbase.CrdbInternalBuiltinFunctionsTableID:     znbaseInternalBuiltinFunctionsTable,
		sqlbase.CrdbInternalClusterQueriesTableID:       znbaseInternalClusterQueriesTable,
		sqlbase.CrdbInternalClusterSessionsTableID:      znbaseInternalClusterSessionsTable,
		sqlbase.CrdbInternalClusterSettingsTableID:      znbaseInternalClusterSettingsTable,
		sqlbase.CrdbInternalCreateStmtsTableID:          znbaseInternalCreateStmtsTable,
		sqlbase.CrdbInternalFeatureUsageID:              znbaseInternalFeatureUsage,
		sqlbase.CrdbInternalForwardDependenciesTableID:  znbaseInternalForwardDependenciesTable,
		sqlbase.CrdbInternalFunctionPrivilegesID:        znbaseInternalFunctionPrivileges,
		sqlbase.CrdbInternalGossipNodesTableID:          znbaseInternalGossipNodesTable,
		sqlbase.CrdbInternalGossipAlertsTableID:         znbaseInternalGossipAlertsTable,
		sqlbase.CrdbInternalGossipLivenessTableID:       znbaseInternalGossipLivenessTable,
		sqlbase.CrdbInternalGossipNetworkTableID:        znbaseInternalGossipNetworkTable,
		sqlbase.CrdbInternalIndexColumnsTableID:         znbaseInternalIndexColumnsTable,
		sqlbase.CrdbInternalJobsTableID:                 znbaseInternalJobsTable,
		sqlbase.CrdbInternalKVNodeStatusTableID:         znbaseInternalKVNodeStatusTable,
		sqlbase.CrdbInternalKVStoreStatusTableID:        znbaseInternalKVStoreStatusTable,
		sqlbase.CrdbInternalLeasesTableID:               znbaseInternalLeasesTable,
		sqlbase.CrdbInternalLocalQueriesTableID:         znbaseInternalLocalQueriesTable,
		sqlbase.CrdbInternalLocalSessionsTableID:        znbaseInternalLocalSessionsTable,
		sqlbase.CrdbInternalLocalMetricsTableID:         znbaseInternalLocalMetricsTable,
		sqlbase.CrdbInternalPartitionsTableID:           znbaseInternalPartitionsTable,
		sqlbase.CrdbInternalPartitionViewsTableID:       znbaseInternalPartitionViewsTable,
		sqlbase.CrdbInternalPredefinedCommentsTableID:   znbaseInternalPredefinedCommentsTable,
		sqlbase.CrdbInternalRangesNoLeasesTableID:       znbaseInternalRangesNoLeasesTable,
		sqlbase.CrdbInternalRangesViewID:                znbaseInternalRangesView,
		sqlbase.CrdbInternalRuntimeInfoTableID:          znbaseInternalRuntimeInfoTable,
		sqlbase.CrdbInternalSavePointStatusTableID:      znbaseInternalSavepointStatusTable,
		sqlbase.CrdbInternalSchemaChangesTableID:        znbaseInternalSchemaChangesTable,
		sqlbase.CrdbInternalDatabasesTableID:            znbaseInternalDatabasesTable,
		sqlbase.CrdbInternalSessionTraceTableID:         znbaseInternalSessionTraceTable,
		sqlbase.CrdbInternalSessionVariablesTableID:     znbaseInternalSessionVariablesTable,
		sqlbase.CrdbInternalStmtStatsTableID:            znbaseInternalStmtStatsTable,
		sqlbase.CrdbInternalTableColumnsTableID:         znbaseInternalTableColumnsTable,
		sqlbase.CrdbInternalTableIndexesTableID:         znbaseInternalTableIndexesTable,
		sqlbase.CrdbInternalTablesTableLastStatsID:      znbaseInternalTablesTableLastStats,
		sqlbase.CrdbInternalTablesTableID:               znbaseInternalTablesTable,
		sqlbase.CrdbInternalZonesTableID:                znbaseInternalZonesTable,
	},
	validWithNoDatabaseContext: true,
}

// TODO(tbg): prefix with node_.
var znbaseInternalBuildInfoTable = virtualSchemaTable{
	comment: `detailed identification strings (RAM, local node only)`,
	schema: `
CREATE TABLE zbdb_internal.node_build_info (
  node_id INT NOT NULL,
  field   STRING NOT NULL,
  value   STRING NOT NULL
)`,
	populate: func(_ context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		execCfg := p.ExecCfg()
		nodeID := tree.NewDInt(tree.DInt(int64(execCfg.NodeID.Get())))

		info := build.GetInfo()
		for k, v := range map[string]string{
			"Name":         "ZNBaseDB",
			"ClusterID":    execCfg.ClusterID().String(),
			"Organization": execCfg.Organization(),
			"Build":        info.Short(),
			"Version":      info.Tag,
			"Channel":      info.Channel,
		} {
			if err := addRow(
				nodeID,
				tree.NewDString(k),
				tree.NewDString(v),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// TODO(tbg): prefix with node_.
var znbaseInternalRuntimeInfoTable = virtualSchemaTable{
	comment: `server parameters, useful to construct connection URLs (RAM, local node only)`,
	schema: `
CREATE TABLE zbdb_internal.node_runtime_info (
  node_id   INT NOT NULL,
  component STRING NOT NULL,
  field     STRING NOT NULL,
  value     STRING NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.RequireAdminRole(ctx, "access the node runtime information"); err != nil {
			return err
		}

		node := p.ExecCfg().NodeInfo

		nodeID := tree.NewDInt(tree.DInt(int64(node.NodeID.Get())))
		dbURL, err := node.PGURL(url.User(security.RootUser))
		if err != nil {
			return err
		}

		for _, item := range []struct {
			component string
			url       *url.URL
		}{
			{"DB", dbURL}, {"UI", node.AdminURL()},
		} {
			var user string
			if item.url.User != nil {
				user = item.url.User.String()
			}
			host, port, err := net.SplitHostPort(item.url.Host)
			if err != nil {
				return err
			}
			for _, kv := range [][2]string{
				{"URL", item.url.String()},
				{"Scheme", item.url.Scheme},
				{"User", user},
				{"Host", host},
				{"Port", port},
				{"URI", item.url.RequestURI()},
			} {
				k, v := kv[0], kv[1]
				if err := addRow(
					nodeID,
					tree.NewDString(item.component),
					tree.NewDString(k),
					tree.NewDString(v),
				); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// TODO(tbg): prefix with kv_.
var znbaseInternalTablesTable = virtualSchemaTable{
	comment: `table descriptors accessible by current user, including non-public and virtual (KV scan; expensive!)`,
	schema: `
CREATE TABLE zbdb_internal.tables (
  table_id                 INT NOT NULL,
  parent_id                INT NOT NULL,
  name                     STRING NOT NULL,
  database_name            STRING,
  version                  INT NOT NULL,
  mod_time                 TIMESTAMP NOT NULL,
  mod_time_logical         DECIMAL NOT NULL,
  format_version           STRING NOT NULL,
  state                    STRING NOT NULL,
  sc_lease_node_id         INT,
  sc_lease_expiration_time TIMESTAMP,
  drop_time                TIMESTAMP,
  audit_mode               STRING NOT NULL,
  schema_name              STRING NOT NULL,
  locate_in                STRING,
  lease_in                 STRING
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		descs, err := p.Tables().getAllDescriptors(ctx, p.txn)
		if err != nil {
			return err
		}
		dbNames := make(map[sqlbase.ID]string)
		schemas := make(map[sqlbase.ID]*sqlbase.SchemaDescriptor)
		// Record database descriptors for name lookups.
		for _, desc := range descs {
			db, ok := desc.(*sqlbase.DatabaseDescriptor)
			if ok {
				dbNames[db.ID] = db.Name
			} else {
				sc, ok := desc.(*sqlbase.SchemaDescriptor)
				if ok {
					schemas[sc.ID] = sc
				}
			}
		}

		addDesc := func(table *sqlbase.TableDescriptor, dbName tree.Datum, scName string) error {
			leaseNodeDatum := tree.DNull
			leaseExpDatum := tree.DNull
			if table.Lease != nil {
				leaseNodeDatum = tree.NewDInt(tree.DInt(int64(table.Lease.NodeID)))
				leaseExpDatum = tree.MakeDTimestamp(
					timeutil.Unix(0, table.Lease.ExpirationTime), time.Nanosecond,
				)
			}
			dropTimeDatum := tree.DNull
			if table.DropTime != 0 {
				dropTimeDatum = tree.MakeDTimestamp(
					timeutil.Unix(0, table.DropTime), time.Nanosecond,
				)
			}
			locateIn, leaseIn := checkSpaceNull(table.LocateSpaceName)
			return addRow(
				tree.NewDInt(tree.DInt(int64(table.ID))),
				tree.NewDInt(tree.DInt(int64(table.GetParentID()))),
				tree.NewDString(table.Name),
				dbName,
				tree.NewDInt(tree.DInt(int64(table.Version))),
				tree.MakeDTimestamp(timeutil.Unix(0, table.ModificationTime.WallTime), time.Microsecond),
				tree.TimestampToDecimal(table.ModificationTime),
				tree.NewDString(table.FormatVersion.String()),
				tree.NewDString(table.State.String()),
				leaseNodeDatum,
				leaseExpDatum,
				dropTimeDatum,
				tree.NewDString(table.AuditMode.String()),
				tree.NewDString(scName),
				locateIn,
				leaseIn)
		}

		// Note: we do not use forEachTableDesc() here because we want to
		// include added and dropped descriptors.
		for _, desc := range descs {
			table, ok := desc.(*sqlbase.TableDescriptor)
			if !ok || p.CheckAnyPrivilege(ctx, table) != nil {
				continue
			}
			scName := "public"
			dbName := dbNames[table.GetParentID()]
			if dbName == "" {
				// The parent database was deleted. This is possible e.g. when
				// a database is dropped with CASCADE, and someone queries
				// this virtual table before the dropped table descriptors are
				// effectively deleted.
				sc, ok := schemas[table.GetParentID()]
				if !ok {
					scName = fmt.Sprintf("[%d]", table.GetParentID())
				} else {
					scName = sc.Name
					if table.Temporary {
						scName = fmt.Sprintf("[%d]", table.GetParentID())
					}
					dbName = dbNames[sc.ParentID]
					if dbName == "" {
						dbName = fmt.Sprintf("[%d]", sc.ParentID)
					}
				}
			}
			if err := addDesc(table, tree.NewDString(dbName), scName); err != nil {
				return err
			}
		}

		// Also add all the virtual descriptors.catconstants.CrdbInternalTablesTableLastStatsID:      crdbInternalTablesTableLastStats,
		vt := p.getVirtualTabler()
		vEntries := vt.getEntries()
		for _, virtSchemaName := range vt.getSchemaNames() {
			e := vEntries[virtSchemaName]
			for _, tName := range e.orderedDefNames {
				vTableEntry := e.defs[tName]
				if err := addDesc(vTableEntry.desc, tree.DNull, virtSchemaName); err != nil {
					return err
				}
			}
		}

		return nil
	},
}

// Display 展示分区名
func Display(spaces []string) string {
	if len(spaces) == 0 {
		return ""
	}
	space := strings.Join(spaces, ", ")
	space = "( " + space + " )"
	return space
}

// checkSpaceNull returns the locate information and lease information in spaceName
func checkSpaceNull(spaceName *roachpb.LocationValue) (locateIn, leaseIn *tree.DString) {
	locateIn = tree.NewDString("")
	leaseIn = tree.NewDString("")
	if spaceName != nil {
		locateIn = tree.NewDString(Display(spaceName.Spaces))
		leaseIn = tree.NewDString(Display(spaceName.Leases))
	}
	return locateIn, leaseIn
}

// TODO(tbg): prefix with kv_.
var znbaseInternalSchemaChangesTable = virtualSchemaTable{
	comment: `ongoing schema changes, across all descriptors accessible by current user (KV scan; expensive!)`,
	schema: `
CREATE TABLE zbdb_internal.schema_changes (
  table_id      INT NOT NULL,
  parent_id     INT NOT NULL,
  name          STRING NOT NULL,
  type          STRING NOT NULL,
  target_id     INT,
  target_name   STRING,
  state         STRING NOT NULL,
  direction     STRING NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		descs, err := p.Tables().getAllDescriptors(ctx, p.txn)
		if err != nil {
			return err
		}
		// Note: we do not use forEachTableDesc() here because we want to
		// include added and dropped descriptors.
		for _, desc := range descs {
			table, ok := desc.(*sqlbase.TableDescriptor)
			if !ok || p.CheckAnyPrivilege(ctx, table) != nil {
				continue
			}
			tableID := tree.NewDInt(tree.DInt(int64(table.ID)))
			parentID := tree.NewDInt(tree.DInt(int64(table.GetParentID())))
			tableName := tree.NewDString(table.Name)
			for _, mut := range table.Mutations {
				mutType := "UNKNOWN"
				targetID := tree.DNull
				targetName := tree.DNull
				switch d := mut.Descriptor_.(type) {
				case *sqlbase.DescriptorMutation_Column:
					mutType = "COLUMN"
					targetID = tree.NewDInt(tree.DInt(int64(d.Column.ID)))
					targetName = tree.NewDString(d.Column.Name)
				case *sqlbase.DescriptorMutation_Index:
					mutType = "INDEX"
					targetID = tree.NewDInt(tree.DInt(int64(d.Index.ID)))
					targetName = tree.NewDString(d.Index.Name)
				case *sqlbase.DescriptorMutation_Constraint:
					mutType = "CONSTRAINT VALIDATION"
					targetName = tree.NewDString(d.Constraint.Name)
				}
				if err := addRow(
					tableID,
					parentID,
					tableName,
					tree.NewDString(mutType),
					targetID,
					targetName,
					tree.NewDString(mut.State.String()),
					tree.NewDString(mut.Direction.String()),
				); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// TODO(tbg): prefix with node_.
var znbaseInternalLeasesTable = virtualSchemaTable{
	comment: `acquired table leases (RAM; local node only)`,
	schema: `
CREATE TABLE zbdb_internal.leases (
  node_id     INT NOT NULL,
  table_id    INT NOT NULL,
  name        STRING NOT NULL,
  parent_id   INT NOT NULL,
  expiration  TIMESTAMP NOT NULL,
  deleted     BOOL NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		leaseMgr := p.LeaseMgr()
		nodeID := tree.NewDInt(tree.DInt(int64(leaseMgr.execCfg.NodeID.Get())))

		leaseMgr.mu.Lock()
		defer leaseMgr.mu.Unlock()

		for tid, ts := range leaseMgr.mu.tables {
			tableID := tree.NewDInt(tree.DInt(int64(tid)))

			adder := func() error {
				ts.mu.Lock()
				defer ts.mu.Unlock()

				dropped := tree.MakeDBool(tree.DBool(ts.mu.dropped))

				for _, state := range ts.mu.active.data {
					if p.CheckAnyPrivilege(ctx, &state.TableDescriptor) != nil {
						continue
					}

					state.mu.Lock()
					lease := state.mu.lease
					state.mu.Unlock()
					if lease == nil {
						continue
					}
					if err := addRow(
						nodeID,
						tableID,
						tree.NewDString(state.Name),
						tree.NewDInt(tree.DInt(int64(state.GetParentID()))),
						&lease.expiration,
						dropped,
					); err != nil {
						return err
					}
				}
				return nil
			}

			if err := adder(); err != nil {
				return err
			}
		}
		return nil
	},
}

func tsOrNull(micros int64) tree.Datum {
	if micros == 0 {
		return tree.DNull
	}
	ts := timeutil.Unix(0, micros*time.Microsecond.Nanoseconds())
	return tree.MakeDTimestamp(ts, time.Microsecond)
}

// TODO(tbg): prefix with kv_.
var znbaseInternalJobsTable = virtualSchemaTable{
	comment: `decoded job metadata from system.jobs (KV scan)`,
	schema: `
CREATE TABLE zbdb_internal.jobs (
	job_id             		INT,
	job_type           		STRING,
	description        		STRING,
	statement          		STRING,
	user_name          		STRING,
	descriptor_ids     		INT[],
	status             		STRING,
	running_status     		STRING,
	created            		TIMESTAMP,
	started            		TIMESTAMP,
	finished           		TIMESTAMP,
	modified           		TIMESTAMP,
	fraction_completed 		FLOAT,
	high_water_timestamp	DECIMAL,
	error              		STRING,
	coordinator_id     		INT
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		query := `SELECT id, status, created, payload, progress FROM system.jobs`
		rows, _ /* cols */, err :=
			p.ExtendedEvalContext().ExecCfg.InternalExecutor.QueryWithUser(
				ctx, "znbase-internal-jobs-table", p.txn,
				p.SessionData().User, query)
		if err != nil {
			return err
		}

		for _, r := range rows {
			id, status, created, payloadBytes, progressBytes := r[0], r[1], r[2], r[3], r[4]

			var jobType, description, statement, username, descriptorIDs, started, runningStatus,
				finished, modified, fractionCompleted, highWaterTimestamp, errorStr, leaseNode = tree.DNull,
				tree.DNull, tree.DNull, tree.DNull, tree.DNull, tree.DNull, tree.DNull, tree.DNull,
				tree.DNull, tree.DNull, tree.DNull, tree.DNull, tree.DNull

			// Extract data from the payload.
			payload, err := jobs.UnmarshalPayload(payloadBytes)
			if err != nil {
				errorStr = tree.NewDString(fmt.Sprintf("error decoding payload: %v", err))
			} else {
				jobType = tree.NewDString(payload.Type().String())
				description = tree.NewDString(payload.Description)
				statement = tree.NewDString(payload.Statement)
				username = tree.NewDString(payload.Username)
				descriptorIDsArr := tree.NewDArray(types.Int)
				for _, descID := range payload.DescriptorIDs {
					if err := descriptorIDsArr.Append(tree.NewDInt(tree.DInt(int(descID)))); err != nil {
						return err
					}
				}
				descriptorIDs = descriptorIDsArr
				started = tsOrNull(payload.StartedMicros)
				finished = tsOrNull(payload.FinishedMicros)
				if payload.Lease != nil {
					leaseNode = tree.NewDInt(tree.DInt(payload.Lease.NodeID))
				}
				errorStr = tree.NewDString(payload.Error)
			}

			// Extract data from the progress field.
			if progressBytes != tree.DNull {
				progress, err := jobs.UnmarshalProgress(progressBytes)
				if err != nil {
					baseErr := ""
					if s, ok := errorStr.(*tree.DString); ok {
						baseErr = string(*s)
						if baseErr != "" {
							baseErr += "\n"
						}
					}
					errorStr = tree.NewDString(fmt.Sprintf("%serror decoding progress: %v", baseErr, err))
				} else {
					// Progress contains either fractionCompleted for traditional jobs,
					// or the highWaterTimestamp for change feeds.
					if highwater := progress.GetHighWater(); highwater != nil {
						highWaterTimestamp = tree.TimestampToDecimal(*highwater)
					} else {
						fractionCompleted = tree.NewDFloat(tree.DFloat(progress.GetFractionCompleted()))
					}
					modified = tsOrNull(progress.ModifiedMicros)

					if len(progress.RunningStatus) > 0 {
						if s, ok := status.(*tree.DString); ok {
							if jobs.Status(string(*s)) == jobs.StatusRunning {
								runningStatus = tree.NewDString(progress.RunningStatus)
							}
						}
					}
				}
			}

			// Report the data.
			if err := addRow(
				id,
				jobType,
				description,
				statement,
				username,
				descriptorIDs,
				status,
				runningStatus,
				created,
				started,
				finished,
				modified,
				fractionCompleted,
				highWaterTimestamp,
				errorStr,
				leaseNode,
			); err != nil {
				return err
			}
		}

		return nil
	},
}

type stmtList []stmtKey

func (s stmtList) Len() int {
	return len(s)
}
func (s stmtList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s stmtList) Less(i, j int) bool {
	return s[i].stmt < s[j].stmt
}

// znbaseInternalTableColumnsTable exposes the session savepoints (actually transaction savepoints but can select in implicit transaction).
var znbaseInternalSavepointStatusTable = virtualSchemaTable{
	comment: `savepoint status`,
	schema: `
CREATE TABLE zbdb_internal.savepoint_status (
  savepoint_name       STRING NOT NULL,
  is_initial_savepoint BOOL NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		for _, entry := range p.extendedEvalCtx.Tracing.ex.extraTxnState.savepoints {
			if err := addRow(
				tree.NewDString(string(entry.name)),
				tree.MakeDBool(tree.DBool(entry.kvToken.Initial())),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// TODO(tbg): prefix with node_.
var znbaseInternalStmtStatsTable = virtualSchemaTable{
	comment: `statement statistics (RAM; local node only)`,
	schema: `
CREATE TABLE zbdb_internal.node_statement_statistics (
  node_id             INT NOT NULL,
  application_name    STRING NOT NULL,
  flags               STRING NOT NULL,
  key                 STRING NOT NULL,
  anonymized          STRING,
  count               INT NOT NULL,
  first_attempt_count INT NOT NULL,
  max_retries         INT NOT NULL,
  last_error          STRING,
  rows_avg            FLOAT NOT NULL,
  rows_var            FLOAT NOT NULL,
  parse_lat_avg       FLOAT NOT NULL,
  parse_lat_var       FLOAT NOT NULL,
  plan_lat_avg        FLOAT NOT NULL,
  plan_lat_var        FLOAT NOT NULL,
  run_lat_avg         FLOAT NOT NULL,
  run_lat_var         FLOAT NOT NULL,
  service_lat_avg     FLOAT NOT NULL,
  service_lat_var     FLOAT NOT NULL,
  overhead_lat_avg    FLOAT NOT NULL,
  overhead_lat_var    FLOAT NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.RequireAdminRole(ctx, "access application statistics"); err != nil {
			return err
		}

		sqlStats := p.statsCollector.SQLStats()
		if sqlStats == nil {
			return errors.New("cannot access sql statistics from this context")
		}

		leaseMgr := p.LeaseMgr()
		nodeID := tree.NewDInt(tree.DInt(int64(leaseMgr.execCfg.NodeID.Get())))

		// Retrieve the application names and sort them to ensure the
		// output is deterministic.
		var appNames []string
		sqlStats.Lock()
		for n := range sqlStats.apps {
			appNames = append(appNames, n)
		}
		sqlStats.Unlock()
		sort.Strings(appNames)

		// Now retrieve the application stats proper.
		for _, appName := range appNames {
			appStats := sqlStats.getStatsForApplication(appName)

			// Retrieve the statement keys and sort them to ensure the
			// output is deterministic.
			var stmtKeys stmtList
			appStats.Lock()
			for k := range appStats.stmts {
				stmtKeys = append(stmtKeys, k)
			}
			appStats.Unlock()

			// Now retrieve the per-stmt stats proper.
			for _, stmtKey := range stmtKeys {
				anonymized := tree.DNull
				anonStr, ok := scrubStmtStatKey(p.getVirtualTabler(), stmtKey.stmt)
				if ok {
					anonymized = tree.NewDString(anonStr)
				}

				s := appStats.getStatsForStmtWithKey(stmtKey, true /* createIfNonexistent */)

				s.Lock()
				errString := tree.DNull
				if s.data.SensitiveInfo.LastErr != "" {
					errString = tree.NewDString(s.data.SensitiveInfo.LastErr)
				}
				err := addRow(
					nodeID,
					tree.NewDString(appName),
					tree.NewDString(stmtKey.flags()),
					tree.NewDString(stmtKey.stmt),
					anonymized,
					tree.NewDInt(tree.DInt(s.data.Count)),
					tree.NewDInt(tree.DInt(s.data.FirstAttemptCount)),
					tree.NewDInt(tree.DInt(s.data.MaxRetries)),
					errString,
					tree.NewDFloat(tree.DFloat(s.data.NumRows.Mean)),
					tree.NewDFloat(tree.DFloat(s.data.NumRows.GetVariance(s.data.Count))),
					tree.NewDFloat(tree.DFloat(s.data.ParseLat.Mean)),
					tree.NewDFloat(tree.DFloat(s.data.ParseLat.GetVariance(s.data.Count))),
					tree.NewDFloat(tree.DFloat(s.data.PlanLat.Mean)),
					tree.NewDFloat(tree.DFloat(s.data.PlanLat.GetVariance(s.data.Count))),
					tree.NewDFloat(tree.DFloat(s.data.RunLat.Mean)),
					tree.NewDFloat(tree.DFloat(s.data.RunLat.GetVariance(s.data.Count))),
					tree.NewDFloat(tree.DFloat(s.data.ServiceLat.Mean)),
					tree.NewDFloat(tree.DFloat(s.data.ServiceLat.GetVariance(s.data.Count))),
					tree.NewDFloat(tree.DFloat(s.data.OverheadLat.Mean)),
					tree.NewDFloat(tree.DFloat(s.data.OverheadLat.GetVariance(s.data.Count))),
				)
				s.Unlock()
				if err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// znbaseInternalSessionTraceTable exposes the latest trace collected on this
// session (via SET TRACING={ON/OFF})
//
// TODO(tbg): prefix with node_.
var znbaseInternalSessionTraceTable = virtualSchemaTable{
	comment: `session trace accumulated so far (RAM)`,
	schema: `
CREATE TABLE zbdb_internal.session_trace (
  span_idx    INT NOT NULL,        -- The span's index.
  message_idx INT NOT NULL,        -- The message's index within its span.
  timestamp   TIMESTAMPTZ NOT NULL,-- The message's timestamp.
  duration    INTERVAL,            -- The span's duration. Set only on the first
                                   -- (dummy) message on a span.
                                   -- NULL if the span was not finished at the time
                                   -- the trace has been collected.
  operation   STRING NULL,         -- The span's operation.
  loc         STRING NOT NULL,     -- The file name / line number prefix, if any.
  tag         STRING NOT NULL,     -- The logging tag, if any.
  message     STRING NOT NULL,     -- The logged message.
  age         INTERVAL NOT NULL    -- The age of this message relative to the beginning of the trace.
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		rows, err := p.ExtendedEvalContext().Tracing.getSessionTrace()
		if err != nil {
			return err
		}
		for _, r := range rows {
			if err := addRow(r[:]...); err != nil {
				return err
			}
		}
		return nil
	},
}

// znbaseInternalClusterSettingsTable exposes the list of current
// cluster settings.
//
// TODO(tbg): prefix with node_.
var znbaseInternalClusterSettingsTable = virtualSchemaTable{
	comment: `cluster settings (RAM)`,
	schema: `
CREATE TABLE zbdb_internal.cluster_settings (
  variable      STRING NOT NULL,
  value         STRING NOT NULL,
  type          STRING NOT NULL,
  description   STRING NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.RequireAdminRole(ctx, "read zbdb_internal.cluster_settings"); err != nil {
			return err
		}
		for _, k := range settings.Keys() {
			setting, _ := settings.Lookup(k)
			if err := addRow(
				tree.NewDString(k),
				tree.NewDString(setting.String(&p.ExecCfg().Settings.SV)),
				tree.NewDString(setting.Typ()),
				tree.NewDString(setting.Description()),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// znbaseInternalSessionVariablesTable exposes the session variables.
var znbaseInternalSessionVariablesTable = virtualSchemaTable{
	comment: `session variables (RAM)`,
	schema: `
CREATE TABLE zbdb_internal.session_variables (
  variable STRING NOT NULL,
  value    STRING NOT NULL,
  hidden   BOOL   NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		for _, vName := range varNames {
			gen := varGen[vName]
			value := gen.Get(&p.extendedEvalCtx)
			if vName == `database` && value == "" {
				continue
			}
			if err := addRow(
				tree.NewDString(vName),
				tree.NewDString(value),
				tree.MakeDBool(tree.DBool(gen.Hidden)),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

const queriesSchemaPattern = `
CREATE TABLE zbdb_internal.%s (
  query_id         STRING,         -- the cluster-unique ID of the query
  node_id          INT NOT NULL,   -- the node on which the query is running
  user_name        STRING,         -- the user running the query
  start            TIMESTAMP,      -- the start time of the query
  query            STRING,         -- the SQL code of the query
  client_address   STRING,         -- the address of the client that issued the query
  application_name STRING,         -- the name of the application as per SET application_name
  distributed      BOOL,           -- whether the query is running distributed
  phase            STRING          -- the current execution phase
)`

func (p *planner) makeSessionsRequest(ctx context.Context) serverpb.ListSessionsRequest {
	req := serverpb.ListSessionsRequest{Username: p.SessionData().User}
	if err := p.RequireAdminRole(ctx, "list sessions"); err == nil {
		// The root user can see all sessions.
		req.Username = ""
	}
	return req
}

// znbaseInternalLocalQueriesTable exposes the list of running queries
// on the current node. The results are dependent on the current user.
var znbaseInternalLocalQueriesTable = virtualSchemaTable{
	comment: "running queries visible by current user (RAM; local node only)",
	schema:  fmt.Sprintf(queriesSchemaPattern, "node_queries"),
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		req := p.makeSessionsRequest(ctx)
		response, err := p.extendedEvalCtx.StatusServer.ListLocalSessions(ctx, &req)
		if err != nil {
			return err
		}
		return populateQueriesTable(ctx, addRow, response)
	},
}

// znbaseInternalClusterQueriesTable exposes the list of running queries
// on the entire cluster. The result is dependent on the current user.
var znbaseInternalClusterQueriesTable = virtualSchemaTable{
	comment: "running queries visible by current user (cluster RPC; expensive!)",
	schema:  fmt.Sprintf(queriesSchemaPattern, "cluster_queries"),
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		req := p.makeSessionsRequest(ctx)
		response, err := p.extendedEvalCtx.StatusServer.ListSessions(ctx, &req)
		if err != nil {
			return err
		}
		return populateQueriesTable(ctx, addRow, response)
	},
}

func populateQueriesTable(
	ctx context.Context, addRow func(...tree.Datum) error, response *serverpb.ListSessionsResponse,
) error {
	for _, session := range response.Sessions {
		for _, query := range session.ActiveQueries {
			isDistributedDatum := tree.DNull
			phase := strings.ToLower(query.Phase.String())
			if phase == "executing" {
				isDistributedDatum = tree.DBoolFalse
				if query.IsDistributed {
					isDistributedDatum = tree.DBoolTrue
				}
			}
			if err := addRow(
				tree.NewDString(query.ID),
				tree.NewDInt(tree.DInt(session.NodeID)),
				tree.NewDString(session.Username),
				tree.MakeDTimestamp(query.Start, time.Microsecond),
				tree.NewDString(query.Sql),
				tree.NewDString(session.ClientAddress),
				tree.NewDString(session.ApplicationName),
				isDistributedDatum,
				tree.NewDString(phase),
			); err != nil {
				return err
			}
		}
	}

	for _, rpcErr := range response.Errors {
		log.Warning(ctx, rpcErr.Message)
		if rpcErr.NodeID != 0 {
			// Add a row with this node ID, the error for query, and
			// nulls for all other columns.
			if err := addRow(
				tree.DNull,                             // query ID
				tree.NewDInt(tree.DInt(rpcErr.NodeID)), // node ID
				tree.DNull,                             // username
				tree.DNull,                             // start
				tree.NewDString("-- "+rpcErr.Message),  // query
				tree.DNull,                             // client_address
				tree.DNull,                             // application_name
				tree.DNull,                             // distributed
				tree.DNull,                             // phase
			); err != nil {
				return err
			}
		}
	}
	return nil
}

const sessionsSchemaPattern = `
CREATE TABLE zbdb_internal.%s (
  node_id            INT NOT NULL,   -- the node on which the query is running
  session_id         STRING,         -- the ID of the session
  user_name          STRING,         -- the user running the query
  client_address     STRING,         -- the address of the client that issued the query
  application_name   STRING,         -- the name of the application as per SET application_name
  active_queries     STRING,         -- the currently running queries as SQL
  last_active_query  STRING,         -- the query that finished last on this session as SQL
  session_start      TIMESTAMP,      -- the time when the session was opened
  oldest_query_start TIMESTAMP,      -- the time when the oldest query in the session was started
  kv_txn             STRING,         -- the ID of the current KV transaction
  alloc_bytes        INT,            -- the number of bytes allocated by the session
  max_alloc_bytes    INT             -- the high water mark of bytes allocated by the session
)
`

// znbaseInternalLocalSessionsTable exposes the list of running sessions
// on the current node. The results are dependent on the current user.
var znbaseInternalLocalSessionsTable = virtualSchemaTable{
	comment: "running sessions visible by current user (RAM; local node only)",
	schema:  fmt.Sprintf(sessionsSchemaPattern, "node_sessions"),
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		req := p.makeSessionsRequest(ctx)
		response, err := p.extendedEvalCtx.StatusServer.ListLocalSessions(ctx, &req)
		if err != nil {
			return err
		}
		return populateSessionsTable(ctx, addRow, response)
	},
}

// znbaseInternalClusterSessionsTable exposes the list of running sessions
// on the entire cluster. The result is dependent on the current user.
var znbaseInternalClusterSessionsTable = virtualSchemaTable{
	comment: "running sessions visible to current user (cluster RPC; expensive!)",
	schema:  fmt.Sprintf(sessionsSchemaPattern, "cluster_sessions"),
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		req := p.makeSessionsRequest(ctx)
		response, err := p.extendedEvalCtx.StatusServer.ListSessions(ctx, &req)
		if err != nil {
			return err
		}
		return populateSessionsTable(ctx, addRow, response)
	},
}

func populateSessionsTable(
	ctx context.Context, addRow func(...tree.Datum) error, response *serverpb.ListSessionsResponse,
) error {
	for _, session := range response.Sessions {
		// Generate active_queries and oldest_query_start
		var activeQueries bytes.Buffer
		var oldestStart time.Time
		var oldestStartDatum tree.Datum

		for idx, query := range session.ActiveQueries {
			if idx > 0 {
				activeQueries.WriteString("; ")
			}
			activeQueries.WriteString(query.Sql)

			if oldestStart.IsZero() || query.Start.Before(oldestStart) {
				oldestStart = query.Start
			}
		}

		if oldestStart.IsZero() {
			oldestStartDatum = tree.DNull
		} else {
			oldestStartDatum = tree.MakeDTimestamp(oldestStart, time.Microsecond)
		}

		kvTxnIDDatum := tree.DNull
		if session.KvTxnID != nil {
			kvTxnIDDatum = tree.NewDString(session.KvTxnID.String())
		}

		// TODO(knz): serverpb.Session is always constructed with an ID
		// set from a 16-byte session ID. Yet we get crash reports
		// that fail in BytesToClusterWideID() with a byte slice that's
		// too short. See #32517.
		var sessionID tree.Datum
		if session.ID == nil {
			// TODO(knz): NewInternalTrackingError is misdesigned. Change to
			// not use this. See the other facilities in
			// pgerror/internal_errors.go.
			telemetry.RecordError(
				pgerror.NewInternalTrackingError(32517 /* issue */, "null"))
			sessionID = tree.DNull
		} else if len(session.ID) != 16 {
			// TODO(knz): ditto above.
			telemetry.RecordError(
				pgerror.NewInternalTrackingError(32517 /* issue */, fmt.Sprintf("len=%d", len(session.ID))))
			sessionID = tree.NewDString("<invalid>")
		} else {
			clusterSessionID := BytesToClusterWideID(session.ID)
			sessionID = tree.NewDString(clusterSessionID.String())
		}

		if err := addRow(
			tree.NewDInt(tree.DInt(session.NodeID)),
			sessionID,
			tree.NewDString(session.Username),
			tree.NewDString(session.ClientAddress),
			tree.NewDString(session.ApplicationName),
			tree.NewDString(activeQueries.String()),
			tree.NewDString(session.LastActiveQuery),
			tree.MakeDTimestamp(session.Start, time.Microsecond),
			oldestStartDatum,
			kvTxnIDDatum,
			tree.NewDInt(tree.DInt(session.AllocBytes)),
			tree.NewDInt(tree.DInt(session.MaxAllocBytes)),
		); err != nil {
			return err
		}
	}

	for _, rpcErr := range response.Errors {
		log.Warning(ctx, rpcErr.Message)
		if rpcErr.NodeID != 0 {
			// Add a row with this node ID, error in active queries, and nulls
			// for all other columns.
			if err := addRow(
				tree.NewDInt(tree.DInt(rpcErr.NodeID)), // node ID
				tree.DNull,                             // session ID
				tree.DNull,                             // username
				tree.DNull,                             // client address
				tree.DNull,                             // application name
				tree.NewDString("-- "+rpcErr.Message),  // active queries
				tree.DNull,                             // last active query
				tree.DNull,                             // session start
				tree.DNull,                             // oldest_query_start
				tree.DNull,                             // kv_txn
				tree.DNull,                             // alloc_bytes
				tree.DNull,                             // max_alloc_bytes
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// znbaseInternalLocalMetricsTable exposes a snapshot of the metrics on the
// current node.
var znbaseInternalLocalMetricsTable = virtualSchemaTable{
	comment: "current values for metrics (RAM; local node only)",
	schema: `CREATE TABLE zbdb_internal.node_metrics (
  store_id 	         INT NULL,         -- the store, if any, for this metric
  name               STRING NOT NULL,  -- name of the metric
  value							 FLOAT NOT NULL    -- value of the metric
)`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.RequireAdminRole(ctx, "read zbdb_internal.node_metrics"); err != nil {
			return err
		}

		mr := p.ExecCfg().MetricsRecorder
		if mr == nil {
			return nil
		}
		nodeStatus := mr.GenerateNodeStatus(ctx)
		for i := 0; i <= len(nodeStatus.StoreStatuses); i++ {
			storeID := tree.DNull
			mtr := nodeStatus.Metrics
			if i > 0 {
				storeID = tree.NewDInt(tree.DInt(nodeStatus.StoreStatuses[i-1].Desc.StoreID))
				mtr = nodeStatus.StoreStatuses[i-1].Metrics
			}
			for name, value := range mtr {
				if err := addRow(
					storeID,
					tree.NewDString(name),
					tree.NewDFloat(tree.DFloat(value)),
				); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// znbaseInternalBuiltinFunctionsTable exposes the built-in function
// metadata.
var znbaseInternalBuiltinFunctionsTable = virtualSchemaTable{
	comment: "built-in functions (RAM/static)",
	schema: `
CREATE TABLE zbdb_internal.builtin_functions (
  function  STRING NOT NULL,
  signature STRING NOT NULL,
  category  STRING NOT NULL,
  details   STRING NOT NULL
)`,
	populate: func(ctx context.Context, _ *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		for _, name := range builtins.AllBuiltinNames {
			props, overloads := builtins.GetBuiltinProperties(name)
			for _, f := range overloads {
				if err := addRow(
					tree.NewDString(name),
					tree.NewDString(f.Signature(false /* simplify */)),
					tree.NewDString(props.Category),
					tree.NewDString(f.Info),
				); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// znbaseInternalCreateStmtsTable exposes the CREATE TABLE/CREATE VIEW
// statements.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalCreateStmtsTable = virtualSchemaTable{
	comment: `CREATE and ALTER statements for all tables accessible by current user in current database (KV scan)`,
	schema: `
CREATE TABLE zbdb_internal.create_statements (
  database_id         INT,
  database_name       STRING,
  schema_name         STRING NOT NULL,
  descriptor_id       INT,
  descriptor_type     STRING NOT NULL,
  descriptor_name     STRING NOT NULL,
  create_statement    STRING NOT NULL,
  state               STRING NOT NULL,
  create_nofks        STRING NOT NULL,
  alter_statements    STRING[] NOT NULL,
  validate_statements STRING[] NOT NULL
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		contextName := ""
		if dbContext != nil {
			contextName = dbContext.Name
		}

		// Prepare the row populate function.
		typeView := tree.NewDString("view")
		typeTable := tree.NewDString("table")
		typeSequence := tree.NewDString("sequence")

		return forEachTableDescWithTableLookupInternal(ctx, p, dbContext, virtualOnce, true, /*allowAdding*/
			func(db *DatabaseDescriptor, scName string, table *TableDescriptor, lCtx tableLookupFn, p *planner) error {
				parentNameStr := tree.DNull
				if db != nil {
					parentNameStr = tree.NewDString(db.Name)
				}
				scNameStr := tree.NewDString(scName)

				var descType tree.Datum
				var stmt, createNofk string
				alterStmts := tree.NewDArray(types.String)
				validateStmts := tree.NewDArray(types.String)
				var err error
				if table.IsView() {
					descType = typeView
					stmt, err = ShowCreateView(ctx, (*tree.Name)(&table.Name), table)
				} else if table.IsSequence() {
					descType = typeSequence
					stmt, err = ShowCreateSequence(ctx, (*tree.Name)(&table.Name), table)
				} else {
					descType = typeTable
					tn := (*tree.Name)(&table.Name)
					createNofk, err = ShowCreateTable(ctx, tn, contextName, scName, table, lCtx, true /* ignoreFKs */, p)
					if err != nil {
						return err
					}
					allIdx := append(table.Indexes, table.PrimaryIndex)
					for i := range allIdx {
						idx := &allIdx[i]
						if fk := &idx.ForeignKey; fk.IsSet() {
							f := tree.NewFmtCtx(tree.FmtSimple)
							f.WriteString("ALTER TABLE ")
							f.FormatNode(tn)
							f.WriteString(" ADD CONSTRAINT ")
							f.FormatNameP(&fk.Name)
							f.WriteByte(' ')
							if err := printForeignKeyConstraint(ctx, &f.Buffer, contextName, scName, idx, lCtx); err != nil {
								return err
							}
							if err := alterStmts.Append(tree.NewDString(f.CloseAndGetString())); err != nil {
								return err
							}

							f = tree.NewFmtCtx(tree.FmtSimple)
							f.WriteString("ALTER TABLE ")
							f.FormatNode(tn)
							f.WriteString(" VALIDATE CONSTRAINT ")
							f.FormatNameP(&fk.Name)

							if err := validateStmts.Append(tree.NewDString(f.CloseAndGetString())); err != nil {
								return err
							}
						}
					}
					stmt, err = ShowCreateTable(ctx, tn, contextName, scName, table, lCtx, false /* , ignoreFKs */, p)
				}
				if err != nil {
					return err
				}

				descID := tree.NewDInt(tree.DInt(table.ID))
				dbDescID := tree.NewDInt(tree.DInt(table.GetParentID()))
				if createNofk == "" {
					createNofk = stmt
				}
				return addRow(
					dbDescID,
					parentNameStr,
					scNameStr,
					descID,
					descType,
					tree.NewDString(table.Name),
					tree.NewDString(stmt),
					tree.NewDString(table.State.String()),
					tree.NewDString(createNofk),
					alterStmts,
					validateStmts,
				)
			})
	},
}

// znbaseInternalTableColumnsTable exposes the column descriptors.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalTableColumnsTable = virtualSchemaTable{
	comment: "details for all columns accessible by current user in current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.table_columns (
  descriptor_id    INT,
  descriptor_name  STRING NOT NULL,
  column_id        INT NOT NULL,
  column_name      STRING NOT NULL,
  column_type      STRING NOT NULL,
  nullable         BOOL NOT NULL,
  default_expr     STRING,
  hidden           BOOL NOT NULL
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		return forEachTableDescAll(ctx, p, dbContext, hideVirtual,
			func(db *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				tableID := tree.NewDInt(tree.DInt(table.ID))
				tableName := tree.NewDString(table.Name)
				for _, col := range table.Columns {
					defStr := tree.DNull
					if col.DefaultExpr != nil {
						defStr = tree.NewDString(*col.DefaultExpr)
					}
					if err := addRow(
						tableID,
						tableName,
						tree.NewDInt(tree.DInt(col.ID)),
						tree.NewDString(col.Name),
						tree.NewDString(col.Type.String()),
						tree.MakeDBool(tree.DBool(col.Nullable)),
						defStr,
						tree.MakeDBool(tree.DBool(col.Hidden)),
					); err != nil {
						return err
					}
				}
				return nil
			})
	},
}

// znbaseInternalTableIndexesTable exposes the index descriptors.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalTableIndexesTable = virtualSchemaTable{
	comment: "indexes accessible by current user in current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.table_indexes (
  descriptor_id    INT,
  descriptor_name  STRING NOT NULL,
  index_id         INT NOT NULL,
  index_name       STRING NOT NULL,
  index_type       STRING NOT NULL,
  is_unique        BOOL NOT NULL
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		primary := tree.NewDString("primary")
		secondary := tree.NewDString("secondary")
		return forEachTableDescAll(ctx, p, dbContext, hideVirtual,
			func(db *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				tableID := tree.NewDInt(tree.DInt(table.ID))
				tableName := tree.NewDString(table.Name)
				if err := addRow(
					tableID,
					tableName,
					tree.NewDInt(tree.DInt(table.PrimaryIndex.ID)),
					tree.NewDString(table.PrimaryIndex.Name),
					primary,
					tree.MakeDBool(tree.DBool(table.PrimaryIndex.Unique)),
				); err != nil {
					return err
				}
				for _, idx := range table.Indexes {
					if err := addRow(
						tableID,
						tableName,
						tree.NewDInt(tree.DInt(idx.ID)),
						tree.NewDString(idx.Name),
						secondary,
						tree.MakeDBool(tree.DBool(idx.Unique)),
					); err != nil {
						return err
					}
				}
				return nil
			})
	},
}

var znbaseInternalTablesTableLastStats = virtualSchemaTable{
	comment: "the latest stats for all tables accessible by current user in current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.table_row_statistics (
  table_id                   INT         NOT NULL,
  table_name                 STRING      NOT NULL,
  estimated_row_count        INT
)`,
	populate: func(ctx context.Context, p *planner, db *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		// Collect the latests statistics for all tables.
		query := `
           SELECT s."tableID", max(s."rowCount")
             FROM system.table_statistics AS s
             JOIN (
                    SELECT "tableID", max("createdAt") AS last_dt
                      FROM system.table_statistics
                     GROUP BY "tableID"
                  ) AS l ON l."tableID" = s."tableID" AND l.last_dt = s."createdAt"
            GROUP BY s."tableID"`
		statRows, err := p.ExtendedEvalContext().ExecCfg.InternalExecutor.Query(
			ctx, "znbase-internal-statistics-table", p.txn,
			query)
		if err != nil {
			return err
		}

		// Convert statistics into map: tableID -> rowCount.
		statMap := make(map[tree.DInt]tree.Datum)
		for _, r := range statRows {
			statMap[tree.MustBeDInt(r[0])] = r[1]
		}

		// Walk over all available tables and show row count for each of them
		// using collected statistics.
		return forEachTableDescAll(ctx, p, db, virtualMany,
			func(db *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				tableID := tree.DInt(table.GetID())
				rowCount := tree.DNull
				// For Virtual Tables report NULL row count.
				if !table.IsVirtualTable() {
					rowCount = tree.NewDInt(0)
					if cnt, ok := statMap[tableID]; ok {
						rowCount = cnt
					}
				} else {
					return nil
				}
				return addRow(
					tree.NewDInt(tableID),
					tree.NewDString(table.GetName()),
					rowCount,
				)
			},
		)
	},
}

// znbaseInternalIndexColumnsTable exposes the index columns.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalIndexColumnsTable = virtualSchemaTable{
	comment: "index columns for all indexes accessible by current user in current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.index_columns (
  descriptor_id    INT,
  descriptor_name  STRING NOT NULL,
  index_id         INT NOT NULL,
  index_name       STRING NOT NULL,
  column_type      STRING NOT NULL,
  column_id        INT NOT NULL,
  column_name      STRING,
  column_direction STRING,
  locate_in        STRING,
  lease_in		   STRING
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		key := tree.NewDString("key")
		storing := tree.NewDString("storing")
		extra := tree.NewDString("extra")
		composite := tree.NewDString("composite")
		idxDirMap := map[sqlbase.IndexDescriptor_Direction]tree.Datum{
			sqlbase.IndexDescriptor_ASC:  tree.NewDString(sqlbase.IndexDescriptor_ASC.String()),
			sqlbase.IndexDescriptor_DESC: tree.NewDString(sqlbase.IndexDescriptor_DESC.String()),
		}

		return forEachTableDescAll(ctx, p, dbContext, hideVirtual,
			func(parent *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				tableID := tree.NewDInt(tree.DInt(table.ID))
				parentName := parent.Name
				tableName := tree.NewDString(table.Name)

				reportIndex := func(idx *sqlbase.IndexDescriptor) error {
					idxID := tree.NewDInt(tree.DInt(idx.ID))
					idxName := tree.NewDString(idx.Name)
					locateIn, leaseIn := checkSpaceNull(idx.LocateSpaceName)
					// Report the main (key) columns.
					for i, c := range idx.ColumnIDs {
						colName := tree.DNull
						colDir := tree.DNull
						if i >= len(idx.ColumnNames) {
							// We log an error here, instead of reporting an error
							// to the user, because we really want to see the
							// erroneous data in the virtual table.
							log.Errorf(ctx, "index descriptor for [%d@%d] (%s.%s@%s) has more key column IDs (%d) than names (%d) (corrupted schema?)",
								table.ID, idx.ID, parentName, table.Name, idx.Name,
								len(idx.ColumnIDs), len(idx.ColumnNames))
						} else {
							colName = tree.NewDString(idx.ColumnNames[i])
						}
						if i >= len(idx.ColumnDirections) {
							// See comment above.
							log.Errorf(ctx, "index descriptor for [%d@%d] (%s.%s@%s) has more key column IDs (%d) than directions (%d) (corrupted schema?)",
								table.ID, idx.ID, parentName, table.Name, idx.Name,
								len(idx.ColumnIDs), len(idx.ColumnDirections))
						} else {
							colDir = idxDirMap[idx.ColumnDirections[i]]
						}

						if err := addRow(
							tableID, tableName, idxID, idxName,
							key, tree.NewDInt(tree.DInt(c)), colName, colDir, locateIn, leaseIn,
						); err != nil {
							return err
						}
					}

					// Report the stored columns.
					for _, c := range idx.StoreColumnIDs {
						if err := addRow(
							tableID, tableName, idxID, idxName,
							storing, tree.NewDInt(tree.DInt(c)), tree.DNull, tree.DNull, locateIn, leaseIn,
						); err != nil {
							return err
						}
					}

					// Report the extra columns.
					for _, c := range idx.ExtraColumnIDs {
						if err := addRow(
							tableID, tableName, idxID, idxName,
							extra, tree.NewDInt(tree.DInt(c)), tree.DNull, tree.DNull, locateIn, leaseIn,
						); err != nil {
							return err
						}
					}

					// Report the composite columns
					for _, c := range idx.CompositeColumnIDs {
						if err := addRow(
							tableID, tableName, idxID, idxName,
							composite, tree.NewDInt(tree.DInt(c)), tree.DNull, tree.DNull, locateIn, leaseIn,
						); err != nil {
							return err
						}
					}

					return nil
				}

				if err := reportIndex(&table.PrimaryIndex); err != nil {
					return err
				}
				for i := range table.Indexes {
					if err := reportIndex(&table.Indexes[i]); err != nil {
						return err
					}
				}
				return nil
			})
	},
}

// znbaseInternalBackwardDependenciesTable exposes the backward
// inter-descriptor dependencies.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalBackwardDependenciesTable = virtualSchemaTable{
	comment: "backward inter-descriptor dependencies starting from tables accessible by current user in current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.backward_dependencies (
  descriptor_id      INT,
  descriptor_name    STRING NOT NULL,
  index_id           INT,
  column_id          INT,
  dependson_id       INT NOT NULL,
  dependson_type     STRING NOT NULL,
  dependson_index_id INT,
  dependson_name     STRING,
  dependson_details  STRING
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		fkDep := tree.NewDString("fk")
		viewDep := tree.NewDString("view")
		sequenceDep := tree.NewDString("sequence")
		interleaveDep := tree.NewDString("interleave")
		return forEachTableDescAll(ctx, p, dbContext, hideVirtual, /* virtual tables have no backward/forward dependencies*/
			func(db *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				tableID := tree.NewDInt(tree.DInt(table.ID))
				tableName := tree.NewDString(table.Name)

				reportIdxDeps := func(idx *sqlbase.IndexDescriptor) error {
					idxID := tree.NewDInt(tree.DInt(idx.ID))
					if idx.ForeignKey.Table != 0 {
						fkRef := &idx.ForeignKey
						if err := addRow(
							tableID, tableName,
							idxID,
							tree.DNull,
							tree.NewDInt(tree.DInt(fkRef.Table)),
							fkDep,
							tree.NewDInt(tree.DInt(fkRef.Index)),
							tree.NewDString(fkRef.Name),
							tree.NewDString(fmt.Sprintf("SharedPrefixLen: %d", fkRef.SharedPrefixLen)),
						); err != nil {
							return err
						}
					}

					for _, interleaveParent := range idx.Interleave.Ancestors {
						if err := addRow(
							tableID, tableName,
							idxID,
							tree.DNull,
							tree.NewDInt(tree.DInt(interleaveParent.TableID)),
							interleaveDep,
							tree.NewDInt(tree.DInt(interleaveParent.IndexID)),
							tree.DNull,
							tree.NewDString(fmt.Sprintf("SharedPrefixLen: %d",
								interleaveParent.SharedPrefixLen)),
						); err != nil {
							return err
						}
					}
					return nil
				}

				// Record the backward references of the primary index.
				if err := reportIdxDeps(&table.PrimaryIndex); err != nil {
					return err
				}

				// Record the backward references of secondary indexes.
				for i := range table.Indexes {
					if err := reportIdxDeps(&table.Indexes[i]); err != nil {
						return err
					}
				}

				// Record the view dependencies.
				for _, tIdx := range table.DependsOn {
					if err := addRow(
						tableID, tableName,
						tree.DNull,
						tree.DNull,
						tree.NewDInt(tree.DInt(tIdx)),
						viewDep,
						tree.DNull,
						tree.DNull,
						tree.DNull,
					); err != nil {
						return err
					}
				}

				// Record sequence dependencies.
				for _, col := range table.Columns {
					for _, sequenceID := range col.UsesSequenceIds {
						if err := addRow(
							tableID, tableName,
							tree.DNull,
							tree.NewDInt(tree.DInt(col.ID)),
							tree.NewDInt(tree.DInt(sequenceID)),
							sequenceDep,
							tree.DNull,
							tree.DNull,
							tree.DNull,
						); err != nil {
							return err
						}
					}
				}
				return nil
			})
	},
}

// znbaseInternalFeatureUsage exposes the telemetry counters.
var znbaseInternalFeatureUsage = virtualSchemaTable{
	comment: "telemetry counters (RAM; local node only)",
	schema: `
CREATE TABLE zbdb_internal.feature_usage (
  feature_name          STRING NOT NULL,
  usage_count           INT NOT NULL
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		for feature, count := range telemetry.GetFeatureCounts() {
			if count == 0 {
				// Skip over empty counters to avoid polluting the output.
				continue
			}
			if err := addRow(
				tree.NewDString(feature),
				tree.NewDInt(tree.DInt(int64(count))),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// znbaseInternalForwardDependenciesTable exposes the forward
// inter-descriptor dependencies.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalForwardDependenciesTable = virtualSchemaTable{
	comment: "forward inter-descriptor dependencies starting from tables accessible by current user in current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.forward_dependencies (
  descriptor_id         INT,
  descriptor_name       STRING NOT NULL,
  index_id              INT,
  dependedonby_id       INT NOT NULL,
  dependedonby_type     STRING NOT NULL,
  dependedonby_index_id INT,
  dependedonby_name     STRING,
  dependedonby_details  STRING
)
`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		fkDep := tree.NewDString("fk")
		viewDep := tree.NewDString("view")
		interleaveDep := tree.NewDString("interleave")
		sequenceDep := tree.NewDString("sequence")
		return forEachTableDescAll(ctx, p, dbContext, hideVirtual, /* virtual tables have no backward/forward dependencies*/
			func(db *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				tableID := tree.NewDInt(tree.DInt(table.ID))
				tableName := tree.NewDString(table.Name)

				reportIdxDeps := func(idx *sqlbase.IndexDescriptor) error {
					idxID := tree.NewDInt(tree.DInt(idx.ID))
					for _, fkRef := range idx.ReferencedBy {
						if err := addRow(
							tableID, tableName,
							idxID,
							tree.NewDInt(tree.DInt(fkRef.Table)),
							fkDep,
							tree.NewDInt(tree.DInt(fkRef.Index)),
							tree.NewDString(fkRef.Name),
							tree.NewDString(fmt.Sprintf("SharedPrefixLen: %d", fkRef.SharedPrefixLen)),
						); err != nil {
							return err
						}
					}

					for _, interleaveRef := range idx.InterleavedBy {
						if err := addRow(
							tableID, tableName,
							idxID,
							tree.NewDInt(tree.DInt(interleaveRef.Table)),
							interleaveDep,
							tree.NewDInt(tree.DInt(interleaveRef.Index)),
							tree.DNull,
							tree.NewDString(fmt.Sprintf("SharedPrefixLen: %d",
								interleaveRef.SharedPrefixLen)),
						); err != nil {
							return err
						}
					}
					return nil
				}

				// Record the backward references of the primary index.
				if err := reportIdxDeps(&table.PrimaryIndex); err != nil {
					return err
				}

				// Record the backward references of secondary indexes.
				for i := range table.Indexes {
					if err := reportIdxDeps(&table.Indexes[i]); err != nil {
						return err
					}
				}

				if table.IsTable() || table.IsView() {
					// Record the view dependencies.
					for _, dep := range table.DependedOnBy {
						if err := addRow(
							tableID, tableName,
							tree.DNull,
							tree.NewDInt(tree.DInt(dep.ID)),
							viewDep,
							tree.NewDInt(tree.DInt(dep.IndexID)),
							tree.DNull,
							tree.NewDString(fmt.Sprintf("Columns: %v", dep.ColumnIDs)),
						); err != nil {
							return err
						}
					}
				} else if table.IsSequence() {
					// Record the sequence dependencies.
					for _, dep := range table.DependedOnBy {
						if err := addRow(
							tableID, tableName,
							tree.DNull,
							tree.NewDInt(tree.DInt(dep.ID)),
							sequenceDep,
							tree.NewDInt(tree.DInt(dep.IndexID)),
							tree.DNull,
							tree.NewDString(fmt.Sprintf("Columns: %v", dep.ColumnIDs)),
						); err != nil {
							return err
						}
					}
				}
				return nil
			})
	},
}

// znbaseInternalRangesView exposes system ranges.
var znbaseInternalRangesView = virtualSchemaTable{
	schema: `
CREATE TABLE zbdb_internal.ranges(
	range_id    	 INT NOT NULL,
	start_key    	 BYTES NOT NULL,
	start_pretty     STRING NOT NULL,
	end_key    		 BYTES NOT NULL,
	end_pretty  	 STRING NOT NULL,
	database_name    STRING NOT NULL,
	schema_name    	 STRING NOT NULL,
	table_name    	 STRING NOT NULL,
	index_name     	 STRING NOT NULL,
	replicas    	 INT[] NOT NULL,
	lease_holder     INT NOT NULL,
	range_size	 	 INT NOT NULL
)
`,
	generator: func(ctx context.Context, p *planner, _ *DatabaseDescriptor) (virtualTableGenerator, error) {
		if err := p.CheckUserOption(ctx, useroption.CSTATUS); err != nil {
			return nil, err
		}
		descs, err := p.Tables().getAllDescriptors(ctx, p.txn)
		if err != nil {
			return nil, err
		}
		// TODO(knz): maybe this could use internalLookupCtx.
		dbNames := make(map[uint64]string)
		scNames := make(map[uint64]string)
		scParents := make(map[uint64]uint64)
		tableNames := make(map[uint64]string)
		indexNames := make(map[uint64]map[sqlbase.IndexID]string)
		parents := make(map[uint64]uint64)
		for _, desc := range descs {
			id := uint64(desc.GetID())
			switch desc := desc.(type) {
			case *sqlbase.TableDescriptor:
				parents[id] = uint64(desc.ParentID)
				tableNames[id] = desc.GetName()
				indexNames[id] = make(map[sqlbase.IndexID]string)
				for _, idx := range desc.Indexes {
					indexNames[id][idx.ID] = idx.Name
				}
			case *sqlbase.SchemaDescriptor:
				scNames[id] = desc.GetName()
				scParents[id] = uint64(desc.ParentID)
			case *sqlbase.DatabaseDescriptor:
				dbNames[id] = desc.GetName()
			}
		}
		ranges, err := ScanMetaKVs(ctx, p.txn, roachpb.Span{
			Key:    keys.MinKey,
			EndKey: keys.MaxKey,
		})
		if err != nil {
			return nil, err
		}
		var rangeDescs []*roachpb.RangeDescriptor
		var rangeSizes []*tree.DInt

		for _, r := range ranges {
			rangeDesc := &roachpb.RangeDescriptor{}
			if err := r.ValueProto(rangeDesc); err != nil {
				return nil, err
			}
			rangeDescs = append(rangeDescs, rangeDesc)
		}

		wg := sync.WaitGroup{}

		c := make(chan error, LeaseInfoRequestConcurrentCount)
		for i := 0; i < LeaseInfoRequestConcurrentCount; i++ {
			c <- nil
		}
		leaseholders := make([]*tree.DInt, len(rangeDescs))
		for i, rangeDesc := range rangeDescs {
			if err, ok := <-c; ok {
				if err != nil {
					wg.Wait()
					return nil, err
				}
				wg.Add(1)
				go func(idx int, rgDesc *roachpb.RangeDescriptor) {
					//defer func() { <-c }()
					defer wg.Done()
					leaseholderBatch := &client.Batch{}
					leaseholderBatch.AddRawRequest(&roachpb.LeaseInfoRequest{
						RequestHeader: roachpb.RequestHeader{
							Key: roachpb.Key(rgDesc.StartKey),
						},
					})
					if err = p.txn.Run(ctx, leaseholderBatch); err != nil {
						c <- err
						return
					}
					linfoResp := leaseholderBatch.RawResponse().Responses[0].GetInner().(*roachpb.LeaseInfoResponse)
					leaseholders[idx] = tree.NewDInt(tree.DInt(linfoResp.Lease.Replica.StoreID))
					c <- nil
				}(i, rangeDesc)

			}
		}
		wg.Wait()

		rangeSizeBatch := &client.Batch{}
		for _, rgDesc := range rangeDescs {
			rangeSizeBatch.AddRawRequest(&roachpb.RangeStatsRequest{
				RequestHeader: roachpb.RequestHeader{
					Key: roachpb.Key(rgDesc.StartKey),
				},
			})
		}
		if err = p.txn.Run(ctx, rangeSizeBatch); err != nil {
			return nil, err
		}
		rsResponses := rangeSizeBatch.RawResponse().Responses
		getIntFromJSON := func(json tree.Datum, key string) (int, error) {
			if jsonDatum, ok := json.(*tree.DJSON); ok {
				res, err := jsonDatum.JSON.FetchValKey(key)
				if err != nil {
					return 0, err
				}
				text, err := res.AsText()
				if err != nil {
					return 0, err
				}
				if text == nil {
					return 0, nil
				}
				val, err := strconv.Atoi(*text)
				if err == nil {
					return val, nil
				}
			}
			return 0, errors.Errorf("Can't get field %s or or convert %s to int", key, key)

		}
		for _, resp := range rsResponses {
			stats := resp.GetInner().(*roachpb.RangeStatsResponse).MVCCStats
			jsonStr, err := gojson.Marshal(&stats)
			if err != nil {
				return nil, err
			}
			jsonDatum, err := tree.ParseDJSON(string(jsonStr))
			if err != nil {
				return nil, err
			}
			keyBytes, err := getIntFromJSON(jsonDatum, "key_bytes")
			if err != nil {
				return nil, err
			}
			valBytes, err := getIntFromJSON(jsonDatum, "val_bytes")
			if err != nil {
				return nil, err
			}
			rangeSizes = append(rangeSizes, tree.NewDInt(tree.DInt(keyBytes+valBytes)))
		}

		i := 0

		return func() (tree.Datums, error) {
			if i >= len(ranges) {
				return nil, nil
			}

			//r := ranges[i]
			desc := rangeDescs[i]
			lh := leaseholders[i]
			rs := rangeSizes[i]
			i++

			//if err := r.ValueProto(&desc); err != nil {
			//	return nil, err
			//}

			var replicas []int
			for _, rd := range desc.Replicas {
				replicas = append(replicas, int(rd.StoreID))
			}
			sort.Ints(replicas)
			arr := tree.NewDArray(types.Int)
			for _, replica := range replicas {
				if err := arr.Append(tree.NewDInt(tree.DInt(replica))); err != nil {
					return nil, err
				}
			}

			var dbName, scName, tableName, indexName string
			if _, id, err := keys.DecodeTablePrefix(desc.StartKey.AsRawKey()); err == nil {
				parent := parents[id]
				if parent != 0 {
					tableName = tableNames[id]
					scName = scNames[parent]
					dbName = dbNames[scParents[parent]]
					if _, _, idxID, err := sqlbase.DecodeTableIDIndexID(desc.StartKey.AsRawKey()); err == nil {
						indexName = indexNames[id][idxID]
					}
				} else if parent = scParents[id]; parent != 0 {
					scName = scNames[id]
					dbName = dbNames[scParents[id]]
				} else {
					dbName = dbNames[id]
				}
			}

			return tree.Datums{
				tree.NewDInt(tree.DInt(desc.RangeID)),
				tree.NewDBytes(tree.DBytes(desc.StartKey)),
				tree.NewDString(keys.PrettyPrint(nil /* valDirs */, desc.StartKey.AsRawKey())),
				tree.NewDBytes(tree.DBytes(desc.EndKey)),
				tree.NewDString(keys.PrettyPrint(nil /* valDirs */, desc.EndKey.AsRawKey())),
				tree.NewDString(dbName),
				tree.NewDString(scName),
				tree.NewDString(tableName),
				tree.NewDString(indexName),
				arr,
				lh,
				rs,
			}, nil
		}, nil
	},
}

// znbaseInternalRangesNoLeasesTable exposes all ranges in the system without the
// `lease_holder` information.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalRangesNoLeasesTable = virtualSchemaTable{
	comment: `range metadata without leaseholder details (KV join; expensive!)`,
	schema: `
CREATE TABLE zbdb_internal.ranges_no_leases (
  range_id     INT NOT NULL,
  start_key    BYTES NOT NULL,
  start_pretty STRING NOT NULL,
  end_key      BYTES NOT NULL,
  end_pretty   STRING NOT NULL,
  database_name     STRING NOT NULL,
	schema_name     STRING NOT NULL,
  table_name      STRING NOT NULL,
  index_name      STRING NOT NULL,
  replicas     INT[] NOT NULL
)
`,
	generator: func(ctx context.Context, p *planner, _ *DatabaseDescriptor) (virtualTableGenerator, error) {
		if err := p.CheckUserOption(ctx, useroption.CSTATUS); err != nil {
			return nil, err
		}
		descs, err := p.Tables().getAllDescriptors(ctx, p.txn)
		if err != nil {
			return nil, err
		}
		// TODO(knz): maybe this could use internalLookupCtx.
		dbNames := make(map[uint64]string)
		scNames := make(map[uint64]string)
		scParents := make(map[uint64]uint64)
		tableNames := make(map[uint64]string)
		indexNames := make(map[uint64]map[sqlbase.IndexID]string)
		parents := make(map[uint64]uint64)
		for _, desc := range descs {
			id := uint64(desc.GetID())
			switch desc := desc.(type) {
			case *sqlbase.TableDescriptor:
				parents[id] = uint64(desc.ParentID)
				tableNames[id] = desc.GetName()
				indexNames[id] = make(map[sqlbase.IndexID]string)
				for _, idx := range desc.Indexes {
					indexNames[id][idx.ID] = idx.Name
				}
			case *sqlbase.SchemaDescriptor:
				scNames[id] = desc.GetName()
				scParents[id] = uint64(desc.ParentID)
			case *sqlbase.DatabaseDescriptor:
				dbNames[id] = desc.GetName()
			}
		}
		ranges, err := ScanMetaKVs(ctx, p.txn, roachpb.Span{
			Key:    keys.MinKey,
			EndKey: keys.MaxKey,
		})
		if err != nil {
			return nil, err
		}
		var desc roachpb.RangeDescriptor

		i := 0

		return func() (tree.Datums, error) {
			if i >= len(ranges) {
				return nil, nil
			}

			r := ranges[i]
			i++

			if err := r.ValueProto(&desc); err != nil {
				return nil, err
			}

			var replicas []int
			for _, rd := range desc.Replicas {
				replicas = append(replicas, int(rd.StoreID))
			}
			sort.Ints(replicas)
			arr := tree.NewDArray(types.Int)
			for _, replica := range replicas {
				if err := arr.Append(tree.NewDInt(tree.DInt(replica))); err != nil {
					return nil, err
				}
			}

			var dbName, scName, tableName, indexName string
			if _, id, err := keys.DecodeTablePrefix(desc.StartKey.AsRawKey()); err == nil {
				parent := parents[id]
				if parent != 0 {
					tableName = tableNames[id]
					scName = scNames[parent]
					dbName = dbNames[scParents[parent]]
					if _, _, idxID, err := sqlbase.DecodeTableIDIndexID(desc.StartKey.AsRawKey()); err == nil {
						indexName = indexNames[id][idxID]
					}
				} else if parent = scParents[id]; parent != 0 {
					scName = scNames[id]
					dbName = dbNames[scParents[id]]
				} else {
					dbName = dbNames[id]
				}
			}

			return tree.Datums{
				tree.NewDInt(tree.DInt(desc.RangeID)),
				tree.NewDBytes(tree.DBytes(desc.StartKey)),
				tree.NewDString(keys.PrettyPrint(nil /* valDirs */, desc.StartKey.AsRawKey())),
				tree.NewDBytes(tree.DBytes(desc.EndKey)),
				tree.NewDString(keys.PrettyPrint(nil /* valDirs */, desc.EndKey.AsRawKey())),
				tree.NewDString(dbName),
				tree.NewDString(scName),
				tree.NewDString(tableName),
				tree.NewDString(indexName),
				arr,
			}, nil
		}, nil
	},
}

// znbaseInternalZonesTable decodes and exposes the zone configs in the
// system.zones table.
// The cli_specifier column is deprecated and only exists to be used
// as a hidden field by the CLI for backwards compatibility. Use zone_name
// instead.
//
// TODO(tbg): prefix with kv_.
var znbaseInternalZonesTable = virtualSchemaTable{
	comment: "decoded zone configurations from system.zones (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.zones (
  zone_id          INT NOT NULL,
  zone_name        STRING,
  cli_specifier    STRING, -- this column is deprecated in favor of zone_name.
                           -- It is kept for backwards compatibility with the CLI.
  config_yaml      STRING NOT NULL,
  config_sql       STRING, -- this column can be NULL if there is no specifier syntax
                           -- possible (e.g. the object was deleted).
  config_protobuf  BYTES NOT NULL
)
`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		namespace, err := p.getAllNames(ctx)
		if err != nil {
			return err
		}
		resolveID := func(id uint32) (parentID uint32, name string, err error) {
			if entry, ok := namespace[sqlbase.ID(id)]; ok {
				return uint32(entry.parentID), entry.name, nil
			}
			return 0, "", fmt.Errorf("object with ID %d does not exist", id)
		}

		rows, err := p.ExtendedEvalContext().ExecCfg.InternalExecutor.Query(
			ctx, "znbase-internal-zones-table", p.txn, `SELECT id, config FROM system.zones`)
		if err != nil {
			return err
		}
		values := make(tree.Datums, len(showZoneConfigNodeColumns))
		for _, r := range rows {
			id := uint32(tree.MustBeDInt(r[0]))

			var zoneSpecifier *tree.ZoneSpecifier
			zs, err := config.ZoneSpecifierFromID(id, resolveID)
			if err != nil {
				// The database or table has been deleted so there is no way
				// to refer to it anymore. We are still going to show
				// something but the CLI specifier part will become NULL.
				zoneSpecifier = nil
			} else {
				zoneSpecifier = &zs
			}

			configBytes := []byte(*r[1].(*tree.DBytes))
			var configProto config.ZoneConfig
			if err := protoutil.Unmarshal(configBytes, &configProto); err != nil {
				return err
			}
			subzones := configProto.Subzones

			if !configProto.IsSubzonePlaceholder() {
				// Ensure subzones don't infect the value of the config_proto column.
				configProto.Subzones = nil
				configProto.SubzoneSpans = nil

				if err := generateZoneConfigIntrospectionValues(values, r[0], zoneSpecifier, &configProto); err != nil {
					return err
				}
				if err := addRow(values...); err != nil {
					return err
				}
			}

			if len(subzones) > 0 {
				table, err := sqlbase.GetTableDescFromID(ctx, p.txn, sqlbase.ID(id))
				if err != nil {
					return err
				}
				for _, s := range subzones {
					index, err := table.FindIndexByID(sqlbase.IndexID(s.IndexID))
					if err != nil {
						if err == sqlbase.ErrIndexGCMutationsList {
							continue
						}
						return err
					}
					if zoneSpecifier != nil {
						zs := zs
						zs.TableOrIndex.Index = tree.UnrestrictedName(index.Name)
						zs.Partition = tree.Name(s.PartitionName)
						zoneSpecifier = &zs
					}

					if err := generateZoneConfigIntrospectionValues(values, r[0], zoneSpecifier, &s.Config); err != nil {
						return err
					}
					if err := addRow(values...); err != nil {
						return err
					}
				}
			}
		}
		return nil
	},
}

// znbaseInternalGossipNodesTable exposes local information about the cluster nodes.
var znbaseInternalGossipNodesTable = virtualSchemaTable{
	comment: "locally known gossiped node details (RAM; local node only)",
	schema: `
CREATE TABLE zbdb_internal.gossip_nodes (
  node_id         		INT NOT NULL,
  network         		STRING NOT NULL,
  address         		STRING NOT NULL,
  advertise_address   STRING NOT NULL,
  attrs           		JSON NOT NULL,
  locality        		JSON NOT NULL,
  server_version  		STRING NOT NULL,
  build_tag       		STRING NOT NULL,
  started_at     	 		TIMESTAMP NOT NULL,
  is_live          BOOL NOT NULL,
  ranges          		INT NOT NULL,
  leases        	  	INT NOT NULL
)
	`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.CheckUserOption(ctx, useroption.CSTATUS); err != nil {
			return err
		}

		g := p.ExecCfg().Gossip
		var descriptors []roachpb.NodeDescriptor
		if err := g.IterateInfos(gossip.KeyNodeIDPrefix, func(key string, i gossip.Info) error {
			bytes, err := i.Value.GetBytes()
			if err != nil {
				return errors.Wrapf(err, "failed to extract bytes for key %q", key)
			}

			var d roachpb.NodeDescriptor
			if err := protoutil.Unmarshal(bytes, &d); err != nil {
				return errors.Wrapf(err, "failed to parse value for key %q", key)
			}

			// Don't use node descriptors with NodeID 0, because that's meant to
			// indicate that the node has been removed from the cluster.
			if d.NodeID != 0 {
				descriptors = append(descriptors, d)
			}
			return nil
		}); err != nil {
			return err
		}

		alive := make(map[roachpb.NodeID]tree.DBool)
		for _, d := range descriptors {
			if _, err := g.GetInfo(gossip.MakeGossipClientsKey(d.NodeID)); err == nil {
				alive[d.NodeID] = true
			}
		}

		sort.Slice(descriptors, func(i, j int) bool {
			return descriptors[i].NodeID < descriptors[j].NodeID
		})

		type nodeStats struct {
			ranges int32
			leases int32
		}

		stats := make(map[roachpb.NodeID]nodeStats)
		if err := g.IterateInfos(gossip.KeyStorePrefix, func(key string, i gossip.Info) error {
			bytes, err := i.Value.GetBytes()
			if err != nil {
				return errors.Wrapf(err, "failed to extract bytes for key %q", key)
			}

			var desc roachpb.StoreDescriptor
			if err := protoutil.Unmarshal(bytes, &desc); err != nil {
				return errors.Wrapf(err, "failed to parse value for key %q", key)
			}

			s := stats[desc.Node.NodeID]
			s.ranges += desc.Capacity.RangeCount
			s.leases += desc.Capacity.LeaseCount
			stats[desc.Node.NodeID] = s
			return nil
		}); err != nil {
			return err
		}

		for _, d := range descriptors {
			attrs := json.NewArrayBuilder(len(d.Attrs.Attrs))
			for _, a := range d.Attrs.Attrs {
				attrs.Add(json.FromString(a))
			}

			locality := json.NewObjectBuilder(len(d.Locality.Tiers))
			for _, t := range d.Locality.Tiers {
				locality.Add(t.Key, json.FromString(t.Value))
			}

			addr, err := g.GetNodeIDAddress(d.NodeID)
			if err != nil {
				return err
			}

			if err := addRow(
				tree.NewDInt(tree.DInt(d.NodeID)),
				tree.NewDString(d.Address.NetworkField),
				tree.NewDString(d.Address.AddressField),
				tree.NewDString(addr.String()),
				tree.NewDJSON(attrs.Build()),
				tree.NewDJSON(locality.Build()),
				tree.NewDString(d.ServerVersion.String()),
				tree.NewDString(d.BuildTag),
				tree.MakeDTimestamp(timeutil.Unix(0, d.StartedAt), time.Microsecond),
				tree.MakeDBool(alive[d.NodeID]),
				tree.NewDInt(tree.DInt(stats[d.NodeID].ranges)),
				tree.NewDInt(tree.DInt(stats[d.NodeID].leases)),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// znbaseInternalGossipLivenessTable exposes local information about the nodes'
// liveness. The data exposed in this table can be stale/incomplete because
// gossip doesn't provide guarantees around freshness or consistency.
var znbaseInternalGossipLivenessTable = virtualSchemaTable{
	comment: "locally known gossiped node liveness (RAM; local node only)",
	schema: `
CREATE TABLE zbdb_internal.gossip_liveness (
  node_id         INT NOT NULL,
  epoch           INT NOT NULL,
  expiration      STRING NOT NULL,
  draining        BOOL NOT NULL,
  decommissioning BOOL NOT NULL,
  updated_at      TIMESTAMP
)
	`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		// ATTENTION: The contents of this table should only access gossip data
		// which is highly available. DO NOT CALL functions which require the
		// cluster to be healthy, such as StatusServer.Nodes().

		if err := p.RequireAdminRole(ctx, "read zbdb_internal.gossip_liveness"); err != nil {
			return err
		}

		g := p.ExecCfg().Gossip

		type nodeInfo struct {
			liveness  storagepb.Liveness
			updatedAt int64
		}

		var nodes []nodeInfo
		if err := g.IterateInfos(gossip.KeyNodeLivenessPrefix, func(key string, i gossip.Info) error {
			bytes, err := i.Value.GetBytes()
			if err != nil {
				return errors.Wrapf(err, "failed to extract bytes for key %q", key)
			}

			var l storagepb.Liveness
			if err := protoutil.Unmarshal(bytes, &l); err != nil {
				return errors.Wrapf(err, "failed to parse value for key %q", key)
			}
			nodes = append(nodes, nodeInfo{
				liveness:  l,
				updatedAt: i.OrigStamp,
			})
			return nil
		}); err != nil {
			return err
		}

		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].liveness.NodeID < nodes[j].liveness.NodeID
		})

		for i := range nodes {
			n := &nodes[i]
			l := &n.liveness
			if err := addRow(
				tree.NewDInt(tree.DInt(l.NodeID)),
				tree.NewDInt(tree.DInt(l.Epoch)),
				tree.NewDString(l.Expiration.String()),
				tree.MakeDBool(tree.DBool(l.Draining)),
				tree.MakeDBool(tree.DBool(l.Decommissioning)),
				tree.MakeDTimestamp(timeutil.Unix(0, n.updatedAt), time.Microsecond),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// znbaseInternalGossipAlertsTable exposes current health alerts in the cluster.
var znbaseInternalGossipAlertsTable = virtualSchemaTable{
	comment: "locally known gossiped health alerts (RAM; local node only)",
	schema: `
CREATE TABLE zbdb_internal.gossip_alerts (
  node_id         INT NOT NULL,
  store_id        INT NULL,        -- null for alerts not associated to a store
  category        STRING NOT NULL, -- type of alert, usually by subsystem
  description     STRING NOT NULL, -- name of the alert (depends on subsystem)
  value           FLOAT NOT NULL   -- value of the alert (depends on subsystem, can be NaN)
)
	`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.RequireAdminRole(ctx, "read zbdb_internal.gossip_alerts"); err != nil {
			return err
		}

		g := p.ExecCfg().Gossip

		type resultWithNodeID struct {
			roachpb.NodeID
			statuspb.HealthCheckResult
		}
		var results []resultWithNodeID
		if err := g.IterateInfos(gossip.KeyNodeHealthAlertPrefix, func(key string, i gossip.Info) error {
			bytes, err := i.Value.GetBytes()
			if err != nil {
				return errors.Wrapf(err, "failed to extract bytes for key %q", key)
			}

			var d statuspb.HealthCheckResult
			if err := protoutil.Unmarshal(bytes, &d); err != nil {
				return errors.Wrapf(err, "failed to parse value for key %q", key)
			}
			nodeID, err := gossip.NodeIDFromKey(key, gossip.KeyNodeHealthAlertPrefix)
			if err != nil {
				return errors.Wrapf(err, "failed to parse node ID from key %q", key)
			}
			results = append(results, resultWithNodeID{nodeID, d})
			return nil
		}); err != nil {
			return err
		}

		for _, result := range results {
			for _, alert := range result.Alerts {
				storeID := tree.DNull
				if alert.StoreID != 0 {
					storeID = tree.NewDInt(tree.DInt(alert.StoreID))
				}
				if err := addRow(
					tree.NewDInt(tree.DInt(result.NodeID)),
					storeID,
					tree.NewDString(strings.ToLower(alert.Category.String())),
					tree.NewDString(alert.Description),
					tree.NewDFloat(tree.DFloat(alert.Value)),
				); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// znbaseInternalGossipNetwork exposes the local view of the gossip network (i.e
// the gossip client connections from source_id node to target_id node).
var znbaseInternalGossipNetworkTable = virtualSchemaTable{
	comment: "locally known edges in the gossip network (RAM; local node only)",
	schema: `
CREATE TABLE zbdb_internal.gossip_network (
  source_id       INT NOT NULL,    -- source node of a gossip connection
  target_id       INT NOT NULL     -- target node of a gossip connection
)
	`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.RequireAdminRole(ctx, "read zbdb_internal.gossip_network"); err != nil {
			return err
		}

		c := p.ExecCfg().Gossip.Connectivity()
		for _, conn := range c.ClientConns {
			if err := addRow(
				tree.NewDInt(tree.DInt(conn.SourceID)),
				tree.NewDInt(tree.DInt(conn.TargetID)),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

func addPartitioningRows(
	table *sqlbase.TableDescriptor,
	index *sqlbase.IndexDescriptor,
	partitioning *sqlbase.PartitioningDescriptor,
	parentName tree.Datum,
	colOffset int,
	addRow func(...tree.Datum) error,
) error {
	tableID := tree.NewDInt(tree.DInt(table.ID))
	indexID := tree.NewDInt(tree.DInt(index.ID))
	numColumns := tree.NewDInt(tree.DInt(partitioning.NumColumns))

	fakePrefixDatums := make([]tree.Datum, colOffset)
	for i := range fakePrefixDatums {
		fakePrefixDatums[i] = tree.DNull
	}

	column := bytes.Buffer{}
	for i := 0; i < int(partitioning.NumColumns); i++ {
		if i != 0 {
			column.WriteString(", ")
		}
		column.WriteString(index.ColumnNames[colOffset+i])
	}
	for _, l := range partitioning.List {
		buf := bytes.Buffer{}
		buf.WriteString(`IN (`)
		for j, values := range l.Values {
			if j != 0 {
				buf.WriteString(`, `)
			}
			tuple, _, err := sqlbase.DecodePartitionTuple(
				&sqlbase.DatumAlloc{}, table, index, partitioning, values, fakePrefixDatums)
			if err != nil {
				return err
			}
			buf.WriteString(tuple.String())
		}
		buf.WriteString(`)`)
		name := tree.NewDString(l.Name)
		locateIn, leaseIn := checkSpaceNull(l.LocateSpaceName)
		if err := addRow(
			tableID,
			indexID,
			parentName,
			name,
			numColumns,
			locateIn,
			leaseIn,
			tree.NewDString("list"),
			tree.NewDString(column.String()),
			tree.NewDString(buf.String()),
		); err != nil {
			return err
		}
		err := addPartitioningRows(table, index, &l.Subpartitioning, name,
			colOffset+int(partitioning.NumColumns), addRow)
		if err != nil {
			return err
		}
	}

	for _, r := range partitioning.Range {
		buf := bytes.Buffer{}
		buf.WriteString("FROM ")
		fromTuple, _, err := sqlbase.DecodePartitionTuple(
			&sqlbase.DatumAlloc{}, table, index, partitioning, r.FromInclusive, fakePrefixDatums)
		if err != nil {
			return err
		}
		buf.WriteString(fromTuple.String())
		buf.WriteString(" TO ")
		toTuple, _, err := sqlbase.DecodePartitionTuple(
			&sqlbase.DatumAlloc{}, table, index, partitioning, r.ToExclusive, fakePrefixDatums)
		if err != nil {
			return err
		}
		buf.WriteString(toTuple.String())
		locateIn, leaseIn := checkSpaceNull(r.LocateSpaceName)
		if err := addRow(
			tableID,
			indexID,
			parentName,
			tree.NewDString(r.Name),
			numColumns,
			locateIn,
			leaseIn,
			tree.NewDString("range"),
			tree.NewDString(column.String()),
			tree.NewDString(buf.String()),
		); err != nil {
			return err
		}
	}

	return nil
}

// znbaseInternalPartitionsTable decodes and exposes the partitions of each
// table.
//
// TODO(tbg): prefix with cluster_.
var znbaseInternalPartitionsTable = virtualSchemaTable{
	comment: "defined partitions for all tables/indexes accessible by the current user in the current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.partitions (
	table_id    	INT NOT NULL,
	index_id    	INT NOT NULL,
	parent_name 	STRING,
	name        	STRING NOT NULL,
	columns     	INT NOT NULL,
	locate_in   	STRING,
	lease_in		STRING,
	type			STRING,
	column_names	STRING,
	values			STRING
)
	`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		return forEachTableDescAll(ctx, p, dbContext, hideVirtual, /* virtual tables have no partitions*/
			func(db *DatabaseDescriptor, _ string, table *TableDescriptor) error {
				return table.ForeachNonDropIndex(func(index *sqlbase.IndexDescriptor) error {
					return addPartitioningRows(table, index, &index.Partitioning,
						tree.DNull /* parentName */, 0 /* colOffset */, addRow)
				})
			})
	},
}

// DecodeRows converts data into INT.
func DecodeRows(rows []tree.Datums) (int64, error) {
	if len(rows) != 0 {
		row := rows[0]
		if len(row) >= 0 {
			i := row[0].(*tree.DInt)
			return int64(*i), nil
		}
	}
	return 0, nil
}

// getRows make a query to get the table rows.
func getRows(p *planner, tableName string, scName string, dbName string) (int64, error) {
	s := make([]string, 2)
	s[0] = `SELECT count(*) FROM `
	fmtCtx := tree.NewFmtCtx(tree.FmtSimple)
	tbname := tree.MakeTableNameWithSchema(tree.Name(dbName), tree.Name(scName), tree.Name(tableName))
	tbname.Format(fmtCtx)
	s[1] = fmtCtx.String()
	query := strings.Join(s, "")

	op := make([]string, 3)
	op[0] = tbname.SchemaName.String()
	op[1] = "-"
	op[2] = tbname.Table()
	opName := strings.Join(op, "")
	rows, err := p.ExtendedEvalContext().InternalExecutor.Query(
		context.Background(), opName, p.txn, query)
	if err != nil {
		return 0, err
	}
	return DecodeRows(rows)
}

// getPartitionRows get the recent table's partition rows.
func getPartitionRows(
	p *planner, partitionName string, tableName string, scName string, dbName string,
) (int64, error) {
	const partitionquery = `
		SELECT count(*) FROM [partition %[1]s]	of %[2]s; 
	`
	tbName := tree.MakeTableNameWithSchema(tree.Name(dbName), tree.Name(scName), tree.Name(tableName))
	fmtCtx := tree.NewFmtCtx(tree.FmtSimple)
	tbName.Format(fmtCtx)
	opName := fmtCtx.String()
	query := fmt.Sprintf(partitionquery, partitionName, opName)
	rows, err := p.ExtendedEvalContext().InternalExecutor.Query(
		context.Background(), opName, p.txn, query)
	if err != nil {
		return 0, err
	}
	return DecodeRows(rows)
}

// addDefaultRows add default rows in partition_views if default rows > 0,
// and set partitionName is default_indexName.
func addDefaultRows(
	p *planner,
	db *sqlbase.DatabaseDescriptor,
	scName string,
	table *sqlbase.TableDescriptor,
	index *sqlbase.IndexDescriptor,
	partitionRows int,
	addRow func(...tree.Datum) error,
) error {
	tableRows, err := getRows(p, table.Name, scName, db.Name)
	if err != nil {
		return err
	}
	defaultRows := int(tableRows) - partitionRows
	if defaultRows > 0 {
		defaultPartition := fmt.Sprintf("%[1]s_default", index.Name)
		locateIn, leaseIn := checkSpaceNull(index.LocateSpaceName)
		if err := addRow(
			tree.NewDString(db.Name),
			tree.NewDString(scName),
			tree.NewDString(table.Name),
			tree.NewDInt(tree.DInt(table.ID)),
			tree.NewDInt(tree.DInt(index.ID)),
			tree.DNull,
			tree.NewDString(defaultPartition),
			tree.NewDInt(tree.DInt(defaultRows)),
			locateIn,
			leaseIn,
			tree.DBoolTrue,
		); err != nil {
			return err
		}
	}
	return nil
}

// addPartitionViewAllRows is to add all partitionrows and then add defaultrows if default
// partition is not defined.
func addPartitionViewAllRows(
	p *planner,
	db *sqlbase.DatabaseDescriptor,
	scName string,
	table *sqlbase.TableDescriptor,
	index *sqlbase.IndexDescriptor,
	partitioning *sqlbase.PartitioningDescriptor,
	parentName tree.Datum,
	colOffset int,
	addRow func(...tree.Datum) error,
) error {
	partitionRows, err := addPartitioningViewRows(p, db, scName, table, index, partitioning,
		parentName /* parentName */, colOffset /* colOffset */, addRow)
	if err != nil {
		return err
	}
	err = addDefaultRows(p, db, scName, table, index, partitionRows, addRow)
	if err != nil {
		return err
	}
	return nil
}

// addPartitioningViewRows add all defined partition infos and return a sum rows of
// all partitions.
func addPartitioningViewRows(
	p *planner,
	db *sqlbase.DatabaseDescriptor,
	scName string,
	table *sqlbase.TableDescriptor,
	index *sqlbase.IndexDescriptor,
	partitioning *sqlbase.PartitioningDescriptor,
	parentName tree.Datum,
	colOffset int,
	addRow func(...tree.Datum) error,
) (int, error) {
	dbName := tree.NewDString(db.Name)
	tableName := tree.NewDString(table.Name)
	indexID := tree.NewDInt(tree.DInt(index.ID))

	var partitionRows int
	for _, l := range partitioning.List {
		partitionRow, err := getPartitionRows(p, l.Name, table.Name, scName, db.Name)
		if err != nil {
			return 0, err
		}
		if parentName == tree.DNull {
			partitionRows += int(partitionRow)
		}
		partitionName := tree.NewDString(l.Name)
		locateIn, leaseIn := checkSpaceNull(l.LocateSpaceName)
		if err := addRow(
			dbName,
			tree.NewDString(scName),
			tableName,
			tree.NewDInt(tree.DInt(table.ID)),
			indexID,
			parentName,
			partitionName,
			tree.NewDInt(tree.DInt(partitionRow)),
			locateIn,
			leaseIn,
			tree.DBoolFalse,
		); err != nil {
			return 0, err
		}
		_, err = addPartitioningViewRows(p, db, scName, table, index, &l.Subpartitioning, partitionName,
			colOffset+int(partitioning.NumColumns), addRow)
		if err != nil {
			return 0, err
		}
	}

	for _, r := range partitioning.Range {
		partitionRow, err := getPartitionRows(p, r.Name, table.Name, scName, db.Name)
		if err != nil {
			return 0, err
		}
		if parentName == tree.DNull {
			partitionRows += int(partitionRow)
		}
		locateIn, leaseIn := checkSpaceNull(r.LocateSpaceName)
		if err := addRow(
			dbName,
			tree.NewDString(scName),
			tableName,
			tree.NewDInt(tree.DInt(table.ID)),
			indexID,
			parentName,
			tree.NewDString(r.Name),
			tree.NewDInt(tree.DInt(partitionRow)),
			locateIn,
			leaseIn,
			tree.DBoolFalse,
		); err != nil {
			return 0, err
		}
	}

	return partitionRows, nil
}

// znbaseInternalPartitionViewsTable decodes and exposes the partitions of each table.
var znbaseInternalPartitionViewsTable = virtualSchemaTable{
	comment: "defined partition_views for all tables/indexes accessible by the current user in the current database (KV scan)",
	schema: `
CREATE TABLE zbdb_internal.partition_views (
	database_name     STRING NOT NULL,
	schema_name    	  STRING NOT NULL,
	table_name        STRING NOT NULL,
	table_id		  INT NOT NULL, 	
	index_id          INT NOT NULL,
	parent_name       STRING,
	partition_name    STRING NOT NULL,
	table_rows        INT NOT NULL,
	locate_in         STRING,
	lease_in		  STRING,
	is_default        bool
)
	`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		return forEachTableDescAll(ctx, p, dbContext, hideVirtual, /* virtual tables have no partitions*/
			func(db *DatabaseDescriptor, scName string, table *TableDescriptor) error {
				return table.ForeachNonDropIndex(func(index *sqlbase.IndexDescriptor) error {
					if index.Partitioning.NumColumns != 0 {
						return addPartitionViewAllRows(p, db, scName, table, index, &index.Partitioning,
							tree.DNull /* parentName */, 0 /* colOffset */, addRow)
					}
					return nil
				})
			})
	},
}

// znbaseInternalKVNodeStatusTable exposes information from the status server about the cluster nodes.
//
// TODO(tbg): s/kv_/cluster_/
var znbaseInternalKVNodeStatusTable = virtualSchemaTable{
	comment: "node details across the entire cluster (cluster RPC; expensive!)",
	schema: `
CREATE TABLE zbdb_internal.kv_node_status (
  node_id        INT NOT NULL,
  network        STRING NOT NULL,
  address        STRING NOT NULL,
  attrs          JSON NOT NULL,
  locality       JSON NOT NULL,
  server_version STRING NOT NULL,
  go_version     STRING NOT NULL,
  tag            STRING NOT NULL,
  time           STRING NOT NULL,
  revision       STRING NOT NULL,
  cgo_compiler   STRING NOT NULL,
  platform       STRING NOT NULL,
  distribution   STRING NOT NULL,
  type           STRING NOT NULL,
  dependencies   STRING NOT NULL,
  started_at     TIMESTAMP NOT NULL,
  updated_at     TIMESTAMP NOT NULL,
  metrics        JSON NOT NULL,
  args           JSON NOT NULL,
  env            JSON NOT NULL,
  activity       JSON NOT NULL
)
	`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.CheckUserOption(ctx, useroption.CSTATUS); err != nil {
			return err
		}

		response, err := p.ExecCfg().StatusServer.Nodes(ctx, &serverpb.NodesRequest{})
		if err != nil {
			return err
		}

		for _, n := range response.Nodes {
			attrs := json.NewArrayBuilder(len(n.Desc.Attrs.Attrs))
			for _, a := range n.Desc.Attrs.Attrs {
				attrs.Add(json.FromString(a))
			}

			locality := json.NewObjectBuilder(len(n.Desc.Locality.Tiers))
			for _, t := range n.Desc.Locality.Tiers {
				locality.Add(t.Key, json.FromString(t.Value))
			}

			var dependencies string
			if n.BuildInfo.Dependencies == nil {
				dependencies = ""
			} else {
				dependencies = *(n.BuildInfo.Dependencies)
			}

			metrics := json.NewObjectBuilder(len(n.Metrics))
			for k, v := range n.Metrics {
				metric, err := json.FromFloat64(v)
				if err != nil {
					return err
				}
				metrics.Add(k, metric)
			}

			args := json.NewArrayBuilder(len(n.Args))
			for _, a := range n.Args {
				args.Add(json.FromString(a))
			}

			env := json.NewArrayBuilder(len(n.Env))
			for _, v := range n.Env {
				env.Add(json.FromString(v))
			}

			activity := json.NewObjectBuilder(len(n.Activity))
			for nodeID, values := range n.Activity {
				b := json.NewObjectBuilder(3)
				b.Add("incoming", json.FromInt64(values.Incoming))
				b.Add("outgoing", json.FromInt64(values.Outgoing))
				b.Add("latency", json.FromInt64(values.Latency))
				activity.Add(nodeID.String(), b.Build())
			}

			if err := addRow(
				tree.NewDInt(tree.DInt(n.Desc.NodeID)),
				tree.NewDString(n.Desc.Address.NetworkField),
				tree.NewDString(n.Desc.Address.AddressField),
				tree.NewDJSON(attrs.Build()),
				tree.NewDJSON(locality.Build()),
				tree.NewDString(n.Desc.ServerVersion.String()),
				tree.NewDString(n.BuildInfo.GoVersion),
				tree.NewDString(n.BuildInfo.Tag),
				tree.NewDString(n.BuildInfo.Time),
				tree.NewDString(n.BuildInfo.Revision),
				tree.NewDString(n.BuildInfo.CgoCompiler),
				tree.NewDString(n.BuildInfo.Platform),
				tree.NewDString(n.BuildInfo.Distribution),
				tree.NewDString(n.BuildInfo.Type),
				tree.NewDString(dependencies),
				tree.MakeDTimestamp(timeutil.Unix(0, n.StartedAt), time.Microsecond),
				tree.MakeDTimestamp(timeutil.Unix(0, n.UpdatedAt), time.Microsecond),
				tree.NewDJSON(metrics.Build()),
				tree.NewDJSON(args.Build()),
				tree.NewDJSON(env.Build()),
				tree.NewDJSON(activity.Build()),
			); err != nil {
				return err
			}
		}
		return nil
	},
}

// znbaseInternalKVStoreStatusTable exposes information about the cluster stores.
//
// TODO(tbg): s/kv_/cluster_/
var znbaseInternalKVStoreStatusTable = virtualSchemaTable{
	comment: "store details and status (cluster RPC; expensive!)",
	schema: `
CREATE TABLE zbdb_internal.kv_store_status (
  node_id            INT NOT NULL,
  store_id           INT NOT NULL,
  attrs              JSON NOT NULL,
  state              STRING NOT NULL,
  capacity           INT NOT NULL,
  available          INT NOT NULL,
  used               INT NOT NULL,
  logical_bytes      INT NOT NULL,
  range_count        INT NOT NULL,
  lease_count        INT NOT NULL,
  writes_per_second  FLOAT NOT NULL,
  bytes_per_replica  JSON NOT NULL,
  writes_per_replica JSON NOT NULL,
  metrics            JSON NOT NULL
)
	`,
	populate: func(ctx context.Context, p *planner, _ *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		if err := p.CheckUserOption(ctx, useroption.CSTATUS); err != nil {
			return err
		}

		response, err := p.ExecCfg().StatusServer.Nodes(ctx, &serverpb.NodesRequest{})
		if err != nil {
			return err
		}

		for _, n := range response.Nodes {
			for _, s := range n.StoreStatuses {
				attrs := json.NewArrayBuilder(len(s.Desc.Attrs.Attrs))
				for _, a := range s.Desc.Attrs.Attrs {
					attrs.Add(json.FromString(a))
				}

				metrics := json.NewObjectBuilder(len(s.Metrics))
				for k, v := range s.Metrics {
					metric, err := json.FromFloat64(v)
					if err != nil {
						return err
					}
					metrics.Add(k, metric)
				}

				percentilesToJSON := func(ps roachpb.Percentiles) (json.JSON, error) {
					b := json.NewObjectBuilder(5)
					v, err := json.FromFloat64(ps.P10)
					if err != nil {
						return nil, err
					}
					b.Add("P10", v)
					v, err = json.FromFloat64(ps.P25)
					if err != nil {
						return nil, err
					}
					b.Add("P25", v)
					v, err = json.FromFloat64(ps.P50)
					if err != nil {
						return nil, err
					}
					b.Add("P50", v)
					v, err = json.FromFloat64(ps.P75)
					if err != nil {
						return nil, err
					}
					b.Add("P75", v)
					v, err = json.FromFloat64(ps.P90)
					if err != nil {
						return nil, err
					}
					b.Add("P90", v)
					v, err = json.FromFloat64(ps.PMax)
					if err != nil {
						return nil, err
					}
					b.Add("PMax", v)
					return b.Build(), nil
				}

				bytesPerReplica, err := percentilesToJSON(s.Desc.Capacity.BytesPerReplica)
				if err != nil {
					return err
				}
				writesPerReplica, err := percentilesToJSON(s.Desc.Capacity.WritesPerReplica)
				if err != nil {
					return err
				}

				if err := addRow(
					tree.NewDInt(tree.DInt(s.Desc.Node.NodeID)),
					tree.NewDInt(tree.DInt(s.Desc.StoreID)),
					tree.NewDJSON(attrs.Build()),
					tree.NewDString(roachpb.StoreDescriptor_StoreState_name[int32(s.Desc.State)]),
					tree.NewDInt(tree.DInt(s.Desc.Capacity.Capacity)),
					tree.NewDInt(tree.DInt(s.Desc.Capacity.Available)),
					tree.NewDInt(tree.DInt(s.Desc.Capacity.Used)),
					tree.NewDInt(tree.DInt(s.Desc.Capacity.LogicalBytes)),
					tree.NewDInt(tree.DInt(s.Desc.Capacity.RangeCount)),
					tree.NewDInt(tree.DInt(s.Desc.Capacity.LeaseCount)),
					tree.NewDFloat(tree.DFloat(s.Desc.Capacity.WritesPerSecond)),
					tree.NewDJSON(bytesPerReplica),
					tree.NewDJSON(writesPerReplica),
					tree.NewDJSON(metrics.Build()),
				); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// znbaseInternalPredefinedComments exposes the predefined
// comments for virtual tables. This is used by SHOW TABLES WITH COMMENT
// as fall-back when system.comments is silent.
// TODO(knz): extend this with vtable column comments.
//
// TODO(tbg): prefix with node_.
var znbaseInternalPredefinedCommentsTable = virtualSchemaTable{
	comment: `comments for predefined virtual tables (RAM/static)`,
	schema: `
CREATE TABLE zbdb_internal.predefined_comments (
	TYPE      INT,
	OBJECT_ID INT,
	SUB_ID    INT,
	COMMENT   STRING
)`,
	populate: func(
		ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error,
	) error {
		tableCommentKey := tree.NewDInt(keys.TableCommentType)
		vt := p.getVirtualTabler()
		vEntries := vt.getEntries()
		vSchemaNames := vt.getSchemaNames()

		for _, virtSchemaName := range vSchemaNames {
			e := vEntries[virtSchemaName]

			for _, tName := range e.orderedDefNames {
				vTableEntry := e.defs[tName]
				table := vTableEntry.desc

				if vTableEntry.comment != "" {
					if err := addRow(
						tableCommentKey,
						tree.NewDInt(tree.DInt(table.ID)),
						zeroVal,
						tree.NewDString(vTableEntry.comment)); err != nil {
						return err
					}
				}
			}
		}

		return nil
	},
}

var znbaseInternalDatabasesTable = virtualSchemaTable{
	comment: `databases accessible by the current user (KV scan)`,
	schema: `
CREATE TABLE zbdb_internal.databases (
	ID				OID,
	NAME   			STRING NOT NULL,
	OWNER			STRING NOT NULL
)`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		return forEachDatabaseDesc(ctx, p, dbContext, true, func(db *sqlbase.DatabaseDescriptor) error {
			dbNameStr := tree.NewDString(db.Name)
			dbID := tree.NewDOid(tree.DInt(db.ID))
			if err := addRow(
				dbID,
				dbNameStr,                           // table_catalog
				tree.NewDString(getOwnerOfDesc(db)), // owner
			); err != nil {
				return err
			}
			return nil
		})
	},
}

//add functions_privileges table to save procedures and functions privileges
//FIX: NEWSQL-11955
var znbaseInternalFunctionPrivileges = virtualSchemaTable{
	comment: `function privileges (incomplete; may contain excess users or roles)`,
	schema: `
CREATE TABLE zbdb_internal.function_privileges (
	ID				 STRING NOT NULL,
	GRANTOR       	 STRING NOT NULL,
	GRANTEE       	 STRING NOT NULL,
    FUNCTION_CATALOG STRING NOT NULL,
    FUNCTION_SCHEMA  STRING NOT NULL,
    FUNCTION_NAME    STRING NOT NULL,
	ARG_NAME_TYPES   STRING NOT NULL,
	PRIVILEGE_TYPE 	 STRING NOT NULL,
	IS_GRANTABLE   	 STRING
)`,
	populate: func(ctx context.Context, p *planner, dbContext *DatabaseDescriptor, addRow func(...tree.Datum) error) error {
		return forEachFunctionDesc(ctx, p, dbContext,
			func(db *sqlbase.DatabaseDescriptor, scName string, function *sqlbase.FunctionDescriptor) error {
				privs := function.Privileges.Show()
				funcID := strconv.Itoa(int(function.ID))
				dbNameStr := tree.NewDString(db.Name)
				scNameStr := tree.NewDString(scName)

				argAndTypeStr := ""
				for i, arg := range function.Args {
					if i != 0 {
						argAndTypeStr += ", "
					}
					argAndTypeStr += arg.ColumnTypeString
				}

				for _, u := range privs {
					userNameStr := tree.NewDString(u.User)
					for _, priv := range u.Privileges {
						// general user couldn't show grants on object without any privileges.
						hasAdminRole, err := p.HasAdminRole(ctx)
						if err != nil {
							return err
						}
						if !hasAdminRole {
							// if user is not root or in Admin Role
							hasPriv, err := checkRowPriv(ctx, p, priv.Grantor, u.User, p.User())
							if err != nil {
								return err
							}
							if !hasPriv {
								continue
							}
						}

						if err := addRow(
							tree.NewDString(funcID),        // pid
							tree.NewDString(priv.Grantor),  // grantor
							userNameStr,                    // grantee
							dbNameStr,                      // function_catalog
							scNameStr,                      // function_schema
							tree.NewDString(function.Name), // function_name
							tree.NewDString(argAndTypeStr), // arh_name_types
							tree.NewDString(priv.Type),     // privilege_type
							yesOrNoDatum(priv.GrantAble),   // is_grantable
						); err != nil {
							return err
						}
					}
				}
				return nil
			},
		)
	},
}
