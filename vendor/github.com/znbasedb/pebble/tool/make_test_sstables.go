// Copyright 2019 The LevelDB-Go and Pebble Authors. All rights reserved. Use
// of this source code is governed by a BSD-style license that can be found in
// the LICENSE file.

// +build make_test_sstables

// Run using: go run -tags make_test_sstables make_test_sstables.go
package main

import (
	"log"

	"github.com/znbasedb/pebble/internal/private"
	"github.com/znbasedb/pebble/sstable"
	"github.com/znbasedb/pebble/vfs"
)

func makeOutOfOrder() {
	fs := vfs.Default
	f, err := fs.Create("testdata/out-of-order.sst")
	if err != nil {
		log.Fatal(err)
	}
	w := sstable.NewWriter(f, sstable.WriterOptions{})
	private.SSTableWriterDisableKeyOrderChecks(w)

	set := func(key string) {
		if err := w.Set([]byte(key), nil); err != nil {
			log.Fatal(err)
		}
	}

	set("a")
	set("c")
	set("b")

	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	makeOutOfOrder()
}
