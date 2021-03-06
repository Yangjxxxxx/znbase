// Copyright 2016  The Cockroach Authors.
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

package protoutil_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/gossip"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/storage/storagepb"
	"github.com/znbasedb/znbase/pkg/util/protoutil"
)

func TestCloneProto(t *testing.T) {
	testCases := []struct {
		pb          protoutil.Message
		shouldPanic bool
	}{
		// Uncloneable types (all contain UUID fields).
		{&roachpb.StoreIdent{}, true},
		{&enginepb.TxnMeta{}, true},
		{&roachpb.Transaction{}, true},
		{&roachpb.Error{}, true},
		{&protoutil.RecursiveAndUncloneable{}, true},

		// Cloneable types. This includes all types for which a
		// protoutil.Clone call exists in the codebase as of 2016-11-21.
		{&config.ZoneConfig{}, false},
		{&gossip.Info{}, false},
		{&gossip.BootstrapInfo{}, false},
		{&sqlbase.IndexDescriptor{}, false},
		{&roachpb.SplitTrigger{}, false},
		{&roachpb.Value{}, false},
		{&storagepb.ReplicaState{}, false},
		{&roachpb.RangeDescriptor{}, false},
		{&sqlbase.PartitioningDescriptor{}, false},
	}
	for _, tc := range testCases {
		var clone protoutil.Message
		var panicObj interface{}
		func() {
			defer func() {
				panicObj = recover()
			}()
			clone = protoutil.Clone(tc.pb)
		}()

		if tc.shouldPanic {
			if panicObj == nil {
				t.Errorf("%T: expected panic but didn't get one", tc.pb)
			} else {
				if panicStr := fmt.Sprint(panicObj); !strings.Contains(panicStr, "attempt to clone") {
					t.Errorf("%T: got unexpected panic %s", tc.pb, panicStr)
				}
			}
		} else {
			if panicObj != nil {
				t.Errorf("%T: got unexpected panic %v", tc.pb, panicObj)
			}
		}

		if panicObj == nil {
			realClone := proto.Clone(tc.pb)
			if !reflect.DeepEqual(clone, realClone) {
				t.Errorf("%T: clone did not equal original. expected:\n%+v\ngot:\n%+v", tc.pb, realClone, clone)
			}
		}
	}
}
