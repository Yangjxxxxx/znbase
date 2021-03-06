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

package distsqlplan_test

import (
	"context"
	"testing"

	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/kv"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/testutils/distsqlutils"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/testutils/sqlutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestFakeSpanResolver(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	tc := serverutils.StartTestCluster(t, 3, base.TestClusterArgs{})
	defer tc.Stopper().Stop(ctx)

	sqlutils.CreateTable(
		t, tc.ServerConn(0), "t",
		"k INT PRIMARY KEY, v INT",
		100,
		func(row int) []tree.Datum {
			return []tree.Datum{
				tree.NewDInt(tree.DInt(row)),
				tree.NewDInt(tree.DInt(row * row)),
			}
		},
	)

	resolver := distsqlutils.FakeResolverForTestCluster(tc)

	db := tc.Server(0).DB()

	txn := client.NewTxn(ctx, db, tc.Server(0).NodeID(), client.RootTxn)
	it := resolver.NewSpanResolverIterator(txn)

	tableDesc := sqlbase.GetTableDescriptor(db, "test", "public", "t")
	primIdxValDirs := sqlbase.IndexKeyValDirs(&tableDesc.PrimaryIndex)

	span := tableDesc.PrimaryIndexSpan()

	// Make sure we see all the nodes. It will not always happen (due to
	// randomness) but it should happen most of the time.
	for attempt := 0; attempt < 10; attempt++ {
		nodesSeen := make(map[roachpb.NodeID]struct{})
		it.Seek(ctx, span, kv.Ascending)
		lastKey := span.Key
		for {
			if !it.Valid() {
				t.Fatal(it.Error())
			}
			desc := it.Desc()
			rinfo, err := it.ReplicaInfo(ctx)
			if err != nil {
				t.Fatal(err)
			}

			prettyStart := keys.PrettyPrint(primIdxValDirs, desc.StartKey.AsRawKey())
			prettyEnd := keys.PrettyPrint(primIdxValDirs, desc.EndKey.AsRawKey())
			t.Logf("%d %s %s", rinfo.NodeID, prettyStart, prettyEnd)

			if !lastKey.Equal(desc.StartKey.AsRawKey()) {
				t.Errorf("unexpected start key %s, should be %s", prettyStart, keys.PrettyPrint(primIdxValDirs, span.Key))
			}
			if !desc.StartKey.Less(desc.EndKey) {
				t.Errorf("invalid range %s to %s", prettyStart, prettyEnd)
			}
			lastKey = desc.EndKey.AsRawKey()
			nodesSeen[rinfo.NodeID] = struct{}{}

			if !it.NeedAnother() {
				break
			}
			it.Next(ctx)
		}

		if !lastKey.Equal(span.EndKey) {
			t.Errorf("last key %s, should be %s", keys.PrettyPrint(primIdxValDirs, lastKey), keys.PrettyPrint(primIdxValDirs, span.EndKey))
		}

		if len(nodesSeen) == tc.NumServers() {
			// Saw all the nodes.
			break
		}
		t.Logf("not all nodes seen; retrying")
	}
}
