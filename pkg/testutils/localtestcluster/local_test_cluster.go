// Copyright 2015  The Cockroach Authors.
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

package localtestcluster

import (
	"context"
	"sort"
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/gossip"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/rpc"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/tscache"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/hlc"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/metric"
	"github.com/znbasedb/znbase/pkg/util/stop"
)

// A LocalTestCluster encapsulates an in-memory instantiation of a
// znbase node with a single store using a local sender. Example
// usage of a LocalTestCluster follows:
//
//   s := &LocalTestCluster{}
//   s.Start(t, testutils.NewNodeTestBaseContext(),
//           kv.InitFactoryForLocalTestCluster)
//   defer s.Stop()
//
// Note that the LocalTestCluster is different from server.TestCluster
// in that although it uses a distributed sender, there is no RPC traffic.
type LocalTestCluster struct {
	Cfg               storage.StoreConfig
	Manual            *hlc.ManualClock
	Clock             *hlc.Clock
	Gossip            *gossip.Gossip
	Eng               engine.Engine
	Store             *storage.Store
	StoreTestingKnobs *storage.StoreTestingKnobs
	DBContext         *client.DBContext
	DB                *client.DB
	Stores            *storage.Stores
	Stopper           *stop.Stopper
	Latency           time.Duration // sleep for each RPC sent
	tester            testing.TB

	// DisableLivenessHeartbeat, if set, inhibits the heartbeat loop. Some tests
	// need this because, for example, the heartbeat loop increments some
	// transaction metrics.
	// However, note that without heartbeats, ranges with epoch-based leases
	// cannot be accessed because the leases cannot be granted.
	// See also DontCreateSystemRanges.
	DisableLivenessHeartbeat bool

	// DontCreateSystemRanges, if set, makes the cluster start with a single
	// range, not with all the system ranges (as regular cluster start).
	// If DisableLivenessHeartbeat is set, you probably want to also set this so
	// that ranges requiring epoch-based leases are not created automatically.
	DontCreateSystemRanges bool
}

// InitFactoryFn is a callback used to initiate the txn coordinator
// sender factory (we don't do it directly from this package to avoid
// a dependency on kv).
type InitFactoryFn func(
	st *cluster.Settings,
	nodeDesc *roachpb.NodeDescriptor,
	tracer opentracing.Tracer,
	clock *hlc.Clock,
	latency time.Duration,
	stores client.Sender,
	stopper *stop.Stopper,
	gossip *gossip.Gossip,
) client.TxnSenderFactory

// Start starts the test cluster by bootstrapping an in-memory store
// (defaults to maximum of 50M). The server is started, launching the
// node RPC server and all HTTP endpoints. Use the value of
// TestServer.Addr after Start() for client connections. Use Stop()
// to shutdown the server after the test completes.
func (ltc *LocalTestCluster) Start(t testing.TB, baseCtx *base.Config, initFactory InitFactoryFn) {
	ltc.Manual = hlc.NewManualClock(123)
	ltc.Clock = hlc.NewClock(ltc.Manual.UnixNano, 50*time.Millisecond)
	cfg := storage.TestStoreConfig(ltc.Clock)
	ambient := log.AmbientContext{Tracer: cfg.Settings.Tracer}
	nc := &base.NodeIDContainer{}
	ambient.AddLogTag("n", nc)

	nodeID := roachpb.NodeID(1)
	nodeDesc := &roachpb.NodeDescriptor{
		NodeID:  nodeID,
		Address: util.MakeUnresolvedAddr("tcp", "invalid.invalid:26257"),
	}

	ltc.tester = t
	ltc.Stopper = stop.NewStopper()
	rpcContext := rpc.NewContext(ambient, baseCtx, ltc.Clock, ltc.Stopper, &cfg.Settings.Version)
	cfg.RPCContext = rpcContext
	c := &rpcContext.ClusterID
	server := rpc.NewServer(rpcContext) // never started
	ltc.Gossip = gossip.New(ambient, c, nc, rpcContext, server, ltc.Stopper, metric.NewRegistry(), roachpb.Locality{})
	ltc.Eng = engine.NewInMem(roachpb.Attributes{}, 50<<20)
	ltc.Stopper.AddCloser(ltc.Eng)

	ltc.Stores = storage.NewStores(ambient, ltc.Clock, cfg.Settings.Version.MinSupportedVersion, cfg.Settings.Version.ServerVersion)

	factory := initFactory(cfg.Settings, nodeDesc, ambient.Tracer, ltc.Clock, ltc.Latency, ltc.Stores, ltc.Stopper, ltc.Gossip)
	if ltc.DBContext == nil {
		dbCtx := client.DefaultDBContext()
		dbCtx.Stopper = ltc.Stopper
		ltc.DBContext = &dbCtx
	}
	ltc.DBContext.NodeID.Set(context.Background(), nodeID)
	ltc.DB = client.NewDBWithContext(cfg.AmbientCtx, factory, ltc.Clock, *ltc.DBContext)
	transport := storage.NewDummyRaftTransport(cfg.Settings)
	// By default, disable the replica scanner and split queue, which
	// confuse tests using LocalTestCluster.
	if ltc.StoreTestingKnobs == nil {
		cfg.TestingKnobs.DisableScanner = true
		cfg.TestingKnobs.DisableSplitQueue = true
	} else {
		cfg.TestingKnobs = *ltc.StoreTestingKnobs
	}
	cfg.AmbientCtx = ambient
	cfg.DB = ltc.DB
	cfg.Gossip = ltc.Gossip
	cfg.HistogramWindowInterval = metric.TestSampleInterval
	active, renewal := cfg.NodeLivenessDurations()
	cfg.NodeLiveness = storage.NewNodeLiveness(
		cfg.AmbientCtx,
		cfg.Clock,
		cfg.DB,
		[]engine.Engine{ltc.Eng},
		cfg.Gossip,
		active,
		renewal,
		cfg.Settings,
		cfg.HistogramWindowInterval,
	)
	storage.TimeUntilStoreDead.Override(&cfg.Settings.SV, storage.TestTimeUntilStoreDead)
	cfg.StorePool = storage.NewStorePool(
		cfg.AmbientCtx,
		cfg.Settings,
		cfg.Gossip,
		cfg.Clock,
		cfg.NodeLiveness.GetNodeCount,
		storage.MakeStorePoolNodeLivenessFunc(cfg.NodeLiveness),
		/* deterministic */ false,
	)
	cfg.Transport = transport
	cfg.TimestampCachePageSize = tscache.TestSklPageSize
	ctx := context.TODO()

	if err := storage.Bootstrap(ctx, ltc.Eng, roachpb.StoreIdent{NodeID: nodeID, StoreID: 1}, cfg.Settings.Version.BootstrapVersion()); err != nil {
		t.Fatalf("unable to start local test cluster: %s", err)
	}
	ltc.Store = storage.NewStore(cfg, ltc.Eng, nodeDesc)

	var initialValues []roachpb.KeyValue
	var splits []roachpb.RKey
	if !ltc.DontCreateSystemRanges {
		schema := sqlbase.MakeMetadataSchema()
		var tableSplits []roachpb.RKey
		initialValues, tableSplits = schema.GetInitialValues()
		splits = append(config.StaticSplits(), tableSplits...)
		sort.Slice(splits, func(i, j int) bool {
			return splits[i].Less(splits[j])
		})
	}

	if err := ltc.Store.WriteInitialData(
		ctx,
		initialValues,
		cfg.Settings.Version.ServerVersion,
		1, /* numStores */
		splits,
	); err != nil {
		t.Fatalf("unable to start local test cluster: %s", err)
	}

	if !ltc.DisableLivenessHeartbeat {
		cfg.NodeLiveness.StartHeartbeat(ctx, ltc.Stopper, nil /* alive */)
	}

	if err := ltc.Store.Start(ctx, ltc.Stopper); err != nil {
		t.Fatalf("unable to start local test cluster: %s", err)
	}

	ltc.Stores.AddStore(ltc.Store)
	nc.Set(ctx, nodeDesc.NodeID)
	if err := ltc.Gossip.SetNodeDescriptor(nodeDesc); err != nil {
		t.Fatalf("unable to set node descriptor: %s", err)
	}
	ltc.Cfg = cfg
}

// Stop stops the cluster.
func (ltc *LocalTestCluster) Stop() {
	// If the test has failed, we don't attempt to clean up: This often hangs,
	// and leaktest will disable itself for the remaining tests so that no
	// unrelated errors occur from a dirty shutdown.
	if ltc.tester.Failed() {
		return
	}
	if r := recover(); r != nil {
		panic(r)
	}
	ltc.Stopper.Stop(context.TODO())
}
