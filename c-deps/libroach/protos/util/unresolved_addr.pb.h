// Generated by the protocol buffer compiler.  DO NOT EDIT!
// source: util/unresolved_addr.proto

#ifndef PROTOBUF_INCLUDED_util_2funresolved_5faddr_2eproto
#define PROTOBUF_INCLUDED_util_2funresolved_5faddr_2eproto

#include <string>

#include <google/protobuf/stubs/common.h>

#if GOOGLE_PROTOBUF_VERSION < 3006000
#error This file was generated by a newer version of protoc which is
#error incompatible with your Protocol Buffer headers.  Please update
#error your headers.
#endif
#if 3006000 < GOOGLE_PROTOBUF_MIN_PROTOC_VERSION
#error This file was generated by an older version of protoc which is
#error incompatible with your Protocol Buffer headers.  Please
#error regenerate this file with a newer version of protoc.
#endif

#include <google/protobuf/io/coded_stream.h>
#include <google/protobuf/arena.h>
#include <google/protobuf/arenastring.h>
#include <google/protobuf/generated_message_table_driven.h>
#include <google/protobuf/generated_message_util.h>
#include <google/protobuf/inlined_string_field.h>
#include <google/protobuf/metadata_lite.h>
#include <google/protobuf/message_lite.h>
#include <google/protobuf/repeated_field.h>  // IWYU pragma: export
#include <google/protobuf/extension_set.h>  // IWYU pragma: export
// @@protoc_insertion_point(includes)
#define PROTOBUF_INTERNAL_EXPORT_protobuf_util_2funresolved_5faddr_2eproto 

namespace protobuf_util_2funresolved_5faddr_2eproto {
// Internal implementation detail -- do not use these members.
struct TableStruct {
  static const ::google::protobuf::internal::ParseTableField entries[];
  static const ::google::protobuf::internal::AuxillaryParseTableField aux[];
  static const ::google::protobuf::internal::ParseTable schema[1];
  static const ::google::protobuf::internal::FieldMetadata field_metadata[];
  static const ::google::protobuf::internal::SerializationTable serialization_table[];
  static const ::google::protobuf::uint32 offsets[];
};
}  // namespace protobuf_util_2funresolved_5faddr_2eproto
namespace znbase {
namespace util {
class UnresolvedAddr;
class UnresolvedAddrDefaultTypeInternal;
extern UnresolvedAddrDefaultTypeInternal _UnresolvedAddr_default_instance_;
}  // namespace util
}  // namespace znbase
namespace google {
namespace protobuf {
template<> ::znbase::util::UnresolvedAddr* Arena::CreateMaybeMessage<::znbase::util::UnresolvedAddr>(Arena*);
}  // namespace protobuf
}  // namespace google
namespace znbase {
namespace util {

// ===================================================================

class UnresolvedAddr : public ::google::protobuf::MessageLite /* @@protoc_insertion_point(class_definition:znbase.util.UnresolvedAddr) */ {
 public:
  UnresolvedAddr();
  virtual ~UnresolvedAddr();

  UnresolvedAddr(const UnresolvedAddr& from);

  inline UnresolvedAddr& operator=(const UnresolvedAddr& from) {
    CopyFrom(from);
    return *this;
  }
  #if LANG_CXX11
  UnresolvedAddr(UnresolvedAddr&& from) noexcept
    : UnresolvedAddr() {
    *this = ::std::move(from);
  }

  inline UnresolvedAddr& operator=(UnresolvedAddr&& from) noexcept {
    if (GetArenaNoVirtual() == from.GetArenaNoVirtual()) {
      if (this != &from) InternalSwap(&from);
    } else {
      CopyFrom(from);
    }
    return *this;
  }
  #endif
  inline const ::std::string& unknown_fields() const {
    return _internal_metadata_.unknown_fields();
  }
  inline ::std::string* mutable_unknown_fields() {
    return _internal_metadata_.mutable_unknown_fields();
  }

  static const UnresolvedAddr& default_instance();

  static void InitAsDefaultInstance();  // FOR INTERNAL USE ONLY
  static inline const UnresolvedAddr* internal_default_instance() {
    return reinterpret_cast<const UnresolvedAddr*>(
               &_UnresolvedAddr_default_instance_);
  }
  static constexpr int kIndexInFileMessages =
    0;

  void Swap(UnresolvedAddr* other);
  friend void swap(UnresolvedAddr& a, UnresolvedAddr& b) {
    a.Swap(&b);
  }

  // implements Message ----------------------------------------------

  inline UnresolvedAddr* New() const final {
    return CreateMaybeMessage<UnresolvedAddr>(NULL);
  }

  UnresolvedAddr* New(::google::protobuf::Arena* arena) const final {
    return CreateMaybeMessage<UnresolvedAddr>(arena);
  }
  void CheckTypeAndMergeFrom(const ::google::protobuf::MessageLite& from)
    final;
  void CopyFrom(const UnresolvedAddr& from);
  void MergeFrom(const UnresolvedAddr& from);
  void Clear() final;
  bool IsInitialized() const final;

  size_t ByteSizeLong() const final;
  bool MergePartialFromCodedStream(
      ::google::protobuf::io::CodedInputStream* input) final;
  void SerializeWithCachedSizes(
      ::google::protobuf::io::CodedOutputStream* output) const final;
  void DiscardUnknownFields();
  int GetCachedSize() const final { return _cached_size_.Get(); }

  private:
  void SharedCtor();
  void SharedDtor();
  void SetCachedSize(int size) const;
  void InternalSwap(UnresolvedAddr* other);
  private:
  inline ::google::protobuf::Arena* GetArenaNoVirtual() const {
    return NULL;
  }
  inline void* MaybeArenaPtr() const {
    return NULL;
  }
  public:

  ::std::string GetTypeName() const final;

  // nested types ----------------------------------------------------

  // accessors -------------------------------------------------------

  bool has_network_field() const;
  void clear_network_field();
  static const int kNetworkFieldFieldNumber = 1;
  const ::std::string& network_field() const;
  void set_network_field(const ::std::string& value);
  #if LANG_CXX11
  void set_network_field(::std::string&& value);
  #endif
  void set_network_field(const char* value);
  void set_network_field(const char* value, size_t size);
  ::std::string* mutable_network_field();
  ::std::string* release_network_field();
  void set_allocated_network_field(::std::string* network_field);

  bool has_address_field() const;
  void clear_address_field();
  static const int kAddressFieldFieldNumber = 2;
  const ::std::string& address_field() const;
  void set_address_field(const ::std::string& value);
  #if LANG_CXX11
  void set_address_field(::std::string&& value);
  #endif
  void set_address_field(const char* value);
  void set_address_field(const char* value, size_t size);
  ::std::string* mutable_address_field();
  ::std::string* release_address_field();
  void set_allocated_address_field(::std::string* address_field);

  // @@protoc_insertion_point(class_scope:znbase.util.UnresolvedAddr)
 private:
  void set_has_network_field();
  void clear_has_network_field();
  void set_has_address_field();
  void clear_has_address_field();

  ::google::protobuf::internal::InternalMetadataWithArenaLite _internal_metadata_;
  ::google::protobuf::internal::HasBits<1> _has_bits_;
  mutable ::google::protobuf::internal::CachedSize _cached_size_;
  ::google::protobuf::internal::ArenaStringPtr network_field_;
  ::google::protobuf::internal::ArenaStringPtr address_field_;
  friend struct ::protobuf_util_2funresolved_5faddr_2eproto::TableStruct;
};
// ===================================================================


// ===================================================================

#ifdef __GNUC__
  #pragma GCC diagnostic push
  #pragma GCC diagnostic ignored "-Wstrict-aliasing"
#endif  // __GNUC__
// UnresolvedAddr

inline bool UnresolvedAddr::has_network_field() const {
  return (_has_bits_[0] & 0x00000001u) != 0;
}
inline void UnresolvedAddr::set_has_network_field() {
  _has_bits_[0] |= 0x00000001u;
}
inline void UnresolvedAddr::clear_has_network_field() {
  _has_bits_[0] &= ~0x00000001u;
}
inline void UnresolvedAddr::clear_network_field() {
  network_field_.ClearToEmptyNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
  clear_has_network_field();
}
inline const ::std::string& UnresolvedAddr::network_field() const {
  // @@protoc_insertion_point(field_get:znbase.util.UnresolvedAddr.network_field)
  return network_field_.GetNoArena();
}
inline void UnresolvedAddr::set_network_field(const ::std::string& value) {
  set_has_network_field();
  network_field_.SetNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), value);
  // @@protoc_insertion_point(field_set:znbase.util.UnresolvedAddr.network_field)
}
#if LANG_CXX11
inline void UnresolvedAddr::set_network_field(::std::string&& value) {
  set_has_network_field();
  network_field_.SetNoArena(
    &::google::protobuf::internal::GetEmptyStringAlreadyInited(), ::std::move(value));
  // @@protoc_insertion_point(field_set_rvalue:znbase.util.UnresolvedAddr.network_field)
}
#endif
inline void UnresolvedAddr::set_network_field(const char* value) {
  GOOGLE_DCHECK(value != NULL);
  set_has_network_field();
  network_field_.SetNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), ::std::string(value));
  // @@protoc_insertion_point(field_set_char:znbase.util.UnresolvedAddr.network_field)
}
inline void UnresolvedAddr::set_network_field(const char* value, size_t size) {
  set_has_network_field();
  network_field_.SetNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(),
      ::std::string(reinterpret_cast<const char*>(value), size));
  // @@protoc_insertion_point(field_set_pointer:znbase.util.UnresolvedAddr.network_field)
}
inline ::std::string* UnresolvedAddr::mutable_network_field() {
  set_has_network_field();
  // @@protoc_insertion_point(field_mutable:znbase.util.UnresolvedAddr.network_field)
  return network_field_.MutableNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
}
inline ::std::string* UnresolvedAddr::release_network_field() {
  // @@protoc_insertion_point(field_release:znbase.util.UnresolvedAddr.network_field)
  if (!has_network_field()) {
    return NULL;
  }
  clear_has_network_field();
  return network_field_.ReleaseNonDefaultNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
}
inline void UnresolvedAddr::set_allocated_network_field(::std::string* network_field) {
  if (network_field != NULL) {
    set_has_network_field();
  } else {
    clear_has_network_field();
  }
  network_field_.SetAllocatedNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), network_field);
  // @@protoc_insertion_point(field_set_allocated:znbase.util.UnresolvedAddr.network_field)
}

inline bool UnresolvedAddr::has_address_field() const {
  return (_has_bits_[0] & 0x00000002u) != 0;
}
inline void UnresolvedAddr::set_has_address_field() {
  _has_bits_[0] |= 0x00000002u;
}
inline void UnresolvedAddr::clear_has_address_field() {
  _has_bits_[0] &= ~0x00000002u;
}
inline void UnresolvedAddr::clear_address_field() {
  address_field_.ClearToEmptyNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
  clear_has_address_field();
}
inline const ::std::string& UnresolvedAddr::address_field() const {
  // @@protoc_insertion_point(field_get:znbase.util.UnresolvedAddr.address_field)
  return address_field_.GetNoArena();
}
inline void UnresolvedAddr::set_address_field(const ::std::string& value) {
  set_has_address_field();
  address_field_.SetNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), value);
  // @@protoc_insertion_point(field_set:znbase.util.UnresolvedAddr.address_field)
}
#if LANG_CXX11
inline void UnresolvedAddr::set_address_field(::std::string&& value) {
  set_has_address_field();
  address_field_.SetNoArena(
    &::google::protobuf::internal::GetEmptyStringAlreadyInited(), ::std::move(value));
  // @@protoc_insertion_point(field_set_rvalue:znbase.util.UnresolvedAddr.address_field)
}
#endif
inline void UnresolvedAddr::set_address_field(const char* value) {
  GOOGLE_DCHECK(value != NULL);
  set_has_address_field();
  address_field_.SetNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), ::std::string(value));
  // @@protoc_insertion_point(field_set_char:znbase.util.UnresolvedAddr.address_field)
}
inline void UnresolvedAddr::set_address_field(const char* value, size_t size) {
  set_has_address_field();
  address_field_.SetNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(),
      ::std::string(reinterpret_cast<const char*>(value), size));
  // @@protoc_insertion_point(field_set_pointer:znbase.util.UnresolvedAddr.address_field)
}
inline ::std::string* UnresolvedAddr::mutable_address_field() {
  set_has_address_field();
  // @@protoc_insertion_point(field_mutable:znbase.util.UnresolvedAddr.address_field)
  return address_field_.MutableNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
}
inline ::std::string* UnresolvedAddr::release_address_field() {
  // @@protoc_insertion_point(field_release:znbase.util.UnresolvedAddr.address_field)
  if (!has_address_field()) {
    return NULL;
  }
  clear_has_address_field();
  return address_field_.ReleaseNonDefaultNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
}
inline void UnresolvedAddr::set_allocated_address_field(::std::string* address_field) {
  if (address_field != NULL) {
    set_has_address_field();
  } else {
    clear_has_address_field();
  }
  address_field_.SetAllocatedNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), address_field);
  // @@protoc_insertion_point(field_set_allocated:znbase.util.UnresolvedAddr.address_field)
}

#ifdef __GNUC__
  #pragma GCC diagnostic pop
#endif  // __GNUC__

// @@protoc_insertion_point(namespace_scope)

}  // namespace util
}  // namespace znbase

// @@protoc_insertion_point(global_scope)

#endif  // PROTOBUF_INCLUDED_util_2funresolved_5faddr_2eproto
