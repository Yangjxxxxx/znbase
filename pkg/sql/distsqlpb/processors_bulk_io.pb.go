// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: sql/distsqlpb/processors_bulk_io.proto

package distsqlpb

/*
	Beware! This package name must not be changed, even though it doesn't match
	the Go package name, because it defines the Protobuf message names which
	can't be changed without breaking backward compatibility.
*/

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"
import sqlbase "github.com/znbasedb/znbase/pkg/sql/sqlbase"

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

// BulkRowWriterSpec is the specification for a processor that consumes rows and
// writes them to a target table using AddSSTable. It outputs a BulkOpSummary.
type BulkRowWriterSpec struct {
	Table sqlbase.TableDescriptor `protobuf:"bytes,1,opt,name=table" json:"table"`
}

func (m *BulkRowWriterSpec) Reset()         { *m = BulkRowWriterSpec{} }
func (m *BulkRowWriterSpec) String() string { return proto.CompactTextString(m) }
func (*BulkRowWriterSpec) ProtoMessage()    {}
func (*BulkRowWriterSpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_processors_bulk_io_e79a6144b0b4d19c, []int{0}
}
func (m *BulkRowWriterSpec) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *BulkRowWriterSpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *BulkRowWriterSpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BulkRowWriterSpec.Merge(dst, src)
}
func (m *BulkRowWriterSpec) XXX_Size() int {
	return m.Size()
}
func (m *BulkRowWriterSpec) XXX_DiscardUnknown() {
	xxx_messageInfo_BulkRowWriterSpec.DiscardUnknown(m)
}

var xxx_messageInfo_BulkRowWriterSpec proto.InternalMessageInfo

func init() {
	proto.RegisterType((*BulkRowWriterSpec)(nil), "znbase.sql.distsqlrun.BulkRowWriterSpec")
}
func (m *BulkRowWriterSpec) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *BulkRowWriterSpec) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0xa
	i++
	i = encodeVarintProcessorsBulkIo(dAtA, i, uint64(m.Table.Size()))
	n1, err := m.Table.MarshalTo(dAtA[i:])
	if err != nil {
		return 0, err
	}
	i += n1
	return i, nil
}

func encodeVarintProcessorsBulkIo(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *BulkRowWriterSpec) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = m.Table.Size()
	n += 1 + l + sovProcessorsBulkIo(uint64(l))
	return n
}

func sovProcessorsBulkIo(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozProcessorsBulkIo(x uint64) (n int) {
	return sovProcessorsBulkIo(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *BulkRowWriterSpec) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowProcessorsBulkIo
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
			return fmt.Errorf("proto: BulkRowWriterSpec: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: BulkRowWriterSpec: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Table", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowProcessorsBulkIo
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
				return ErrInvalidLengthProcessorsBulkIo
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if err := m.Table.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipProcessorsBulkIo(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthProcessorsBulkIo
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
func skipProcessorsBulkIo(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowProcessorsBulkIo
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
					return 0, ErrIntOverflowProcessorsBulkIo
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
					return 0, ErrIntOverflowProcessorsBulkIo
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
				return 0, ErrInvalidLengthProcessorsBulkIo
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowProcessorsBulkIo
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
				next, err := skipProcessorsBulkIo(dAtA[start:])
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
	ErrInvalidLengthProcessorsBulkIo = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowProcessorsBulkIo   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("sql/distsqlpb/processors_bulk_io.proto", fileDescriptor_processors_bulk_io_e79a6144b0b4d19c)
}

var fileDescriptor_processors_bulk_io_e79a6144b0b4d19c = []byte{
	// 223 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x52, 0x2b, 0x2e, 0xcc, 0xd1,
	0x4f, 0xc9, 0x2c, 0x2e, 0x29, 0x2e, 0xcc, 0x29, 0x48, 0xd2, 0x2f, 0x28, 0xca, 0x4f, 0x4e, 0x2d,
	0x2e, 0xce, 0x2f, 0x2a, 0x8e, 0x4f, 0x2a, 0xcd, 0xc9, 0x8e, 0xcf, 0xcc, 0xd7, 0x2b, 0x28, 0xca,
	0x2f, 0xc9, 0x17, 0x12, 0xad, 0xca, 0x4b, 0x4a, 0x2c, 0x4e, 0xd5, 0x2b, 0x2e, 0xcc, 0xd1, 0x83,
	0x2a, 0x2f, 0x2a, 0xcd, 0x93, 0x92, 0x01, 0x69, 0x2f, 0x2e, 0xcc, 0x01, 0xc9, 0xe9, 0x17, 0x97,
	0x14, 0x95, 0x26, 0x97, 0x94, 0x16, 0xa5, 0xa6, 0x40, 0x34, 0x49, 0x89, 0xa4, 0xe7, 0xa7, 0xe7,
	0x83, 0x99, 0xfa, 0x20, 0x16, 0x44, 0x54, 0x29, 0x84, 0x4b, 0xd0, 0xa9, 0x34, 0x27, 0x3b, 0x28,
	0xbf, 0x3c, 0xbc, 0x28, 0xb3, 0x24, 0xb5, 0x28, 0xb8, 0x20, 0x35, 0x59, 0xc8, 0x9e, 0x8b, 0xb5,
	0x24, 0x31, 0x29, 0x27, 0x55, 0x82, 0x51, 0x81, 0x51, 0x83, 0xdb, 0x48, 0x59, 0x0f, 0xc9, 0x3e,
	0xa8, 0xf9, 0x7a, 0x21, 0x20, 0x05, 0x2e, 0xa9, 0xc5, 0xc9, 0x45, 0x99, 0x05, 0x25, 0xf9, 0x45,
	0x4e, 0x2c, 0x27, 0xee, 0xc9, 0x33, 0x04, 0x41, 0xf4, 0x39, 0x69, 0x9f, 0x78, 0x28, 0xc7, 0x70,
	0xe2, 0x91, 0x1c, 0xe3, 0x85, 0x47, 0x72, 0x8c, 0x37, 0x1e, 0xc9, 0x31, 0x3e, 0x78, 0x24, 0xc7,
	0x38, 0xe1, 0xb1, 0x1c, 0xc3, 0x85, 0xc7, 0x72, 0x0c, 0x37, 0x1e, 0xcb, 0x31, 0x44, 0x71, 0xc2,
	0xbd, 0x09, 0x08, 0x00, 0x00, 0xff, 0xff, 0x2c, 0x69, 0xa7, 0xcf, 0xf6, 0x00, 0x00, 0x00,
}
