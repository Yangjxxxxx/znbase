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

package engine

import (
	"bytes"
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/znbasedb/znbase/pkg/util/leaktest"
	"github.com/znbasedb/znbase/pkg/util/stop"
)

func TestCreateTempDir(t *testing.T) {
	defer leaktest.AfterTest(t)()

	stopper := stop.NewStopper()
	defer stopper.Stop(context.TODO())
	// Temporary parent directory to test this.
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}()

	tempDir, err := CreateTempDir(dir, "test-create-temp", stopper)
	if err != nil {
		t.Fatal(err)
	}

	if dir != filepath.Dir(tempDir) {
		t.Fatalf("unexpected parent directory of temp subdirectory.\nexpected: %s\nactual: %s", dir, filepath.Dir(tempDir))
	}

	_, err = os.Stat(tempDir)
	if os.IsNotExist(err) {
		t.Fatalf("expected %s temp subdirectory to exist", tempDir)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordTempDir(t *testing.T) {
	defer leaktest.AfterTest(t)()
	recordFile := "foobar"

	f, err := ioutil.TempFile("", "record-file")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			t.Fatal(err)
		}
	}()

	// We should close this since RecordTempDir should open the file.
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}

	if err = RecordTempDir(f.Name(), recordFile); err != nil {
		t.Fatal(err)
	}

	actual, err := ioutil.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	expected := append([]byte(recordFile), '\n')
	if !bytes.Equal(expected, actual) {
		t.Fatalf("unexpected record file content after recording temp dir.\nexpected: %s\nactual: %s", expected, actual)
	}
}

func TestCleanupTempDirs(t *testing.T) {
	defer leaktest.AfterTest(t)()

	recordFile, err := ioutil.TempFile("", "record-file")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(recordFile.Name()); err != nil {
			t.Fatal(err)
		}
	}()

	// Generate some temporary directories.
	var tempDirs []string
	for i := 0; i < 5; i++ {
		tempDir, err := ioutil.TempDir("", "temp-dir")
		if err != nil {
			t.Fatal(err)
		}
		// Not strictly necessary, but good form to clean up temporary
		// directories independent of test case.
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Fatal(err)
			}
		}()
		tempDirs = append(tempDirs, tempDir)
		// Record the temporary directories to the file.
		if _, err = recordFile.Write(append([]byte(tempDir), '\n')); err != nil {
			t.Fatal(err)
		}
	}

	if err = recordFile.Close(); err != nil {
		t.Fatal(err)
	}

	// Generate some temporary files inside the temporary directories.
	var tempFiles []string
	content := []byte("whatisthemeaningoflife\n")
	for i := 0; i < 10; i++ {
		dir := tempDirs[rand.Intn(len(tempDirs))]
		tempFile, err := ioutil.TempFile(dir, "temp-file")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tempFile.Write(content); err != nil {
			t.Fatal(err)
		}
		if err = tempFile.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if err = CleanupTempDirs(recordFile.Name()); err != nil {
		t.Fatal(err)
	}

	// We check if all the temporary subdirectories and files were removed.
	for _, fname := range append(tempDirs, tempFiles...) {
		_, err = os.Stat(fname)
		if !os.IsNotExist(err) {
			t.Fatalf("file %s expected to be removed by cleanup", fname)
		}
		if err != nil {
			// We expect the files to not exist anymore.
			if os.IsNotExist(err) {
				continue
			}

			t.Fatal(err)
		}
	}
}
