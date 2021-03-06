// Copyright 2017 The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package cliicl_test

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/cli"
	_ "github.com/znbasedb/znbase/pkg/icl"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/server"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
)

// cliTest is a stripped-down version of package cli's cliTest. It's currently
// easier to reimplement the pieces we need than it is to export cli.cliTest, as
// cli's tests frequently mutate internal package variables. We may need to
// revisit this decision if this package's tests become more complex.
type cliTest struct {
	*server.TestServer
	connArgs []string
}

func newCLITest(args base.TestServerArgs) cliTest {
	s, err := serverutils.StartServerRaw(args)
	if err != nil {
		panic(err)
	}
	host, port, err := net.SplitHostPort(s.ServingAddr())
	if err != nil {
		panic(err)
	}
	return cliTest{
		TestServer: s.(*server.TestServer),
		connArgs:   []string{"--insecure", "--host=" + host + ":" + port},
	}
}

func (c *cliTest) close() {
	c.Stopper().Stop(context.Background())
}

func (c *cliTest) run(line string) {
	c.runWithArgs(strings.Fields(line))
}

func (c *cliTest) runWithArgs(args []string) {
	fmt.Println(strings.Join(args, " "))
	cli.TestingReset()
	if err := cli.Run(append(args, c.connArgs...)); err != nil {
		fmt.Println(err)
	}
}

func Example_iclzone() {
	storeSpec := base.DefaultTestStoreSpec
	storeSpec.Attributes = roachpb.Attributes{Attrs: []string{"ssd"}}
	c := newCLITest(base.TestServerArgs{
		Insecure:   true,
		StoreSpecs: []base.StoreSpec{storeSpec},
		Locality: roachpb.Locality{
			Tiers: []roachpb.Tier{
				{Key: "region", Value: "us-east-1"},
				{Key: "zone", Value: "us-east-1a"},
			},
		},
	})
	defer c.close()

	c.runWithArgs([]string{"sql", "-e", "CREATE DATABASE db"})
	c.runWithArgs([]string{"sql", "-e", `CREATE TABLE db.t (
  c1 STRING PRIMARY KEY,
  c2 STRING
) PARTITION BY LIST (c1) (
  PARTITION p0 VALUES IN ('a'),
  PARTITION p1 VALUES IN (DEFAULT)
)`})
	c.runWithArgs([]string{"sql", "-e", "CREATE INDEX ON db.t (c2)"})
	c.run("zone set db.t@nonexistent --file=./../../cli/testdata/zone_attrs.yaml")
	c.run("zone set db.t.nonexistent --file=./../../cli/testdata/zone_attrs.yaml")
	c.run("zone set db.t.p0@t_c2_idx --file=./../../cli/testdata/zone_attrs.yaml")
	c.run("zone set db.t@primary --file=./../../cli/testdata/zone_attrs.yaml")
	c.run("zone get db.t.p0")
	c.run("zone get db.t")
	c.run("zone get db.t@t_c2_idx")
	c.run("zone set db.t.p1 --file=./../../cli/testdata/zone_range_max_bytes.yaml")
	c.run("zone get db.t.p1")
	c.run("zone get db.t.p0")
	c.run("zone ls")
	c.run("zone rm db.t@primary")
	c.run("zone get db.t.p0")
	c.run("zone get db.t.p1")
	c.run("zone ls")
	c.run("zone rm db.t.p0")
	c.run("zone rm db.t.p1")
	c.run("zone ls")
	c.run("zone set db.t@primary --file=./../../cli/testdata/zone_attrs_advanced.yaml")
	c.run("zone set db.t@primary --file=./../../cli/testdata/zone_attrs_experimental.yaml")

	// Output:
	// sql -e CREATE DATABASE db
	// CREATE DATABASE
	// sql -e CREATE TABLE db.t (
	//   c1 STRING PRIMARY KEY,
	//   c2 STRING
	// ) PARTITION BY LIST (c1) (
	//   PARTITION p0 VALUES IN ('a'),
	//   PARTITION p1 VALUES IN (DEFAULT)
	// )
	// CREATE TABLE
	// sql -e CREATE INDEX ON db.t (c2)
	// CREATE INDEX
	// zone set db.t@nonexistent --file=./../../cli/testdata/zone_attrs.yaml
	// pq: index "nonexistent" does not exist
	// zone set db.t.nonexistent --file=./../../cli/testdata/zone_attrs.yaml
	// pq: syntax error at or near "experimental"
	// zone set db.t.p0@t_c2_idx --file=./../../cli/testdata/zone_attrs.yaml
	// index and partition cannot be specified simultaneously: "db.t.p0@t_c2_idx"
	// zone set db.t@primary --file=./../../cli/testdata/zone_attrs.yaml
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []

	// zone get db.t.p0
	// db.t@primary
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone get db.t
	// .default
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone get db.t@t_c2_idx
	// .default
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone set db.t.p1 --file=./../../cli/testdata/zone_range_max_bytes.yaml
	// pq: syntax error at or near "experimental"
	// zone get db.t.p1
	// db.t@primary
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone get db.t.p0
	// db.t@primary
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone ls
	// .default
	// .liveness
	// .meta
	// .system
	// db.t@primary
	// system
	// system.jobs
	// zone rm db.t@primary
	// zone get db.t.p0
	// .default
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone get db.t.p1
	// .default
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []
	// zone ls
	// .default
	// .liveness
	// .meta
	// .system
	// system
	// system.jobs
	// zone rm db.t.p0
	// pq: syntax error at or near "experimental"
	// zone rm db.t.p1
	// pq: syntax error at or near "experimental"
	// zone ls
	// .default
	// .liveness
	// .meta
	// .system
	// system
	// system.jobs
	// zone set db.t@primary --file=./../../cli/testdata/zone_attrs_advanced.yaml
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []

	// zone set db.t@primary --file=./../../cli/testdata/zone_attrs_experimental.yaml
	// range_min_bytes: 16777216
	// range_max_bytes: 67108864
	// gc:
	//   ttlseconds: 90000
	// num_replicas: 1
	// constraints: []
	// lease_preferences: []

}
