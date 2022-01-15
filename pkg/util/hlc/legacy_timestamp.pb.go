// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: util/hlc/legacy_timestamp.proto

package hlc

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

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

// LegacyTimestamp is convertible to hlc.Timestamp, but uses the
// legacy encoding as it is encoded "below raft".
type LegacyTimestamp struct {
	// Holds a wall time, typically a unix epoch time expressed in
	// nanoseconds.
	WallTime int64 `protobuf:"varint,1,opt,name=wall_time,json=wallTime" json:"wall_time"`
	// The logical component captures causality for events whose wall
	// times are equal. It is effectively bounded by (maximum clock
	// skew)/(minimal ns between events) and nearly impossible to
	// overflow.
	Logical int32 `protobuf:"varint,2,opt,name=logical" json:"logical"`
}

func (m *LegacyTimestamp) Reset()      { *m = LegacyTimestamp{} }
func (*LegacyTimestamp) ProtoMessage() {}
func (*LegacyTimestamp) Descriptor() ([]byte, []int) {
	return fileDescriptor_legacy_timestamp_15a7be1dce2b80b1, []int{0}
}
func (m *LegacyTimestamp) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *LegacyTimestamp) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *LegacyTimestamp) XXX_Merge(src proto.Message) {
	xxx_messageInfo_LegacyTimestamp.Merge(dst, src)
}
func (m *LegacyTimestamp) XXX_Size() int {
	return m.Size()
}
func (m *LegacyTimestamp) XXX_DiscardUnknown() {
	xxx_messageInfo_LegacyTimestamp.DiscardUnknown(m)
}

var xxx_messageInfo_LegacyTimestamp proto.InternalMessageInfo

func init() {
	proto.RegisterType((*LegacyTimestamp)(nil), "znbase.util.hlc.LegacyTimestamp")
}
func (this *LegacyTimestamp) Equal(that interface{}) bool {
	if that == nil {
		return this == nil
	}

	that1, ok := that.(*LegacyTimestamp)
	if !ok {
		that2, ok := that.(LegacyTimestamp)
		if ok {
			that1 = &that2
		} else {
			return false
		}
	}
	if that1 == nil {
		return this == nil
	} else if this == nil {
		return false
	}
	if this.WallTime != that1.WallTime {
		return false
	}
	if this.Logical != that1.Logical {
		return false
	}
	return true
}
func (m *LegacyTimestamp) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *LegacyTimestamp) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0x8
	i++
	i = encodeVarintLegacyTimestamp(dAtA, i, uint64(m.WallTime))
	dAtA[i] = 0x10
	i++
	i = encodeVarintLegacyTimestamp(dAtA, i, uint64(m.Logical))
	return i, nil
}

func encodeVarintLegacyTimestamp(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func NewPopulatedLegacyTimestamp(r randyLegacyTimestamp, easy bool) *LegacyTimestamp {
	this := &LegacyTimestamp{}
	this.WallTime = int64(r.Int63())
	if r.Intn(2) == 0 {
		this.WallTime *= -1
	}
	this.Logical = int32(r.Int31())
	if r.Intn(2) == 0 {
		this.Logical *= -1
	}
	if !easy && r.Intn(10) != 0 {
	}
	return this
}

type randyLegacyTimestamp interface {
	Float32() float32
	Float64() float64
	Int63() int64
	Int31() int32
	Uint32() uint32
	Intn(n int) int
}

func randUTF8RuneLegacyTimestamp(r randyLegacyTimestamp) rune {
	ru := r.Intn(62)
	if ru < 10 {
		return rune(ru + 48)
	} else if ru < 36 {
		return rune(ru + 55)
	}
	return rune(ru + 61)
}
func randStringLegacyTimestamp(r randyLegacyTimestamp) string {
	v1 := r.Intn(100)
	tmps := make([]rune, v1)
	for i := 0; i < v1; i++ {
		tmps[i] = randUTF8RuneLegacyTimestamp(r)
	}
	return string(tmps)
}
func randUnrecognizedLegacyTimestamp(r randyLegacyTimestamp, maxFieldNumber int) (dAtA []byte) {
	l := r.Intn(5)
	for i := 0; i < l; i++ {
		wire := r.Intn(4)
		if wire == 3 {
			wire = 5
		}
		fieldNumber := maxFieldNumber + r.Intn(100)
		dAtA = randFieldLegacyTimestamp(dAtA, r, fieldNumber, wire)
	}
	return dAtA
}
func randFieldLegacyTimestamp(dAtA []byte, r randyLegacyTimestamp, fieldNumber int, wire int) []byte {
	key := uint32(fieldNumber)<<3 | uint32(wire)
	switch wire {
	case 0:
		dAtA = encodeVarintPopulateLegacyTimestamp(dAtA, uint64(key))
		v2 := r.Int63()
		if r.Intn(2) == 0 {
			v2 *= -1
		}
		dAtA = encodeVarintPopulateLegacyTimestamp(dAtA, uint64(v2))
	case 1:
		dAtA = encodeVarintPopulateLegacyTimestamp(dAtA, uint64(key))
		dAtA = append(dAtA, byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)))
	case 2:
		dAtA = encodeVarintPopulateLegacyTimestamp(dAtA, uint64(key))
		ll := r.Intn(100)
		dAtA = encodeVarintPopulateLegacyTimestamp(dAtA, uint64(ll))
		for j := 0; j < ll; j++ {
			dAtA = append(dAtA, byte(r.Intn(256)))
		}
	default:
		dAtA = encodeVarintPopulateLegacyTimestamp(dAtA, uint64(key))
		dAtA = append(dAtA, byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)))
	}
	return dAtA
}
func encodeVarintPopulateLegacyTimestamp(dAtA []byte, v uint64) []byte {
	for v >= 1<<7 {
		dAtA = append(dAtA, uint8(uint64(v)&0x7f|0x80))
		v >>= 7
	}
	dAtA = append(dAtA, uint8(v))
	return dAtA
}
func (m *LegacyTimestamp) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	n += 1 + sovLegacyTimestamp(uint64(m.WallTime))
	n += 1 + sovLegacyTimestamp(uint64(m.Logical))
	return n
}

func sovLegacyTimestamp(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozLegacyTimestamp(x uint64) (n int) {
	return sovLegacyTimestamp(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *LegacyTimestamp) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowLegacyTimestamp
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
			return fmt.Errorf("proto: LegacyTimestamp: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: LegacyTimestamp: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field WallTime", wireType)
			}
			m.WallTime = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowLegacyTimestamp
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.WallTime |= (int64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Logical", wireType)
			}
			m.Logical = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowLegacyTimestamp
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Logical |= (int32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipLegacyTimestamp(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthLegacyTimestamp
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
func skipLegacyTimestamp(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowLegacyTimestamp
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
					return 0, ErrIntOverflowLegacyTimestamp
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
					return 0, ErrIntOverflowLegacyTimestamp
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
				return 0, ErrInvalidLengthLegacyTimestamp
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowLegacyTimestamp
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
				next, err := skipLegacyTimestamp(dAtA[start:])
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
	ErrInvalidLengthLegacyTimestamp = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowLegacyTimestamp   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("util/hlc/legacy_timestamp.proto", fileDescriptor_legacy_timestamp_15a7be1dce2b80b1)
}

var fileDescriptor_legacy_timestamp_15a7be1dce2b80b1 = []byte{
	// 200 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x92, 0x2f, 0x2d, 0xc9, 0xcc,
	0xd1, 0xcf, 0xc8, 0x49, 0xd6, 0xcf, 0x49, 0x4d, 0x4f, 0x4c, 0xae, 0x8c, 0x2f, 0xc9, 0xcc, 0x4d,
	0x2d, 0x2e, 0x49, 0xcc, 0x2d, 0xd0, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0xe2, 0xaf, 0xca, 0x4b,
	0x4a, 0x2c, 0x4e, 0xd5, 0x03, 0xa9, 0xd3, 0xcb, 0xc8, 0x49, 0x96, 0x12, 0x49, 0xcf, 0x4f, 0xcf,
	0x07, 0xcb, 0xe9, 0x83, 0x58, 0x10, 0x65, 0x4a, 0x49, 0x5c, 0xfc, 0x3e, 0x60, 0x03, 0x42, 0x60,
	0xfa, 0x85, 0x14, 0xb9, 0x38, 0xcb, 0x13, 0x73, 0x72, 0xc0, 0x26, 0x4a, 0x30, 0x2a, 0x30, 0x6a,
	0x30, 0x3b, 0xb1, 0x9c, 0xb8, 0x27, 0xcf, 0x10, 0xc4, 0x01, 0x12, 0x06, 0xa9, 0x13, 0x92, 0xe3,
	0x62, 0xcf, 0xc9, 0x4f, 0xcf, 0x4c, 0x4e, 0xcc, 0x91, 0x60, 0x52, 0x60, 0xd4, 0x60, 0x85, 0x2a,
	0x80, 0x09, 0x5a, 0xf1, 0xcc, 0x58, 0x20, 0xcf, 0xb0, 0x63, 0x81, 0x3c, 0xe3, 0x8b, 0x05, 0xf2,
	0x8c, 0x4e, 0xaa, 0x27, 0x1e, 0xca, 0x31, 0x9c, 0x78, 0x24, 0xc7, 0x78, 0xe1, 0x91, 0x1c, 0xe3,
	0x8d, 0x47, 0x72, 0x8c, 0x0f, 0x1e, 0xc9, 0x31, 0x4e, 0x78, 0x2c, 0xc7, 0x70, 0xe1, 0xb1, 0x1c,
	0xc3, 0x8d, 0xc7, 0x72, 0x0c, 0x51, 0xcc, 0x19, 0x39, 0xc9, 0x80, 0x00, 0x00, 0x00, 0xff, 0xff,
	0x4a, 0xaf, 0x60, 0x81, 0xd3, 0x00, 0x00, 0x00,
}