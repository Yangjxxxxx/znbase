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

package xform

import (
	"math"
	"testing"

	"github.com/znbasedb/znbase/pkg/config"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"gopkg.in/yaml.v2"
)

func TestLocalityMatchScore(t *testing.T) {
	defer leaktest.AfterTest(t)()

	testCases := []struct {
		locality    string
		constraints string
		leasePrefs  string
		expected    float64
	}{
		{locality: "region=us,dc=east", constraints: "[]", expected: 0.0},
		{locality: "region=us,dc=east", constraints: "[+region=eu,+dc=uk,+dc=de]", expected: 0.0},
		{locality: "region=us,dc=east", constraints: "[-region=us,+dc=east]", expected: 0.0},
		{locality: "region=us,dc=east", constraints: "[+region=eu,+dc=east]", expected: 0.0},
		{locality: "region=us,dc=east", constraints: "[-region=eu]", expected: 0.0},
		{locality: "region=us,dc=east", constraints: "[+region=us]", expected: 0.5},
		{locality: "region=us,dc=east", constraints: "[+region=us,+region=eu]", expected: 0.5},
		{locality: "region=us,dc=east", constraints: "[+region=eu,+region=ap,+region=us]", expected: 0.5},
		{locality: "region=us,dc=east", constraints: "[+region=us,-dc=east]", expected: 0.5},
		{locality: "region=us,dc=east", constraints: "[+region=us,+dc=west]", expected: 0.5},
		{locality: "region=us,dc=east", constraints: "[+region=us,+dc=east]", expected: 1.0},
		{locality: "region=us,dc=east", constraints: "[+dc=east]", expected: 1.0},
		{locality: "region=us,dc=east", constraints: "[+dc=west,+dc=east]", expected: 1.0},
		{locality: "region=us,dc=east", constraints: "[-region=eu,+dc=east]", expected: 1.0},
		{locality: "region=us,dc=east", constraints: "[+region=eu,+dc=east,+region=us,+dc=west]", expected: 1.0},
		{locality: "region=us,dc=east", constraints: "[+region=us,+dc=east,+rack=1,-ssd]", expected: 1.0},

		{locality: "region=us,dc=east", constraints: `{"+region=us,+dc=east":3,"-dc=east":2}`, expected: 0.0},
		{locality: "region=us,dc=east", constraints: `{"+region=us,+dc=east":3,"+region=eu,+dc=east":2}`, expected: 0.0},
		{locality: "region=us,dc=east", constraints: `{"+region=us,+dc=east":3,"+region=us,+region=eu":2}`, expected: 0.5},
		{locality: "region=us,dc=east", constraints: `{"+region=us,+dc=east":3,"+dc=east,+dc=west":2}`, expected: 1.0},

		{locality: "region=us,dc=east", leasePrefs: "[[]]", expected: 0.0},
		{locality: "region=us,dc=east", leasePrefs: "[[+dc=west]]", expected: 0.0},
		{locality: "region=us,dc=east", leasePrefs: "[[+region=us]]", expected: 0.17},
		{locality: "region=us,dc=east", leasePrefs: "[[+region=us,+dc=east]]", expected: 0.33},

		{locality: "region=us,dc=east", constraints: "[+region=eu]", leasePrefs: "[[+dc=west]]", expected: 0.0},
		{locality: "region=us,dc=east", constraints: "[+region=eu]", leasePrefs: "[[+region=us]]", expected: 0.17},
		{locality: "region=us,dc=east", constraints: "[+region=eu]", leasePrefs: "[[+dc=east]]", expected: 0.33},
		{locality: "region=us,dc=east", constraints: "[+region=us]", leasePrefs: "[[+dc=west]]", expected: 0.33},
		{locality: "region=us,dc=east", constraints: "[+region=us]", leasePrefs: "[[+region=us]]", expected: 0.50},
		{locality: "region=us,dc=east", constraints: "[+region=us]", leasePrefs: "[[+dc=east]]", expected: 0.67},
		{locality: "region=us,dc=east", constraints: "[+dc=east]", leasePrefs: "[[+region=us]]", expected: 0.83},
		{locality: "region=us,dc=east", constraints: "[+dc=east]", leasePrefs: "[[+dc=east]]", expected: 1.0},
		{locality: "region=us,dc=east", constraints: "[+region=us,+dc=east]", leasePrefs: "[[+region=us,+dc=east]]", expected: 1.0},
	}

	for _, tc := range testCases {
		zone := &config.ZoneConfig{}

		var locality roachpb.Locality
		if err := locality.Set(tc.locality); err != nil {
			t.Fatal(err)
		}

		if tc.constraints != "" {
			constraintsList := &config.ConstraintsList{}
			if err := yaml.UnmarshalStrict([]byte(tc.constraints), constraintsList); err != nil {
				t.Fatal(err)
			}
			zone.Constraints = constraintsList.Constraints
		}

		if tc.leasePrefs != "" {
			if err := yaml.UnmarshalStrict([]byte(tc.leasePrefs), &zone.LeasePreferences); err != nil {
				t.Fatal(err)
			}
		}

		actual := math.Round(localityMatchScore(zone, locality)*100) / 100
		if actual != tc.expected {
			t.Errorf("locality=%v, constraints=%v, leasePrefs=%v: expected %v, got %v",
				tc.locality, tc.constraints, tc.leasePrefs, tc.expected, actual)
		}
	}
}
