// Copyright 2019  The Cockroach Authors.
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

package goroutinedumper

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/testutils"
)

type goroutinesVal struct {
	secs       time.Duration // the time at which this goroutines value was emitted
	threshold  int64
	goroutines int64
}

func TestHeuristic(t *testing.T) {
	const dumpDir = "dump_dir"
	st := &cluster.Settings{}

	cases := []struct {
		name          string
		heuristics    []heuristic
		vals          []goroutinesVal
		dumpsToFail   []string
		expectedDumps []string
	}{
		// N is the number of goroutines
		{
			name:       "Use only doubleSinceLastDumpHeuristic",
			heuristics: []heuristic{doubleSinceLastDumpHeuristic},
			vals: []goroutinesVal{
				{0, 100, 30},    // not trigger since N < numGoroutinesThreshold
				{10, 100, 40},   // not trigger since N < numGoroutinesThreshold
				{20, 100, 120},  // trigger since N >= numGoroutinesThreshold
				{50, 100, 35},   // not trigger since N has not doubled since last dump
				{70, 100, 150},  // not trigger since N has not doubled since last dump
				{80, 100, 250},  // trigger since N has doubled since last dump
				{100, 100, 135}, // not trigger since N has not doubled since last dump
				{180, 100, 30},  // not trigger since N has not doubled since last dump
				{190, 100, 80},  // not trigger since N has not doubled since last dump
				{220, 100, 500}, // trigger since N has doubled since last dump
			},
			expectedDumps: []string{
				"goroutine_dump.2019-01-01T00_00_20.double_since_last_dump.000000120",
				"goroutine_dump.2019-01-01T00_01_20.double_since_last_dump.000000250",
				"goroutine_dump.2019-01-01T00_03_40.double_since_last_dump.000000500",
			},
		},
		{
			name: "Fail some dumps when doubleSinceLastDumpHeuristic is used",
			heuristics: []heuristic{
				doubleSinceLastDumpHeuristic,
			},
			vals: []goroutinesVal{
				{0, 100, 20},    // not trigger since N < numGoroutinesThreshold
				{10, 100, 35},   // not trigger since N < numGoroutinesThreshold
				{20, 100, 110},  // trigger since N >= numGoroutinesThreshold
				{50, 100, 150},  // not trigger since N has not doubled since last dump
				{70, 100, 170},  // not trigger since N has not doubled since last dump
				{80, 100, 230},  // trigger but dump will fail
				{100, 100, 220}, // trigger since N has doubled since last dump
				{180, 100, 85},  // not trigger since N has not doubled since last dump
				{200, 100, 450}, // trigger since N has doubled since last dump
				{220, 100, 500}, // not trigger since N has not doubled since last dump
			},
			expectedDumps: []string{
				"goroutine_dump.2019-01-01T00_00_20.double_since_last_dump.000000110",
				"goroutine_dump.2019-01-01T00_01_40.double_since_last_dump.000000220",
				"goroutine_dump.2019-01-01T00_03_20.double_since_last_dump.000000450",
			},
			dumpsToFail: []string{
				"goroutine_dump.2019-01-01T00_01_20.double_since_last_dump.000000230",
			},
		},
		{
			name:       "Change in threshold resets the maxGoroutinesDumped",
			heuristics: []heuristic{doubleSinceLastDumpHeuristic},
			vals: []goroutinesVal{
				{0, 100, 30},    // not trigger since N < numGoroutinesThreshold
				{10, 100, 40},   // not trigger since N < numGoroutinesThreshold
				{20, 100, 120},  // trigger since N >= numGoroutinesThreshold
				{50, 100, 135},  // not trigger since N has not doubled since last dump
				{70, 100, 150},  // not trigger since N has not doubled since last dump
				{80, 200, 150},  // update numGoroutinesThreshold, which resets maxGoroutinesDumped
				{90, 200, 210},  // trigger since maxGoroutinesDumped was reset and N >= threshold
				{100, 200, 235}, // not trigger since N has not doubled since last dump
				{180, 200, 230}, // not trigger since N has not doubled since last dump
				{190, 200, 280}, // not trigger since N has not doubled since last dump
				{220, 200, 500}, // trigger since N has doubled since last dump
			},
			expectedDumps: []string{
				"goroutine_dump.2019-01-01T00_00_20.double_since_last_dump.000000120",
				"goroutine_dump.2019-01-01T00_01_30.double_since_last_dump.000000210",
				"goroutine_dump.2019-01-01T00_03_40.double_since_last_dump.000000500",
			},
		},
		{
			name:       "No heuristic is used",
			heuristics: []heuristic{},
			vals: []goroutinesVal{
				{0, 100, 10},
				{10, 100, 15},
				{20, 100, 50},
				{50, 100, 35},
				{70, 100, 80},
				{80, 100, 150},
				{100, 100, 120},
				{180, 100, 85},
				{200, 100, 130},
				{220, 100, 500},
			},
			expectedDumps: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			baseTime := time.Date(2019, time.January, 1, 0, 0, 0, 0, time.UTC)
			var dumps []string
			var currentTime time.Time
			gd := GoroutineDumper{
				maxGoroutinesDumped: 0,
				heuristics:          c.heuristics,
				currentTime: func() time.Time {
					return currentTime
				},
				takeGoroutineDump: func(dir string, filename string) error {
					assert.Equal(t, dumpDir, dir)
					for _, d := range c.dumpsToFail {
						if filename == d {
							return errors.New("this dump is set to fail")
						}
					}
					dumps = append(dumps, filename)
					return nil
				},
				gc:  func(ctx context.Context, dir string, sizeLimit int64) {},
				dir: dumpDir,
			}

			ctx := context.TODO()
			for _, v := range c.vals {
				currentTime = baseTime.Add(v.secs * time.Second)
				numGoroutinesThreshold.Override(&st.SV, v.threshold)
				gd.MaybeDump(ctx, st, v.goroutines)
			}
			assert.Equal(t, c.expectedDumps, dumps)
		})
	}
}

func TestNewGoroutineDumper(t *testing.T) {
	t.Run("fails because no directory is specified", func(t *testing.T) {
		_, err := NewGoroutineDumper("")
		assert.EqualError(t, err, "directory to store dumps could not be determined")
	})

	t.Run("fails because goroutine_dump already exists as a file", func(t *testing.T) {
		tempDir, dirCleanupFn := testutils.TempDir(t)
		defer dirCleanupFn()
		path := filepath.Join(tempDir, "goroutine_dump")
		emptyFile, err := os.Create(path)
		assert.NoError(t, err, "failed to create goroutine_dump file")
		err = emptyFile.Close()
		assert.NoError(t, err, "failed to close goroutine_dump file")

		_, err = NewGoroutineDumper(tempDir)
		assert.IsType(t, &os.PathError{}, err)
	})

	t.Run("succeeds", func(t *testing.T) {
		tempDir, dirCleanupFn := testutils.TempDir(t)
		defer dirCleanupFn()
		gd, err := NewGoroutineDumper(tempDir)
		assert.NoError(t, err, "unexpected error in NewGoroutineDumper")
		assert.Equal(t, int64(0), gd.goroutinesThreshold)
		assert.Equal(t, int64(0), gd.maxGoroutinesDumped)
		assert.Equal(t, filepath.Join(tempDir, "goroutine_dump"), gd.dir)
	})
}

func TestGC(t *testing.T) {
	type file struct {
		name string
		size int64
	}
	cases := []struct {
		name      string
		files     []file
		sizeLimit int64
		expected  []string
	}{
		{
			name: "total size smaller than size limit",
			files: []file{
				{name: "goroutine_dump.2019-01-01T00_00_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_10_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_20_00", size: 1},
			},
			sizeLimit: 5,
			expected: []string{
				"goroutine_dump.2019-01-01T00_00_00",
				"goroutine_dump.2019-01-01T00_10_00",
				"goroutine_dump.2019-01-01T00_20_00",
			},
		},
		{
			name: "total size smaller than size limit and unknown files are removed",
			files: []file{
				{name: "goroutine_dump.2019-01-01T00_00_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_10_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_20_00", size: 1},
				{name: "unknown_file", size: 1},
				{name: "unknown_file2", size: 1},
			},
			sizeLimit: 5,
			expected: []string{
				"goroutine_dump.2019-01-01T00_00_00",
				"goroutine_dump.2019-01-01T00_10_00",
				"goroutine_dump.2019-01-01T00_20_00",
			},
		},
		{
			name: "total size larger than size limit",
			files: []file{
				{name: "goroutine_dump.2019-01-01T00_00_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_10_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_20_00", size: 1},
				{name: "unknown_file", size: 1},
			},
			sizeLimit: 2,
			expected: []string{
				"goroutine_dump.2019-01-01T00_10_00",
				"goroutine_dump.2019-01-01T00_20_00",
			},
		},
		{
			name: "newest dump is already larger than size limit",
			files: []file{
				{name: "goroutine_dump.2019-01-01T00_00_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_10_00", size: 1},
				{name: "goroutine_dump.2019-01-01T00_20_00", size: 10},
				{name: "unknown_file", size: 1},
			},
			sizeLimit: 5,
			expected: []string{
				"goroutine_dump.2019-01-01T00_20_00",
			},
		},
		{
			name: "no dump in directory",
			files: []file{
				{name: "unknown_file", size: 1},
			},
			sizeLimit: 5,
			expected:  nil,
		},
		{
			name:      "no file in directory",
			files:     []file{},
			sizeLimit: 5,
			expected:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tempDir, dirCleanupFn := testutils.TempDir(t)
			defer dirCleanupFn()

			for _, f := range c.files {
				path := filepath.Join(tempDir, f.name)
				fi, err := os.Create(path)
				assert.NoError(t, err, "unexpected error when creating file %s", path)
				err = fi.Close()
				assert.NoError(t, err, "unexpected error when closing file %s", path)
				err = os.Truncate(path, f.size)
				assert.NoError(t, err, "unexpected error when truncating file %s", path)
			}
			ctx := context.TODO()
			gc(ctx, tempDir, c.sizeLimit)
			files, err := ioutil.ReadDir(tempDir)
			assert.NoError(t, err, "unexpected error when listing files in %s", tempDir)
			var actual []string
			for _, f := range files {
				actual = append(actual, f.Name())
			}
			assert.Equal(t, c.expected, actual)
		})
	}
}

func TestTakeGoroutineDump(t *testing.T) {
	t.Run("fails because dump already exists as a directory", func(t *testing.T) {
		tempDir, dirCleanupFn := testutils.TempDir(t)
		defer dirCleanupFn()
		filename := "goroutine_dump"
		path := filepath.Join(tempDir, filename)
		err := os.Mkdir(path, 0755)
		assert.NoError(t, err, "failed to make dump directory %s", path)

		err = takeGoroutineDump(tempDir, filename)
		assert.Error(t, err)
		assert.Contains(
			t,
			err.Error(),
			fmt.Sprintf("error creating file %s for goroutine dump", path),
		)
	})

	t.Run("succeeds", func(t *testing.T) {
		tempDir, dirCleanupFn := testutils.TempDir(t)
		defer dirCleanupFn()
		err := takeGoroutineDump(tempDir, "goroutine_dump")
		assert.NoError(t, err, "unexpected error when dumping goroutines")
	})
}
