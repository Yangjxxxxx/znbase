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
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package tpcc

import (
	"context"
	gosql "database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/syncutil"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/util/uint128"
	"github.com/znbasedb/znbase/pkg/util/uuid"
	"github.com/znbasedb/znbase/pkg/workload"
	"github.com/znbasedb/znbase/pkg/workload/histogram"
	"golang.org/x/exp/rand"
	"golang.org/x/sync/errgroup"
)

type tpcc struct {
	flags     workload.Flags
	connFlags *workload.ConnFlags

	seed             uint64
	warehouses       int
	activeWarehouses int
	interleaved      bool
	nowString        string
	numConns         int

	mix        string
	doWaits    bool
	workers    int
	fks        bool
	rc         bool
	dbOverride string

	txInfos []txInfo
	// deck contains indexes into the txInfos slice.
	deck []int

	auditor *auditor

	reg *histogram.Registry

	split   bool
	scatter bool

	partitions         int
	numReplicas        int
	affinityPartitions []int
	zones              []string
	spaces             []string
	wPart              *partitioner

	usePostgres  bool
	serializable bool
	txOpts       *pgx.TxOptions

	expensiveChecks bool

	randomCIDsCache struct {
		syncutil.Mutex
		values [][]int
	}
	localsPool *sync.Pool
}

func init() {
	workload.Register(tpccMeta)
}

// FromWarehouses returns a tpcc generator pre-configured with the specified
// number of warehouses.
func FromWarehouses(warehouses int) workload.Generator {
	return workload.FromFlags(tpccMeta, fmt.Sprintf(`--warehouses=%d`, warehouses))
}

var tpccMeta = workload.Meta{
	Name: `tpcc`,
	Description: `TPC-C simulates a transaction processing workload` +
		` using a rich schema of multiple tables`,
	// TODO(anyone): when bumping this version and regenerating fixtures, please
	// address the TODO in PostLoad.
	Version:      `2.0.1`,
	PublicFacing: true,
	New: func() workload.Generator {
		g := &tpcc{}
		g.flags.FlagSet = pflag.NewFlagSet(`tpcc`, pflag.ContinueOnError)
		g.flags.Meta = map[string]workload.FlagMeta{
			`db`:                 {RuntimeOnly: true},
			`fks`:                {RuntimeOnly: true},
			`mix`:                {RuntimeOnly: true},
			`partitions`:         {RuntimeOnly: true},
			`replicas`:           {RuntimeOnly: true},
			`partition-affinity`: {RuntimeOnly: true},
			`scatter`:            {RuntimeOnly: true},
			`serializable`:       {RuntimeOnly: true},
			`split`:              {RuntimeOnly: true},
			`wait`:               {RuntimeOnly: true},
			`workers`:            {RuntimeOnly: true},
			`conns`:              {RuntimeOnly: true},
			`zones`:              {RuntimeOnly: true},
			`active-warehouses`:  {RuntimeOnly: true},
			`expensive-checks`:   {RuntimeOnly: true, CheckConsistencyOnly: true},
			`rc`:                 {RuntimeOnly: true},
		}

		g.flags.Uint64Var(&g.seed, `seed`, 1, `Random number generator seed`)
		g.flags.IntVar(&g.warehouses, `warehouses`, 1, `Number of warehouses for loading`)
		g.flags.BoolVar(&g.interleaved, `interleaved`, false, `Use interleaved tables`)
		// Hardcode this since it doesn't seem like anyone will want to change
		// it and it's really noisy in the generated fixture paths.
		g.nowString = `2006-01-02 15:04:05`

		g.flags.StringVar(&g.mix, `mix`,
			`newOrder=10,payment=10,orderStatus=1,delivery=1,stockLevel=1`,
			`Weights for the transaction mix. The default matches the TPCC spec.`)

		g.flags.BoolVar(&g.doWaits, `wait`, true, `Run in wait mode (include think/keying sleeps)`)
		g.flags.StringVar(&g.dbOverride, `db`, ``,
			`Override for the SQL database to use. If empty, defaults to the generator name`)

		g.flags.IntVar(&g.workers, `workers`, 0, fmt.Sprintf(
			`Number of concurrent workers. Defaults to --warehouses * %d`, numWorkersPerWarehouse,
		))
		g.flags.IntVar(&g.numConns, `conns`, 0, fmt.Sprintf(
			`Number of connections. Defaults to --warehouses * %d (except in nowait mode, where it defaults to --workers`,
			numConnsPerWarehouse,
		))

		g.flags.BoolVar(&g.fks, `fks`, true, `Add the foreign keys`)
		g.flags.IntVar(&g.partitions, `partitions`, 1, `Partition tables (requires split)`)
		g.flags.IntVar(&g.numReplicas, `replicas`, 1, `number of replicas used for partitions`)
		g.flags.IntSliceVar(&g.affinityPartitions, `partition-affinity`, nil, `Run load generator against specific partition (requires partitions). `+
			`Note that if one value is provided, the assumption is that all urls are associated with that partition. In all other cases the assumption `+
			`is that the URLs are distributed evenly over the partitions`)
		g.flags.IntVar(&g.activeWarehouses, `active-warehouses`, 0, `Run the load generator against a specific number of warehouses. Defaults to --warehouses'`)
		g.flags.BoolVar(&g.scatter, `scatter`, false, `Scatter ranges`)
		g.flags.BoolVar(&g.serializable, `serializable`, false, `Force serializable mode`)
		g.flags.BoolVar(&g.split, `split`, false, `Split tables`)
		g.flags.StringSliceVar(&g.zones, "zones", []string{}, "Zones for partitioning, the number of zones should match the number of partitions and the zones used to start znbase.")
		g.flags.StringSliceVar(&g.spaces, "spaces", []string{}, "Space for partitioning, the number of zones should match the number of partitions and the zones used to start znbase.")

		g.flags.BoolVar(&g.expensiveChecks, `expensive-checks`, false, `Run expensive checks`)
		g.flags.BoolVar(&g.rc, `rc`, false, `read committed`)
		g.connFlags = workload.NewConnFlags(&g.flags)
		return g
	},
}

// Meta implements the Generator interface.
func (*tpcc) Meta() workload.Meta { return tpccMeta }

// Flags implements the Flagser interface.
func (w *tpcc) Flags() workload.Flags { return w.flags }

// Hooks implements the Hookser interface.
func (w *tpcc) Hooks() workload.Hooks {
	return workload.Hooks{
		Validate: func() error {
			if w.warehouses < 1 {
				return errors.Errorf(`--warehouses must be positive`)
			}

			if w.activeWarehouses > w.warehouses {
				return errors.Errorf(`--active-warehouses needs to be less than or equal to warehouses`)
			} else if w.activeWarehouses == 0 {
				w.activeWarehouses = w.warehouses
			}

			if w.partitions < 1 {
				return errors.Errorf(`--partitions must be positive`)
			} else if w.partitions > 1 && !w.split {
				return errors.Errorf(`multiple partitions requires --split`)
			}

			for _, p := range w.affinityPartitions {
				if p < 0 || p >= w.partitions {
					return errors.Errorf(`--partition-affinity out of bounds of --partitions`)
				}
			}

			if len(w.zones) > 0 && (len(w.spaces) != w.partitions) {
				return errors.Errorf(`--spaces should have the sames length as --partitions.`)
			}

			if w.workers == 0 {
				w.workers = w.activeWarehouses * numWorkersPerWarehouse
			}

			if w.numConns == 0 {
				// If we're not waiting, open up a connection for each worker. If we are
				// waiting, we only use up to a set number of connections per warehouse.
				// This isn't mandated by the spec, but opening a connection per worker
				// when they each spend most of their time waiting is wasteful.
				if !w.doWaits {
					w.numConns = w.workers
				} else {
					w.numConns = w.activeWarehouses * numConnsPerWarehouse
				}
			}

			if w.doWaits && w.workers != w.activeWarehouses*numWorkersPerWarehouse {
				return errors.Errorf(`--wait=true and --warehouses=%d requires --workers=%d`,
					w.activeWarehouses, w.warehouses*numWorkersPerWarehouse)
			}

			if w.serializable {
				w.txOpts = &pgx.TxOptions{IsoLevel: pgx.Serializable}
			}

			w.auditor = newAuditor(w.warehouses)

			return initializeMix(w)
		},
		PostLoad: func(sqlDB *gosql.DB) error {
			if w.fks {
				fkStmts := []string{
					`alter table district add foreign key (d_w_id) references warehouse (w_id)`,
					`alter table customer add foreign key (c_w_id, c_d_id) references district (d_w_id, d_id)`,
					`alter table history add foreign key (h_c_w_id, h_c_d_id, h_c_id) references customer (c_w_id, c_d_id, c_id)`,
					`alter table history add foreign key (h_w_id, h_d_id) references district (d_w_id, d_id)`,
					`alter table "order" add foreign key (o_w_id, o_d_id, o_c_id) references customer (c_w_id, c_d_id, c_id)`,
					`alter table stock add foreign key (s_w_id) references warehouse (w_id)`,
					`alter table stock add foreign key (s_i_id) references item (i_id)`,
					`alter table order_line add foreign key (ol_w_id, ol_d_id, ol_o_id) references "order" (o_w_id, o_d_id, o_id)`,
				}

				// TODO(anyone): Remove this check. Once fixtures are
				// regenerated and the meta version is bumped on this workload,
				// we won't need it anymore.
				{
					const q = `SELECT column_name
						       FROM information_schema.statistics
						       WHERE index_name = 'order_line_fk'
						         AND seq_in_index = 2`
					var fkCol string
					if err := sqlDB.QueryRow(q).Scan(&fkCol); err != nil {
						return err
					}
					var fkStmt string
					switch fkCol {
					case "ol_i_id":
						// The corrected column. When the TODO above is addressed,
						// this should be moved into fkStmts.
						fkStmt = `alter table order_line add foreign key (ol_supply_w_id, ol_i_id) references stock (s_w_id, s_i_id)`
					case "ol_d_id":
						// The old, incorrect column. When the TODO above is addressed,
						// this should be removed entirely.
						fkStmt = `alter table order_line add foreign key (ol_supply_w_id, ol_d_id) references stock (s_w_id, s_i_id)`
					default:
						return errors.Errorf("unexpected column %q in order_line_fk", fkCol)
					}
					fkStmts = append(fkStmts, fkStmt)
				}

				for _, fkStmt := range fkStmts {
					if _, err := sqlDB.Exec(fkStmt); err != nil {
						// If the statement failed because the fk already exists,
						// ignore it. Return the error for any other reason.
						const duplFKErr = "columns cannot be used by multiple foreign key constraints"
						if !strings.Contains(err.Error(), duplFKErr) {
							return err
						}
					}
				}
			}
			return nil
		},
		PostRun: func(startElapsed time.Duration) error {
			w.auditor.runChecks()
			const totalHeader = "\n_elapsed_______tpmC____efc__avg(ms)__p50(ms)__p90(ms)__p95(ms)__p99(ms)_pMax(ms)"
			fmt.Println(totalHeader)

			const newOrderName = `newOrder`
			w.reg.Tick(func(t histogram.Tick) {
				if newOrderName == t.Name {
					tpmC := float64(t.Cumulative.TotalCount()) / startElapsed.Seconds() * 60
					fmt.Printf("%7.1fs %10.1f %5.1f%% %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f\n",
						startElapsed.Seconds(),
						tpmC,
						100*tpmC/(SpecWarehouseFactor*float64(w.activeWarehouses)),
						time.Duration(t.Cumulative.Mean()).Seconds()*1000,
						time.Duration(t.Cumulative.ValueAtQuantile(50)).Seconds()*1000,
						time.Duration(t.Cumulative.ValueAtQuantile(90)).Seconds()*1000,
						time.Duration(t.Cumulative.ValueAtQuantile(95)).Seconds()*1000,
						time.Duration(t.Cumulative.ValueAtQuantile(99)).Seconds()*1000,
						time.Duration(t.Cumulative.ValueAtQuantile(100)).Seconds()*1000,
					)
				}
			})
			return nil
		},
		CheckConsistency: func(ctx context.Context, db *gosql.DB) error {
			// TODO(arjun): We should run each test in a single transaction as
			// currently we have to shut down load before running the checks.
			for _, check := range allChecks() {
				if !w.expensiveChecks && check.expensive {
					continue
				}
				start := timeutil.Now()
				err := check.f(db)
				log.Infof(ctx, `check %s took %s`, check.name, timeutil.Since(start))
				if err != nil {
					return errors.Wrapf(err, `check failed: %s`, check.name)
				}
			}
			return nil
		},
	}
}

// Tables implements the Generator interface.
func (w *tpcc) Tables() []workload.Table {
	if w.localsPool == nil {
		w.localsPool = &sync.Pool{
			New: func() interface{} {
				return &generateLocals{
					rng: rand.New(rand.NewSource(uint64(timeutil.Now().UnixNano()))),
				}
			},
		}
	}

	// splits is a convenience method for constructing table splits that returns
	// a zero value if the workload does not have splits enabled.
	splits := func(t workload.BatchedTuples) workload.BatchedTuples {
		if w.split {
			return t
		}
		return workload.BatchedTuples{}
	}

	// numBatches is a helper to calculate how many split batches exist exist given
	// the total number of rows and the desired number of rows per split.
	numBatches := func(total, per int) int {
		batches := total / per
		if total%per == 0 {
			batches--
		}
		return batches
	}
	warehouse := workload.Table{
		Name:   `warehouse`,
		Schema: fmt.Sprintf(tpccWarehouseSchema, ""),
		InitialRows: workload.Tuples(
			w.warehouses,
			w.tpccWarehouseInitialRow,
		),
		Splits: splits(workload.BatchedTuples{
			NumBatches: numBatches(w.warehouses, numWarehousesPerRange),
			NumTotal:   w.warehouses,
			Batch: func(i int) [][]interface{} {
				return [][]interface{}{{(i + 1) * numWarehousesPerRange}}
			},
		}),
		Stats: w.tpccWarehouseStats(),
	}
	district := workload.Table{
		Name:   `district`,
		Schema: fmt.Sprintf(tpccDistrictSchema, ""),
		InitialRows: workload.Tuples(
			numDistrictsPerWarehouse*w.warehouses,
			w.tpccDistrictInitialRow,
		),
		Splits: splits(workload.BatchedTuples{
			NumBatches: numBatches(w.warehouses, numWarehousesPerRange),
			NumTotal:   w.warehouses,
			Batch: func(i int) [][]interface{} {
				return [][]interface{}{{(i + 1) * numWarehousesPerRange, 0}}
			},
		}),
		Stats: w.tpccDistrictStats(),
	}
	customer := workload.Table{
		Name:   `customer`,
		Schema: fmt.Sprintf(tpccCustomerSchema, "", ""),
		InitialRows: workload.Tuples(
			numCustomersPerWarehouse*w.warehouses,
			w.tpccCustomerInitialRow,
		),
		Stats: w.tpccCustomerStats(),
	}
	history := workload.Table{
		Name:   `history`,
		Schema: fmt.Sprintf(tpccHistorySchema, "", ""),
		InitialRows: workload.Tuples(
			numCustomersPerWarehouse*w.warehouses,
			w.tpccHistoryInitialRow,
		),
		Splits: splits(workload.BatchedTuples{
			NumBatches: historyRanges - 1,
			Batch: func(i int) [][]interface{} {
				at := uint128.FromInts(uint64(i+1)*numHistoryValsPerRange, 0)
				return [][]interface{}{
					{uuid.FromUint128(at).String()},
				}
			},
		}),
		Stats: w.tpccHistoryStats(),
	}
	order := workload.Table{
		Name:   `order`,
		Schema: fmt.Sprintf(tpccOrderSchema, "", "", ""),
		InitialRows: workload.Tuples(
			numOrdersPerWarehouse*w.warehouses,
			w.tpccOrderInitialRow,
		),
		Stats: w.tpccOrderStats(),
	}
	newOrder := workload.Table{
		Name:   `new_order`,
		Schema: fmt.Sprintf(tpccNewOrderSchema, ""),
		InitialRows: workload.Tuples(
			numNewOrdersPerWarehouse*w.warehouses,
			w.tpccNewOrderInitialRow,
		),
		Stats: w.tpccNewOrderStats(),
	}
	item := workload.Table{
		Name:   `item`,
		Schema: tpccItemSchema,
		InitialRows: workload.Tuples(
			numItems,
			w.tpccItemInitialRow,
		),
		Splits: splits(workload.BatchedTuples{
			NumBatches: numBatches(numItems, numItemsPerRange),
			NumTotal:   numItems,
			Batch: func(i int) [][]interface{} {
				return [][]interface{}{{numItemsPerRange * (i + 1)}}
			},
		}),
		Stats: w.tpccItemStats(),
	}
	stock := workload.Table{
		Name:   `stock`,
		Schema: fmt.Sprintf(tpccStockSchema, ""),
		InitialRows: workload.Tuples(
			numStockPerWarehouse*w.warehouses,
			w.tpccStockInitialRow,
		),
		Stats: w.tpccStockStats(),
	}
	orderLine := workload.Table{
		Name:   `order_line`,
		Schema: fmt.Sprintf(tpccOrderLineSchema, "", ""),
		InitialRows: workload.BatchedTuples{
			NumBatches: numOrdersPerWarehouse * w.warehouses,
			Batch:      w.tpccOrderLineInitialRowBatch,
		},
		Stats: w.tpccOrderLineStats(),
	}

	if w.partitions*w.numReplicas > 1 {
		var err error
		w.wPart, err = makePartitioner(w.warehouses, w.activeWarehouses, w.partitions*w.numReplicas, w.numReplicas)
		if err != nil {
			panic(err)
		}

		warehouse.Schema = fmt.Sprintf(tpccWarehouseSchema,
			partitionObjectInit(w.wPart, w.spaces, "w_id", 0),
		)
		district.Schema = fmt.Sprintf(tpccDistrictSchema,
			partitionObjectInit(w.wPart, w.spaces, "d_w_id", 0),
		)
		customer.Schema = fmt.Sprintf(tpccCustomerSchema,
			partitionObjectInit(w.wPart, w.spaces, "c_w_id", 0),
			partitionObjectInit(w.wPart, w.spaces, "c_w_id", 1),
		)
		history.Schema = fmt.Sprintf(tpccHistorySchema,
			partitionObjectInit(w.wPart, w.spaces, "h_w_id", 1),
			partitionObjectInit(w.wPart, w.spaces, "h_c_w_id", 2),
		)
		order.Schema = fmt.Sprintf(tpccOrderSchema,
			partitionObjectInit(w.wPart, w.spaces, "o_w_id", 0),
			partitionObjectInit(w.wPart, w.spaces, "o_w_id", 1),
			partitionObjectInit(w.wPart, w.spaces, "o_w_id", 2),
		)
		newOrder.Schema = fmt.Sprintf(tpccNewOrderSchema,
			partitionObjectInit(w.wPart, w.spaces, "no_w_id", 0),
		)
		stock.Schema = fmt.Sprintf(tpccStockSchema,
			partitionObjectInit(w.wPart, w.spaces, "s_w_id", 0),
		)
		orderLine.Schema = fmt.Sprintf(tpccOrderLineSchema,
			partitionObjectInit(w.wPart, w.spaces, "ol_w_id", 0),
			partitionObjectInit(w.wPart, w.spaces, "ol_supply_w_id", 1),
		)
	}

	if w.interleaved {
		district.Schema += tpccDistrictSchemaInterleave
		customer.Schema += tpccCustomerSchemaInterleave
		order.Schema += tpccOrderSchemaInterleave
		stock.Schema += tpccStockSchemaInterleave
		orderLine.Schema += tpccOrderLineSchemaInterleave
		// This natural-seeming interleave makes performance worse, because this
		// table has a ton of churn and produces a lot of MVCC tombstones, which
		// then will gum up the works of scans over the parent table.
		_ = tpccNewOrderSchemaInterleave
	}
	return []workload.Table{
		warehouse, district, customer, history, order, newOrder, item, stock, orderLine,
	}
}

// Ops implements the Opser interface.
func (w *tpcc) Ops(urls []string, reg *histogram.Registry) (workload.QueryLoad, error) {
	sqlDatabase, err := workload.SanitizeUrls(w, w.dbOverride, urls)
	if err != nil {
		return workload.QueryLoad{}, err
	}
	parsedURL, err := url.Parse(urls[0])
	if err != nil {
		return workload.QueryLoad{}, err
	}

	w.reg = reg
	w.usePostgres = parsedURL.Port() == "5432"

	// We can't use a single MultiConnPool because we want to implement partition
	// affinity. Instead we have one MultiConnPool per server.
	cfg := workload.MultiConnPoolCfg{
		MaxTotalConnections: (w.numConns + len(urls) - 1) / len(urls), // round up
		// Limit the number of connections per pool (otherwise preparing statements
		// at startup can be slow).
		MaxConnsPerPool: 50,
	}
	fmt.Printf("Initializing %d connections...\n", w.numConns)
	dbs := make([]*workload.MultiConnPool, len(urls))
	var g errgroup.Group
	for i := range urls {
		i := i
		g.Go(func() error {
			var err error
			dbs[i], err = workload.NewMultiConnPool(cfg, urls[i])
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return workload.QueryLoad{}, err
	}

	// Create a partitioner to help us partition the warehouses. The base-case is
	// where w.warehouses == w.activeWarehouses and w.partitions == 1.
	w.wPart, err = makePartitioner(w.warehouses, w.activeWarehouses, w.partitions*w.numReplicas, w.numReplicas)
	if err != nil {
		return workload.QueryLoad{}, err
	}

	// We're adding this check here because repartitioning a table can take
	// upwards of 10 minutes so if a cluster is already set up correctly we won't
	// do this operation again.
	alreadyPartitioned, err := isTableAlreadyPartitioned(dbs[0].Get())
	if err != nil {
		return workload.QueryLoad{}, err
	}

	if shouldPartition := w.split && w.partitions*w.numReplicas > 1; shouldPartition && !alreadyPartitioned {
		partitionTables(dbs[0].Get(), w.wPart, w.spaces)
	} else if shouldPartition {
		fmt.Println("Tables are not being partitioned because they've been previously partitioned.")
	}

	if w.scatter {
		scatterRanges(dbs[0].Get())
	}

	// Assign each DB connection pool to a local partition. This assumes that
	// dbs[i] is a machine that holds partition "i % *partitions". If we have an
	// affinity partition, all connections will be for the same partition.
	partitions := w.partitions * w.numReplicas
	partitionDBs := make([][]*workload.MultiConnPool, partitions)
	if len(w.affinityPartitions) == 1 {
		// All connections are for our local partitions.
		partitionDBs[w.affinityPartitions[0]] = dbs

	} else {
		for i, db := range dbs {
			p := i % partitions
			partitionDBs[p] = append(partitionDBs[p], db)
		}
		for i := range partitionDBs {
			// Possible if we have more partitions than DB connections.
			if partitionDBs[i] == nil {
				partitionDBs[i] = dbs
			}
		}
	}

	fmt.Printf("Initializing %d workers and preparing statements...\n", w.workers)
	ql := workload.QueryLoad{SQLDatabase: sqlDatabase}
	ql.WorkerFns = make([]func(context.Context) error, 0, w.workers)
	var group errgroup.Group

	// Determines whether a partition is in the local workload's set of affinity
	// partitions.
	isMyPart := func(p int) bool {
		for _, ap := range w.affinityPartitions {
			if p == ap {
				return true
			}
		}
		// If nothing is mine, then everything is mine.
		return len(w.affinityPartitions) == 0
	}

	// Limit the amount of workers we initialize in parallel, to avoid running out
	// of memory (#36897).
	sem := make(chan struct{}, 100)
	for workerIdx := 0; workerIdx < w.workers; workerIdx++ {
		workerIdx := workerIdx
		warehouse := w.wPart.totalElems[workerIdx%len(w.wPart.totalElems)]

		p := w.wPart.partElemsMap[warehouse]
		// This isn't part of our local partition.
		if !isMyPart(p) {
			continue
		}
		dbs := partitionDBs[p]
		db := dbs[warehouse%len(dbs)]

		// NB: ql.WorkerFns is sized so this never re-allocs.
		ql.WorkerFns = append(ql.WorkerFns, nil)
		idx := len(ql.WorkerFns) - 1
		sem <- struct{}{}
		group.Go(func() error {
			worker, err := newWorker(context.TODO(), w, db, reg.GetHandle(), warehouse)
			if err == nil {
				ql.WorkerFns[idx] = worker.run
			}
			<-sem
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return workload.QueryLoad{}, err
	}
	// Preregister all of the histograms so they always print.
	for _, tx := range allTxs {
		reg.GetHandle().Get(tx.name)
	}
	return ql, nil
}
