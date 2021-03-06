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

	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

// Motivation:
// Although there are unit tests that test query cancellation, they have been
// insufficient in detecting problems with canceling long-running, multi-node
// DistSQL queries. This is because, unlike local queries which only need to
// cancel the transaction's context, DistSQL queries must cancel flow contexts
// on each node involed in the query. Typical strategies for local execution
// testing involve using a builtin like generate_series to create artificially
// long-running queries, but these approaches don't create multi-node DistSQL
// queries; the only way to do so is by querying a large dataset split across
// multiple nodes. Due to the high cost of loading the pre-requisite data, these
// tests are best suited as nightlies.
//
// Once DistSQL queries provide more testing knobs, these tests can likely be
// replaced with unit tests.
func registerCancel(r *registry) {
	runCancel := func(ctx context.Context, t *test, c *cluster,
		queries []string, warehouses int, useDistsql bool) {
		c.Put(ctx, znbase, "./znbase", c.All())
		c.Put(ctx, workload, "./workload", c.All())
		c.Start(ctx, t, c.All())

		m := newMonitor(ctx, c, c.All())
		m.Go(func(ctx context.Context) error {
			t.Status("importing TPCC fixture")
			c.Run(ctx, c.Node(1), fmt.Sprintf(
				"./workload fixtures load tpcc --warehouses=%d {pgurl:1}", warehouses))

			conn := c.Conn(ctx, 1)
			defer conn.Close()

			var queryPrefix string
			if !useDistsql {
				queryPrefix = "SET distsql = off;"
			}

			t.Status("running queries to cancel")
			for _, q := range queries {
				sem := make(chan struct{}, 1)
				go func() {
					fmt.Printf("executing \"%s\"\n", q)
					sem <- struct{}{}
					_, err := conn.Exec(queryPrefix + q)
					if err == nil {
						close(sem)
						t.Fatal("query completed before it could be canceled")
					} else {
						fmt.Printf("query failed with error: %s\n", err)
					}
					sem <- struct{}{}
				}()

				<-sem

				// The cancel query races with the execution of the query it's trying to
				// cancel, which may result in attempting to cancel the query before it
				// has started.  To be more confident that the query is executing, wait
				// a bit before attempting to cancel it.
				time.Sleep(100 * time.Millisecond)

				const cancelQuery = `CANCEL QUERIES
	SELECT query_id FROM [SHOW CLUSTER QUERIES] WHERE query not like '%SHOW CLUSTER QUERIES%'`
				c.Run(ctx, c.Node(1), `./znbase sql --insecure -e "`+cancelQuery+`"`)
				cancelStartTime := timeutil.Now()

				select {
				case _, ok := <-sem:
					if !ok {
						t.Fatal("query could not be canceled")
					}
					timeToCancel := timeutil.Now().Sub(cancelStartTime)
					fmt.Printf("canceling \"%s\" took %s\n", q, timeToCancel)

				case <-time.After(5 * time.Second):
					t.Fatal("query took too long to respond to cancellation")
				}
			}

			return nil
		})
		m.Wait()
	}

	const warehouses = 10
	const numNodes = 3
	queries := []string{
		`SELECT * FROM tpcc.stock`,
		`SELECT * FROM tpcc.stock WHERE s_quantity > 100`,
		`SELECT s_i_id, sum(s_quantity) FROM tpcc.stock GROUP BY s_i_id`,
		`SELECT * FROM tpcc.stock ORDER BY s_quantity`,
		`SELECT * FROM tpcc.order_line JOIN tpcc.stock ON s_i_id=ol_i_id`,
		`SELECT ol_number, sum(s_quantity) FROM tpcc.stock JOIN tpcc.order_line ON s_i_id=ol_i_id WHERE ol_number > 10 GROUP BY ol_number ORDER BY ol_number`,
	}

	r.Add(testSpec{
		Name:    fmt.Sprintf("cancel/tpcc/distsql/w=%d,nodes=%d", warehouses, numNodes),
		Cluster: makeClusterSpec(numNodes),
		Run: func(ctx context.Context, t *test, c *cluster) {
			runCancel(ctx, t, c, queries, warehouses, true /* useDistsql */)
		},
	})

	r.Add(testSpec{
		Name:    fmt.Sprintf("cancel/tpcc/local/w=%d,nodes=%d", warehouses, numNodes),
		Cluster: makeClusterSpec(numNodes),
		Run: func(ctx context.Context, t *test, c *cluster) {
			runCancel(ctx, t, c, queries, warehouses, false /* useDistsql */)
		},
	})
}
