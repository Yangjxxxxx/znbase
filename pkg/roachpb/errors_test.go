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

package roachpb

import (
	"strings"
	"testing"

	"github.com/znbasedb/znbase/pkg/util/hlc"
)

type testError struct{}

func (t *testError) Error() string              { return "test" }
func (t *testError) message(pErr *Error) string { return "test" }

// TestNewError verifies that a test error that
// implements retryable or indexed is converted properly into a generic error.
func TestNewError(t *testing.T) {
	pErr := NewError(&testError{})
	if pErr.GoError().Error() != "test" {
		t.Errorf("unexpected error: %s", pErr)
	}
}

// TestNewErrorNil verifies that a nil error can be set
// and retrieved from a response header.
func TestNewErrorNil(t *testing.T) {
	pErr := NewError(nil)
	if pErr != nil {
		t.Errorf("expected nil error; got %s", pErr)
	}
}

// TestSetTxn vefifies that SetTxn updates the error message.
func TestSetTxn(t *testing.T) {
	e := NewError(NewTransactionAbortedError(ABORT_REASON_ABORTED_RECORD_FOUND))
	txn := MakeTransaction("test", Key("a"), 1, hlc.Timestamp{}, 0)
	e.SetTxn(&txn)
	if !strings.HasPrefix(
		e.Message, "TransactionAbortedError(ABORT_REASON_ABORTED_RECORD_FOUND): \"test\"") {
		t.Errorf("unexpected message: %s", e.Message)
	}
}

func TestErrorTxn(t *testing.T) {
	var pErr *Error
	if txn := pErr.GetTxn(); txn != nil {
		t.Fatalf("wanted nil, unexpected: %+v", txn)
	}
	pErr = &Error{}
	const name = "X"
	pErr.SetTxn(&Transaction{Name: name})
	if txn := pErr.GetTxn(); txn == nil || txn.Name != name {
		t.Fatalf("wanted name %s, unexpected: %+v", name, txn)
	}
}

func TestReadWithinUncertaintyIntervalError(t *testing.T) {
	{
		rwueNew := NewReadWithinUncertaintyIntervalError(
			hlc.Timestamp{WallTime: 1}, hlc.Timestamp{WallTime: 2},
			&Transaction{
				MaxTimestamp:       hlc.Timestamp{WallTime: 3},
				ObservedTimestamps: []ObservedTimestamp{{NodeID: 12, Timestamp: hlc.Timestamp{WallTime: 4}}},
			})
		expNew := "ReadWithinUncertaintyIntervalError: read at time 0.000000001,0 encountered " +
			"previous write with future timestamp 0.000000002,0 within uncertainty interval " +
			"`t <= 0.000000003,0`; observed timestamps: [{12 0.000000004,0}]"
		if a := rwueNew.Error(); a != expNew {
			t.Fatalf("expected: %s\ngot: %s", a, expNew)
		}
	}

	{
		rwueOld := NewReadWithinUncertaintyIntervalError(
			hlc.Timestamp{WallTime: 1}, hlc.Timestamp{WallTime: 2}, nil)

		expOld := "ReadWithinUncertaintyIntervalError: read at time 0.000000001,0 encountered " +
			"previous write with future timestamp 0.000000002,0 within uncertainty interval " +
			"`t <= <nil>`; observed timestamps: []"
		if a := rwueOld.Error(); a != expOld {
			t.Fatalf("expected: %s\ngot: %s", a, expOld)
		}
	}
}
