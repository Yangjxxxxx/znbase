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
	"runtime/pprof"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/settings"
	"github.com/znbasedb/znbase/pkg/settings/cluster"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/timeutil"
)

const (
	goroutineDumpPrefix = "goroutine_dump"
	timeFormat          = "2006-01-02T15_04_05.999"
)

var (
	numGoroutinesThreshold = settings.RegisterIntSetting(
		"server.goroutine_dump.num_goroutines_threshold",
		"a threshold beyond which if number of goroutines increases, "+
			"then goroutine dump can be triggered",
		1000,
	)
	totalDumpSizeLimit = settings.RegisterByteSizeSetting(
		"server.goroutine_dump.total_dump_size_limit",
		"total size of goroutine dumps to be kept. "+
			"Dumps are GC'ed in the order of creation time. The latest dump is "+
			"always kept even if its size exceeds the limit.",
		500<<20, // 500MiB
	)
)

// heuristic represents whether goroutine dump is triggered. It is true when
// we think a goroutine dump is helpful in debugging OOM issues.
type heuristic struct {
	name   string
	isTrue func(s *GoroutineDumper) bool
}

var doubleSinceLastDumpHeuristic = heuristic{
	name: "double_since_last_dump",
	isTrue: func(gd *GoroutineDumper) bool {
		return gd.goroutines > gd.goroutinesThreshold &&
			gd.goroutines >= 2*gd.maxGoroutinesDumped
	},
}

// GoroutineDumper stores relevant functions and stats to take goroutine dumps
// if an abnormal change in number of goroutines is detected.
type GoroutineDumper struct {
	goroutines          int64
	goroutinesThreshold int64
	maxGoroutinesDumped int64
	heuristics          []heuristic
	currentTime         func() time.Time
	takeGoroutineDump   func(dir string, filename string) error
	gc                  func(ctx context.Context, dir string, sizeLimit int64)
	dir                 string
}

// MaybeDump takes a goroutine dump only when at least one heuristic in
// GoroutineDumper is true.
// At most one dump is taken in a call of this function.
func (gd *GoroutineDumper) MaybeDump(ctx context.Context, st *cluster.Settings, goroutines int64) {
	gd.goroutines = goroutines
	if gd.goroutinesThreshold != numGoroutinesThreshold.Get(&st.SV) {
		gd.goroutinesThreshold = numGoroutinesThreshold.Get(&st.SV)
		gd.maxGoroutinesDumped = 0
	}
	for _, h := range gd.heuristics {
		if h.isTrue(gd) {
			filename := fmt.Sprintf(
				"%s.%s.%s.%09d",
				goroutineDumpPrefix,
				gd.currentTime().Format(timeFormat),
				h.name,
				goroutines,
			)
			if err := gd.takeGoroutineDump(gd.dir, filename); err != nil {
				log.Errorf(ctx, "error dumping goroutines: %s", err)
				continue
			}
			gd.maxGoroutinesDumped = goroutines
			gd.gc(ctx, gd.dir, totalDumpSizeLimit.Get(&st.SV))
			break
		}
	}
}

// NewGoroutineDumper returns a GoroutineDumper which enables
// doubleSinceLastDumpHeuristic.
// dir is the directory in which dumps are stored.
func NewGoroutineDumper(dir string) (*GoroutineDumper, error) {
	if dir == "" {
		return nil, errors.New("directory to store dumps could not be determined")
	}
	dir = filepath.Join(dir, "goroutine_dump")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	gd := &GoroutineDumper{
		heuristics: []heuristic{
			doubleSinceLastDumpHeuristic,
		},
		goroutinesThreshold: 0,
		maxGoroutinesDumped: 0,
		currentTime:         timeutil.Now,
		takeGoroutineDump:   takeGoroutineDump,
		gc:                  gc,
		dir:                 dir,
	}
	return gd, nil
}

// gc removes oldest dumps when the total size of all dumps is larger
// than sizeLimit. Requires that the name of the dumps indicates dump time
// such that sorting the filenames corresponds to ordering the dumps
// from oldest to newest.
// Newest dump in the directory is not considered for GC.
func gc(ctx context.Context, dir string, sizeLimit int64) {
	// ReadDir returns a list of directory entries sorted by filename, which means
	// it is sorted by dump time.
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Errorf(ctx, "cannot read directory %s, err: %s", dir, err)
		return
	}

	var totalSize int64
	isLatestDump := true
	for i := len(files) - 1; i >= 0; i-- {
		f := files[i]
		path := filepath.Join(dir, f.Name())
		if strings.HasPrefix(f.Name(), goroutineDumpPrefix) {
			totalSize += f.Size()
			// Skipping the latest dump in gc
			if isLatestDump {
				isLatestDump = false
				continue
			}
			if totalSize > sizeLimit {
				if err := os.Remove(path); err != nil {
					log.Warningf(ctx, "Cannot remove dump file %s, err: %s", path, err)
				}
			}
		} else {
			log.Infof(ctx, "Removing unknown file %s in goroutine dump dir %s", f.Name(), dir)
			if err := os.Remove(path); err != nil {
				log.Warningf(ctx, "Cannot remove file %s, err: %s", path, err)
			}
		}
	}
}

func takeGoroutineDump(dir string, filename string) error {
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "error creating file %s for goroutine dump", path)
	}
	defer f.Close()
	if err = pprof.Lookup("goroutine").WriteTo(f, 2); err != nil {
		return errors.Wrapf(err, "error writing goroutine dump to %s", path)
	}
	return nil
}
