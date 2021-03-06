// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: storage/engine/enginepb/file_registry.proto

package enginepb

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import github_com_gogo_protobuf_sortkeys "github.com/gogo/protobuf/sortkeys"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type RegistryVersion int32

const (
	// The only version so far.
	RegistryVersion_Base RegistryVersion = 0
)

var RegistryVersion_name = map[int32]string{
	0: "Base",
}
var RegistryVersion_value = map[string]int32{
	"Base": 0,
}

func (x RegistryVersion) String() string {
	return proto.EnumName(RegistryVersion_name, int32(x))
}
func (RegistryVersion) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_file_registry_22f58cb8ad5a5179, []int{0}
}

// EnvType determines which rocksdb::Env is used and for what purpose.
type EnvType int32

const (
	// The default Env when no encryption is used.
	// File using Plaintext are not recorded in the file registry.
	EnvType_Plaintext EnvType = 0
	// The Env using store-level keys.
	// Used only to read/write the data key registry.
	EnvType_Store EnvType = 1
	// The Env using data-level keys.
	// Used as the default rocksdb Env when encryption is enabled.
	EnvType_Data EnvType = 2
)

var EnvType_name = map[int32]string{
	0: "Plaintext",
	1: "Store",
	2: "Data",
}
var EnvType_value = map[string]int32{
	"Plaintext": 0,
	"Store":     1,
	"Data":      2,
}

func (x EnvType) String() string {
	return proto.EnumName(EnvType_name, int32(x))
}
func (EnvType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_file_registry_22f58cb8ad5a5179, []int{1}
}

// Registry describes how a files are handled. This includes the
// rockdb::Env responsible for each file as well as opaque env details.
type FileRegistry struct {
	// version is currently always Base.
	Version RegistryVersion `protobuf:"varint,1,opt,name=version,proto3,enum=znbase.storage.engine.enginepb.RegistryVersion" json:"version,omitempty"`
	// Map of filename -> FileEntry.
	// Filename is relative to the rocksdb dir if the file is inside it.
	// Otherwise it is an absolute path.
	// TODO(mberhault): figure out if we need anything special for Windows.
	Files map[string]*FileEntry `protobuf:"bytes,2,rep,name=files,proto3" json:"files,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (m *FileRegistry) Reset()         { *m = FileRegistry{} }
func (m *FileRegistry) String() string { return proto.CompactTextString(m) }
func (*FileRegistry) ProtoMessage()    {}
func (*FileRegistry) Descriptor() ([]byte, []int) {
	return fileDescriptor_file_registry_22f58cb8ad5a5179, []int{0}
}
func (m *FileRegistry) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *FileRegistry) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *FileRegistry) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FileRegistry.Merge(dst, src)
}
func (m *FileRegistry) XXX_Size() int {
	return m.Size()
}
func (m *FileRegistry) XXX_DiscardUnknown() {
	xxx_messageInfo_FileRegistry.DiscardUnknown(m)
}

var xxx_messageInfo_FileRegistry proto.InternalMessageInfo

type FileEntry struct {
	// Env type identifies which rocksdb::Env is responsible for this file.
	EnvType EnvType `protobuf:"varint,1,opt,name=env_type,json=envType,proto3,enum=znbase.storage.engine.enginepb.EnvType" json:"env_type,omitempty"`
	// Env-specific fields for non-0 env. These are known by ICL code only.
	// This is a serialized protobuf. We cannot use protobuf.Any since we use
	// MessageLite in C++.
	EncryptionSettings []byte `protobuf:"bytes,2,opt,name=encryption_settings,json=encryptionSettings,proto3" json:"encryption_settings,omitempty"`
}

func (m *FileEntry) Reset()         { *m = FileEntry{} }
func (m *FileEntry) String() string { return proto.CompactTextString(m) }
func (*FileEntry) ProtoMessage()    {}
func (*FileEntry) Descriptor() ([]byte, []int) {
	return fileDescriptor_file_registry_22f58cb8ad5a5179, []int{1}
}
func (m *FileEntry) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *FileEntry) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *FileEntry) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FileEntry.Merge(dst, src)
}
func (m *FileEntry) XXX_Size() int {
	return m.Size()
}
func (m *FileEntry) XXX_DiscardUnknown() {
	xxx_messageInfo_FileEntry.DiscardUnknown(m)
}

var xxx_messageInfo_FileEntry proto.InternalMessageInfo

func init() {
	proto.RegisterType((*FileRegistry)(nil), "znbase.storage.engine.enginepb.FileRegistry")
	proto.RegisterMapType((map[string]*FileEntry)(nil), "znbase.storage.engine.enginepb.FileRegistry.FilesEntry")
	proto.RegisterType((*FileEntry)(nil), "znbase.storage.engine.enginepb.FileEntry")
	proto.RegisterEnum("znbase.storage.engine.enginepb.RegistryVersion", RegistryVersion_name, RegistryVersion_value)
	proto.RegisterEnum("znbase.storage.engine.enginepb.EnvType", EnvType_name, EnvType_value)
}
func (m *FileRegistry) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *FileRegistry) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.Version != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintFileRegistry(dAtA, i, uint64(m.Version))
	}
	if len(m.Files) > 0 {
		keysForFiles := make([]string, 0, len(m.Files))
		for k := range m.Files {
			keysForFiles = append(keysForFiles, string(k))
		}
		github_com_gogo_protobuf_sortkeys.Strings(keysForFiles)
		for _, k := range keysForFiles {
			dAtA[i] = 0x12
			i++
			v := m.Files[string(k)]
			msgSize := 0
			if v != nil {
				msgSize = v.Size()
				msgSize += 1 + sovFileRegistry(uint64(msgSize))
			}
			mapSize := 1 + len(k) + sovFileRegistry(uint64(len(k))) + msgSize
			i = encodeVarintFileRegistry(dAtA, i, uint64(mapSize))
			dAtA[i] = 0xa
			i++
			i = encodeVarintFileRegistry(dAtA, i, uint64(len(k)))
			i += copy(dAtA[i:], k)
			if v != nil {
				dAtA[i] = 0x12
				i++
				i = encodeVarintFileRegistry(dAtA, i, uint64(v.Size()))
				n1, err := v.MarshalTo(dAtA[i:])
				if err != nil {
					return 0, err
				}
				i += n1
			}
		}
	}
	return i, nil
}

func (m *FileEntry) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *FileEntry) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if m.EnvType != 0 {
		dAtA[i] = 0x8
		i++
		i = encodeVarintFileRegistry(dAtA, i, uint64(m.EnvType))
	}
	if len(m.EncryptionSettings) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintFileRegistry(dAtA, i, uint64(len(m.EncryptionSettings)))
		i += copy(dAtA[i:], m.EncryptionSettings)
	}
	return i, nil
}

func encodeVarintFileRegistry(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *FileRegistry) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Version != 0 {
		n += 1 + sovFileRegistry(uint64(m.Version))
	}
	if len(m.Files) > 0 {
		for k, v := range m.Files {
			_ = k
			_ = v
			l = 0
			if v != nil {
				l = v.Size()
				l += 1 + sovFileRegistry(uint64(l))
			}
			mapEntrySize := 1 + len(k) + sovFileRegistry(uint64(len(k))) + l
			n += mapEntrySize + 1 + sovFileRegistry(uint64(mapEntrySize))
		}
	}
	return n
}

func (m *FileEntry) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.EnvType != 0 {
		n += 1 + sovFileRegistry(uint64(m.EnvType))
	}
	l = len(m.EncryptionSettings)
	if l > 0 {
		n += 1 + l + sovFileRegistry(uint64(l))
	}
	return n
}

func sovFileRegistry(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozFileRegistry(x uint64) (n int) {
	return sovFileRegistry(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *FileRegistry) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowFileRegistry
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: FileRegistry: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: FileRegistry: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Version", wireType)
			}
			m.Version = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowFileRegistry
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Version |= (RegistryVersion(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Files", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowFileRegistry
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthFileRegistry
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Files == nil {
				m.Files = make(map[string]*FileEntry)
			}
			var mapkey string
			var mapvalue *FileEntry
			for iNdEx < postIndex {
				entryPreIndex := iNdEx
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return ErrIntOverflowFileRegistry
					}
					if iNdEx >= l {
						return io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					wire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)
				if fieldNum == 1 {
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowFileRegistry
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapkey |= (uint64(b) & 0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return ErrInvalidLengthFileRegistry
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey > l {
						return io.ErrUnexpectedEOF
					}
					mapkey = string(dAtA[iNdEx:postStringIndexmapkey])
					iNdEx = postStringIndexmapkey
				} else if fieldNum == 2 {
					var mapmsglen int
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowFileRegistry
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						mapmsglen |= (int(b) & 0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					if mapmsglen < 0 {
						return ErrInvalidLengthFileRegistry
					}
					postmsgIndex := iNdEx + mapmsglen
					if mapmsglen < 0 {
						return ErrInvalidLengthFileRegistry
					}
					if postmsgIndex > l {
						return io.ErrUnexpectedEOF
					}
					mapvalue = &FileEntry{}
					if err := mapvalue.Unmarshal(dAtA[iNdEx:postmsgIndex]); err != nil {
						return err
					}
					iNdEx = postmsgIndex
				} else {
					iNdEx = entryPreIndex
					skippy, err := skipFileRegistry(dAtA[iNdEx:])
					if err != nil {
						return err
					}
					if skippy < 0 {
						return ErrInvalidLengthFileRegistry
					}
					if (iNdEx + skippy) > postIndex {
						return io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}
			m.Files[mapkey] = mapvalue
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipFileRegistry(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthFileRegistry
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *FileEntry) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowFileRegistry
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: FileEntry: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: FileEntry: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field EnvType", wireType)
			}
			m.EnvType = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowFileRegistry
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.EnvType |= (EnvType(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field EncryptionSettings", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowFileRegistry
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthFileRegistry
			}
			postIndex := iNdEx + byteLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.EncryptionSettings = append(m.EncryptionSettings[:0], dAtA[iNdEx:postIndex]...)
			if m.EncryptionSettings == nil {
				m.EncryptionSettings = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipFileRegistry(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthFileRegistry
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipFileRegistry(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowFileRegistry
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowFileRegistry
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
			return iNdEx, nil
		case 1:
			iNdEx += 8
			return iNdEx, nil
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowFileRegistry
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			iNdEx += length
			if length < 0 {
				return 0, ErrInvalidLengthFileRegistry
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowFileRegistry
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					innerWire |= (uint64(b) & 0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				innerWireType := int(innerWire & 0x7)
				if innerWireType == 4 {
					break
				}
				next, err := skipFileRegistry(dAtA[start:])
				if err != nil {
					return 0, err
				}
				iNdEx = start + next
			}
			return iNdEx, nil
		case 4:
			return iNdEx, nil
		case 5:
			iNdEx += 4
			return iNdEx, nil
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
	}
	panic("unreachable")
}

var (
	ErrInvalidLengthFileRegistry = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowFileRegistry   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("storage/engine/enginepb/file_registry.proto", fileDescriptor_file_registry_22f58cb8ad5a5179)
}

var fileDescriptor_file_registry_22f58cb8ad5a5179 = []byte{
	// 367 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x92, 0xcf, 0x6a, 0xdb, 0x40,
	0x10, 0xc6, 0xb5, 0x72, 0x5d, 0x5b, 0x63, 0xb7, 0x15, 0xdb, 0x8b, 0x69, 0x61, 0x31, 0xbe, 0xd4,
	0x75, 0xa9, 0x04, 0xee, 0xa1, 0x21, 0x97, 0x80, 0x89, 0x03, 0x39, 0x04, 0x82, 0x1c, 0x72, 0xc8,
	0xc5, 0xc8, 0x66, 0x22, 0x96, 0x88, 0x95, 0xd8, 0xdd, 0x88, 0x28, 0xa7, 0xbc, 0x40, 0x20, 0x8f,
	0xe5, 0xa3, 0x8f, 0x3e, 0x26, 0xf2, 0x8b, 0x04, 0xeb, 0x4f, 0x1c, 0x72, 0x88, 0x73, 0xda, 0xd9,
	0x9d, 0xf9, 0x7d, 0xdf, 0xc7, 0xb0, 0xf0, 0x47, 0xe9, 0x48, 0xfa, 0x01, 0xba, 0x28, 0x02, 0x2e,
	0xaa, 0x23, 0x9e, 0xb9, 0x97, 0x3c, 0xc4, 0xa9, 0xc4, 0x80, 0x2b, 0x2d, 0x53, 0x27, 0x96, 0x91,
	0x8e, 0x28, 0xbb, 0x15, 0x33, 0x5f, 0xa1, 0x53, 0x32, 0x4e, 0x31, 0xec, 0x54, 0x4c, 0xef, 0xde,
	0x84, 0xf6, 0x11, 0x0f, 0xd1, 0x2b, 0x31, 0x7a, 0x0c, 0x8d, 0x04, 0xa5, 0xe2, 0x91, 0xe8, 0x90,
	0x2e, 0xe9, 0x7f, 0x1d, 0xba, 0xce, 0xfb, 0x12, 0x4e, 0x85, 0x9e, 0x17, 0x98, 0x57, 0xf1, 0xf4,
	0x04, 0xea, 0x9b, 0x48, 0xaa, 0x63, 0x76, 0x6b, 0xfd, 0xd6, 0xf0, 0xff, 0x2e, 0xa1, 0xd7, 0x39,
	0xf2, 0x8b, 0x1a, 0x0b, 0x2d, 0x53, 0xaf, 0x50, 0xf9, 0x31, 0x07, 0xd8, 0x3e, 0x52, 0x1b, 0x6a,
	0x57, 0x98, 0xe6, 0x19, 0x2d, 0x6f, 0x53, 0xd2, 0x03, 0xa8, 0x27, 0x7e, 0x78, 0x8d, 0x1d, 0xb3,
	0x4b, 0xfa, 0xad, 0xe1, 0xef, 0x8f, 0xd8, 0x95, 0x06, 0x39, 0xb7, 0x6f, 0xee, 0x91, 0xde, 0x1d,
	0x01, 0xeb, 0xa5, 0x41, 0x47, 0xd0, 0x44, 0x91, 0x4c, 0x75, 0x1a, 0x63, 0xb9, 0x8d, 0x5f, 0xbb,
	0x54, 0xc7, 0x22, 0x39, 0x4b, 0x63, 0xf4, 0x1a, 0x58, 0x14, 0xd4, 0x85, 0xef, 0x28, 0xe6, 0x32,
	0x8d, 0x35, 0x8f, 0xc4, 0x54, 0xa1, 0xd6, 0x5c, 0x04, 0x2a, 0x0f, 0xd9, 0xf6, 0xe8, 0xb6, 0x35,
	0x29, 0x3b, 0x83, 0x9f, 0xf0, 0xed, 0xcd, 0x4a, 0x69, 0x13, 0x3e, 0x8d, 0x7c, 0x85, 0xb6, 0x31,
	0xf8, 0x0b, 0x8d, 0xd2, 0x81, 0x7e, 0x01, 0xeb, 0x34, 0xf4, 0xb9, 0xd0, 0x78, 0xa3, 0x6d, 0x83,
	0x5a, 0x50, 0x9f, 0xe8, 0x48, 0xa2, 0x4d, 0x36, 0xe3, 0x87, 0xbe, 0xf6, 0x6d, 0x73, 0x34, 0x58,
	0x3c, 0x31, 0x63, 0x91, 0x31, 0xb2, 0xcc, 0x18, 0x59, 0x65, 0x8c, 0x3c, 0x66, 0x8c, 0x3c, 0xac,
	0x99, 0xb1, 0x5c, 0x33, 0x63, 0xb5, 0x66, 0xc6, 0x45, 0xb3, 0x4a, 0x3e, 0xfb, 0x9c, 0xff, 0x98,
	0x7f, 0xcf, 0x01, 0x00, 0x00, 0xff, 0xff, 0xca, 0xc0, 0xba, 0xd0, 0x60, 0x02, 0x00, 0x00,
}
