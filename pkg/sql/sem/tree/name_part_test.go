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
// permissions and limitations under the License.

package tree_test

import (
	"testing"

	"github.com/znbasedb/znbase/pkg/sql/parser"
	"github.com/znbasedb/znbase/pkg/testutils"
)

func TestUnresolvedObjectName(t *testing.T) {
	testCases := []struct {
		in, out  string
		expanded string
		err      string
	}{
		{`a`, `a`, `""."".a`, ``},
		{`a.b`, `a.b`, `"".a.b`, ``},
		{`a.b.c`, `a.b.c`, `a.b.c`, ``},
		{`a.b.c.d`, ``, ``, `syntax error at or near "\."`},
		{`a.""`, ``, ``, `invalid table name: a\.""`},
		{`a.b.""`, ``, ``, `invalid table name: a\.b\.""`},
		{`a.b.c.""`, ``, ``, `syntax error at or near "\."`},
		{`a."".c`, ``, ``, `invalid table name: a\.""\.c`},

		// ZNBaseDB extension: empty catalog name.
		{`"".b.c`, `"".b.c`, `"".b.c`, ``},

		// Check keywords: disallowed in first position, ok afterwards.
		{`user.x.y`, ``, ``, `syntax error`},
		{`"user".x.y`, `"user".x.y`, `"user".x.y`, ``},
		{`x.user.y`, `x."user".y`, `x."user".y`, ``},
		{`x.user`, `x."user"`, `"".x."user"`, ``},

		{`foo@bar`, ``, ``, `syntax error at or near "@"`},
		{`test.*`, ``, ``, `syntax error at or near "\*"`},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			tn, err := parser.ParseTableName(tc.in, 0)
			if !testutils.IsError(err, tc.err) {
				t.Fatalf("%s: expected %s, but found %v", tc.in, tc.err, err)
			}
			if tc.err != "" {
				return
			}
			if out := tn.String(); tc.out != out {
				t.Fatalf("%s: expected %s, but found %s", tc.in, tc.out, out)
			}
			tn.ExplicitSchema = true
			tn.ExplicitCatalog = true
			if out := tn.String(); tc.expanded != out {
				t.Fatalf("%s: expected full %s, but found %s", tc.in, tc.expanded, out)
			}
		})
	}
}
