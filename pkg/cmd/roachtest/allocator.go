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
	gosql "database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

func registerAllocator(r *registry) {
	runAllocator := func(ctx context.Context, t *test, c *cluster, start int, maxStdDev float64) {
		const fixturePath = `gs://znbase-fixtures/workload/tpch/scalefactor=10/backup`
		c.Put(ctx, znbase, "./znbase")
		c.Put(ctx, workload, "./workload")

		// Start the first `start` nodes and restore the fixture
		args := startArgs("--args=--vmodule=store_rebalancer=5,allocator=5,allocator_scorer=5,replicate_queue=5")
		c.Start(ctx, t, c.Range(1, start), args)
		db := c.Conn(ctx, 1)
		defer db.Close()

		m := newMonitor(ctx, c, c.Range(1, start))
		m.Go(func(ctx context.Context) error {
			t.Status("loading fixture")
			if _, err := db.Exec(`RESTORE DATABASE tpch FROM $1`, fixturePath); err != nil {
				t.Fatal(err)
			}
			return nil
		})
		m.Wait()

		// Start the remaining nodes to kick off upreplication/rebalancing.
		c.Start(ctx, t, c.Range(start+1, c.nodes), args)

		c.Run(ctx, c.Node(1), `./workload init kv --drop`)
		for node := 1; node <= c.nodes; node++ {
			node := node
			// TODO(dan): Ideally, the test would fail if this queryload failed,
			// but we can't put it in monitor as-is because the test deadlocks.
			go func() {
				const cmd = `./workload run kv --tolerate-errors --min-block-bytes=8 --max-block-bytes=128`
				l, err := t.l.ChildLogger(fmt.Sprintf(`kv-%d`, node))
				if err != nil {
					t.Fatal(err)
				}
				defer l.close()
				_ = execCmd(ctx, t.l, roachprod, "ssh", c.makeNodes(c.Node(node)), "--", cmd)
			}()
		}

		m = newMonitor(ctx, c, c.All())
		m.Go(func(ctx context.Context) error {
			t.Status("waiting for reblance")
			return waitForRebalance(ctx, t.l, db, maxStdDev)
		})
		m.Wait()
	}

	r.Add(testSpec{
		Name:    `replicate/up/1to3`,
		Cluster: makeClusterSpec(3),
		Run: func(ctx context.Context, t *test, c *cluster) {
			runAllocator(ctx, t, c, 1, 10.0)
		},
	})
	r.Add(testSpec{
		Name:    `replicate/rebalance/3to5`,
		Cluster: makeClusterSpec(5),
		Run: func(ctx context.Context, t *test, c *cluster) {
			runAllocator(ctx, t, c, 3, 42.0)
		},
	})
	r.Add(testSpec{
		Name:    `replicate/wide`,
		Timeout: 10 * time.Minute,
		Cluster: makeClusterSpec(9, cpu(1)),
		Run:     runWideReplication,
	})
}

// printRebalanceStats prints the time it took for rebalancing to finish and the
// final standard deviation of replica counts across stores.
func printRebalanceStats(l *logger, db *gosql.DB) error {
	// TODO(cuongdo): Output these in a machine-friendly way and graph.

	// Output time it took to rebalance.
	{
		var rebalanceIntervalStr string
		if err := db.QueryRow(
			`SELECT (SELECT max(timestamp) FROM system.rangelog) - `+
				`(SELECT max(timestamp) FROM system.eventlog WHERE "eventType"=$1)`,
			`node_join`, /* sql.EventLogNodeJoin */
		).Scan(&rebalanceIntervalStr); err != nil {
			return err
		}
		l.Printf("cluster took %s to rebalance\n", rebalanceIntervalStr)
	}

	// Output # of range events that occurred. All other things being equal,
	// larger numbers are worse and potentially indicate thrashing.
	{
		var rangeEvents int64
		q := `SELECT count(*) from system.rangelog`
		if err := db.QueryRow(q).Scan(&rangeEvents); err != nil {
			return err
		}
		l.Printf("%d range events\n", rangeEvents)
	}

	// Output standard deviation of the replica counts for all stores.
	{
		var stdDev float64
		if err := db.QueryRow(
			`SELECT stddev(range_count) FROM zbdb_internal.kv_store_status`,
		).Scan(&stdDev); err != nil {
			return err
		}
		l.Printf("stdDev(replica count) = %.2f\n", stdDev)
	}

	// Output the number of ranges on each store.
	{
		rows, err := db.Query(`SELECT store_id, range_count FROM zbdb_internal.kv_store_status`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var storeID, rangeCount int64
			if err := rows.Scan(&storeID, &rangeCount); err != nil {
				return err
			}
			l.Printf("s%d has %d ranges\n", storeID, rangeCount)
		}
	}

	return nil
}

type replicationStats struct {
	SecondsSinceLastEvent int64
	EventType             string
	RangeID               int64
	StoreID               int64
	ReplicaCountStdDev    float64
}

func (s replicationStats) String() string {
	return fmt.Sprintf("last range event: %s for range %d/store %d (%ds ago)",
		s.EventType, s.RangeID, s.StoreID, s.SecondsSinceLastEvent)
}

// allocatorStats returns the duration of stability (i.e. no replication
// changes) and the standard deviation in replica counts. Only unrecoverable
// errors are returned.
func allocatorStats(db *gosql.DB) (s replicationStats, err error) {
	defer func() {
		if err != nil {
			s.ReplicaCountStdDev = math.MaxFloat64
		}
	}()

	// NB: These are the storage.RangeLogEventType enum, but it's intentionally
	// not used to avoid pulling in the dep.
	eventTypes := []interface{}{
		`split`, `add`, `remove`,
	}

	q := `SELECT extract_duration(seconds FROM now()-timestamp), "rangeID", "storeID", "eventType"` +
		`FROM system.rangelog WHERE "eventType" IN ($1, $2, $3) ORDER BY timestamp DESC LIMIT 1`

	row := db.QueryRow(q, eventTypes...)
	if row == nil {
		// This should never happen, because the archived store we're starting with
		// will always have some range events.
		return replicationStats{}, errors.New("couldn't find any range events")
	}
	if err := row.Scan(&s.SecondsSinceLastEvent, &s.RangeID, &s.StoreID, &s.EventType); err != nil {
		return replicationStats{}, err
	}

	if err := db.QueryRow(
		`SELECT stddev(range_count) FROM zbdb_internal.kv_store_status`,
	).Scan(&s.ReplicaCountStdDev); err != nil {
		return replicationStats{}, err
	}

	return s, nil
}

// waitForRebalance waits until there's been no recent range adds, removes, and
// splits. We wait until the cluster is balanced or until `StableInterval`
// elapses, whichever comes first. Then, it prints stats about the rebalancing
// process. If the replica count appears unbalanced, an error is returned.
//
// This method is crude but necessary. If we were to wait until range counts
// were just about even, we'd miss potential post-rebalance thrashing.
func waitForRebalance(ctx context.Context, l *logger, db *gosql.DB, maxStdDev float64) error {
	// const statsInterval = 20 * time.Second
	const statsInterval = 2 * time.Second
	const stableSeconds = 3 * 60

	var statsTimer timeutil.Timer
	defer statsTimer.Stop()
	statsTimer.Reset(statsInterval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-statsTimer.C:
			statsTimer.Read = true
			stats, err := allocatorStats(db)
			if err != nil {
				return err
			}

			l.Printf("%v\n", stats)
			if stableSeconds <= stats.SecondsSinceLastEvent {
				l.Printf("replica count stddev = %f, max allowed stddev = %f\n", stats.ReplicaCountStdDev, maxStdDev)
				if stats.ReplicaCountStdDev > maxStdDev {
					_ = printRebalanceStats(l, db)
					return errors.Errorf(
						"%ds elapsed without changes, but replica count standard "+
							"deviation is %.2f (>%.2f)", stats.SecondsSinceLastEvent,
						stats.ReplicaCountStdDev, maxStdDev)
				}
				return printRebalanceStats(l, db)
			}
			statsTimer.Reset(statsInterval)
		}
	}
}

func runWideReplication(ctx context.Context, t *test, c *cluster) {
	nodes := c.nodes
	if nodes != 9 {
		t.Fatalf("9-node cluster required")
	}

	args := startArgs("--env=ZNBASE_SCAN_MAX_IDLE_TIME=5ms")
	c.Put(ctx, znbase, "./znbase")
	c.Start(ctx, t, c.All(), args)

	db := c.Conn(ctx, 1)
	defer db.Close()

	zones := func() []string {
		rows, err := db.Query(`SELECT zone_name FROM zbdb_internal.zones`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var results []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			results = append(results, name)
		}
		return results
	}

	run := func(stmt string) {
		t.l.Printf("%s\n", stmt)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	setReplication := func(width int) {
		// Change every zone to have the same number of replicas as the number of
		// nodes in the cluster.
		for _, zone := range zones() {
			which := "RANGE"
			if zone[0] == '.' {
				zone = zone[1:]
			} else if strings.Count(zone, ".") == 0 {
				which = "DATABASE"
			} else {
				which = "TABLE"
			}
			run(fmt.Sprintf(`ALTER %s %s CONFIGURE ZONE USING num_replicas = %d`,
				which, zone, width))
		}
	}
	setReplication(nodes)

	countMisreplicated := func(width int) int {
		var count int
		if err := db.QueryRow(
			"SELECT count(*) FROM zbdb_internal.ranges WHERE array_length(replicas,1) != $1",
			width,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	waitForReplication := func(width int) {
		for count := -1; count != 0; time.Sleep(time.Second) {
			count = countMisreplicated(width)
			t.l.Printf("%d mis-replicated ranges\n", count)
		}
	}

	waitForReplication(nodes)

	numRanges := func() int {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM zbdb_internal.ranges`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}()

	// Stop the cluster and restart 2/3 of the nodes.
	c.Stop(ctx)
	c.Start(ctx, t, c.Range(1, 6), args)

	waitForUnderReplicated := func(count int) {
		for ; ; time.Sleep(time.Second) {
			query := `
SELECT sum((metrics->>'ranges.unavailable')::DECIMAL)::INT AS ranges_unavailable,
       sum((metrics->>'ranges.underreplicated')::DECIMAL)::INT AS ranges_underreplicated
FROM zbdb_internal.kv_store_status
`
			var unavailable, underReplicated int
			if err := db.QueryRow(query).Scan(&unavailable, &underReplicated); err != nil {
				t.Fatal(err)
			}
			t.l.Printf("%d unavailable, %d under-replicated ranges\n", unavailable, underReplicated)
			if unavailable != 0 {
				t.Fatalf("%d unavailable ranges", unavailable)
			}
			if underReplicated >= count {
				break
			}
		}
	}

	waitForUnderReplicated(numRanges)
	if n := countMisreplicated(9); n != 0 {
		t.Fatalf("expected 0 mis-replicated ranges, but found %d", n)
	}

	decom := func(id int) {
		c.Run(ctx, c.Node(1),
			fmt.Sprintf("./znbase node decommission --insecure --wait=none %d", id))
	}

	// Decommission a node. The ranges should down-replicate to 7 replicas.
	decom(9)
	waitForReplication(7)

	// Set the replication width to 5. The replicas should down-replicate, though
	// this currently requires the time-until-store-dead threshold to pass
	// because the allocator cannot select a replica for removal that is on a
	// store for which it doesn't have a store descriptor.
	run(`SET CLUSTER SETTING server.time_until_store_dead = '90s'`)
	setReplication(5)
	waitForReplication(5)
}
