// Copyright 2015  The Cockroach Authors.
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
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.

package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/znbasedb/znbase/pkg/storage/storagebase"
	"github.com/znbasedb/znbase/pkg/util/log"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
)

// init installs an adapter to use clog for log messages from raft which
// don't belong to any range.
func init() {
	raft.SetLogger(&raftLogger{ctx: context.Background()})
}

// *clogLogger implements the raft.Logger interface. Note that all methods
// must be defined on the pointer type rather than the value type because
// (at least in the go 1.4 compiler), methods on a value type called through
// an interface pointer go through an additional layer of indirection that
// appears on the stack, and would make all our stack frame offsets incorrect.
//
// Raft is fairly verbose at the "info" level, so we map "info" messages to
// clog.V(1) and "debug" messages to clog.V(2).
//
// This file is named raft.go instead of something like logger.go because this
// file's name is used to determine the vmodule parameter: --vmodule=raft=1
type raftLogger struct {
	ctx context.Context
}

func (r *raftLogger) Debug(v ...interface{}) {
	if log.V(3) {
		log.InfofDepth(r.ctx, 1, "", v...)
	}
}

func (r *raftLogger) Debugf(format string, v ...interface{}) {
	if log.V(3) {
		log.InfofDepth(r.ctx, 1, format, v...)
	}
}

func (r *raftLogger) Info(v ...interface{}) {
	if log.V(2) {
		log.InfofDepth(r.ctx, 1, "", v...)
	}
}

func (r *raftLogger) Infof(format string, v ...interface{}) {
	if log.V(2) {
		log.InfofDepth(r.ctx, 1, format, v...)
	}
}

func (r *raftLogger) Warning(v ...interface{}) {
	log.WarningfDepth(r.ctx, 1, "", v...)
}

func (r *raftLogger) Warningf(format string, v ...interface{}) {
	log.WarningfDepth(r.ctx, 1, format, v...)
}

func (r *raftLogger) Error(v ...interface{}) {
	log.ErrorfDepth(r.ctx, 1, "", v...)
}

func (r *raftLogger) Errorf(format string, v ...interface{}) {
	log.ErrorfDepth(r.ctx, 1, format, v...)
}

func (r *raftLogger) Fatal(v ...interface{}) {
	log.FatalfDepth(r.ctx, 1, "", v...)
}

func (r *raftLogger) Fatalf(format string, v ...interface{}) {
	log.FatalfDepth(r.ctx, 1, format, v...)
}

func (r *raftLogger) Panic(v ...interface{}) {
	s := fmt.Sprint(v...)
	log.ErrorfDepth(r.ctx, 1, s)
	panic(s)
}

func (r *raftLogger) Panicf(format string, v ...interface{}) {
	log.ErrorfDepth(r.ctx, 1, format, v...)
	panic(fmt.Sprintf(format, v...))
}

func verboseRaftLoggingEnabled() bool {
	return log.V(5)
}

func logRaftReady(ctx context.Context, ready raft.Ready) {
	if !verboseRaftLoggingEnabled() {
		return
	}

	var buf bytes.Buffer
	if ready.SoftState != nil {
		fmt.Fprintf(&buf, "  SoftState updated: %+v\n", *ready.SoftState)
	}
	if !raft.IsEmptyHardState(ready.HardState) {
		fmt.Fprintf(&buf, "  HardState updated: %+v\n", ready.HardState)
	}
	for i, e := range ready.Entries {
		fmt.Fprintf(&buf, "  New Entry[%d]: %.200s\n",
			i, raft.DescribeEntry(e, raftEntryFormatter))
	}
	for i, e := range ready.CommittedEntries {
		fmt.Fprintf(&buf, "  Committed Entry[%d]: %.200s\n",
			i, raft.DescribeEntry(e, raftEntryFormatter))
	}
	if !raft.IsEmptySnap(ready.Snapshot) {
		snap := ready.Snapshot
		snap.Data = nil
		fmt.Fprintf(&buf, "  Snapshot updated: %v\n", snap)
	}
	for i, m := range ready.Messages {
		fmt.Fprintf(&buf, "  Outgoing Message[%d]: %.200s\n",
			i, raftDescribeMessage(m, raftEntryFormatter))
	}
	log.Infof(ctx, "raft ready (must-sync=%t)\n%s", ready.MustSync, buf.String())
}

// This is a fork of raft.DescribeMessage with a tweak to avoid logging
// snapshot data.
func raftDescribeMessage(m raftpb.Message, f raft.EntryFormatter) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%x->%x %v Term:%d Log:%d/%d", m.From, m.To, m.Type, m.Term, m.LogTerm, m.Index)
	if m.Reject {
		fmt.Fprintf(&buf, " Rejected (Hint: %d)", m.RejectHint)
	}
	if m.Commit != 0 {
		fmt.Fprintf(&buf, " Commit:%d", m.Commit)
	}
	if len(m.Entries) > 0 {
		fmt.Fprintf(&buf, " Entries:[")
		for i, e := range m.Entries {
			if i != 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(raft.DescribeEntry(e, f))
		}
		fmt.Fprintf(&buf, "]")
	}
	if !raft.IsEmptySnap(m.Snapshot) {
		snap := m.Snapshot
		snap.Data = nil
		fmt.Fprintf(&buf, " Snapshot:%v", snap)
	}
	return buf.String()
}

func raftEntryFormatter(data []byte) string {
	if len(data) == 0 {
		return "[empty]"
	}
	commandID, _ := DecodeRaftCommand(data)
	return fmt.Sprintf("[%x] [%d]", commandID, len(data))
}

// IsPreemptive returns whether this is a preemptive snapshot or a Raft
// snapshot.
func (h *SnapshotRequest_Header) IsPreemptive() bool {
	// Preemptive snapshots are addressed to replica ID 0. No other requests to
	// replica ID 0 are allowed.
	return h.RaftMessageRequest.ToReplica.ReplicaID == 0
}

// traceEntries records the provided event for all proposals corresponding
// to the entries contained in ents. The vmodule level for raft must be at
// least 1.
func (r *Replica) traceEntries(ents []raftpb.Entry, event string) {
	if log.V(1) {
		ids := extractIDs(nil, ents)
		traceProposals(r, ids, event)
	}
}

// traceMessageSends records the provided event for all proposals contained in
// in entries contained in msgs. The vmodule level for raft must be at
// least 1.
func (r *Replica) traceMessageSends(msgs []raftpb.Message, event string) {
	if log.V(1) {
		var ids []storagebase.CmdIDKey
		for _, m := range msgs {
			ids = extractIDs(ids, m.Entries)
		}
		traceProposals(r, ids, event)
	}
}

// extractIDs decodes and appends each of the ids corresponding to the entries
// in ents to ids and returns the result.
func extractIDs(ids []storagebase.CmdIDKey, ents []raftpb.Entry) []storagebase.CmdIDKey {
	for _, e := range ents {
		if e.Type == raftpb.EntryNormal && len(e.Data) > 0 {
			id, _ := DecodeRaftCommand(e.Data)
			ids = append(ids, id)
		}
	}
	return ids
}

// traceLocalProposals logs a trace event with the provided string for each
// locally proposed command which corresponds to an id in ids.
func traceProposals(r *Replica, ids []storagebase.CmdIDKey, event string) {
	ctxs := make([]context.Context, 0, len(ids))
	r.mu.RLock()
	for _, id := range ids {
		if prop, ok := r.mu.proposals[id]; ok {
			ctxs = append(ctxs, prop.ctx)
		}
	}
	r.mu.RUnlock()
	for _, ctx := range ctxs {
		log.Event(ctx, event)
	}
}
