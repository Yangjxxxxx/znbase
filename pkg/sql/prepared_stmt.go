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

package sql

import (
	"context"
	"unsafe"

	"github.com/znbasedb/znbase/pkg/sql/opt/memo"
	"github.com/znbasedb/znbase/pkg/sql/pgwire/pgwirebase"
	"github.com/znbasedb/znbase/pkg/sql/sem/tree"
	"github.com/znbasedb/znbase/pkg/sql/sqlbase"
	"github.com/znbasedb/znbase/pkg/util/log"
	"github.com/znbasedb/znbase/pkg/util/mon"
)

// PreparedStatement is a SQL statement that has been parsed and the types
// of arguments and results have been determined.
//
// Note that PreparedStatemts maintain a reference counter internally.
// References need to be registered with incRef() and de-registered with
// decRef().
type PreparedStatement struct {
	sqlbase.PrepareMetadata

	// Memo is the memoized data structure constructed by the cost-based optimizer
	// during prepare of a SQL statement. It can significantly speed up execution
	// if it is used by the optimizer as a starting point.
	Memo *memo.Memo

	// IsCorrelated memoizes whether the query contained correlated
	// subqueries during planning (prior to de-correlation).
	IsCorrelated bool

	// refCount keeps track of the number of references to this PreparedStatement.
	// New references are registered through incRef().
	// Once refCount hits 0 (through calls to decRef()), the following memAcc is
	// closed.
	// Most references are being held by portals created from this prepared
	// statement.
	refCount int
	memAcc   mon.BoundAccount
}

// MemoryEstimate returns a rough estimate of the PreparedStatement's memory
// usage, in bytes.
func (p *PreparedStatement) MemoryEstimate() int64 {
	// Account for the memory used by this prepared statement:
	//   1. Size of the prepare metadata.
	//   2. Size of the prepared memo, if using the cost-based optimizer.
	size := p.PrepareMetadata.MemoryEstimate()
	if p.Memo != nil {
		size += p.Memo.MemoryEstimate()
	}
	return size
}

func (p *PreparedStatement) decRef(ctx context.Context) {
	if p.refCount <= 0 {
		log.Fatal(ctx, "corrupt PreparedStatement refcount")
	}
	p.refCount--
	if p.refCount == 0 {
		p.memAcc.Close(ctx)
	}
}

func (p *PreparedStatement) incRef(ctx context.Context) {
	if p.refCount <= 0 {
		log.Fatal(ctx, "corrupt PreparedStatement refcount")
	}
	p.refCount++
}

// preparedStatementsAccessor gives a planner access to a session's collection
// of prepared statements.
type preparedStatementsAccessor interface {
	// Get returns the prepared statement with the given name. The returned bool
	// is false if a statement with the given name doesn't exist.
	Get(name string) (*PreparedStatement, bool)
	// Delete removes the PreparedStatement with the provided name from the
	// collection. If a portal exists for that statement, it is also removed.
	// The method returns true if statement with that name was found and removed,
	// false otherwise.
	Delete(ctx context.Context, name string) bool
	// DeleteAll removes all prepared statements and portals from the coolection.
	DeleteAll(ctx context.Context)
}

// PreparedPortal is a PreparedStatement that has been bound with query arguments.
//
// Note that PreparedPortals maintain a reference counter internally.
// References need to be registered with incRef() and de-registered with
// decRef().
//
// TODO(andrei): In Postgres, portals can only be used to "execute a query" once
// (but they allow one to move back and forth through the results). Our portals
// can be used to execute a query multiple times, which is a bug (executing an
// exhausted portal in Postres returns 0 results; in ZNBase executing a portal a
// second time always restarts the query).
type PreparedPortal struct {
	Stmt  *PreparedStatement
	Qargs tree.QueryArguments

	// OutFormats contains the requested formats for the output columns.
	OutFormats []pgwirebase.FormatCode

	// refCount keeps track of the number of references to this PreparedStatement.
	// New references are registered through incRef().
	// Once refCount hits 0 (through calls to decRef()), the following memAcc is
	// closed.
	// Most references are being held by portals created from this prepared
	// statement.
	refCount int

	memAcc mon.BoundAccount
}

// newPreparedPortal creates a new PreparedPortal.
//
// incRef() doesn't need to be called on the result.
// When no longer in use, the PreparedPortal needs to be decRef()d.
func (ex *connExecutor) newPreparedPortal(
	ctx context.Context,
	name string,
	stmt *PreparedStatement,
	qargs tree.QueryArguments,
	outFormats []pgwirebase.FormatCode,
) (*PreparedPortal, error) {
	portal := &PreparedPortal{
		Stmt:       stmt,
		Qargs:      qargs,
		OutFormats: outFormats,
		memAcc:     ex.sessionMon.MakeBoundAccount(),
		refCount:   1,
	}
	sz := int64(uintptr(len(name)) + unsafe.Sizeof(*portal))
	if err := portal.memAcc.Grow(ctx, sz); err != nil {
		return nil, err
	}
	// The portal keeps a reference to the PreparedStatement, so register it.
	stmt.incRef(ctx)
	return portal, nil
}

func (p *PreparedPortal) incRef(ctx context.Context) {
	if p.refCount <= 0 {
		log.Fatal(ctx, "corrupt PreparedStatement refcount")
	}
	p.refCount++
}

func (p *PreparedPortal) decRef(ctx context.Context) {
	if p.refCount <= 0 {
		log.Fatal(ctx, "corrupt PreparedPrepared refcount")
	}
	p.refCount--

	if p.refCount == 0 {
		p.memAcc.Close(ctx)
		p.Stmt.decRef(ctx)
	}
}
