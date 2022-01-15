// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: sql/pgwire/pgerror/errors.proto

package pgerror

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import github_com_znbasedb_znbase_pkg_sql_pgwire_pgcode "github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode"

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

// Error contains all Postgres wire protocol error fields.
// See https://www.postgresql.org/docs/current/static/protocol-error-fields.html
// for a list of all Postgres error fields, most of which are optional and can
// be used to provide auxiliary error information.
type Error struct {
	// standard pg error fields. This can be passed
	// over the pg wire protocol.
	Code     github_com_znbasedb_znbase_pkg_sql_pgwire_pgcode.Code `protobuf:"bytes,1,opt,name=code,proto3,casttype=github.com/znbasedb/znbase/pkg/sql/pgwire/pgcode.Code" json:"code,omitempty"`
	Message  string                                                `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	Detail   string                                                `protobuf:"bytes,3,opt,name=detail,proto3" json:"detail,omitempty"`
	Hint     string                                                `protobuf:"bytes,4,opt,name=hint,proto3" json:"hint,omitempty"`
	Severity string                                                `protobuf:"bytes,8,opt,name=severity,proto3" json:"severity,omitempty"`
	Source   *Error_Source                                         `protobuf:"bytes,5,opt,name=source,proto3" json:"source,omitempty"`
	// a telemetry key, used as telemetry counter name.
	// Typically of the form [<prefix>.]#issuenum[.details]
	TelemetryKey string `protobuf:"bytes,6,opt,name=telemetry_key,json=telemetryKey,proto3" json:"telemetry_key,omitempty"`
	// complement to the detail field that can be reported
	// in sentry reports. This is scrubbed of PII.
	SafeDetail []*Error_SafeDetail `protobuf:"bytes,7,rep,name=safe_detail,json=safeDetail,proto3" json:"safe_detail,omitempty"`
}

func (m *Error) Reset()         { *m = Error{} }
func (m *Error) String() string { return proto.CompactTextString(m) }
func (*Error) ProtoMessage()    {}
func (*Error) Descriptor() ([]byte, []int) {
	return fileDescriptor_errors_a65f069871a6212e, []int{0}
}
func (m *Error) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Error) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *Error) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Error.Merge(dst, src)
}
func (m *Error) XXX_Size() int {
	return m.Size()
}
func (m *Error) XXX_DiscardUnknown() {
	xxx_messageInfo_Error.DiscardUnknown(m)
}

var xxx_messageInfo_Error proto.InternalMessageInfo

type Error_Source struct {
	File     string `protobuf:"bytes,1,opt,name=file,proto3" json:"file,omitempty"`
	Line     int32  `protobuf:"varint,2,opt,name=line,proto3" json:"line,omitempty"`
	Function string `protobuf:"bytes,3,opt,name=function,proto3" json:"function,omitempty"`
}

func (m *Error_Source) Reset()         { *m = Error_Source{} }
func (m *Error_Source) String() string { return proto.CompactTextString(m) }
func (*Error_Source) ProtoMessage()    {}
func (*Error_Source) Descriptor() ([]byte, []int) {
	return fileDescriptor_errors_a65f069871a6212e, []int{0, 0}
}
func (m *Error_Source) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Error_Source) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *Error_Source) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Error_Source.Merge(dst, src)
}
func (m *Error_Source) XXX_Size() int {
	return m.Size()
}
func (m *Error_Source) XXX_DiscardUnknown() {
	xxx_messageInfo_Error_Source.DiscardUnknown(m)
}

var xxx_messageInfo_Error_Source proto.InternalMessageInfo

type Error_SafeDetail struct {
	SafeMessage       string `protobuf:"bytes,1,opt,name=safe_message,json=safeMessage,proto3" json:"safe_message,omitempty"`
	EncodedStackTrace string `protobuf:"bytes,2,opt,name=encoded_stack_trace,json=encodedStackTrace,proto3" json:"encoded_stack_trace,omitempty"`
}

func (m *Error_SafeDetail) Reset()         { *m = Error_SafeDetail{} }
func (m *Error_SafeDetail) String() string { return proto.CompactTextString(m) }
func (*Error_SafeDetail) ProtoMessage()    {}
func (*Error_SafeDetail) Descriptor() ([]byte, []int) {
	return fileDescriptor_errors_a65f069871a6212e, []int{0, 1}
}
func (m *Error_SafeDetail) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Error_SafeDetail) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	b = b[:cap(b)]
	n, err := m.MarshalTo(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
func (dst *Error_SafeDetail) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Error_SafeDetail.Merge(dst, src)
}
func (m *Error_SafeDetail) XXX_Size() int {
	return m.Size()
}
func (m *Error_SafeDetail) XXX_DiscardUnknown() {
	xxx_messageInfo_Error_SafeDetail.DiscardUnknown(m)
}

var xxx_messageInfo_Error_SafeDetail proto.InternalMessageInfo

func init() {
	proto.RegisterType((*Error)(nil), "znbase.pgerror.Error")
	proto.RegisterType((*Error_Source)(nil), "znbase.pgerror.Error.Source")
	proto.RegisterType((*Error_SafeDetail)(nil), "znbase.pgerror.Error.SafeDetail")
}
func (m *Error) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Error) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.Code) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Code)))
		i += copy(dAtA[i:], m.Code)
	}
	if len(m.Message) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Message)))
		i += copy(dAtA[i:], m.Message)
	}
	if len(m.Detail) > 0 {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Detail)))
		i += copy(dAtA[i:], m.Detail)
	}
	if len(m.Hint) > 0 {
		dAtA[i] = 0x22
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Hint)))
		i += copy(dAtA[i:], m.Hint)
	}
	if m.Source != nil {
		dAtA[i] = 0x2a
		i++
		i = encodeVarintErrors(dAtA, i, uint64(m.Source.Size()))
		n1, err := m.Source.MarshalTo(dAtA[i:])
		if err != nil {
			return 0, err
		}
		i += n1
	}
	if len(m.TelemetryKey) > 0 {
		dAtA[i] = 0x32
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.TelemetryKey)))
		i += copy(dAtA[i:], m.TelemetryKey)
	}
	if len(m.SafeDetail) > 0 {
		for _, msg := range m.SafeDetail {
			dAtA[i] = 0x3a
			i++
			i = encodeVarintErrors(dAtA, i, uint64(msg.Size()))
			n, err := msg.MarshalTo(dAtA[i:])
			if err != nil {
				return 0, err
			}
			i += n
		}
	}
	if len(m.Severity) > 0 {
		dAtA[i] = 0x42
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Severity)))
		i += copy(dAtA[i:], m.Severity)
	}
	return i, nil
}

func (m *Error_Source) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Error_Source) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.File) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.File)))
		i += copy(dAtA[i:], m.File)
	}
	if m.Line != 0 {
		dAtA[i] = 0x10
		i++
		i = encodeVarintErrors(dAtA, i, uint64(m.Line))
	}
	if len(m.Function) > 0 {
		dAtA[i] = 0x1a
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.Function)))
		i += copy(dAtA[i:], m.Function)
	}
	return i, nil
}

func (m *Error_SafeDetail) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalTo(dAtA)
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Error_SafeDetail) MarshalTo(dAtA []byte) (int, error) {
	var i int
	_ = i
	var l int
	_ = l
	if len(m.SafeMessage) > 0 {
		dAtA[i] = 0xa
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.SafeMessage)))
		i += copy(dAtA[i:], m.SafeMessage)
	}
	if len(m.EncodedStackTrace) > 0 {
		dAtA[i] = 0x12
		i++
		i = encodeVarintErrors(dAtA, i, uint64(len(m.EncodedStackTrace)))
		i += copy(dAtA[i:], m.EncodedStackTrace)
	}
	return i, nil
}

func encodeVarintErrors(dAtA []byte, offset int, v uint64) int {
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return offset + 1
}
func (m *Error) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Code)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.Message)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.Detail)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.Hint)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	if m.Source != nil {
		l = m.Source.Size()
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.TelemetryKey)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	if len(m.SafeDetail) > 0 {
		for _, e := range m.SafeDetail {
			l = e.Size()
			n += 1 + l + sovErrors(uint64(l))
		}
	}
	l = len(m.Severity)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	return n
}

func (m *Error_Source) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.File)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	if m.Line != 0 {
		n += 1 + sovErrors(uint64(m.Line))
	}
	l = len(m.Function)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	return n
}

func (m *Error_SafeDetail) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.SafeMessage)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	l = len(m.EncodedStackTrace)
	if l > 0 {
		n += 1 + l + sovErrors(uint64(l))
	}
	return n
}

func sovErrors(x uint64) (n int) {
	for {
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	return n
}
func sozErrors(x uint64) (n int) {
	return sovErrors(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Error) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowErrors
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
			return fmt.Errorf("proto: Error: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Error: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Code", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Code = github_com_znbasedb_znbase_pkg_sql_pgwire_pgcode.Code(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Message", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Message = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Detail", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Detail = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 4:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Hint", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Hint = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 5:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Source", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Source == nil {
				m.Source = &Error_Source{}
			}
			if err := m.Source.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field TelemetryKey", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.TelemetryKey = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 7:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field SafeDetail", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + msglen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.SafeDetail = append(m.SafeDetail, &Error_SafeDetail{})
			if err := m.SafeDetail[len(m.SafeDetail)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 8:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Severity", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Severity = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipErrors(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthErrors
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
func (m *Error_Source) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowErrors
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
			return fmt.Errorf("proto: Source: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Source: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field File", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.File = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Line", wireType)
			}
			m.Line = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Line |= (int32(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Function", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Function = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipErrors(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthErrors
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
func (m *Error_SafeDetail) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowErrors
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
			return fmt.Errorf("proto: SafeDetail: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: SafeDetail: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field SafeMessage", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.SafeMessage = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field EncodedStackTrace", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowErrors
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
				return ErrInvalidLengthErrors
			}
			postIndex := iNdEx + intStringLen
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.EncodedStackTrace = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipErrors(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthErrors
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
func skipErrors(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowErrors
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
					return 0, ErrIntOverflowErrors
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
					return 0, ErrIntOverflowErrors
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
				return 0, ErrInvalidLengthErrors
			}
			return iNdEx, nil
		case 3:
			for {
				var innerWire uint64
				var start int = iNdEx
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, ErrIntOverflowErrors
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
				next, err := skipErrors(dAtA[start:])
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
	ErrInvalidLengthErrors = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowErrors   = fmt.Errorf("proto: integer overflow")
)

func init() {
	proto.RegisterFile("sql/pgwire/pgerror/errors.proto", fileDescriptor_errors_a65f069871a6212e)
}

var fileDescriptor_errors_a65f069871a6212e = []byte{
	// 414 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x52, 0xb1, 0x8e, 0xd3, 0x40,
	0x10, 0x8d, 0xb9, 0xc4, 0x39, 0x26, 0x07, 0x12, 0x0b, 0x42, 0xab, 0x08, 0xed, 0x05, 0x68, 0x42,
	0xb3, 0x96, 0x0e, 0x28, 0x28, 0x39, 0xa0, 0x82, 0x6b, 0x7c, 0x54, 0x34, 0x96, 0x63, 0x4f, 0x7c,
	0xab, 0x38, 0xde, 0xb0, 0xbb, 0x01, 0x85, 0x9e, 0x9e, 0xcf, 0xba, 0xf2, 0xca, 0xab, 0x10, 0x24,
	0x7f, 0x41, 0x85, 0x76, 0xbc, 0x31, 0xba, 0x82, 0xc6, 0x7a, 0x33, 0x6f, 0xfc, 0xfc, 0xe6, 0x8d,
	0xe1, 0xd8, 0x7e, 0xae, 0x93, 0x55, 0xf5, 0x55, 0x19, 0x4c, 0x56, 0x15, 0x1a, 0xa3, 0x4d, 0x42,
	0x4f, 0x2b, 0x57, 0x46, 0x3b, 0xcd, 0xee, 0x7e, 0x6b, 0x66, 0xb9, 0x45, 0x19, 0xc8, 0xf1, 0x83,
	0x4a, 0x57, 0x9a, 0xa8, 0xc4, 0xa3, 0x76, 0xea, 0xc9, 0xf7, 0x3e, 0x0c, 0xde, 0x79, 0x9e, 0x9d,
	0x41, 0xbf, 0xd0, 0x25, 0xf2, 0x68, 0x12, 0x4d, 0x6f, 0x9f, 0xbe, 0xfa, 0xf3, 0xf3, 0xf8, 0x65,
	0xa5, 0xdc, 0xc5, 0x7a, 0x26, 0x0b, 0xbd, 0x4c, 0x5a, 0xb1, 0x72, 0x16, 0x40, 0xb2, 0x5a, 0x54,
	0xc9, 0x0d, 0x07, 0xfe, 0x5d, 0xf9, 0x46, 0x97, 0x98, 0x92, 0x0c, 0xe3, 0x30, 0x5c, 0xa2, 0xb5,
	0x79, 0x85, 0xfc, 0x96, 0x57, 0x4c, 0xf7, 0x25, 0x7b, 0x08, 0x71, 0x89, 0x2e, 0x57, 0x35, 0x3f,
	0x20, 0x22, 0x54, 0x8c, 0x41, 0xff, 0x42, 0x35, 0x8e, 0xf7, 0xa9, 0x4b, 0x98, 0xbd, 0x80, 0xd8,
	0xea, 0xb5, 0x29, 0x90, 0x0f, 0x26, 0xd1, 0x74, 0x74, 0xf2, 0x48, 0xde, 0xdc, 0x4a, 0x92, 0x77,
	0x79, 0x4e, 0x33, 0x69, 0x98, 0x65, 0x4f, 0xe1, 0x8e, 0xc3, 0x1a, 0x97, 0xe8, 0xcc, 0x26, 0x5b,
	0xe0, 0x86, 0xc7, 0x24, 0x79, 0xd4, 0x35, 0xdf, 0xe3, 0x86, 0xbd, 0x86, 0x91, 0xcd, 0xe7, 0x98,
	0x05, 0x2f, 0xc3, 0xc9, 0xc1, 0x74, 0x74, 0x32, 0xf9, 0x8f, 0x7e, 0x3e, 0xc7, 0xb7, 0x34, 0x97,
	0x82, 0xed, 0x30, 0x1b, 0xc3, 0xa1, 0xc5, 0x2f, 0x68, 0x94, 0xdb, 0xf0, 0x43, 0xfa, 0x44, 0x57,
	0x8f, 0x3f, 0x40, 0xdc, 0xba, 0xf2, 0x7b, 0xcd, 0x55, 0x1d, 0x82, 0x4d, 0x09, 0xfb, 0x5e, 0xad,
	0x9a, 0x36, 0x9a, 0x41, 0x4a, 0xd8, 0xab, 0xcd, 0xd7, 0x4d, 0xe1, 0x94, 0x6e, 0x42, 0x32, 0x5d,
	0x3d, 0xce, 0x00, 0xfe, 0x79, 0x60, 0x8f, 0xe1, 0x88, 0xac, 0xef, 0x03, 0x6e, 0x95, 0x69, 0x9d,
	0xb3, 0x10, 0xb2, 0x84, 0xfb, 0xd8, 0xf8, 0x43, 0x94, 0x99, 0x75, 0x79, 0xb1, 0xc8, 0x9c, 0xc9,
	0x8b, 0xfd, 0x29, 0xee, 0x05, 0xea, 0xdc, 0x33, 0x1f, 0x3d, 0x71, 0xfa, 0xec, 0xf2, 0xb7, 0xe8,
	0x5d, 0x6e, 0x45, 0x74, 0xb5, 0x15, 0xd1, 0xf5, 0x56, 0x44, 0xbf, 0xb6, 0x22, 0xfa, 0xb1, 0x13,
	0xbd, 0xab, 0x9d, 0xe8, 0x5d, 0xef, 0x44, 0xef, 0xd3, 0x30, 0x44, 0x32, 0x8b, 0xe9, 0xcf, 0x79,
	0xfe, 0x37, 0x00, 0x00, 0xff, 0xff, 0x64, 0x69, 0x48, 0xdc, 0x82, 0x02, 0x00, 0x00,
}