// Code generated by gen_cpp_keys.go; DO NOT EDIT.
// GENERATED FILE DO NOT EDIT

#pragma once

namespace znbase {

const rocksdb::Slice kLocalMax("\x02", 1);
const rocksdb::Slice kLocalRangeIDPrefix("\x01\x69", 2);
const rocksdb::Slice kLocalRangeIDReplicatedInfix("\x72", 1);
const rocksdb::Slice kLocalRangeAppliedStateSuffix("\x72\x61\x73\x6b", 4);
const rocksdb::Slice kMeta2KeyMax("\x03\xff\xff", 3);
const rocksdb::Slice kMinKey("", 0);
const rocksdb::Slice kMaxKey("\xff\xff", 2);

const std::vector<std::pair<rocksdb::Slice, rocksdb::Slice> > kSortedNoSplitSpans = {
  std::make_pair(rocksdb::Slice("\x88", 1), rocksdb::Slice("\x93", 1)),
  std::make_pair(rocksdb::Slice("\x04\x00\x6c\x69\x76\x65\x6e\x65\x73\x73\x2d", 11), rocksdb::Slice("\x04\x00\x6c\x69\x76\x65\x6e\x65\x73\x73\x2e", 11)),
  std::make_pair(rocksdb::Slice("\x03\xff\xff", 3), rocksdb::Slice("\x04", 1)),
  std::make_pair(rocksdb::Slice("", 0), rocksdb::Slice("\x03", 1)),
};

}  // namespace znbase
