// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: acceptance/cluster/testconfig.proto

package cluster

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import time "time"

import io "io"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

// InitMode specifies different ways to initialize the cluster.
type InitMode int32

const (
	// INIT_COMMAND starts every node with a join flag and issues the
	// init command.
	INIT_COMMAND InitMode = 0
	// INIT_BOOTSTRAP_NODE_ZERO uses the legacy protocol of omitting the
	// join flag from node zero.
	INIT_BOOTSTRAP_NODE_ZERO InitMode = 1
	// INIT_NONE starts every node with a join flag and leaves the
	// cluster uninitialized.
	INIT_NONE InitMode = 2
)

var InitMode_name = map[int32]string{
	0: "INIT_COMMAND",
	1: "INIT_BOOTSTRAP_NODE_ZERO",
	2: "INIT_NONE",
}
var InitMode_value = map[string]int32{
	"INIT_COMMAND":             0,
	"INIT_BOOTSTRAP_NODE_ZERO": 1,
	"INIT_NONE":                2,
}

func (x InitMode) Enum() *InitMode {
	p := new(InitMode)
	*p = x
	return p
}
func (x InitMode) String() string {
	return proto.EnumName(InitMode_name, int32(x))
}
func (x *InitMode) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(InitMode_value, data, "InitMode")
	if err != nil {
		return err
	}
	*x = InitMode(value)
	return nil
}
func (InitMode) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_testconfig_969b2aee6f6c7de0, []int{0}
}

// StoreConfig holds the configuration of a collection of similar stores.
type StoreConfig struct {
	MaxRanges int32 `protobuf:"varint,2,opt,name=max_ranges,json=maxRanges" json:"max_ranges"`
}

func (m *StoreConfig) Reset()         { *m = StoreConfig{} }
func (m *StoreConfig) String() string { return proto.CompactTextString(m) }
func (*StoreConfig) ProtoMessage()    {}
func (*StoreConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_testconfig_969b2aee6f6c7de0, []int{0}
}
func (m *StoreConfig) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *StoreConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *StoreConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StoreConfig.Merge(dst, src)
}
func (m *StoreConfig) XXX_Size() int {
	return m.Size()
}
func (m *StoreConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_StoreConfig.DiscardUnknown(m)
}

var xxx_messageInfo_StoreConfig proto.InternalMessageInfo

// NodeConfig holds the configuration of a collection of similar nodes.
type NodeConfig struct {
	Version string        `protobuf:"bytes,1,opt,name=version" json:"version"`
	Stores  []StoreConfig `protobuf:"bytes,2,rep,name=stores" json:"stores"`
}

func (m *NodeConfig) Reset()         { *m = NodeConfig{} }
func (m *NodeConfig) String() string { return proto.CompactTextString(m) }
func (*NodeConfig) ProtoMessage()    {}
func (*NodeConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_testconfig_969b2aee6f6c7de0, []int{1}
}
func (m *NodeConfig) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *NodeConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *NodeConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_NodeConfig.Merge(dst, src)
}
func (m *NodeConfig) XXX_Size() int {
	return m.Size()
}
func (m *NodeConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_NodeConfig.DiscardUnknown(m)
}

var xxx_messageInfo_NodeConfig proto.InternalMessageInfo

type TestConfig struct {
	Name  string       `protobuf:"bytes,1,opt,name=name" json:"name"`
	Nodes []NodeConfig `protobuf:"bytes,2,rep,name=nodes" json:"nodes"`
	// Duration is the total time that the test should run for. Important for
	// tests such as TestPut that will run indefinitely.
	Duration time.Duration `protobuf:"varint,3,opt,name=duration,casttype=time.Duration" json:"duration"`
	InitMode InitMode      `protobuf:"varint,4,opt,name=init_mode,json=initMode,enum=znbase.acceptance.cluster.InitMode" json:"init_mode"`
	// When set, the cluster is started as quickly as possible, without waiting
	// for ranges to replicate, or even ports to be opened.
	NoWait bool `protobuf:"varint,5,opt,name=no_wait,json=noWait" json:"no_wait"`
}

func (m *TestConfig) Reset()         { *m = TestConfig{} }
func (m *TestConfig) String() string { return proto.CompactTextString(m) }
func (*TestConfig) ProtoMessage()    {}
func (*TestConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_testconfig_969b2aee6f6c7de0, []int{2}
}
func (m *TestConfig) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *TestConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *TestConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TestConfig.Merge(dst, src)
}
func (m *TestConfig) XXX_Size() int {
	return m.Size()
}
func (m *TestConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_TestConfig.DiscardUnknown(m)
}

var xxx_messageInfo_TestConfig proto.InternalMessageInfo

func init() {
	proto.RegisterType((*StoreConfig)(nil), "znbase.acceptance.cluster.StoreConfig")
	proto.RegisterType((*NodeConfig)(nil), "znbase.acceptance.cluster.NodeConfig")
	proto.RegisterType((*TestConfig)(nil), "znbase.acceptance.cluster.TestConfig")
	proto.RegisterEnum("znbase.acceptance.cluster.InitMode", InitMode_name, InitMode_value)
}
func (m *StoreConfig) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *StoreConfig) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0x10
	i++
	i = encodeVarintTestconfig(dAtA, i, uint64(m.MaxRanges))
	return i, nil
}

func (m *NodeConfig) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *NodeConfig) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0xa
	i++
	i = encodeVarintTestconfig(dAtA, i, uint64(len(m.Version)))
	i += copy(dAtA[i:], m.Version)
	if len(m.Stores) > 0 {
		for _, msg := range m.Stores {
			dAtA[i] = 0x12
			i++
			i = encodeVarintTestconfig(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	return i, nil
}

func (m *TestConfig) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *TestConfig) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	dAtA[i] = 0xa
	i++
	i = encodeVarintTestconfig(dAtA, i, uint64(len(m.Name)))
	i += copy(dAtA[i:], m.Name)
	if len(m.Nodes) > 0 {
		for _, msg := range m.Nodes {
			dAtA[i] = 0x12
			i++
			i = encodeVarintTestconfig(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	dAtA[i] = 0x18
	i++
	i = encodeVarintTestconfig(dAtA, i, uint64(m.Duration))
	dAtA[i] = 0x20
	i++
	i = encodeVarintTestconfig(dAtA, i, uint64(m.InitMode))
	dAtA[i] = 0x28
	i++
	if m.NoWait {
		dAtA[i] = 1
	} else {
		dAtA[i] = 0
	}
	i++
	return i, nil
}

func encodeVarintTestconfig(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *StoreConfig) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	n += 1 + sovTestconfig(uint64(m.MaxRanges))
	return n
}

func (m *NodeConfig) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Version)
	n += 1 + l + sovTestconfig(uint64(l))
	if len(m.Stores) > 0 {
		for _, e := range m.Stores {
			l = e.Size()
			n += 1 + l + sovTestconfig(uint64(l))
		}
	}
	return n
}

func (m *TestConfig) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Name)
	n += 1 + l + sovTestconfig(uint64(l))
	if len(m.Nodes) > 0 {
		for _, e := range m.Nodes {
			l = e.Size()
			n += 1 + l + sovTestconfig(uint64(l))
		}
	}
	n += 1 + sovTestconfig(uint64(m.Duration))
	n += 1 + sovTestconfig(uint64(m.InitMode))
	n += 2
	return n
}

func sovTestconfig(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozTestconfig(x uint64) (n int) {
	return sovTestconfig(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *StoreConfig) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTestconfig
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
			return fmt.Errorf("proto: StoreConfig: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: StoreConfig: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field MaxRanges", wireType)
			}
			m.MaxRanges = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.MaxRanges |= (int32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipTestconfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthTestconfig
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
func (m *NodeConfig) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTestconfig
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
			return fmt.Errorf("proto: NodeConfig: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: NodeConfig: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Version", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTestconfig
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Version = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Stores", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
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
				return ErrInvalidLengthTestconfig
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Stores = append(m.Stores, StoreConfig{})
			if err := m.Stores[len(m.Stores)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTestconfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthTestconfig
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
func (m *TestConfig) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTestconfig
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
			return fmt.Errorf("proto: TestConfig: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: TestConfig: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Name", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= (uint64(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTestconfig
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Name = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Nodes", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
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
				return ErrInvalidLengthTestconfig
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Nodes = append(m.Nodes, NodeConfig{})
			if err := m.Nodes[len(m.Nodes)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Duration", wireType)
			}
			m.Duration = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Duration |= (time.Duration(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field InitMode", wireType)
			}
			m.InitMode = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.InitMode |= (InitMode(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field NoWait", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTestconfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.NoWait = bool(v != 0)
		default:
			iNdEx = preIndex
			skippy, err := skipTestconfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthTestconfig
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
func skipTestconfig(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowTestconfig
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
					return 0, ErrIntOverflowTestconfig
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
					return 0, ErrIntOverflowTestconfig
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
				return 0, ErrInvalidLengthTestconfig
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowTestconfig
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
				next, err := skipTestconfig(dAtA[start:])
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
	ErrInvalidLengthTestconfig = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowTestconfig   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("acceptance/cluster/testconfig.proto", fileDescriptor_testconfig_969b2aee6f6c7de0)
}

var fileDescriptor_testconfig_969b2aee6f6c7de0 = []byte{
	// 422 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0xcd, 0x6e, 0xd3, 0x40,
	0x14, 0x85, 0x3d, 0xf9, 0x21, 0xc9, 0x2d, 0x45, 0xd6, 0x08, 0x24, 0x53, 0xc1, 0xd4, 0x4a, 0x04,
	0x32, 0x2c, 0x1c, 0x91, 0x37, 0x88, 0xeb, 0x20, 0x65, 0x11, 0x1b, 0xb9, 0x96, 0x90, 0xba, 0xb1,
	0x06, 0x7b, 0xb0, 0x46, 0xc2, 0x33, 0x95, 0x3d, 0x85, 0x8a, 0x2d, 0x1b, 0x96, 0xbc, 0x03, 0x2f,
	0x93, 0x65, 0x97, 0x5d, 0x55, 0xe0, 0xbc, 0x05, 0x2b, 0x64, 0x77, 0xdc, 0xc2, 0xa2, 0xdd, 0xd9,
	0xe7, 0x9e, 0x73, 0xbe, 0x7b, 0x07, 0x66, 0x34, 0x4d, 0xd9, 0xa9, 0xa2, 0x22, 0x65, 0xf3, 0xf4,
	0xd3, 0x59, 0xa5, 0x58, 0x39, 0x57, 0xac, 0x52, 0xa9, 0x14, 0x1f, 0x79, 0xee, 0x9e, 0x96, 0x52,
	0x49, 0xfc, 0xf4, 0xab, 0xf8, 0x40, 0x2b, 0xe6, 0xde, 0x7a, 0x5d, 0xed, 0x3d, 0x78, 0x9c, 0xcb,
	0x5c, 0xb6, 0xae, 0x79, 0xf3, 0x75, 0x1d, 0x98, 0x2e, 0x60, 0xef, 0x58, 0xc9, 0x92, 0x1d, 0xb5,
	0x2d, 0x78, 0x06, 0x50, 0xd0, 0xf3, 0xa4, 0xa4, 0x22, 0x67, 0x95, 0xd5, 0xb3, 0x91, 0x33, 0xf4,
	0x06, 0xdb, 0xab, 0x43, 0x23, 0x9a, 0x14, 0xf4, 0x3c, 0x6a, 0xe5, 0x69, 0x09, 0x10, 0xc8, 0xac,
	0x8b, 0x10, 0x18, 0x7d, 0x66, 0x65, 0xc5, 0xa5, 0xb0, 0x90, 0x8d, 0x9c, 0x89, 0xf6, 0x77, 0x22,
	0xf6, 0xe1, 0x41, 0xd5, 0x10, 0x9a, 0xba, 0xbe, 0xb3, 0xb7, 0x78, 0xe9, 0xde, 0xb9, 0xa3, 0xfb,
	0xcf, 0x2a, 0xba, 0x46, 0x67, 0xa7, 0xdf, 0x7a, 0x00, 0x31, 0xab, 0x94, 0x86, 0x5a, 0x30, 0x10,
	0xb4, 0x60, 0xff, 0x11, 0x5b, 0x05, 0x2f, 0x61, 0x28, 0x64, 0x76, 0x43, 0x7b, 0x71, 0x0f, 0xed,
	0xf6, 0x08, 0xdd, 0x70, 0x9d, 0xc4, 0x6f, 0x60, 0x9c, 0x9d, 0x95, 0x54, 0x35, 0x27, 0xf5, 0x6d,
	0xe4, 0xf4, 0xbd, 0x27, 0xcd, 0xf8, 0xcf, 0xd5, 0xe1, 0xbe, 0xe2, 0x05, 0x73, 0x7d, 0x3d, 0x8c,
	0x6e, 0x6c, 0xf8, 0x2d, 0x4c, 0xb8, 0xe0, 0x2a, 0x29, 0x64, 0xc6, 0xac, 0x81, 0x8d, 0x9c, 0x47,
	0x8b, 0xd9, 0x3d, 0xe4, 0xb5, 0xe0, 0x6a, 0x23, 0x33, 0xa6, 0xb9, 0x63, 0xae, 0xff, 0xf1, 0x73,
	0x18, 0x09, 0x99, 0x7c, 0xa1, 0x5c, 0x59, 0x43, 0x1b, 0x39, 0xe3, 0xee, 0x15, 0x84, 0x7c, 0x4f,
	0xb9, 0x7a, 0x1d, 0xc2, 0xb8, 0x8b, 0x62, 0x13, 0x1e, 0xae, 0x83, 0x75, 0x9c, 0x1c, 0x85, 0x9b,
	0xcd, 0x32, 0xf0, 0x4d, 0x03, 0x3f, 0x03, 0xab, 0x55, 0xbc, 0x30, 0x8c, 0x8f, 0xe3, 0x68, 0xf9,
	0x2e, 0x09, 0x42, 0x7f, 0x95, 0x9c, 0xac, 0xa2, 0xd0, 0x44, 0x78, 0x1f, 0x26, 0xed, 0x34, 0x08,
	0x83, 0x95, 0xd9, 0x3b, 0x18, 0x7c, 0xff, 0x49, 0x0c, 0xef, 0xd5, 0xf6, 0x37, 0x31, 0xb6, 0x35,
	0x41, 0x17, 0x35, 0x41, 0x97, 0x35, 0x41, 0xbf, 0x6a, 0x82, 0x7e, 0xec, 0x88, 0x71, 0xb1, 0x23,
	0xc6, 0xe5, 0x8e, 0x18, 0x27, 0x23, 0xbd, 0xf3, 0xdf, 0x00, 0x00, 0x00, 0xff, 0xff, 0xfb, 0x20,
	0x7d, 0xd0, 0x80, 0x02, 0x00, 0x00,
}