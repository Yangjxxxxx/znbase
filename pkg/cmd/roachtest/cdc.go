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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Shopify/sarama"
	"github.com/codahale/hdrhistogram"
	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/icl/changefeedicl/cdctest"
	"github.com/znbasedb/znbase/pkg/jobs/jobspb"
	"github.com/znbasedb/znbase/pkg/util/protoutil"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
	"github.com/znbasedb/znbase/pkg/util/version"
)

type workloadType string

const (
	tpccWorkloadType   workloadType = "tpcc"
	ledgerWorkloadType workloadType = "ledger"
)

type cdcTestArgs struct {
	workloadType       workloadType
	tpccWarehouseCount int
	workloadDuration   string
	initialScan        bool
	rangefeed          bool
	kafkaChaos         bool
	znbaseChaos        bool
	cloudStorageSink   bool
	fixturesImport     bool

	targetInitialScanLatency time.Duration
	targetSteadyLatency      time.Duration
	targetTxnPerSecond       float64
}

func cdcBasicTest(ctx context.Context, t *test, c *cluster, args cdcTestArgs) {
	znbaseNodes := c.Range(1, c.nodes-1)
	workloadNode := c.Node(c.nodes)
	kafkaNode := c.Node(c.nodes)
	c.Put(ctx, znbase, "./znbase", znbaseNodes)
	c.Put(ctx, workload, "./workload", workloadNode)
	c.Start(ctx, t, znbaseNodes)

	db := c.Conn(ctx, 1)
	defer stopFeeds(db)
	if _, err := db.Exec(
		`SET CLUSTER SETTING kv.rangefeed.enabled = $1`, args.rangefeed,
	); err != nil {
		t.Fatal(err)
	}
	// The 2.1 branch doesn't have this cluster setting, so ignore the error if
	// it's about an unknown cluster setting
	if _, err := db.Exec(
		`SET CLUSTER SETTING changefeed.push.enabled = $1`, args.rangefeed,
	); err != nil && !strings.Contains(err.Error(), "unknown cluster setting") {
		t.Fatal(err)
	}
	kafka := kafkaManager{
		c:     c,
		nodes: kafkaNode,
	}

	var sinkURI string
	if args.cloudStorageSink {
		ts := timeutil.Now().Format(`20060102150405`)
		// znbase-tmp is a multi-region bucket with a TTL to clean up old
		// data.
		sinkURI = `experimental-gs://znbase-tmp/roachtest/` + ts
	} else {
		t.Status("installing kafka")
		kafka.install(ctx)
		kafka.start(ctx)
		sinkURI = kafka.sinkURL(ctx)
	}

	m := newMonitor(ctx, c, znbaseNodes)
	workloadCompleteCh := make(chan struct{}, 1)

	workloadStart := timeutil.Now()
	if args.workloadType == tpccWorkloadType {
		t.Status("installing TPCC")
		tpcc := tpccWorkload{
			sqlNodes:           znbaseNodes,
			workloadNodes:      workloadNode,
			tpccWarehouseCount: args.tpccWarehouseCount,
			// TolerateErrors if znbaseChaos is true; otherwise, the workload will fail
			// if it attempts to use the node which was brought down by chaos.
			tolerateErrors: args.znbaseChaos,
		}
		// TODO(dan): Remove this when we fix whatever is causing the "duplicate key
		// value" errors #34025.
		tpcc.tolerateErrors = true

		tpcc.install(ctx, c, args.fixturesImport)
		// TODO(dan,ajwerner): sleeping momentarily before running the workload
		// mitigates errors like "error in newOrder: missing stock row" from tpcc.
		time.Sleep(2 * time.Second)
		t.Status("initiating workload")
		m.Go(func(ctx context.Context) error {
			defer func() { close(workloadCompleteCh) }()
			tpcc.run(ctx, c, args.workloadDuration)
			return nil
		})
	} else {
		t.Status("installing Ledger Workload")
		lw := ledgerWorkload{
			sqlNodes:      znbaseNodes,
			workloadNodes: workloadNode,
		}
		lw.install(ctx, c)

		t.Status("initiating workload")
		m.Go(func(ctx context.Context) error {
			defer func() { close(workloadCompleteCh) }()
			lw.run(ctx, c, args.workloadDuration)
			return nil
		})
	}

	changefeedLogger, err := t.l.ChildLogger("changefeed")
	if err != nil {
		t.Fatal(err)
	}
	defer changefeedLogger.close()
	verifier := makeLatencyVerifier(
		args.targetInitialScanLatency,
		args.targetSteadyLatency,
		changefeedLogger,
		args.znbaseChaos,
	)
	defer verifier.maybeLogLatencyHist()

	m.Go(func(ctx context.Context) error {
		// Some of the tests have a tight enough bound on targetSteadyLatency
		// that the default for kv.closed_timestamp.target_duration means the
		// changefeed is never considered sufficiently caught up. We could
		// instead make targetSteadyLatency less aggressive, but it'd be nice to
		// keep it where it is.
		if _, err := db.Exec(
			`SET CLUSTER SETTING kv.closed_timestamp.target_duration='10s'`,
		); err != nil {
			t.Fatal(err)
		}

		var targets string
		if args.workloadType == tpccWorkloadType {
			targets = `tpcc.warehouse, tpcc.district, tpcc.customer, tpcc.history,
			tpcc.order, tpcc.new_order, tpcc.item, tpcc.stock,
			tpcc.order_line`
		} else {
			targets = `ledger.customer, ledger.transaction, ledger.entry, ledger.session`
		}

		jobID, err := createChangefeed(db, targets, sinkURI, args)
		if err != nil {
			return err
		}

		info, err := getChangefeedInfo(db, jobID)
		if err != nil {
			return err
		}
		verifier.statementTime = info.statementTime
		changefeedLogger.Printf("started changefeed at (%d) %s\n",
			verifier.statementTime.UnixNano(), verifier.statementTime)
		t.Status("watching changefeed")
		return verifier.pollLatency(ctx, db, jobID, time.Second, workloadCompleteCh)
	})

	if args.kafkaChaos {
		m.Go(func(ctx context.Context) error {
			period, downTime := 2*time.Minute, 20*time.Second
			return kafka.chaosLoop(ctx, period, downTime, workloadCompleteCh)
		})
	}

	if args.znbaseChaos {
		chaosDuration, err := time.ParseDuration(args.workloadDuration)
		if err != nil {
			t.Fatal(err)
		}
		ch := Chaos{
			Timer:   Periodic{Period: 2 * time.Minute, DownTime: 20 * time.Second},
			Target:  znbaseNodes.randNode,
			Stopper: time.After(chaosDuration),
		}
		m.Go(ch.Runner(c, m))
	}
	m.Wait()

	verifier.assertValid(t)
	workloadEnd := timeutil.Now()
	if args.targetTxnPerSecond > 0.0 {
		verifyTxnPerSecond(
			ctx, c, t, znbaseNodes.randNode(), workloadStart, workloadEnd, args.targetTxnPerSecond, 0.05,
		)
	}
}

func runCDCBank(ctx context.Context, t *test, c *cluster) {
	// Make the logs dir on every node to work around the `roachprod get logs`
	// spam.
	c.Run(ctx, c.All(), `mkdir -p logs`)

	znbaseNodes, workloadNode, kafkaNode := c.Range(1, c.nodes-1), c.Node(c.nodes), c.Node(c.nodes)
	c.Put(ctx, znbase, "./znbase", znbaseNodes)
	c.Put(ctx, workload, "./workload", workloadNode)
	c.Start(ctx, t, znbaseNodes)
	kafka := kafkaManager{
		c:     c,
		nodes: kafkaNode,
	}
	kafka.install(ctx)
	if !kafka.c.isLocal() {
		// TODO(dan): This test currently connects to kafka from the test
		// runner, so kafka needs to advertise the external address. Better
		// would be a binary we could run on one of the roachprod machines.
		c.Run(ctx, kafka.nodes, `echo "advertised.listeners=PLAINTEXT://`+kafka.consumerURL(ctx)+`" >> `+
			kafka.basePath()+`/confluent-4.0.0/etc/kafka/server.properties`)
	}
	kafka.start(ctx)
	defer kafka.stop(ctx)

	c.Run(ctx, workloadNode, `./workload init bank {pgurl:1}`)
	db := c.Conn(ctx, 1)
	defer stopFeeds(db)

	if _, err := db.Exec(
		`SET CLUSTER SETTING kv.rangefeed.enabled = true`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`SET CLUSTER SETTING changefeed.experimental_poll_interval = '10ms'`,
	); err != nil {
		t.Fatal(err)
	}
	var jobID string
	if err := db.QueryRow(
		`CREATE CHANGEFEED FOR bank.bank INTO $1 WITH updated, resolved`, kafka.sinkURL(ctx),
	).Scan(&jobID); err != nil {
		t.Fatal(err)
	}

	t.Status("running workload")
	workloadCtx, workloadCancel := context.WithCancel(ctx)
	m := newMonitor(workloadCtx, c, znbaseNodes)
	var doneAtomic int64
	m.Go(func(ctx context.Context) error {
		err := c.RunE(ctx, workloadNode, `./workload run bank {pgurl:1} --max-rate=10`)
		if atomic.LoadInt64(&doneAtomic) > 0 {
			return nil
		}
		return err
	})
	m.Go(func(ctx context.Context) error {
		defer workloadCancel()
		l, err := t.l.ChildLogger(`changefeed`)
		if err != nil {
			return err
		}
		defer l.close()

		tc, err := kafka.consumer(ctx, `bank`)
		if err != nil {
			return err
		}
		defer tc.Close()

		// TODO(dan): Force multiple partitions and re-enable this check. It
		// used to be the only thing that verified our correctness with multiple
		// partitions but TestValidations/enterprise also does that now, so this
		// isn't a big deal.
		//
		// if len(tc.partitions) <= 1 {
		//  return errors.New("test requires at least 2 partitions to be interesting")
		// }

		var requestedResolved = 100
		if local {
			requestedResolved = 10
		}
		var numResolved, rowsSinceResolved int
		v := cdctest.Validators{
			cdctest.NewOrderValidator(`bank`),
			cdctest.NewFingerprintValidator(db, `bank.bank`, `fprint`, tc.partitions),
		}
		if _, err := db.Exec(
			`CREATE TABLE fprint (id INT PRIMARY KEY, balance INT, payload STRING)`,
		); err != nil {
			return err
		}

		for {
			m := tc.Next(ctx)
			if m == nil {
				return fmt.Errorf("unexpected end of changefeed")
			}
			updated, resolved, err := cdctest.ParseJSONValueTimestamps(m.Value)
			if err != nil {
				return err
			}

			partitionStr := strconv.Itoa(int(m.Partition))
			if len(m.Key) > 0 {
				v.NoteRow(partitionStr, string(m.Key), string(m.Value), updated)
				rowsSinceResolved++
			} else {
				if err := v.NoteResolved(partitionStr, resolved); err != nil {
					return err
				}
				if rowsSinceResolved > 0 {
					numResolved++
					if numResolved > requestedResolved {
						atomic.StoreInt64(&doneAtomic, 1)
						break
					}
				}
				rowsSinceResolved = 0
			}
		}
		if failures := v.Failures(); len(failures) > 0 {
			return errors.New(strings.Join(failures, "\n"))
		}
		return nil
	})
	m.Wait()

}

func registerCDC(r *registry) {
	useRangeFeed := true
	if r.buildVersion.Compare(version.MustParse(`v2.2.0-0`)) < 0 {
		// RangeFeed is not production ready in 2.1, so run the tests with the
		// poller.
		useRangeFeed = false
	}

	r.Add(testSpec{
		Name:       fmt.Sprintf("cdc/tpcc-1000/rangefeed=%t", useRangeFeed),
		MinVersion: "v2.1.0",
		Cluster:    makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType:             tpccWorkloadType,
				tpccWarehouseCount:       1000,
				workloadDuration:         "120m",
				rangefeed:                useRangeFeed,
				targetInitialScanLatency: 3 * time.Minute,
				targetSteadyLatency:      10 * time.Minute,
			})
		},
	})
	r.Add(testSpec{
		Name:       fmt.Sprintf("cdc/initial-scan/rangefeed=%t", useRangeFeed),
		MinVersion: "v2.1.0",
		Cluster:    makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType:             tpccWorkloadType,
				tpccWarehouseCount:       100,
				workloadDuration:         "30m",
				initialScan:              true,
				rangefeed:                useRangeFeed,
				targetInitialScanLatency: 30 * time.Minute,
				targetSteadyLatency:      time.Minute,
			})
		},
	})
	r.Add(testSpec{
		Name: "cdc/poller/rangefeed=false",
		// When testing a 2.1 binary, we use the poller for all the other tests
		// and this is close enough to cdc/tpcc-1000 test to be redundant, so
		// skip it.
		MinVersion: "v2.2.0",
		Cluster:    makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType:             tpccWorkloadType,
				tpccWarehouseCount:       1000,
				workloadDuration:         "30m",
				rangefeed:                false,
				targetInitialScanLatency: 30 * time.Minute,
				targetSteadyLatency:      2 * time.Minute,
			})
		},
	})
	r.Add(testSpec{
		Name:       fmt.Sprintf("cdc/sink-chaos/rangefeed=%t", useRangeFeed),
		MinVersion: "v2.1.0",
		Cluster:    makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType:             tpccWorkloadType,
				tpccWarehouseCount:       100,
				workloadDuration:         "30m",
				rangefeed:                useRangeFeed,
				kafkaChaos:               true,
				targetInitialScanLatency: 3 * time.Minute,
				targetSteadyLatency:      5 * time.Minute,
			})
		},
	})
	r.Add(testSpec{
		Name:       fmt.Sprintf("cdc/znbase-chaos/rangefeed=%t", useRangeFeed),
		MinVersion: "v2.1.0",
		Cluster:    makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType:             tpccWorkloadType,
				tpccWarehouseCount:       100,
				workloadDuration:         "30m",
				rangefeed:                useRangeFeed,
				znbaseChaos:              true,
				targetInitialScanLatency: 3 * time.Minute,
				targetSteadyLatency:      10 * time.Minute,
			})
		},
	})
	r.Add(testSpec{
		Name:       fmt.Sprintf("cdc/ledger/rangefeed=%t", useRangeFeed),
		MinVersion: "v2.1.0",
		// TODO(mrtracy): This workload is designed to be running on a 20CPU nodes,
		// but this cannot be allocated without some sort of configuration outside
		// of this test. Look into it.
		Cluster: makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType:             ledgerWorkloadType,
				workloadDuration:         "30m",
				initialScan:              true,
				rangefeed:                useRangeFeed,
				targetInitialScanLatency: 10 * time.Minute,
				targetSteadyLatency:      time.Minute,
				targetTxnPerSecond:       575,
			})
		},
	})
	r.Add(testSpec{
		Name:       "cdc/cloud-sink-gcs/rangefeed=true",
		MinVersion: "v2.2.0",
		Cluster:    makeClusterSpec(4, cpu(16)),
		Run: func(ctx context.Context, t *test, c *cluster) {
			cdcBasicTest(ctx, t, c, cdcTestArgs{
				workloadType: tpccWorkloadType,
				// Sending data to Google Cloud Storage is a bit slower than sending to
				// Kafka on an adjacent machine, so use half the data of the
				// initial-scan test. Consider adding a test that writes to nodelocal,
				// which should be much faster, with a larger warehouse count.
				tpccWarehouseCount:       50,
				workloadDuration:         "30m",
				initialScan:              true,
				rangefeed:                true,
				cloudStorageSink:         true,
				fixturesImport:           true,
				targetInitialScanLatency: 30 * time.Minute,
				targetSteadyLatency:      time.Minute,
			})
		},
	})
	// TODO(dan): This currently gets its own cluster during the nightly
	// acceptance tests. Decide whether it's safe to share with the one made for
	// "acceptance/*".
	//
	// TODO(dan): Ideally, these would run on every PR, but because of
	// enterprise license checks, there currently isn't a good way to do that
	// without potentially leaking secrets.
	r.Add(testSpec{
		Name:       "cdc/bank",
		Skip:       `#35614`,
		MinVersion: "v2.1.0",
		Cluster:    makeClusterSpec(4),
		Run: func(ctx context.Context, t *test, c *cluster) {
			runCDCBank(ctx, t, c)
		},
	})
}

type kafkaManager struct {
	c     *cluster
	nodes nodeListOption
}

func (k kafkaManager) basePath() string {
	if k.c.isLocal() {
		return `/tmp/confluent`
	}
	return `/mnt/data1/confluent`
}

func (k kafkaManager) install(ctx context.Context) {
	folder := k.basePath()
	k.c.Run(ctx, k.nodes, `mkdir -p `+folder)
	k.c.Run(ctx, k.nodes, `curl -s https://packages.confluent.io/archive/4.0/confluent-oss-4.0.0-2.11.tar.gz | tar -xz -C `+folder)
	if !k.c.isLocal() {
		k.c.Run(ctx, k.nodes, `mkdir -p logs`)
		k.c.Run(ctx, k.nodes, `sudo apt-get -q update 2>&1 > logs/apt-get-update.log`)
		k.c.Run(ctx, k.nodes, `yes | sudo apt-get -q install default-jre 2>&1 > logs/apt-get-install.log`)
	}
}

func (k kafkaManager) start(ctx context.Context) {
	folder := k.basePath()
	// This isn't necessary for the nightly tests, but it's nice for iteration.
	k.c.Run(ctx, k.nodes, `CONFLUENT_CURRENT=`+folder+` `+folder+`/confluent-4.0.0/bin/confluent destroy || true`)
	k.restart(ctx)
}

func (k kafkaManager) restart(ctx context.Context) {
	folder := k.basePath()
	k.c.Run(ctx, k.nodes, `CONFLUENT_CURRENT=`+folder+` `+folder+`/confluent-4.0.0/bin/confluent start kafka`)
}

func (k kafkaManager) stop(ctx context.Context) {
	folder := k.basePath()
	k.c.Run(ctx, k.nodes, `CONFLUENT_CURRENT=`+folder+` `+folder+`/confluent-4.0.0/bin/confluent stop`)
}

func (k kafkaManager) chaosLoop(
	ctx context.Context, period, downTime time.Duration, stopper chan struct{},
) error {
	t := time.NewTicker(period)
	for {
		select {
		case <-stopper:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}

		k.stop(ctx)

		select {
		case <-stopper:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(downTime):
		}

		k.restart(ctx)
	}
}

func (k kafkaManager) sinkURL(ctx context.Context) string {
	return `kafka://` + k.c.InternalIP(ctx, k.nodes)[0] + `:9092`
}

func (k kafkaManager) consumerURL(ctx context.Context) string {
	return k.c.ExternalIP(ctx, k.nodes)[0] + `:9092`
}

func (k kafkaManager) consumer(ctx context.Context, topic string) (*topicConsumer, error) {
	kafkaAddrs := []string{k.consumerURL(ctx)}
	config := sarama.NewConfig()
	// I was seeing "error processing FetchRequest: kafka: error decoding
	// packet: unknown magic byte (2)" errors which
	// https://github.com/Shopify/sarama/issues/962 identifies as the
	// consumer's fetch size being less than the "max.message.bytes" that
	// kafka is configured with. Kafka notes that this is required in
	// https://kafka.apache.org/documentation.html#upgrade_11_message_format
	config.Consumer.Fetch.Default = 1000012
	consumer, err := sarama.NewConsumer(kafkaAddrs, config)
	if err != nil {
		return nil, err
	}
	tc, err := makeTopicConsumer(consumer, `bank`)
	if err != nil {
		_ = consumer.Close()
		return nil, err
	}
	return tc, nil
}

type tpccWorkload struct {
	workloadNodes      nodeListOption
	sqlNodes           nodeListOption
	tpccWarehouseCount int
	tolerateErrors     bool
}

func (tw *tpccWorkload) install(ctx context.Context, c *cluster, fixturesImport bool) {
	command := `fixtures load`
	if fixturesImport {
		command = `fixtures import`
	}
	c.Run(ctx, tw.workloadNodes, fmt.Sprintf(
		`./workload %s tpcc --warehouses=%d --checks=false {pgurl%s}`,
		command,
		tw.tpccWarehouseCount,
		tw.sqlNodes.randNode(),
	))
}

func (tw *tpccWorkload) run(ctx context.Context, c *cluster, workloadDuration string) {
	tolerateErrors := ""
	if tw.tolerateErrors {
		tolerateErrors = "--tolerate-errors"
	}
	c.Run(ctx, tw.workloadNodes, fmt.Sprintf(
		`./workload run tpcc --warehouses=%d --duration=%s %s {pgurl%s} `,
		tw.tpccWarehouseCount, workloadDuration, tolerateErrors, tw.sqlNodes,
	))
}

type ledgerWorkload struct {
	workloadNodes nodeListOption
	sqlNodes      nodeListOption
}

func (lw *ledgerWorkload) install(ctx context.Context, c *cluster) {
	c.Run(ctx, lw.workloadNodes.randNode(), fmt.Sprintf(
		`./workload init ledger {pgurl%s}`,
		lw.sqlNodes.randNode(),
	))
}

func (lw *ledgerWorkload) run(ctx context.Context, c *cluster, workloadDuration string) {
	c.Run(ctx, lw.workloadNodes, fmt.Sprintf(
		`./workload run ledger --mix=balance=0,withdrawal=50,deposit=50,reversal=0 {pgurl%s} --duration=%s`,
		lw.sqlNodes,
		workloadDuration,
	))
}

type latencyVerifier struct {
	statementTime            time.Time
	targetSteadyLatency      time.Duration
	targetInitialScanLatency time.Duration
	tolerateErrors           bool
	logger                   *logger

	initialScanLatency   time.Duration
	maxSeenSteadyLatency time.Duration
	latencyBecameSteady  bool

	latencyHist *hdrhistogram.Histogram
}

func makeLatencyVerifier(
	targetInitialScanLatency time.Duration,
	targetSteadyLatency time.Duration,
	l *logger,
	tolerateErrors bool,
) *latencyVerifier {
	const sigFigs, minLatency, maxLatency = 1, 100 * time.Microsecond, 100 * time.Second
	hist := hdrhistogram.New(minLatency.Nanoseconds(), maxLatency.Nanoseconds(), sigFigs)
	return &latencyVerifier{
		targetInitialScanLatency: targetInitialScanLatency,
		targetSteadyLatency:      targetSteadyLatency,
		logger:                   l,
		latencyHist:              hist,
		tolerateErrors:           tolerateErrors,
	}
}

func (lv *latencyVerifier) noteHighwater(highwaterTime time.Time) {
	if highwaterTime.Before(lv.statementTime) {
		return
	}
	if lv.initialScanLatency == 0 {
		lv.initialScanLatency = timeutil.Since(lv.statementTime)
		lv.logger.Printf("initial scan completed: latency %s\n", lv.initialScanLatency)
		return
	}

	latency := timeutil.Since(highwaterTime)
	if latency < lv.targetSteadyLatency/2 {
		lv.latencyBecameSteady = true
	}
	if !lv.latencyBecameSteady {
		// Before we have RangeFeed, the polls just get
		// progressively smaller after the initial one. Start
		// tracking the max latency once we seen a latency
		// that's less than the max allowed. Verify at the end
		// of the test that this happens at some point.
		lv.logger.Printf("end-to-end latency %s not yet below target steady latency %s\n",
			latency, lv.targetSteadyLatency)
		return
	}
	if err := lv.latencyHist.RecordValue(latency.Nanoseconds()); err != nil {
		lv.logger.Printf("could not record value %s: %s\n", latency, err)
	}
	if latency > lv.maxSeenSteadyLatency {
		lv.maxSeenSteadyLatency = latency
	}
	lv.logger.Printf("end-to-end steady latency %s; max steady latency so far %s\n",
		latency, lv.maxSeenSteadyLatency)
}

func (lv *latencyVerifier) pollLatency(
	ctx context.Context, db *gosql.DB, jobID int, interval time.Duration, stopper chan struct{},
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stopper:
			return nil
		case <-time.After(time.Second):
		}

		info, err := getChangefeedInfo(db, jobID)
		if err != nil {
			if lv.tolerateErrors {
				lv.logger.Printf("error getting changefeed info: %s", err)
				continue
			}
			return err
		}
		if info.status != `running` {
			lv.logger.Printf("unexpected status: %s, error: %s", info.status, info.errMsg)
			return errors.Errorf(`unexpected status: %s`, info.status)
		}
		lv.noteHighwater(info.highwaterTime)
	}
}

func (lv *latencyVerifier) assertValid(t *test) {
	if lv.initialScanLatency == 0 {
		t.Fatalf("initial scan did not complete")
	}
	if lv.initialScanLatency > lv.targetInitialScanLatency {
		t.Fatalf("initial scan latency was more than target: %s vs %s",
			lv.initialScanLatency, lv.targetInitialScanLatency)
	}
	if !lv.latencyBecameSteady {
		t.Fatalf("latency never dropped to acceptable steady level: %s", lv.targetSteadyLatency)
	}
	if lv.maxSeenSteadyLatency > lv.targetSteadyLatency {
		t.Fatalf("max latency was more than allowed: %s vs %s",
			lv.maxSeenSteadyLatency, lv.targetSteadyLatency)
	}
}

func (lv *latencyVerifier) maybeLogLatencyHist() {
	if lv.latencyHist == nil {
		return
	}
	lv.logger.Printf(
		"changefeed end-to-end __avg(ms)__p50(ms)__p75(ms)__p90(ms)__p95(ms)__p99(ms)_pMax(ms)\n")
	lv.logger.Printf("changefeed end-to-end  %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f %8.1f\n",
		time.Duration(lv.latencyHist.Mean()).Seconds()*1000,
		time.Duration(lv.latencyHist.ValueAtQuantile(50)).Seconds()*1000,
		time.Duration(lv.latencyHist.ValueAtQuantile(75)).Seconds()*1000,
		time.Duration(lv.latencyHist.ValueAtQuantile(90)).Seconds()*1000,
		time.Duration(lv.latencyHist.ValueAtQuantile(95)).Seconds()*1000,
		time.Duration(lv.latencyHist.ValueAtQuantile(99)).Seconds()*1000,
		time.Duration(lv.latencyHist.ValueAtQuantile(100)).Seconds()*1000,
	)
}

func createChangefeed(db *gosql.DB, targets, sinkURL string, args cdcTestArgs) (int, error) {
	var jobID int
	createStmt := fmt.Sprintf(`CREATE CHANGEFEED FOR %s INTO $1`, targets)
	extraArgs := []interface{}{sinkURL}
	if args.cloudStorageSink {
		createStmt += ` WITH resolved='10s', envelope=wrapped`
	} else {
		createStmt += ` WITH resolved`
	}
	if !args.initialScan {
		createStmt += `, cursor='-1s'`
	}
	if err := db.QueryRow(createStmt, extraArgs...).Scan(&jobID); err != nil {
		return 0, err
	}
	return jobID, nil
}

type changefeedInfo struct {
	status        string
	errMsg        string
	statementTime time.Time
	highwaterTime time.Time
}

func getChangefeedInfo(db *gosql.DB, jobID int) (changefeedInfo, error) {
	var status string
	var payloadBytes []byte
	var progressBytes []byte
	if err := db.QueryRow(
		`SELECT status, payload, progress FROM system.jobs WHERE id = $1`, jobID,
	).Scan(&status, &payloadBytes, &progressBytes); err != nil {
		return changefeedInfo{}, err
	}
	var payload jobspb.Payload
	if err := protoutil.Unmarshal(payloadBytes, &payload); err != nil {
		return changefeedInfo{}, err
	}
	var progress jobspb.Progress
	if err := protoutil.Unmarshal(progressBytes, &progress); err != nil {
		return changefeedInfo{}, err
	}
	var highwaterTime time.Time
	highwater := progress.GetHighWater()
	if highwater != nil {
		highwaterTime = highwater.GoTime()
	}
	return changefeedInfo{
		status:        status,
		errMsg:        payload.Error,
		statementTime: payload.GetChangefeed().StatementTime.GoTime(),
		highwaterTime: highwaterTime,
	}, nil
}

// stopFeeds cancels any running feeds on the cluster. Not necessary for the
// nightly, but nice for development.
func stopFeeds(db *gosql.DB) {
	_, _ = db.Exec(`CANCEL JOBS (
			SELECT job_id FROM [SHOW JOBS] WHERE status = 'running'
		)`)
}

type topicConsumer struct {
	sarama.Consumer

	topic              string
	partitions         []string
	partitionConsumers []sarama.PartitionConsumer
}

func makeTopicConsumer(c sarama.Consumer, topic string) (*topicConsumer, error) {
	t := &topicConsumer{Consumer: c, topic: topic}
	partitions, err := t.Partitions(t.topic)
	if err != nil {
		return nil, err
	}
	for _, partition := range partitions {
		t.partitions = append(t.partitions, strconv.Itoa(int(partition)))
		pc, err := t.ConsumePartition(topic, partition, sarama.OffsetOldest)
		if err != nil {
			return nil, err
		}
		t.partitionConsumers = append(t.partitionConsumers, pc)
	}
	return t, nil
}

func (c *topicConsumer) tryNextMessage(ctx context.Context) *sarama.ConsumerMessage {
	for _, pc := range c.partitionConsumers {
		select {
		case <-ctx.Done():
			return nil
		case m := <-pc.Messages():
			return m
		default:
		}
	}
	return nil
}

func (c *topicConsumer) Next(ctx context.Context) *sarama.ConsumerMessage {
	m := c.tryNextMessage(ctx)
	for ; m == nil; m = c.tryNextMessage(ctx) {
		if ctx.Err() != nil {
			return nil
		}
	}
	return m
}

func (c *topicConsumer) Close() {
	for _, pc := range c.partitionConsumers {
		pc.AsyncClose()
		// Drain the messages and errors as required by AsyncClose.
		for range pc.Messages() {
		}
		for range pc.Errors() {
		}
	}
	_ = c.Consumer.Close()
}
