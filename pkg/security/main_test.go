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

package security_test

import (
	"os"
	"testing"

	"github.com/znbasedb/znbase/pkg/security"
	"github.com/znbasedb/znbase/pkg/security/securitytest"
	"github.com/znbasedb/znbase/pkg/server"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	_ "github.com/znbasedb/znbase/pkg/util/log" // for flags
)

// ResetTest sets up the test environment. In particular, it embeds the
// EmbeddedCertsDir folder and makes the tls package load from there.
func ResetTest() {
	security.SetAssetLoader(securitytest.EmbeddedAssets)
}

func TestMain(m *testing.M) {
	ResetTest()
	serverutils.InitTestServerFactory(server.TestServerFactory)
	os.Exit(m.Run())
}

//go:generate ../util/leaktest/add-leaktest.sh *_test.go
