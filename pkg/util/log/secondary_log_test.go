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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/log/logtags"
)

func TestSecondaryLog(t *testing.T) {
	defer leaktest.AfterTest(t)()

	s := ScopeWithoutShowLogs(t)
	defer s.Close(t)
	setFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Make a new logger, in the same directory.
	l := NewSecondaryLogger(ctx, &logging.logDir, "woo", true, false)

	// Interleave some messages.
	Infof(context.Background(), "test1")
	ctx = logtags.AddTag(ctx, "hello", "world")
	l.Logf(ctx, "story time")
	Infof(context.Background(), "test2")

	// Make sure the content made it to disk.
	Flush()

	// Check that the messages indeed made it to different files.

	contents, err := ioutil.ReadFile(logging.file.(*syncBuffer).file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "test1") || !strings.Contains(string(contents), "test2") {
		t.Errorf("log does not contain error text\n%s", contents)
	}
	if strings.Contains(string(contents), "world") {
		t.Errorf("secondary log spilled into primary\n%s", contents)
	}

	contents, err = ioutil.ReadFile(l.logger.file.(*syncBuffer).file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "hello") ||
		!strings.Contains(string(contents), "world") ||
		!strings.Contains(string(contents), "story time") {
		t.Errorf("secondary log does not contain text\n%s", contents)
	}
	if strings.Contains(string(contents), "test1") {
		t.Errorf("primary log spilled into secondary\n%s", contents)
	}

}

func TestRedirectStderrWithSecondaryLoggersActive(t *testing.T) {
	s := ScopeWithoutShowLogs(t)
	defer s.Close(t)

	setFlags()
	logging.stderrThreshold = Severity_NONE

	// Ensure that the main log is initialized. This should take over
	// stderr.
	Infof(context.Background(), "test123")

	// Now create a secondary logger in the same directory.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := NewSecondaryLogger(ctx, &logging.logDir, "woo", true, false)

	// Log something on the secondary logger.
	l.Logf(context.Background(), "test456")

	// Send something on stderr.
	const stderrText = "hello stderr"
	fmt.Fprint(os.Stderr, stderrText)

	// Check the main log file: we want our stderr text there.
	contents, err := ioutil.ReadFile(logging.file.(*syncBuffer).file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), stderrText) {
		t.Errorf("log does not contain stderr text\n%s", contents)
	}

	// Check the secondary log file: we don't want our stderr text there.
	contents2, err := ioutil.ReadFile(l.logger.file.(*syncBuffer).file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents2), stderrText) {
		t.Errorf("secondary log erronously contains stderr text\n%s", contents2)
	}
}
