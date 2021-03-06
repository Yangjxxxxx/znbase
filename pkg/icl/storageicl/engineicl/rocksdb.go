// Copyright 2017 The Cockroach Authors.
//
// Licensed as a Cockroach Enterprise file under the ZNBase Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/znbasedb/znbase/blob/master/licenses/ICL.txt

package engineicl

import (
	"unsafe"

	"github.com/pkg/errors"
	"github.com/znbasedb/znbase/pkg/roachpb"
	"github.com/znbasedb/znbase/pkg/storage/engine"
	"github.com/znbasedb/znbase/pkg/storage/engine/enginepb"
)

// TODO(tamird): why does rocksdb not link jemalloc,snappy statically?

// #cgo CPPFLAGS: -I../../../../c-deps/libroach/include
// #cgo LDFLAGS: -lroachicl
// #cgo LDFLAGS: -lroach
// #cgo LDFLAGS: -lprotobuf
// #cgo LDFLAGS: -lrocksdb
// #cgo LDFLAGS: -lsnappy
// #cgo LDFLAGS: -lcryptopp
// #cgo linux LDFLAGS: -lrt -lpthread
// #cgo windows LDFLAGS: -lshlwapi -lrpcrt4
//
// #include <stdlib.h>
// #include <libroachicl.h>
import "C"

func init() {
	engine.SetRocksDBOpenHook(C.DBOpenHookICL)
}

// VerifyBatchRepr asserts that all keys in a BatchRepr are between the specified
// start and end keys and computes the enginepb.MVCCStats for it.
func VerifyBatchRepr(
	repr []byte, start, end engine.MVCCKey, nowNanos int64,
) (enginepb.MVCCStats, error) {
	// We store a 4 byte checksum of each key/value entry in the value. Make
	// sure the all ones in this BatchRepr validate.
	//
	// TODO(dan): After a number of hours of trying, I was unable to get a
	// performant c++ implementation of crc32 ieee, so this verifies the
	// checksums in go and constructs the MVCCStats in c++. It'd be nice to move
	// one or the other.
	{
		r, err := engine.NewRocksDBBatchReader(repr)
		if err != nil {
			return enginepb.MVCCStats{}, errors.Wrapf(err, "verifying key/value checksums")
		}
		for r.Next() {
			switch r.BatchType() {
			case engine.BatchTypeValue:
				mvccKey, err := r.MVCCKey()
				if err != nil {
					return enginepb.MVCCStats{}, errors.Wrapf(err, "verifying key/value checksums")
				}
				v := roachpb.Value{RawBytes: r.Value()}
				if err := v.Verify(mvccKey.Key); err != nil {
					return enginepb.MVCCStats{}, err
				}
			default:
				return enginepb.MVCCStats{}, errors.Errorf(
					"unexpected entry type in batch: %d", r.BatchType())
			}
		}
		if err := r.Error(); err != nil {
			return enginepb.MVCCStats{}, errors.Wrapf(err, "verifying key/value checksums")
		}
	}

	var stats C.MVCCStatsResult
	if err := statusToError(C.DBBatchReprVerify(
		goToCSlice(repr), goToCKey(start), goToCKey(end), C.int64_t(nowNanos), &stats,
	)); err != nil {
		return enginepb.MVCCStats{}, err
	}
	return cStatsToGoStats(stats, nowNanos)
}

// TODO(dan): The following are all duplicated from storage/engine/rocksdb.go,
// but if you export the ones there and reuse them here, it doesn't work.
//
// `cannot use engine.GoToCSlice(repr) (type engine.C.struct___0) as type C.struct___0 in argument to _Cfunc_DBBatchReprVerify`
//
// The issue is tracked upstream at https://github.com/golang/go/issues/13467.
// At worst, we could split these out into a separate file in storage/engine and
// use `go generate` to copy it here so they stay in sync.

// goToCSlice converts a go byte slice to a DBSlice. Note that this is
// potentially dangerous as the DBSlice holds a reference to the go
// byte slice memory that the Go GC does not know about. This method
// is only intended for use in converting arguments to C
// functions. The C function must copy any data that it wishes to
// retain once the function returns.
func goToCSlice(b []byte) C.DBSlice {
	if len(b) == 0 {
		return C.DBSlice{data: nil, len: 0}
	}
	return C.DBSlice{
		data: (*C.char)(unsafe.Pointer(&b[0])),
		len:  C.int(len(b)),
	}
}

func goToCKey(key engine.MVCCKey) C.DBKey {
	return C.DBKey{
		key:       goToCSlice(key.Key),
		wall_time: C.int64_t(key.Timestamp.WallTime),
		logical:   C.int32_t(key.Timestamp.Logical),
	}
}

func cStringToGoString(s C.DBString) string {
	if s.data == nil {
		return ""
	}
	result := C.GoStringN(s.data, s.len)
	C.free(unsafe.Pointer(s.data))
	return result
}

func statusToError(s C.DBStatus) error {
	if s.data == nil {
		return nil
	}
	return errors.New(cStringToGoString(s))
}

func cStatsToGoStats(stats C.MVCCStatsResult, nowNanos int64) (enginepb.MVCCStats, error) {
	ms := enginepb.MVCCStats{}
	if err := statusToError(stats.status); err != nil {
		return ms, err
	}
	ms.ContainsEstimates = false
	ms.LiveBytes = int64(stats.live_bytes)
	ms.KeyBytes = int64(stats.key_bytes)
	ms.ValBytes = int64(stats.val_bytes)
	ms.IntentBytes = int64(stats.intent_bytes)
	ms.LiveCount = int64(stats.live_count)
	ms.KeyCount = int64(stats.key_count)
	ms.ValCount = int64(stats.val_count)
	ms.IntentCount = int64(stats.intent_count)
	ms.IntentAge = int64(stats.intent_age)
	ms.GCBytesAge = int64(stats.gc_bytes_age)
	ms.SysBytes = int64(stats.sys_bytes)
	ms.SysCount = int64(stats.sys_count)
	ms.LastUpdateNanos = nowNanos
	return ms, nil
}
