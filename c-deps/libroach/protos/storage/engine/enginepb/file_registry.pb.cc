// Generated by the protocol buffer compiler.  DO NOT EDIT!
// source: storage/engine/enginepb/file_registry.proto

#include "storage/engine/enginepb/file_registry.pb.h"

#include <algorithm>

#include <google/protobuf/stubs/common.h>
#include <google/protobuf/stubs/port.h>
#include <google/protobuf/io/coded_stream.h>
#include <google/protobuf/wire_format_lite_inl.h>
#include <google/protobuf/io/zero_copy_stream_impl_lite.h>
// This is a temporary google only hack
#ifdef GOOGLE_PROTOBUF_ENFORCE_UNIQUENESS
#include "third_party/protobuf/version.h"
#endif
// @@protoc_insertion_point(includes)

namespace protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto {
extern PROTOBUF_INTERNAL_EXPORT_protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto ::google::protobuf::internal::SCCInfo<0> scc_info_FileEntry;
extern PROTOBUF_INTERNAL_EXPORT_protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto ::google::protobuf::internal::SCCInfo<1> scc_info_FileRegistry_FilesEntry_DoNotUse;
}  // namespace protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto
namespace znbase {
namespace storage {
namespace engine {
namespace enginepb {
class FileRegistry_FilesEntry_DoNotUseDefaultTypeInternal {
 public:
  ::google::protobuf::internal::ExplicitlyConstructed<FileRegistry_FilesEntry_DoNotUse>
      _instance;
} _FileRegistry_FilesEntry_DoNotUse_default_instance_;
class FileRegistryDefaultTypeInternal {
 public:
  ::google::protobuf::internal::ExplicitlyConstructed<FileRegistry>
      _instance;
} _FileRegistry_default_instance_;
class FileEntryDefaultTypeInternal {
 public:
  ::google::protobuf::internal::ExplicitlyConstructed<FileEntry>
      _instance;
} _FileEntry_default_instance_;
}  // namespace enginepb
}  // namespace engine
}  // namespace storage
}  // namespace znbase
namespace protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto {
static void InitDefaultsFileRegistry_FilesEntry_DoNotUse() {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  {
    void* ptr = &::znbase::storage::engine::enginepb::_FileRegistry_FilesEntry_DoNotUse_default_instance_;
    new (ptr) ::znbase::storage::engine::enginepb::FileRegistry_FilesEntry_DoNotUse();
  }
  ::znbase::storage::engine::enginepb::FileRegistry_FilesEntry_DoNotUse::InitAsDefaultInstance();
}

::google::protobuf::internal::SCCInfo<1> scc_info_FileRegistry_FilesEntry_DoNotUse =
    {{ATOMIC_VAR_INIT(::google::protobuf::internal::SCCInfoBase::kUninitialized), 1, InitDefaultsFileRegistry_FilesEntry_DoNotUse}, {
      &protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto::scc_info_FileEntry.base,}};

static void InitDefaultsFileRegistry() {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  {
    void* ptr = &::znbase::storage::engine::enginepb::_FileRegistry_default_instance_;
    new (ptr) ::znbase::storage::engine::enginepb::FileRegistry();
    ::google::protobuf::internal::OnShutdownDestroyMessage(ptr);
  }
  ::znbase::storage::engine::enginepb::FileRegistry::InitAsDefaultInstance();
}

::google::protobuf::internal::SCCInfo<1> scc_info_FileRegistry =
    {{ATOMIC_VAR_INIT(::google::protobuf::internal::SCCInfoBase::kUninitialized), 1, InitDefaultsFileRegistry}, {
      &protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto::scc_info_FileRegistry_FilesEntry_DoNotUse.base,}};

static void InitDefaultsFileEntry() {
  GOOGLE_PROTOBUF_VERIFY_VERSION;

  {
    void* ptr = &::znbase::storage::engine::enginepb::_FileEntry_default_instance_;
    new (ptr) ::znbase::storage::engine::enginepb::FileEntry();
    ::google::protobuf::internal::OnShutdownDestroyMessage(ptr);
  }
  ::znbase::storage::engine::enginepb::FileEntry::InitAsDefaultInstance();
}

::google::protobuf::internal::SCCInfo<0> scc_info_FileEntry =
    {{ATOMIC_VAR_INIT(::google::protobuf::internal::SCCInfoBase::kUninitialized), 0, InitDefaultsFileEntry}, {}};

void InitDefaults() {
  ::google::protobuf::internal::InitSCC(&scc_info_FileRegistry_FilesEntry_DoNotUse.base);
  ::google::protobuf::internal::InitSCC(&scc_info_FileRegistry.base);
  ::google::protobuf::internal::InitSCC(&scc_info_FileEntry.base);
}

}  // namespace protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto
namespace znbase {
namespace storage {
namespace engine {
namespace enginepb {
bool RegistryVersion_IsValid(int value) {
  switch (value) {
    case 0:
      return true;
    default:
      return false;
  }
}

bool EnvType_IsValid(int value) {
  switch (value) {
    case 0:
    case 1:
    case 2:
      return true;
    default:
      return false;
  }
}


// ===================================================================

FileRegistry_FilesEntry_DoNotUse::FileRegistry_FilesEntry_DoNotUse() {}
FileRegistry_FilesEntry_DoNotUse::FileRegistry_FilesEntry_DoNotUse(::google::protobuf::Arena* arena) : SuperType(arena) {}
void FileRegistry_FilesEntry_DoNotUse::MergeFrom(const FileRegistry_FilesEntry_DoNotUse& other) {
  MergeFromInternal(other);
}

// ===================================================================

void FileRegistry::InitAsDefaultInstance() {
}
#if !defined(_MSC_VER) || _MSC_VER >= 1900
const int FileRegistry::kVersionFieldNumber;
const int FileRegistry::kFilesFieldNumber;
#endif  // !defined(_MSC_VER) || _MSC_VER >= 1900

FileRegistry::FileRegistry()
  : ::google::protobuf::MessageLite(), _internal_metadata_(NULL) {
  ::google::protobuf::internal::InitSCC(
      &protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto::scc_info_FileRegistry.base);
  SharedCtor();
  // @@protoc_insertion_point(constructor:znbase.storage.engine.enginepb.FileRegistry)
}
FileRegistry::FileRegistry(const FileRegistry& from)
  : ::google::protobuf::MessageLite(),
      _internal_metadata_(NULL) {
  _internal_metadata_.MergeFrom(from._internal_metadata_);
  files_.MergeFrom(from.files_);
  version_ = from.version_;
  // @@protoc_insertion_point(copy_constructor:znbase.storage.engine.enginepb.FileRegistry)
}

void FileRegistry::SharedCtor() {
  version_ = 0;
}

FileRegistry::~FileRegistry() {
  // @@protoc_insertion_point(destructor:znbase.storage.engine.enginepb.FileRegistry)
  SharedDtor();
}

void FileRegistry::SharedDtor() {
}

void FileRegistry::SetCachedSize(int size) const {
  _cached_size_.Set(size);
}
const FileRegistry& FileRegistry::default_instance() {
  ::google::protobuf::internal::InitSCC(&protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto::scc_info_FileRegistry.base);
  return *internal_default_instance();
}


void FileRegistry::Clear() {
// @@protoc_insertion_point(message_clear_start:znbase.storage.engine.enginepb.FileRegistry)
  ::google::protobuf::uint32 cached_has_bits = 0;
  // Prevent compiler warnings about cached_has_bits being unused
  (void) cached_has_bits;

  files_.Clear();
  version_ = 0;
  _internal_metadata_.Clear();
}

bool FileRegistry::MergePartialFromCodedStream(
    ::google::protobuf::io::CodedInputStream* input) {
#define DO_(EXPRESSION) if (!GOOGLE_PREDICT_TRUE(EXPRESSION)) goto failure
  ::google::protobuf::uint32 tag;
  ::google::protobuf::internal::LiteUnknownFieldSetter unknown_fields_setter(
      &_internal_metadata_);
  ::google::protobuf::io::StringOutputStream unknown_fields_output(
      unknown_fields_setter.buffer());
  ::google::protobuf::io::CodedOutputStream unknown_fields_stream(
      &unknown_fields_output, false);
  // @@protoc_insertion_point(parse_start:znbase.storage.engine.enginepb.FileRegistry)
  for (;;) {
    ::std::pair<::google::protobuf::uint32, bool> p = input->ReadTagWithCutoffNoLastTag(127u);
    tag = p.first;
    if (!p.second) goto handle_unusual;
    switch (::google::protobuf::internal::WireFormatLite::GetTagFieldNumber(tag)) {
      // .znbase.storage.engine.enginepb.RegistryVersion version = 1;
      case 1: {
        if (static_cast< ::google::protobuf::uint8>(tag) ==
            static_cast< ::google::protobuf::uint8>(8u /* 8 & 0xFF */)) {
          int value;
          DO_((::google::protobuf::internal::WireFormatLite::ReadPrimitive<
                   int, ::google::protobuf::internal::WireFormatLite::TYPE_ENUM>(
                 input, &value)));
          set_version(static_cast< ::znbase::storage::engine::enginepb::RegistryVersion >(value));
        } else {
          goto handle_unusual;
        }
        break;
      }

      // map<string, .znbase.storage.engine.enginepb.FileEntry> files = 2;
      case 2: {
        if (static_cast< ::google::protobuf::uint8>(tag) ==
            static_cast< ::google::protobuf::uint8>(18u /* 18 & 0xFF */)) {
          FileRegistry_FilesEntry_DoNotUse::Parser< ::google::protobuf::internal::MapFieldLite<
              FileRegistry_FilesEntry_DoNotUse,
              ::std::string, ::znbase::storage::engine::enginepb::FileEntry,
              ::google::protobuf::internal::WireFormatLite::TYPE_STRING,
              ::google::protobuf::internal::WireFormatLite::TYPE_MESSAGE,
              0 >,
            ::google::protobuf::Map< ::std::string, ::znbase::storage::engine::enginepb::FileEntry > > parser(&files_);
          DO_(::google::protobuf::internal::WireFormatLite::ReadMessageNoVirtual(
              input, &parser));
          DO_(::google::protobuf::internal::WireFormatLite::VerifyUtf8String(
            parser.key().data(), static_cast<int>(parser.key().length()),
            ::google::protobuf::internal::WireFormatLite::PARSE,
            "znbase.storage.engine.enginepb.FileRegistry.FilesEntry.key"));
        } else {
          goto handle_unusual;
        }
        break;
      }

      default: {
      handle_unusual:
        if (tag == 0) {
          goto success;
        }
        DO_(::google::protobuf::internal::WireFormatLite::SkipField(
            input, tag, &unknown_fields_stream));
        break;
      }
    }
  }
success:
  // @@protoc_insertion_point(parse_success:znbase.storage.engine.enginepb.FileRegistry)
  return true;
failure:
  // @@protoc_insertion_point(parse_failure:znbase.storage.engine.enginepb.FileRegistry)
  return false;
#undef DO_
}

void FileRegistry::SerializeWithCachedSizes(
    ::google::protobuf::io::CodedOutputStream* output) const {
  // @@protoc_insertion_point(serialize_start:znbase.storage.engine.enginepb.FileRegistry)
  ::google::protobuf::uint32 cached_has_bits = 0;
  (void) cached_has_bits;

  // .znbase.storage.engine.enginepb.RegistryVersion version = 1;
  if (this->version() != 0) {
    ::google::protobuf::internal::WireFormatLite::WriteEnum(
      1, this->version(), output);
  }

  // map<string, .znbase.storage.engine.enginepb.FileEntry> files = 2;
  if (!this->files().empty()) {
    typedef ::google::protobuf::Map< ::std::string, ::znbase::storage::engine::enginepb::FileEntry >::const_pointer
        ConstPtr;
    typedef ConstPtr SortItem;
    typedef ::google::protobuf::internal::CompareByDerefFirst<SortItem> Less;
    struct Utf8Check {
      static void Check(ConstPtr p) {
        ::google::protobuf::internal::WireFormatLite::VerifyUtf8String(
          p->first.data(), static_cast<int>(p->first.length()),
          ::google::protobuf::internal::WireFormatLite::SERIALIZE,
          "znbase.storage.engine.enginepb.FileRegistry.FilesEntry.key");
      }
    };

    if (output->IsSerializationDeterministic() &&
        this->files().size() > 1) {
      ::std::unique_ptr<SortItem[]> items(
          new SortItem[this->files().size()]);
      typedef ::google::protobuf::Map< ::std::string, ::znbase::storage::engine::enginepb::FileEntry >::size_type size_type;
      size_type n = 0;
      for (::google::protobuf::Map< ::std::string, ::znbase::storage::engine::enginepb::FileEntry >::const_iterator
          it = this->files().begin();
          it != this->files().end(); ++it, ++n) {
        items[static_cast<ptrdiff_t>(n)] = SortItem(&*it);
      }
      ::std::sort(&items[0], &items[static_cast<ptrdiff_t>(n)], Less());
      ::std::unique_ptr<FileRegistry_FilesEntry_DoNotUse> entry;
      for (size_type i = 0; i < n; i++) {
        entry.reset(files_.NewEntryWrapper(
            items[static_cast<ptrdiff_t>(i)]->first, items[static_cast<ptrdiff_t>(i)]->second));
        ::google::protobuf::internal::WireFormatLite::WriteMessage(
            2, *entry, output);
        Utf8Check::Check(items[static_cast<ptrdiff_t>(i)]);
      }
    } else {
      ::std::unique_ptr<FileRegistry_FilesEntry_DoNotUse> entry;
      for (::google::protobuf::Map< ::std::string, ::znbase::storage::engine::enginepb::FileEntry >::const_iterator
          it = this->files().begin();
          it != this->files().end(); ++it) {
        entry.reset(files_.NewEntryWrapper(
            it->first, it->second));
        ::google::protobuf::internal::WireFormatLite::WriteMessage(
            2, *entry, output);
        Utf8Check::Check(&*it);
      }
    }
  }

  output->WriteRaw((::google::protobuf::internal::GetProto3PreserveUnknownsDefault()   ? _internal_metadata_.unknown_fields()   : _internal_metadata_.default_instance()).data(),
                   static_cast<int>((::google::protobuf::internal::GetProto3PreserveUnknownsDefault()   ? _internal_metadata_.unknown_fields()   : _internal_metadata_.default_instance()).size()));
  // @@protoc_insertion_point(serialize_end:znbase.storage.engine.enginepb.FileRegistry)
}

size_t FileRegistry::ByteSizeLong() const {
// @@protoc_insertion_point(message_byte_size_start:znbase.storage.engine.enginepb.FileRegistry)
  size_t total_size = 0;

  total_size += (::google::protobuf::internal::GetProto3PreserveUnknownsDefault()   ? _internal_metadata_.unknown_fields()   : _internal_metadata_.default_instance()).size();

  // map<string, .znbase.storage.engine.enginepb.FileEntry> files = 2;
  total_size += 1 *
      ::google::protobuf::internal::FromIntSize(this->files_size());
  {
    ::std::unique_ptr<FileRegistry_FilesEntry_DoNotUse> entry;
    for (::google::protobuf::Map< ::std::string, ::znbase::storage::engine::enginepb::FileEntry >::const_iterator
        it = this->files().begin();
        it != this->files().end(); ++it) {
      entry.reset(files_.NewEntryWrapper(it->first, it->second));
      total_size += ::google::protobuf::internal::WireFormatLite::
          MessageSizeNoVirtual(*entry);
    }
  }

  // .znbase.storage.engine.enginepb.RegistryVersion version = 1;
  if (this->version() != 0) {
    total_size += 1 +
      ::google::protobuf::internal::WireFormatLite::EnumSize(this->version());
  }

  int cached_size = ::google::protobuf::internal::ToCachedSize(total_size);
  SetCachedSize(cached_size);
  return total_size;
}

void FileRegistry::CheckTypeAndMergeFrom(
    const ::google::protobuf::MessageLite& from) {
  MergeFrom(*::google::protobuf::down_cast<const FileRegistry*>(&from));
}

void FileRegistry::MergeFrom(const FileRegistry& from) {
// @@protoc_insertion_point(class_specific_merge_from_start:znbase.storage.engine.enginepb.FileRegistry)
  GOOGLE_DCHECK_NE(&from, this);
  _internal_metadata_.MergeFrom(from._internal_metadata_);
  ::google::protobuf::uint32 cached_has_bits = 0;
  (void) cached_has_bits;

  files_.MergeFrom(from.files_);
  if (from.version() != 0) {
    set_version(from.version());
  }
}

void FileRegistry::CopyFrom(const FileRegistry& from) {
// @@protoc_insertion_point(class_specific_copy_from_start:znbase.storage.engine.enginepb.FileRegistry)
  if (&from == this) return;
  Clear();
  MergeFrom(from);
}

bool FileRegistry::IsInitialized() const {
  return true;
}

void FileRegistry::Swap(FileRegistry* other) {
  if (other == this) return;
  InternalSwap(other);
}
void FileRegistry::InternalSwap(FileRegistry* other) {
  using std::swap;
  files_.Swap(&other->files_);
  swap(version_, other->version_);
  _internal_metadata_.Swap(&other->_internal_metadata_);
}

::std::string FileRegistry::GetTypeName() const {
  return "znbase.storage.engine.enginepb.FileRegistry";
}


// ===================================================================

void FileEntry::InitAsDefaultInstance() {
}
#if !defined(_MSC_VER) || _MSC_VER >= 1900
const int FileEntry::kEnvTypeFieldNumber;
const int FileEntry::kEncryptionSettingsFieldNumber;
#endif  // !defined(_MSC_VER) || _MSC_VER >= 1900

FileEntry::FileEntry()
  : ::google::protobuf::MessageLite(), _internal_metadata_(NULL) {
  ::google::protobuf::internal::InitSCC(
      &protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto::scc_info_FileEntry.base);
  SharedCtor();
  // @@protoc_insertion_point(constructor:znbase.storage.engine.enginepb.FileEntry)
}
FileEntry::FileEntry(const FileEntry& from)
  : ::google::protobuf::MessageLite(),
      _internal_metadata_(NULL) {
  _internal_metadata_.MergeFrom(from._internal_metadata_);
  encryption_settings_.UnsafeSetDefault(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
  if (from.encryption_settings().size() > 0) {
    encryption_settings_.AssignWithDefault(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), from.encryption_settings_);
  }
  env_type_ = from.env_type_;
  // @@protoc_insertion_point(copy_constructor:znbase.storage.engine.enginepb.FileEntry)
}

void FileEntry::SharedCtor() {
  encryption_settings_.UnsafeSetDefault(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
  env_type_ = 0;
}

FileEntry::~FileEntry() {
  // @@protoc_insertion_point(destructor:znbase.storage.engine.enginepb.FileEntry)
  SharedDtor();
}

void FileEntry::SharedDtor() {
  encryption_settings_.DestroyNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
}

void FileEntry::SetCachedSize(int size) const {
  _cached_size_.Set(size);
}
const FileEntry& FileEntry::default_instance() {
  ::google::protobuf::internal::InitSCC(&protobuf_storage_2fengine_2fenginepb_2ffile_5fregistry_2eproto::scc_info_FileEntry.base);
  return *internal_default_instance();
}


void FileEntry::Clear() {
// @@protoc_insertion_point(message_clear_start:znbase.storage.engine.enginepb.FileEntry)
  ::google::protobuf::uint32 cached_has_bits = 0;
  // Prevent compiler warnings about cached_has_bits being unused
  (void) cached_has_bits;

  encryption_settings_.ClearToEmptyNoArena(&::google::protobuf::internal::GetEmptyStringAlreadyInited());
  env_type_ = 0;
  _internal_metadata_.Clear();
}

bool FileEntry::MergePartialFromCodedStream(
    ::google::protobuf::io::CodedInputStream* input) {
#define DO_(EXPRESSION) if (!GOOGLE_PREDICT_TRUE(EXPRESSION)) goto failure
  ::google::protobuf::uint32 tag;
  ::google::protobuf::internal::LiteUnknownFieldSetter unknown_fields_setter(
      &_internal_metadata_);
  ::google::protobuf::io::StringOutputStream unknown_fields_output(
      unknown_fields_setter.buffer());
  ::google::protobuf::io::CodedOutputStream unknown_fields_stream(
      &unknown_fields_output, false);
  // @@protoc_insertion_point(parse_start:znbase.storage.engine.enginepb.FileEntry)
  for (;;) {
    ::std::pair<::google::protobuf::uint32, bool> p = input->ReadTagWithCutoffNoLastTag(127u);
    tag = p.first;
    if (!p.second) goto handle_unusual;
    switch (::google::protobuf::internal::WireFormatLite::GetTagFieldNumber(tag)) {
      // .znbase.storage.engine.enginepb.EnvType env_type = 1;
      case 1: {
        if (static_cast< ::google::protobuf::uint8>(tag) ==
            static_cast< ::google::protobuf::uint8>(8u /* 8 & 0xFF */)) {
          int value;
          DO_((::google::protobuf::internal::WireFormatLite::ReadPrimitive<
                   int, ::google::protobuf::internal::WireFormatLite::TYPE_ENUM>(
                 input, &value)));
          set_env_type(static_cast< ::znbase::storage::engine::enginepb::EnvType >(value));
        } else {
          goto handle_unusual;
        }
        break;
      }

      // bytes encryption_settings = 2;
      case 2: {
        if (static_cast< ::google::protobuf::uint8>(tag) ==
            static_cast< ::google::protobuf::uint8>(18u /* 18 & 0xFF */)) {
          DO_(::google::protobuf::internal::WireFormatLite::ReadBytes(
                input, this->mutable_encryption_settings()));
        } else {
          goto handle_unusual;
        }
        break;
      }

      default: {
      handle_unusual:
        if (tag == 0) {
          goto success;
        }
        DO_(::google::protobuf::internal::WireFormatLite::SkipField(
            input, tag, &unknown_fields_stream));
        break;
      }
    }
  }
success:
  // @@protoc_insertion_point(parse_success:znbase.storage.engine.enginepb.FileEntry)
  return true;
failure:
  // @@protoc_insertion_point(parse_failure:znbase.storage.engine.enginepb.FileEntry)
  return false;
#undef DO_
}

void FileEntry::SerializeWithCachedSizes(
    ::google::protobuf::io::CodedOutputStream* output) const {
  // @@protoc_insertion_point(serialize_start:znbase.storage.engine.enginepb.FileEntry)
  ::google::protobuf::uint32 cached_has_bits = 0;
  (void) cached_has_bits;

  // .znbase.storage.engine.enginepb.EnvType env_type = 1;
  if (this->env_type() != 0) {
    ::google::protobuf::internal::WireFormatLite::WriteEnum(
      1, this->env_type(), output);
  }

  // bytes encryption_settings = 2;
  if (this->encryption_settings().size() > 0) {
    ::google::protobuf::internal::WireFormatLite::WriteBytesMaybeAliased(
      2, this->encryption_settings(), output);
  }

  output->WriteRaw((::google::protobuf::internal::GetProto3PreserveUnknownsDefault()   ? _internal_metadata_.unknown_fields()   : _internal_metadata_.default_instance()).data(),
                   static_cast<int>((::google::protobuf::internal::GetProto3PreserveUnknownsDefault()   ? _internal_metadata_.unknown_fields()   : _internal_metadata_.default_instance()).size()));
  // @@protoc_insertion_point(serialize_end:znbase.storage.engine.enginepb.FileEntry)
}

size_t FileEntry::ByteSizeLong() const {
// @@protoc_insertion_point(message_byte_size_start:znbase.storage.engine.enginepb.FileEntry)
  size_t total_size = 0;

  total_size += (::google::protobuf::internal::GetProto3PreserveUnknownsDefault()   ? _internal_metadata_.unknown_fields()   : _internal_metadata_.default_instance()).size();

  // bytes encryption_settings = 2;
  if (this->encryption_settings().size() > 0) {
    total_size += 1 +
      ::google::protobuf::internal::WireFormatLite::BytesSize(
        this->encryption_settings());
  }

  // .znbase.storage.engine.enginepb.EnvType env_type = 1;
  if (this->env_type() != 0) {
    total_size += 1 +
      ::google::protobuf::internal::WireFormatLite::EnumSize(this->env_type());
  }

  int cached_size = ::google::protobuf::internal::ToCachedSize(total_size);
  SetCachedSize(cached_size);
  return total_size;
}

void FileEntry::CheckTypeAndMergeFrom(
    const ::google::protobuf::MessageLite& from) {
  MergeFrom(*::google::protobuf::down_cast<const FileEntry*>(&from));
}

void FileEntry::MergeFrom(const FileEntry& from) {
// @@protoc_insertion_point(class_specific_merge_from_start:znbase.storage.engine.enginepb.FileEntry)
  GOOGLE_DCHECK_NE(&from, this);
  _internal_metadata_.MergeFrom(from._internal_metadata_);
  ::google::protobuf::uint32 cached_has_bits = 0;
  (void) cached_has_bits;

  if (from.encryption_settings().size() > 0) {

    encryption_settings_.AssignWithDefault(&::google::protobuf::internal::GetEmptyStringAlreadyInited(), from.encryption_settings_);
  }
  if (from.env_type() != 0) {
    set_env_type(from.env_type());
  }
}

void FileEntry::CopyFrom(const FileEntry& from) {
// @@protoc_insertion_point(class_specific_copy_from_start:znbase.storage.engine.enginepb.FileEntry)
  if (&from == this) return;
  Clear();
  MergeFrom(from);
}

bool FileEntry::IsInitialized() const {
  return true;
}

void FileEntry::Swap(FileEntry* other) {
  if (other == this) return;
  InternalSwap(other);
}
void FileEntry::InternalSwap(FileEntry* other) {
  using std::swap;
  encryption_settings_.Swap(&other->encryption_settings_, &::google::protobuf::internal::GetEmptyStringAlreadyInited(),
    GetArenaNoVirtual());
  swap(env_type_, other->env_type_);
  _internal_metadata_.Swap(&other->_internal_metadata_);
}

::std::string FileEntry::GetTypeName() const {
  return "znbase.storage.engine.enginepb.FileEntry";
}


// @@protoc_insertion_point(namespace_scope)
}  // namespace enginepb
}  // namespace engine
}  // namespace storage
}  // namespace znbase
namespace google {
namespace protobuf {
template<> GOOGLE_PROTOBUF_ATTRIBUTE_NOINLINE ::znbase::storage::engine::enginepb::FileRegistry_FilesEntry_DoNotUse* Arena::CreateMaybeMessage< ::znbase::storage::engine::enginepb::FileRegistry_FilesEntry_DoNotUse >(Arena* arena) {
  return Arena::CreateInternal< ::znbase::storage::engine::enginepb::FileRegistry_FilesEntry_DoNotUse >(arena);
}
template<> GOOGLE_PROTOBUF_ATTRIBUTE_NOINLINE ::znbase::storage::engine::enginepb::FileRegistry* Arena::CreateMaybeMessage< ::znbase::storage::engine::enginepb::FileRegistry >(Arena* arena) {
  return Arena::CreateInternal< ::znbase::storage::engine::enginepb::FileRegistry >(arena);
}
template<> GOOGLE_PROTOBUF_ATTRIBUTE_NOINLINE ::znbase::storage::engine::enginepb::FileEntry* Arena::CreateMaybeMessage< ::znbase::storage::engine::enginepb::FileEntry >(Arena* arena) {
  return Arena::CreateInternal< ::znbase::storage::engine::enginepb::FileEntry >(arena);
}
}  // namespace protobuf
}  // namespace google

// @@protoc_insertion_point(global_scope)
