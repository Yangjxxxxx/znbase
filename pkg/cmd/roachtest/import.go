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
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/util/retry"
)

const (
	gcsTestBucket = `znbase-tmp`
)

func registerImportTPCC(r *registry) {
	runImportTPCC := func(ctx context.Context, t *test, c *cluster, warehouses int) {
		c.Put(ctx, znbase, "./znbase")
		c.Put(ctx, workload, "./workload")
		t.Status("starting csv servers")
		c.Start(ctx, t)
		c.Run(ctx, c.All(), `./workload csv-server --port=8081 &> logs/workload-csv-server.log < /dev/null &`)

		t.Status("running workload")
		m := newMonitor(ctx, c)
		dul := NewDiskUsageLogger(c)
		m.Go(dul.Runner)
		hc := NewHealthChecker(c, c.All())
		m.Go(hc.Runner)

		m.Go(func(ctx context.Context) error {
			defer dul.Done()
			defer hc.Done()
			cmd := fmt.Sprintf(
				`./workload fixtures make tpcc --warehouses=%d --csv-server='http://localhost:8081' `+
					`--gcs-bucket-override=%s --gcs-prefix-override=%s`,
				warehouses, gcsTestBucket, c.name)
			c.Run(ctx, c.Node(1), cmd)
			return nil
		})
		m.Wait()
	}

	const warehouses = 1000
	for _, numNodes := range []int{4, 32} {
		r.Add(testSpec{
			Name:    fmt.Sprintf("import/tpcc/warehouses=%d/nodes=%d", warehouses, numNodes),
			Cluster: makeClusterSpec(numNodes),
			Timeout: 5 * time.Hour,
			Run: func(ctx context.Context, t *test, c *cluster) {
				runImportTPCC(ctx, t, c, warehouses)
			},
		})
	}
}

func registerImportTPCH(r *registry) {
	for _, item := range []struct {
		nodes   int
		timeout time.Duration
	}{
		{4, 6 * time.Hour}, // typically 4-5h
		{8, 4 * time.Hour}, // typically 3h
		{32, 3 * time.Hour},
	} {
		item := item
		r.Add(testSpec{
			Name:    fmt.Sprintf(`import/tpch/nodes=%d`, item.nodes),
			Cluster: makeClusterSpec(item.nodes),
			Timeout: item.timeout,
			Run: func(ctx context.Context, t *test, c *cluster) {
				c.Put(ctx, znbase, "./znbase")
				c.Start(ctx, t)
				conn := c.Conn(ctx, 1)
				if _, err := conn.Exec(`
					CREATE DATABASE csv;
					SET CLUSTER SETTING jobs.registry.leniency = '5m';
				`); err != nil {
					t.Fatal(err)
				}
				// Wait for all nodes to be ready.
				if err := retry.ForDuration(time.Second*30, func() error {
					var nodes int
					if err := conn.
						QueryRowContext(ctx, `select count(*) from zbdb_internal.gossip_liveness where updated_at > now() - interval '8s'`).
						Scan(&nodes); err != nil {
						t.Fatal(err)
					} else if nodes != item.nodes {
						return errors.Errorf("expected %d nodes, got %d", item.nodes, nodes)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				m := newMonitor(ctx, c)
				dul := NewDiskUsageLogger(c)
				m.Go(dul.Runner)
				hc := NewHealthChecker(c, c.All())
				m.Go(hc.Runner)

				// TODO(peter): This currently causes the test to fail because we see a
				// flurry of valid merges when the import finishes.
				//
				// m.Go(func(ctx context.Context) error {
				// 	// Make sure the merge queue doesn't muck with our import.
				// 	return verifyMetrics(ctx, c, map[string]float64{
				// 		"cr.store.queue.merge.process.success": 10,
				// 		"cr.store.queue.merge.process.failure": 10,
				// 	})
				// })

				m.Go(func(ctx context.Context) error {
					defer dul.Done()
					defer hc.Done()
					t.WorkerStatus(`running import`)
					defer t.WorkerStatus()
					_, err := conn.Exec(`
				IMPORT TABLE csv.lineitem
				CREATE USING 'gs://znbase-fixtures/tpch-csv/schema/lineitem.sql'
				CSV DATA (
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.1',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.2',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.3',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.4',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.5',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.6',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.7',
				'gs://znbase-fixtures/tpch-csv/sf-100/lineitem.tbl.8'
				) WITH  delimiter='|'
			`)
					return err
				})

				t.Status("waiting")
				m.Wait()
			},
		})
	}
}
