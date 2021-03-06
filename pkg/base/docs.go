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

package base

import "github.com/znbasedb/znbase/pkg/build"

// DocsURLBase is the root URL for the version of the docs associated with this
// binary.
//var DocsURLBase = "https://www.znbaselabs.com/docs/" + build.VersionPrefix()
var DocsURLBase = "official docs about version " + build.GetInfo().Tag

// DocsURL generates the URL to pageName in the version of the docs associated
// with this binary.
func DocsURL(pageName string) string { return DocsURLBase }
