// Generated by the protocol buffer compiler.  DO NOT EDIT!
// source: icl/storageicl/engineicl/enginepbicl/stats.proto

#ifndef PROTOBUF_INCLUDED_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto
#define PROTOBUF_INCLUDED_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto

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
#include "icl/storageicl/engineicl/enginepbicl/key_registry.pb.h"
// @@protoc_insertion_point(includes)
#define PROTOBUF_INTERNAL_EXPORT_protobuf_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto 

namespace protobuf_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto {
// Internal implementation detail -- do not use these members.
struct TableStruct {
  static const ::google::protobuf::internal::ParseTableField entries[];
  static const ::google::protobuf::internal::AuxillaryParseTableField aux[];
  static const ::google::protobuf::internal::ParseTable schema[1];
  static const ::google::protobuf::internal::FieldMetadata field_metadata[];
  static const ::google::protobuf::internal::SerializationTable serialization_table[];
  static const ::google::protobuf::uint32 offsets[];
};
}  // namespace protobuf_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto
namespace znbase {
namespace icl {
namespace storageicl {
namespace engineicl {
namespace enginepbicl {
class EncryptionStatus;
class EncryptionStatusDefaultTypeInternal;
extern EncryptionStatusDefaultTypeInternal _EncryptionStatus_default_instance_;
}  // namespace enginepbicl
}  // namespace engineicl
}  // namespace storageicl
}  // namespace icl
}  // namespace znbase
namespace google {
namespace protobuf {
template<> ::znbase::icl::storageicl::engineicl::enginepbicl::EncryptionStatus* Arena::CreateMaybeMessage<::znbase::icl::storageicl::engineicl::enginepbicl::EncryptionStatus>(Arena*);
}  // namespace protobuf
}  // namespace google
namespace znbase {
namespace icl {
namespace storageicl {
namespace engineicl {
namespace enginepbicl {

// ===================================================================

class EncryptionStatus : public ::google::protobuf::MessageLite /* @@protoc_insertion_point(class_definition:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus) */ {
 public:
  EncryptionStatus();
  virtual ~EncryptionStatus();

  EncryptionStatus(const EncryptionStatus& from);

  inline EncryptionStatus& operator=(const EncryptionStatus& from) {
    CopyFrom(from);
    return *this;
  }
  #if LANG_CXX11
  EncryptionStatus(EncryptionStatus&& from) noexcept
    : EncryptionStatus() {
    *this = ::std::move(from);
  }

  inline EncryptionStatus& operator=(EncryptionStatus&& from) noexcept {
    if (GetArenaNoVirtual() == from.GetArenaNoVirtual()) {
      if (this != &from) InternalSwap(&from);
    } else {
      CopyFrom(from);
    }
    return *this;
  }
  #endif
  static const EncryptionStatus& default_instance();

  static void InitAsDefaultInstance();  // FOR INTERNAL USE ONLY
  static inline const EncryptionStatus* internal_default_instance() {
    return reinterpret_cast<const EncryptionStatus*>(
               &_EncryptionStatus_default_instance_);
  }
  static constexpr int kIndexInFileMessages =
    0;

  void Swap(EncryptionStatus* other);
  friend void swap(EncryptionStatus& a, EncryptionStatus& b) {
    a.Swap(&b);
  }

  // implements Message ----------------------------------------------

  inline EncryptionStatus* New() const final {
    return CreateMaybeMessage<EncryptionStatus>(NULL);
  }

  EncryptionStatus* New(::google::protobuf::Arena* arena) const final {
    return CreateMaybeMessage<EncryptionStatus>(arena);
  }
  void CheckTypeAndMergeFrom(const ::google::protobuf::MessageLite& from)
    final;
  void CopyFrom(const EncryptionStatus& from);
  void MergeFrom(const EncryptionStatus& from);
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
  void InternalSwap(EncryptionStatus* other);
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

  // .znbase.icl.storageicl.engineicl.enginepbicl.KeyInfo active_store_key = 1;
  bool has_active_store_key() const;
  void clear_active_store_key();
  static const int kActiveStoreKeyFieldNumber = 1;
  private:
  const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& _internal_active_store_key() const;
  public:
  const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& active_store_key() const;
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* release_active_store_key();
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* mutable_active_store_key();
  void set_allocated_active_store_key(::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* active_store_key);

  // .znbase.icl.storageicl.engineicl.enginepbicl.KeyInfo active_data_key = 2;
  bool has_active_data_key() const;
  void clear_active_data_key();
  static const int kActiveDataKeyFieldNumber = 2;
  private:
  const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& _internal_active_data_key() const;
  public:
  const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& active_data_key() const;
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* release_active_data_key();
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* mutable_active_data_key();
  void set_allocated_active_data_key(::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* active_data_key);

  // @@protoc_insertion_point(class_scope:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus)
 private:

  ::google::protobuf::internal::InternalMetadataWithArenaLite _internal_metadata_;
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* active_store_key_;
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* active_data_key_;
  mutable ::google::protobuf::internal::CachedSize _cached_size_;
  friend struct ::protobuf_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto::TableStruct;
};
// ===================================================================


// ===================================================================

#ifdef __GNUC__
  #pragma GCC diagnostic push
  #pragma GCC diagnostic ignored "-Wstrict-aliasing"
#endif  // __GNUC__
// EncryptionStatus

// .znbase.icl.storageicl.engineicl.enginepbicl.KeyInfo active_store_key = 1;
inline bool EncryptionStatus::has_active_store_key() const {
  return this != internal_default_instance() && active_store_key_ != NULL;
}
inline const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& EncryptionStatus::_internal_active_store_key() const {
  return *active_store_key_;
}
inline const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& EncryptionStatus::active_store_key() const {
  const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* p = active_store_key_;
  // @@protoc_insertion_point(field_get:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_store_key)
  return p != NULL ? *p : *reinterpret_cast<const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo*>(
      &::znbase::icl::storageicl::engineicl::enginepbicl::_KeyInfo_default_instance_);
}
inline ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* EncryptionStatus::release_active_store_key() {
  // @@protoc_insertion_point(field_release:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_store_key)
  
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* temp = active_store_key_;
  active_store_key_ = NULL;
  return temp;
}
inline ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* EncryptionStatus::mutable_active_store_key() {
  
  if (active_store_key_ == NULL) {
    auto* p = CreateMaybeMessage<::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo>(GetArenaNoVirtual());
    active_store_key_ = p;
  }
  // @@protoc_insertion_point(field_mutable:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_store_key)
  return active_store_key_;
}
inline void EncryptionStatus::set_allocated_active_store_key(::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* active_store_key) {
  ::google::protobuf::Arena* message_arena = GetArenaNoVirtual();
  if (message_arena == NULL) {
    delete reinterpret_cast< ::google::protobuf::MessageLite*>(active_store_key_);
  }
  if (active_store_key) {
    ::google::protobuf::Arena* submessage_arena = NULL;
    if (message_arena != submessage_arena) {
      active_store_key = ::google::protobuf::internal::GetOwnedMessage(
          message_arena, active_store_key, submessage_arena);
    }
    
  } else {
    
  }
  active_store_key_ = active_store_key;
  // @@protoc_insertion_point(field_set_allocated:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_store_key)
}

// .znbase.icl.storageicl.engineicl.enginepbicl.KeyInfo active_data_key = 2;
inline bool EncryptionStatus::has_active_data_key() const {
  return this != internal_default_instance() && active_data_key_ != NULL;
}
inline const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& EncryptionStatus::_internal_active_data_key() const {
  return *active_data_key_;
}
inline const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo& EncryptionStatus::active_data_key() const {
  const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* p = active_data_key_;
  // @@protoc_insertion_point(field_get:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_data_key)
  return p != NULL ? *p : *reinterpret_cast<const ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo*>(
      &::znbase::icl::storageicl::engineicl::enginepbicl::_KeyInfo_default_instance_);
}
inline ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* EncryptionStatus::release_active_data_key() {
  // @@protoc_insertion_point(field_release:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_data_key)
  
  ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* temp = active_data_key_;
  active_data_key_ = NULL;
  return temp;
}
inline ::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* EncryptionStatus::mutable_active_data_key() {
  
  if (active_data_key_ == NULL) {
    auto* p = CreateMaybeMessage<::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo>(GetArenaNoVirtual());
    active_data_key_ = p;
  }
  // @@protoc_insertion_point(field_mutable:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_data_key)
  return active_data_key_;
}
inline void EncryptionStatus::set_allocated_active_data_key(::znbase::icl::storageicl::engineicl::enginepbicl::KeyInfo* active_data_key) {
  ::google::protobuf::Arena* message_arena = GetArenaNoVirtual();
  if (message_arena == NULL) {
    delete reinterpret_cast< ::google::protobuf::MessageLite*>(active_data_key_);
  }
  if (active_data_key) {
    ::google::protobuf::Arena* submessage_arena = NULL;
    if (message_arena != submessage_arena) {
      active_data_key = ::google::protobuf::internal::GetOwnedMessage(
          message_arena, active_data_key, submessage_arena);
    }
    
  } else {
    
  }
  active_data_key_ = active_data_key;
  // @@protoc_insertion_point(field_set_allocated:znbase.icl.storageicl.engineicl.enginepbicl.EncryptionStatus.active_data_key)
}

#ifdef __GNUC__
  #pragma GCC diagnostic pop
#endif  // __GNUC__

// @@protoc_insertion_point(namespace_scope)

}  // namespace enginepbicl
}  // namespace engineicl
}  // namespace storageicl
}  // namespace icl
}  // namespace znbase

// @@protoc_insertion_point(global_scope)

#endif  // PROTOBUF_INCLUDED_icl_2fstorageicl_2fengineicl_2fenginepbicl_2fstats_2eproto
