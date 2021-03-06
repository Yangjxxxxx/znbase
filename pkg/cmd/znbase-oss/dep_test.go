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

package main

import (
	"testing"

	"github.com/znbasedb/znbase/pkg/testutils/buildutil"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestNoLinkForbidden(t *testing.T) {
	defer leaktest.AfterTest(t)()
	// Verify that the znbase floss binary doesn't depend on certain packages.
	buildutil.VerifyNoImports(t,
		"github.com/znbasedb/znbase/pkg/cmd/znbase-oss",
		true,
		nil,
		[]string{
			//"github.com/znbasedb/znbase/pkg/icl",
			"github.com/znbasedb/znbase/pkg/ui/disticl",
		},
	)
}
