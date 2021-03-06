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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"fmt"

	"github.com/znbasedb/znbase/pkg/keys"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
	"github.com/znbasedb/znbase/pkg/util/bufalloc"
	"github.com/znbasedb/znbase/pkg/util/hlc"
)

// MVICLogicalOpType is an enum with values corresponding to each of the
// enginepb.MVICLogicalOp variants.
//
// LogLogicalOp takes an MVICLogicalOpType and a corresponding
// MVICLogicalOpDetails instead of an enginepb.MVICLogicalOp variant for two
// reasons. First, it serves as a form of abstraction so that callers of the
// method don't need to construct protos themselves. More importantly, it also
// avoids allocations in the common case where Writer.LogLogicalOp is a no-op.
// This makes LogLogicalOp essentially free for cases where logical op logging
// is disabled.
type MVICLogicalOpType int

const (
	// MVCCWriteValueOpType corresponds to the MVCCWriteValueOp variant.
	MVCCWriteValueOpType MVICLogicalOpType = iota
	// MVCCWriteIntentOpType corresponds to the MVCCWriteIntentOp variant.
	MVCCWriteIntentOpType
	// MVCCUpdateIntentOpType corresponds to the MVCCUpdateIntentOp variant.
	MVCCUpdateIntentOpType
	// MVCCCommitIntentOpType corresponds to the MVCCCommitIntentOp variant.
	MVCCCommitIntentOpType
	// MVCCAbortIntentOpType corresponds to the MVCCAbortIntentOp variant.
	MVCCAbortIntentOpType
)

// MVICLogicalOpDetails contains details about the occurrence of an MVCC logical
// operation.
type MVICLogicalOpDetails struct {
	Txn       enginepb.TxnMeta
	Key       roachpb.Key
	Timestamp hlc.Timestamp

	// Safe indicates that the values in this struct will never be invalidated
	// at a later point. If the details object cannot promise that its values
	// will never be invalidated, an OpLoggerBatch will make a copy of all
	// references before adding it to the log. TestMVCCOpLogWriter fails without
	// this.
	Safe     bool
	BackFill bool
}

// OpLoggerBatch records a log of logical MVCC operations.
type OpLoggerBatch struct {
	Batch
	distinct     DistinctOpLoggerBatch
	distinctOpen bool

	ops      []enginepb.MVICLogicalOp
	opsAlloc bufalloc.ByteAllocator
	BackFill bool
}

// NewOpLoggerBatch creates a new batch that logs logical mvcc operations and
// wraps the provided batch.
func NewOpLoggerBatch(b Batch) *OpLoggerBatch {
	ol := &OpLoggerBatch{Batch: b}
	ol.distinct.parent = ol
	return ol
}

var _ Batch = &OpLoggerBatch{}

// LogLogicalOp implements the Writer interface.
func (ol *OpLoggerBatch) LogLogicalOp(op MVICLogicalOpType, details MVICLogicalOpDetails) {
	if ol.distinctOpen {
		panic("distinct batch already open")
	}
	ol.logLogicalOp(op, details)
	ol.Batch.LogLogicalOp(op, details)
}

func (ol *OpLoggerBatch) logLogicalOp(op MVICLogicalOpType, details MVICLogicalOpDetails) {
	if keys.IsLocal(details.Key) {
		// Ignore mvcc operations on local keys.
		return
	}

	switch op {
	case MVCCWriteValueOpType:
		if !details.Safe {
			ol.opsAlloc, details.Key = ol.opsAlloc.Copy(details.Key, 0)
		}

		ol.recordOp(&enginepb.MVCCWriteValueOp{
			Key:       details.Key,
			Timestamp: details.Timestamp,
			BackFill:  details.BackFill,
			TxnID:     details.Txn.ID,
		})
	case MVCCWriteIntentOpType:
		if !details.Safe {
			ol.opsAlloc, details.Txn.Key = ol.opsAlloc.Copy(details.Txn.Key, 0)
		}

		ol.recordOp(&enginepb.MVCCWriteIntentOp{
			TxnID:     details.Txn.ID,
			TxnKey:    details.Txn.Key,
			Timestamp: details.Timestamp,
			BackFill:  details.BackFill,
		})
	case MVCCUpdateIntentOpType:
		ol.recordOp(&enginepb.MVCCUpdateIntentOp{
			TxnID:     details.Txn.ID,
			Timestamp: details.Timestamp,
		})
	case MVCCCommitIntentOpType:
		if !details.Safe {
			ol.opsAlloc, details.Key = ol.opsAlloc.Copy(details.Key, 0)
		}
		ol.recordOp(&enginepb.MVCCCommitIntentOp{
			TxnID:     details.Txn.ID,
			Key:       details.Key,
			Timestamp: details.Timestamp,
			BackFill:  details.BackFill,
		})
	case MVCCAbortIntentOpType:
		ol.recordOp(&enginepb.MVCCAbortIntentOp{
			TxnID: details.Txn.ID,
		})
	default:
		panic(fmt.Sprintf("unexpected op type %v", op))
	}
}

func (ol *OpLoggerBatch) recordOp(op interface{}) {
	ol.ops = append(ol.ops, enginepb.MVICLogicalOp{})
	ol.ops[len(ol.ops)-1].MustSetValue(op)
}

// LogicalOps returns the list of all logical MVCC operations that have been
// recorded by the logger.
func (ol *OpLoggerBatch) LogicalOps() []enginepb.MVICLogicalOp {
	if ol == nil {
		return nil
	}
	return ol.ops
}

// Distinct implements the Batch interface.
func (ol *OpLoggerBatch) Distinct() ReadWriter {
	if ol.distinctOpen {
		panic("distinct batch already open")
	}
	ol.distinctOpen = true
	ol.distinct.ReadWriter = ol.Batch.Distinct()
	return &ol.distinct
}

//DistinctOpLoggerBatch distinct OpLoggerBatch
type DistinctOpLoggerBatch struct {
	ReadWriter
	parent *OpLoggerBatch
}

// LogLogicalOp implements the Writer interface.
func (dlw *DistinctOpLoggerBatch) LogLogicalOp(op MVICLogicalOpType, details MVICLogicalOpDetails) {
	dlw.parent.logLogicalOp(op, details)
	dlw.ReadWriter.LogLogicalOp(op, details)
}

// Close implements the Reader interface.
func (dlw *DistinctOpLoggerBatch) Close() {
	if !dlw.parent.distinctOpen {
		panic("distinct batch not open")
	}
	dlw.parent.distinctOpen = false
	dlw.ReadWriter.Close()
}
