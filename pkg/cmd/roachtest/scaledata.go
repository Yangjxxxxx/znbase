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
	"runtime"
	"strings"
	"time"

	"github.com/znbasedb/znbase/pkg/util/binfetcher"
)

func registerScaleData(r *registry) {
	// apps is a suite of Sqlapp applications designed to be used to check the
	// consistency of a database under load. Each Sqlapp application launches a
	// set of workers who perform database operations while another worker
	// periodically checks invariants to capture any inconsistencies. The
	// application suite has been pulled from:
	// github.com/scaledata/rksql/tree/master/src/go/src/rubrik/sqlapp
	//
	// The map provides a mapping between application name and command-line
	// flags unique to that application.
	apps := map[string]string{
		"distributed_semaphore": "",
		"filesystem_simulator":  "",
		"jobcoordinator":        "--num_jobs_per_worker=8 --job_period_scale_millis=100",
	}

	for app, flags := range apps {
		app, flags := app, flags // copy loop iterator vars
		const duration = 10 * time.Minute
		for _, n := range []int{3, 6} {
			r.Add(testSpec{
				Name:    fmt.Sprintf("scaledata/%s/nodes=%d", app, n),
				Timeout: 2 * duration,
				Cluster: makeClusterSpec(n + 1),
				Run: func(ctx context.Context, t *test, c *cluster) {
					runSqlapp(ctx, t, c, app, flags, duration)
				},
			})
		}
	}
}

func runSqlapp(ctx context.Context, t *test, c *cluster, app, flags string, dur time.Duration) {
	roachNodeCount := c.nodes - 1
	roachNodes := c.Range(1, roachNodeCount)
	appNode := c.Node(c.nodes)

	if local && runtime.GOOS != "linux" {
		t.Fatalf("must run on linux os, found %s", runtime.GOOS)
	}
	b, err := binfetcher.Download(ctx, binfetcher.Options{
		Component: "rubrik",
		Binary:    app,
		Version:   "LATEST",
		GOOS:      "linux",
		GOARCH:    "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}

	c.Put(ctx, b, app, appNode)
	c.Put(ctx, znbase, "./znbase", roachNodes)
	c.Start(ctx, t, roachNodes)

	// TODO(nvanbenschoten): We are currently running these consistency checks with
	// basic chaos. We should also run them in more chaotic environments which
	// could introduce network partitions, ENOSPACE, clock issues, etc.

	// Sqlapps each take a `--znbase_ip_addresses_csv` flag, which is a
	// comma-separated list of node IP addresses with optional port specifiers.
	addrStr := strings.Join(c.InternalAddr(ctx, c.Range(1, roachNodeCount)), ",")

	m := newMonitor(ctx, c, roachNodes)
	{
		// Kill one node at a time, with a minute of healthy cluster and thirty
		// seconds of down node.
		ch := Chaos{
			Timer:   Periodic{Period: 90 * time.Second, DownTime: 30 * time.Second},
			Target:  roachNodes.randNode,
			Stopper: time.After(dur),
		}
		m.Go(ch.Runner(c, m))
	}
	m.Go(func(ctx context.Context) error {
		// Sqlapp logs are very noisy - so noisy that if not directed to /dev/null
		// they often have the effect of slowing down the test so much that it
		// fails. To get around this we create a new logger that writes to an
		// artifacts file but does not output to stdout or stderr.
		sqlappL, err := t.l.ChildLogger("sqlapp", logPrefix(""), quietStdout, quietStderr)
		if err != nil {
			return err
		}
		defer sqlappL.close()

		t.Status("installing schema")
		err = c.RunL(ctx, sqlappL, appNode, fmt.Sprintf("./%s --install_schema "+
			"--znbase_ip_addresses_csv='%s' %s", app, addrStr, flags))
		if err != nil {
			return err
		}

		t.Status("running consistency checker")
		const workers = 16
		return c.RunL(ctx, sqlappL, appNode, fmt.Sprintf("./%s  --duration_secs=%d "+
			"--num_workers=%d --znbase_ip_addresses_csv='%s' %s",
			app, int(dur.Seconds()), workers, addrStr, flags))
	})
	m.Wait()
}
