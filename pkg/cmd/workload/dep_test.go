// Copyright 2018 The Cockroach Authors.
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
	"testing"

	"github.com/znbasedb/znbase/pkg/testutils/buildutil"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestNoLinkForbidden(t *testing.T) {
	defer leaktest.AfterTest(t)()

	// We want workload to be lightweight. Ensure it doesn't depend on c-deps,
	// which are slow to compile and bloat the binary.
	buildutil.VerifyNoImports(t, "github.com/znbasedb/znbase/pkg/cmd/workload",
		true /* cgo */, []string{"c-deps"}, nil /* forbiddenPrefixes */)
}
