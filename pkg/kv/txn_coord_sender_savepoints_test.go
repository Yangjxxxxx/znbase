package kv

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/znbasedb/znbase/pkg/base"
	"github.com/znbasedb/znbase/pkg/internal/client"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage"
	"github.com/znbasedb/znbase/pkg/storage/storagebase"
	"github.com/znbasedb/znbase/pkg/testutils/datadriven"
	"github.com/znbasedb/znbase/pkg/testutils/serverutils"
	"github.com/znbasedb/znbase/pkg/util/leaktest"
)

func TestSavepoints(t *testing.T) {
	defer leaktest.AfterTest(t)()

	ctx := context.Background()
	abortKey := roachpb.Key("abort")
	errKey := roachpb.Key("injectErr")

	datadriven.Walk(t, "testdata/savepoints", func(t *testing.T, path string) {
		// We want to inject txn abort errors in some cases.
		//
		// We do this by injecting the error from "underneath" the
		// TxnCoordSender, from storage.
		params := base.TestServerArgs{}
		var doAbort int64
		params.Knobs.Store = &storage.StoreTestingKnobs{
			EvalKnobs: storagebase.BatchEvalTestingKnobs{
				TestingEvalFilter: func(args storagebase.FilterArgs) *roachpb.Error {
					key := args.Req.Header().Key
					if atomic.LoadInt64(&doAbort) != 0 && key.Equal(abortKey) {
						return roachpb.NewErrorWithTxn(
							roachpb.NewTransactionAbortedError(roachpb.ABORT_REASON_UNKNOWN), args.Hdr.Txn)
					}
					if key.Equal(errKey) {
						return roachpb.NewErrorf("injected error")
					}
					return nil
				},
			},
		}

		// New database for each test file.
		s, _, db := serverutils.StartServer(t, params)
		defer s.Stopper().Stop(ctx)

		// Transient state during the test.
		sp := make(map[string]client.SavepointToken)
		var txn *client.Txn

		datadriven.RunTest(t, path, func(td *datadriven.TestData) string {
			var buf strings.Builder

			ptxn := func() {
				tc := txn.Sender().(*TxnCoordSender)
				fmt.Fprintf(&buf, "%d ", tc.interceptorAlloc.txnSeqNumAllocator.writeSeq)
				if len(tc.mu.txn.IgnoredSeqNums) == 0 {
					buf.WriteString("<noignore>")
				}
				for _, r := range tc.mu.txn.IgnoredSeqNums {
					fmt.Fprintf(&buf, "[%d-%d]", r.Start, r.End)
				}
				fmt.Fprintln(&buf)
			}

			switch td.Cmd {
			case "begin":
				txn = client.NewTxn(ctx, db, 0, client.RootTxn)
				ptxn()

			case "commit":
				if err := txn.CommitOrCleanup(ctx); err != nil {
					fmt.Fprintf(&buf, "(%T) %v\n", err, err)
				}

			case "retry":
				epochBefore := txn.Epoch()
				retryErr := txn.GenerateForcedRetryableError(ctx, "forced retry")
				epochAfter := txn.Epoch()
				fmt.Fprintf(&buf, "synthetic error: %v\n", retryErr)
				fmt.Fprintf(&buf, "epoch: %d -> %d\n", epochBefore, epochAfter)

			// inject-error runs a Get with an untyped error injected into request
			// evaluation.
			case "inject-error":
				_, err := txn.Get(ctx, errKey)
				require.Regexp(t, "injected error", err.Error())
				fmt.Fprint(&buf, "injected error\n")

			case "abort":
				prevID := txn.ID()
				atomic.StoreInt64(&doAbort, 1)
				defer func() { atomic.StoreInt64(&doAbort, 00) }()
				err := txn.Put(ctx, abortKey, []byte("value"))
				fmt.Fprintf(&buf, "(%T)\n", err)
				changed := "changed"
				if prevID == txn.ID() {
					changed = "not changed"
				}
				fmt.Fprintf(&buf, "txn id %s\n", changed)

			case "put":
				if err := txn.Put(ctx,
					roachpb.Key(td.CmdArgs[0].Key),
					[]byte(td.CmdArgs[1].Key)); err != nil {
					fmt.Fprintf(&buf, "(%T) %v\n", err, err)
				}

			// cput takes <key> <value> <expected value>. The expected value can be
			// "nil".
			case "cput":
				expS := td.CmdArgs[2].Key
				var expVal *roachpb.Value
				if expS != "nil" {
					val := roachpb.MakeValueFromBytes([]byte(expS))
					expVal = &val
				}
				if err := txn.CPut(ctx,
					roachpb.Key(td.CmdArgs[0].Key),
					[]byte(td.CmdArgs[1].Key),
					expVal,
				); err != nil {
					if _, ok := err.(*roachpb.ConditionFailedError); ok {
						// Print an easier to match message.
						fmt.Fprintf(&buf, "(%T) unexpected value\n", err)
					} else {
						fmt.Fprintf(&buf, "(%T) %v\n", err, err)
					}
				}

			case "get":
				v, err := txn.Get(ctx, td.CmdArgs[0].Key)
				if err != nil {
					fmt.Fprintf(&buf, "(%T) %v\n", err, err)
				} else {
					ba, _ := v.Value.GetBytes()
					fmt.Fprintf(&buf, "%v -> %v\n", v.Key, string(ba))
				}

			case "savepoint":
				spt, err := txn.CreateSavepoint(ctx)
				if err != nil {
					fmt.Fprintf(&buf, "(%T) %v\n", err, err)
				} else {
					sp[td.CmdArgs[0].Key] = spt
					ptxn()
				}

			case "release":
				spn := td.CmdArgs[0].Key
				spt := sp[spn]
				if err := txn.ReleaseSavepoint(ctx, spt); err != nil {
					fmt.Fprintf(&buf, "(%T) %v\n", err, err)
				} else {
					ptxn()
				}

			case "rollback":
				spn := td.CmdArgs[0].Key
				spt := sp[spn]
				if err := txn.RollbackToSavepoint(ctx, spt); err != nil {
					fmt.Fprintf(&buf, "(%T) %v\n", err, err)
				} else {
					ptxn()
				}
			case "subtest":
			default:
				td.Fatalf(t, "unknown directive: %s", td.Cmd)
			}
			return buf.String()
		})
	})
}
