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

package sqlutils

import (
	"context"
	gosql "database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/testutils"
)

// SQLRunner wraps a testing.TB and *gosql.DB connection and provides
// convenience functions to run SQL statements and fail the test on any errors.
type SQLRunner struct {
	DB DBHandle
}

// DBHandle is an interface that applies to *gosql.DB, *gosql.Conn, and
// *gosql.Tx.
type DBHandle interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (gosql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*gosql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *gosql.Row
}

var _ DBHandle = &gosql.DB{}
var _ DBHandle = &gosql.Conn{}
var _ DBHandle = &gosql.Tx{}

// MakeSQLRunner returns a SQLRunner for the given database connection.
// The argument can be a *gosql.DB, *gosql.Conn, or *gosql.Tx object.
func MakeSQLRunner(db DBHandle) *SQLRunner {
	return &SQLRunner{DB: db}
}

// Exec is a wrapper around gosql.Exec that kills the test on error.
func (sr *SQLRunner) Exec(t testing.TB, query string, args ...interface{}) gosql.Result {
	t.Helper()
	r, err := sr.DB.ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("error executing '%s': %s", query, err)
	}
	return r
}

// ExecRowsAffected executes the statement and verifies that RowsAffected()
// matches the expected value. It kills the test on errors.
func (sr *SQLRunner) ExecRowsAffected(
	t testing.TB, expRowsAffected int, query string, args ...interface{},
) {
	t.Helper()
	r := sr.Exec(t, query, args...)
	numRows, err := r.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if numRows != int64(expRowsAffected) {
		t.Fatalf("expected %d affected rows, got %d on '%s'", expRowsAffected, numRows, query)
	}
}

// ExpectErr runs the given statement and verifies that it returns an error
// matching the given regex.
func (sr *SQLRunner) ExpectErr(t testing.TB, errRE string, query string, args ...interface{}) {
	t.Helper()
	_, err := sr.DB.ExecContext(context.Background(), query, args...)
	if !testutils.IsError(err, errRE) {
		t.Fatalf("expected error '%s', got: %v", errRE, err)
	}
}

// Query is a wrapper around gosql.Query that kills the test on error.
func (sr *SQLRunner) Query(t testing.TB, query string, args ...interface{}) *gosql.Rows {
	t.Helper()
	r, err := sr.DB.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("error executing '%s': %s", query, err)
	}
	return r
}

// Row is a wrapper around gosql.Row that kills the test on error.
type Row struct {
	testing.TB
	row *gosql.Row
}

// Scan is a wrapper around (*gosql.Row).Scan that kills the test on error.
func (r *Row) Scan(dest ...interface{}) {
	r.Helper()
	if err := r.row.Scan(dest...); err != nil {
		r.Fatalf("error scanning '%v': %+v", r.row, err)
	}
}

// QueryRow is a wrapper around gosql.QueryRow that kills the test on error.
func (sr *SQLRunner) QueryRow(t testing.TB, query string, args ...interface{}) *Row {
	t.Helper()
	return &Row{t, sr.DB.QueryRowContext(context.Background(), query, args...)}
}

// QueryStr runs a Query and converts the result using RowsToStrMatrix. Kills
// the test on errors.
func (sr *SQLRunner) QueryStr(t testing.TB, query string, args ...interface{}) [][]string {
	t.Helper()
	rows := sr.Query(t, query, args...)
	r, err := RowsToStrMatrix(rows)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// RowsToStrMatrix converts the given result rows to a string matrix; nulls are
// represented as "NULL". Empty results are represented by an empty (but
// non-nil) slice.
func RowsToStrMatrix(rows *gosql.Rows) ([][]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	vals := make([]interface{}, len(cols))
	for i := range vals {
		vals[i] = new(interface{})
	}
	res := [][]string{}
	for rows.Next() {
		if err := rows.Scan(vals...); err != nil {
			return nil, err
		}
		row := make([]string, len(vals))
		for j, v := range vals {
			if val := *v.(*interface{}); val != nil {
				switch t := val.(type) {
				case []byte:
					row[j] = string(t)
				default:
					row[j] = fmt.Sprint(val)
				}
			} else {
				row[j] = "NULL"
			}
		}
		res = append(res, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// MatrixToStr converts a set of rows into a single string where each row is on
// a separate line and the columns with a row are comma separated.
func MatrixToStr(rows [][]string) string {
	res := strings.Builder{}
	for _, row := range rows {
		res.WriteString(strings.Join(row, ", "))
		res.WriteRune('\n')
	}
	return res.String()
}

// CheckQueryResults checks that the rows returned by a query match the expected
// response.
func (sr *SQLRunner) CheckQueryResults(t testing.TB, query string, expected [][]string) {
	t.Helper()
	res := sr.QueryStr(t, query)
	if !reflect.DeepEqual(res, expected) {
		t.Errorf("query '%s': expected:\n%v\ngot:\n%v\n",
			query, MatrixToStr(expected), MatrixToStr(res),
		)
	}
}

// CheckQueryResultsRetry checks that the rows returned by a query match the
// expected response. If the results don't match right away, it will retry
// using testutils.SucceedsSoon.
func (sr *SQLRunner) CheckQueryResultsRetry(t testing.TB, query string, expected [][]string) {
	t.Helper()
	testutils.SucceedsSoon(t, func() error {
		res := sr.QueryStr(t, query)
		if !reflect.DeepEqual(res, expected) {
			return errors.Errorf("query '%s': expected:\n%v\ngot:\n%v\n",
				query, MatrixToStr(expected), MatrixToStr(res),
			)
		}
		return nil
	})
}
