// Copyright 2018 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package vecexec

import (
	"testing"

	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestBoolVecToSelOp(t *testing.T) {
	defer leaktest.AfterTest(t)()
	tcs := []struct {
		boolCol  uint32
		tuples   tuples
		expected tuples
	}{
		{
			boolCol:  0,
			tuples:   tuples{{true}, {false}, {true}},
			expected: tuples{{true}, {true}},
		},
	}
	for _, tc := range tcs {
		runTests(t, []tuples{tc.tuples}, tc.expected, orderedVerifier, func(input []Operator) (Operator, error) {
			return NewBoolVecToSelOp(input[0], 0), nil
		})
	}
}
