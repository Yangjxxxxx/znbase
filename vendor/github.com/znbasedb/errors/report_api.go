// Copyright 2019 The Cockroach Authors.
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

package errors

import (
	"github.com/znbasedb/errors/report"
	raven "github.com/getsentry/raven-go"
)

// BuildSentryReport forwards a definition.
func BuildSentryReport(err error) (string, []raven.Interface, map[string]interface{}) {
	return report.BuildSentryReport(err)
}

// ReportError forwards a definition.
func ReportError(err error) (string, error) { return report.ReportError(err) }
