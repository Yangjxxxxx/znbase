// Copyright 2019  The Cockroach Authors.
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

package tree

import (
	"fmt"
	"testing"

	"github.com/znbasedb/znbase/pkg/sql/sem/types"
)

func TestPlaceholderTypesEquals(t *testing.T) {
	testCases := []struct {
		a, b  PlaceholderTypes
		equal bool
	}{
		{ // 0
			PlaceholderTypes{},
			PlaceholderTypes{},
			true,
		},
		{ // 1
			PlaceholderTypes{types.Int, types.Int},
			PlaceholderTypes{types.Int, types.Int},
			true,
		},
		{ // 2
			PlaceholderTypes{types.Int},
			PlaceholderTypes{types.Int, types.Int},
			false,
		},
		{ // 3
			PlaceholderTypes{types.Int, nil},
			PlaceholderTypes{types.Int, types.Int},
			false,
		},
		{ // 4
			PlaceholderTypes{types.Int, types.Int},
			PlaceholderTypes{types.Int, nil},
			false,
		},
		{ // 5
			PlaceholderTypes{types.Int, nil},
			PlaceholderTypes{types.Int, nil},
			true,
		},
		{ // 6
			PlaceholderTypes{types.Int},
			PlaceholderTypes{types.Int, nil},
			false,
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			res := tc.a.Equals(tc.b)
			if res != tc.equal {
				t.Errorf("%v vs %v: expected %t, got %t", tc.a, tc.b, tc.equal, res)
			}
			res2 := tc.b.Equals(tc.a)
			if res != res2 {
				t.Errorf("%v vs %v: not commutative", tc.a, tc.b)
			}
		})
	}
}
