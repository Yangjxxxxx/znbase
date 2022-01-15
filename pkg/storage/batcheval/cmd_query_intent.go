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

package batcheval

import (
	"context"

	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage/batcheval/result"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/spanset"
	"github.com/znbasedb/znbase/pkg/util"
	"github.com/znbasedb/znbase/pkg/util/hlc"
)

func init() {
	RegisterCommand(roachpb.QueryIntent, declareKeysQueryIntent, QueryIntent)
}

func declareKeysQueryIntent(
	_ *roachpb.RangeDescriptor,
	header roachpb.Header,
	req roachpb.Request,
	latchSpans, _ *spanset.SpanSet,
) {
	// QueryIntent requests read the specified keys at the maximum timestamp in
	// order to read any intent present, if one exists, regardless of the
	// timestamp it was written at.
	latchSpans.AddNonMVCC(spanset.SpanReadOnly, req.Header().Span())
}

// QueryIntent checks if an intent exists for the specified transaction at the
// given key. If the intent is missing, the request prevents the intent from
// ever being written at the specified timestamp (but the actual prevention
// happens during the timestamp cache update).
//
// QueryIntent returns an error if the intent is missing and its ErrorIfMissing
// field is set to true. This error is typically an IntentMissingError, but the
// request is special-cased to return a SERIALIZABLE retry error if a transaction
// queries its own intent and finds it has been pushed.
func QueryIntent(
	ctx context.Context, batch engine.ReadWriter, cArgs CommandArgs, resp roachpb.Response,
) (result.Result, error) {
	args := cArgs.Args.(*roachpb.QueryIntentRequest)
	h := cArgs.Header
	reply := resp.(*roachpb.QueryIntentResponse)

	// Read at the specified key at the maximum timestamp. This ensures that we
	// see an intent if one exists, regardless of what timestamp it is written
	// at.
	_, intent, err := engine.MVCCGet(ctx, batch, args.Key, hlc.MaxTimestamp, engine.MVCCGetOptions{
		// Perform an inconsistent read so that intents are returned instead of
		// causing WriteIntentErrors.
		Inconsistent: true,
		// Even if the request header contains a txn, perform the engine lookup
		// without a transaction so that intents for a matching transaction are
		// not returned as values (i.e. we don't want to see our own writes).
		Txn: nil,
	})
	if err != nil {
		return result.Result{}, err
	}

	// Determine if the request is querying an intent in its own transaction.
	ownTxn := h.Txn != nil && h.Txn.ID == args.Txn.ID

	var curIntentPushed bool
	if intent != nil {
		// See comment on QueryIntentRequest.Txn for an explanation of this
		// comparison.
		// TODO(nvanbenschoten): Now that we have a full intent history,
		// we can look at the exact sequence! That won't serve as much more
		// than an assertion that QueryIntent is being used correctly.
		reply.FoundIntent = (args.Txn.ID == intent.Txn.ID) &&
			(args.Txn.Epoch == intent.Txn.Epoch) &&
			(args.Txn.Sequence <= intent.Txn.Sequence)

		// If we found a matching intent, check whether the intent was pushed
		// past its expected timestamp.
		if reply.FoundIntent {
			// If the request is querying an intent for its own transaction, forward
			// the timestamp we compare against to the provisional commit timestamp
			// in the batch header.
			cmpTS := args.Txn.WriteTimestamp
			if ownTxn {
				cmpTS.Forward(h.Txn.WriteTimestamp)
			}
			if cmpTS.Less(intent.Txn.WriteTimestamp) {
				// The intent matched but was pushed to a later timestamp. Consider a
				// pushed intent a missing intent.
				curIntentPushed = true
				reply.FoundIntent = false

				// If the request was querying an intent in its own transaction, update
				// the response transaction.
				if ownTxn {
					reply.Txn = h.Txn.Clone()
					reply.Txn.WriteTimestamp.Forward(intent.Txn.WriteTimestamp)
				}
			}
		}
	}

	if !reply.FoundIntent && args.ErrorIfMissing {
		if ownTxn && curIntentPushed {
			if h.Txn.IsolationLevel == util.ReadCommittedIsolation {
				reply.FoundIntent = true
				return result.Result{}, nil
			}
			// If the transaction's own intent was pushed, go ahead and
			// return a TransactionRetryError immediately with an updated
			// transaction proto. This is an optimization that can help
			// the txn use refresh spans more effectively.
			return result.Result{}, roachpb.NewTransactionRetryError(roachpb.RETRY_SERIALIZABLE, "intent pushed")
		}
		return result.Result{}, roachpb.NewIntentMissingError(args.Key, intent)
	}
	return result.Result{}, nil
}
