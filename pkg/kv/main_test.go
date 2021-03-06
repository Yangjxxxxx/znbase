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

package kv_test

import (
	"os"
	"testing"

	"github.com/znbasedb/znbase/pkg/security"
	"github.com/znbasedb/znbase/pkg/security/securitytest"
	"github.com/znbasedb/znbase/pkg/server"
	"github.com/znbasedb/znbase/pkg/testutils/buildutil"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

//go:generate ../util/leaktest/add-leaktest.sh *_test.go

func init() {
	security.SetAssetLoader(securitytest.EmbeddedAssets)
}

func TestForbiddenDeps(t *testing.T) {
	defer leaktest.AfterTest(t)()
	// Verify kv does not depend on storage (or any of its subpackages).
	buildutil.VerifyNoImports(t,
		"github.com/znbasedb/znbase/pkg/kv", true,
		// TODO(tschottdorf): should really disallow ./storage/... but at the
		// time of writing there's a (legit) dependency on `enginepb`.
		[]string{
			"github.com/znbasedb/znbase/pkg/storage",
			"github.com/znbasedb/znbase/pkg/storage/engine",
		},
		[]string{})
}

func TestMain(m *testing.M) {
	serverutils.InitTestServerFactory(server.TestServerFactory)
	os.Exit(m.Run())
}
