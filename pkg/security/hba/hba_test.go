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

package hba

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/kr/pretty"
	"github.com/znbasedb/znbase/pkg/testutils/datadriven"
)

func TestParse(t *testing.T) {
	datadriven.RunTest(t, filepath.Join("testdata", "parse"), func(d *datadriven.TestData) string {
		conf, err := Parse(d.Input)
		if err != nil {
			return fmt.Sprintf("error: %v\n", err)
		}
		return fmt.Sprintf("%# v", pretty.Formatter(conf))
	})
}

// TODO(mjibson): these are untested outside icl +gss builds.
var _ = Entry.GetOption
var _ = Entry.GetOptions
