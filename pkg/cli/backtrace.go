// Copyright 2016  The Cockroach Authors.
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

// +build linux freebsd

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/backtrace-labs/go-bcd"
	"github.com/znbasedb/znbase/pkg/build"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/stop"
	"golang.org/x/sys/unix"
)

// Currently disabled as backtrace appears to be obscuring problems when test
// clusters encounter panics. See #10872.
const backtraceEnabled = false

func initBacktrace(logDir string, options ...stop.Option) *stop.Stopper {
	if !backtraceEnabled {
		return stop.NewStopper(options...)
	}

	ctx := context.TODO()

	const ptracePath = "/opt/backtrace/bin/ptrace"
	if _, err := os.Stat(ptracePath); err != nil {
		log.Infof(ctx, "backtrace disabled: %s", err)
		return stop.NewStopper(options...)
	}

	if err := bcd.EnableTracing(); err != nil {
		log.Infof(ctx, "unable to enable backtrace: %s", err)
		return stop.NewStopper(options...)
	}

	bcd.UpdateConfig(bcd.GlobalConfig{
		PanicOnKillFailure: true,
		ResendSignal:       true,
		RateLimit:          time.Second * 3,
		SynchronousPut:     true,
	})

	// Use the default tracer implementation.
	// false: Exclude system goroutines.
	tracer := bcd.New(bcd.NewOptions{
		IncludeSystemGs: false,
	})
	if err := tracer.SetOutputPath(logDir, 0755); err != nil {
		log.Infof(ctx, "unable to set output path: %s", err)
		// Not a fatal error, continue.
	}

	// Enable WARNING log output from the tracer.
	tracer.AddOptions(nil, "-L", "WARNING")

	info := build.GetInfo()
	tracer.AddKV(nil, "cgo-compiler", info.CgoCompiler)
	tracer.AddKV(nil, "go-version", info.GoVersion)
	tracer.AddKV(nil, "platform", info.Platform)
	tracer.AddKV(nil, "type", info.Type)
	tracer.AddKV(nil, "tag", info.Tag)
	tracer.AddKV(nil, "time", info.Time)

	// Register for traces on signal reception.
	tracer.SetSigset(
		unix.SIGABRT,
		unix.SIGBUS,
		unix.SIGFPE,
		unix.SIGILL,
		unix.SIGSEGV,
	)
	bcd.Register(tracer)

	// Hook log.Fatal*.
	log.SetExitFunc(false /* hideStack */, func(code int) {
		_ = bcd.Trace(tracer, fmt.Errorf("exit %d", code), nil)
		os.Exit(code)
	})

	options = append(options,
		stop.OnPanic(func(val interface{}) {
			err, ok := val.(error)
			if !ok {
				err = fmt.Errorf("%v", val)
			}
			_ = bcd.Trace(tracer, err, nil)
			panic(val)
		}))

	stopper := stop.NewStopper(options...)

	// Internally, backtrace uses an external program (/opt/backtrace/bin/ptrace)
	// to generate traces. We direct the stdout for this program to a file for
	// debugging our usage of backtrace.
	if f, err := os.OpenFile(filepath.Join(logDir, "backtrace.out"),
		os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666); err != nil {
		log.Infof(ctx, "unable to open: %s", err)
	} else {
		stopper.AddCloser(stop.CloserFn(func() {
			f.Close()
		}))
		tracer.SetPipes(nil, f)
	}

	tracer.SetLogLevel(bcd.LogMax)
	log.Infof(ctx, "backtrace enabled")
	return stopper
}
