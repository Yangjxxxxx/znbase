// Autogenerated by Thrift Compiler (1.0.0-dev)
// DO NOT EDIT UNLESS YOU ARE SURE THAT YOU KNOW WHAT YOU ARE DOING

package scribe

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/apache/thrift/lib/go/thrift"
	"reflect"
)

// (needed to ensure safety because of naive import list construction.)
var _ = thrift.ZERO
var _ = fmt.Printf
var _ = context.Background
var _ = reflect.DeepEqual
var _ = bytes.Equal

type ResultCode int64

const (
	ResultCode_OK        ResultCode = 0
	ResultCode_TRY_LATER ResultCode = 1
)

func (p ResultCode) String() string {
	switch p {
	case ResultCode_OK:
		return "OK"
	case ResultCode_TRY_LATER:
		return "TRY_LATER"
	}
	return "<UNSET>"
}

func ResultCodeFromString(s string) (ResultCode, error) {
	switch s {
	case "OK":
		return ResultCode_OK, nil
	case "TRY_LATER":
		return ResultCode_TRY_LATER, nil
	}
	return ResultCode(0), fmt.Errorf("not a valid ResultCode string")
}

func ResultCodePtr(v ResultCode) *ResultCode { return &v }

func (p ResultCode) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *ResultCode) UnmarshalText(text []byte) error {
	q, err := ResultCodeFromString(string(text))
	if err != nil {
		return err
	}
	*p = q
	return nil
}

func (p *ResultCode) Scan(value interface{}) error {
	v, ok := value.(int64)
	if !ok {
		return errors.New("Scan value is not int64")
	}
	*p = ResultCode(v)
	return nil
}

func (p *ResultCode) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return int64(*p), nil
}

// Attributes:
//  - Category
//  - Message
type LogEntry struct {
	Category string `thrift:"category,1" db:"category" json:"category"`
	Message  string `thrift:"message,2" db:"message" json:"message"`
}

func NewLogEntry() *LogEntry {
	return &LogEntry{}
}

func (p *LogEntry) GetCategory() string {
	return p.Category
}

func (p *LogEntry) GetMessage() string {
	return p.Message
}
func (p *LogEntry) Read(iprot thrift.TProtocol) error {
	if _, err := iprot.ReadStructBegin(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T read error: ", p), err)
	}

	for {
		_, fieldTypeId, fieldId, err := iprot.ReadFieldBegin()
		if err != nil {
			return thrift.PrependError(fmt.Sprintf("%T field %d read error: ", p, fieldId), err)
		}
		if fieldTypeId == thrift.STOP {
			break
		}
		switch fieldId {
		case 1:
			if fieldTypeId == thrift.STRING {
				if err := p.ReadField1(iprot); err != nil {
					return err
				}
			} else {
				if err := iprot.Skip(fieldTypeId); err != nil {
					return err
				}
			}
		case 2:
			if fieldTypeId == thrift.STRING {
				if err := p.ReadField2(iprot); err != nil {
					return err
				}
			} else {
				if err := iprot.Skip(fieldTypeId); err != nil {
					return err
				}
			}
		default:
			if err := iprot.Skip(fieldTypeId); err != nil {
				return err
			}
		}
		if err := iprot.ReadFieldEnd(); err != nil {
			return err
		}
	}
	if err := iprot.ReadStructEnd(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T read struct end error: ", p), err)
	}
	return nil
}

func (p *LogEntry) ReadField1(iprot thrift.TProtocol) error {
	if v, err := iprot.ReadString(); err != nil {
		return thrift.PrependError("error reading field 1: ", err)
	} else {
		p.Category = v
	}
	return nil
}

func (p *LogEntry) ReadField2(iprot thrift.TProtocol) error {
	if v, err := iprot.ReadString(); err != nil {
		return thrift.PrependError("error reading field 2: ", err)
	} else {
		p.Message = v
	}
	return nil
}

func (p *LogEntry) Write(oprot thrift.TProtocol) error {
	if err := oprot.WriteStructBegin("LogEntry"); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write struct begin error: ", p), err)
	}
	if p != nil {
		if err := p.writeField1(oprot); err != nil {
			return err
		}
		if err := p.writeField2(oprot); err != nil {
			return err
		}
	}
	if err := oprot.WriteFieldStop(); err != nil {
		return thrift.PrependError("write field stop error: ", err)
	}
	if err := oprot.WriteStructEnd(); err != nil {
		return thrift.PrependError("write struct stop error: ", err)
	}
	return nil
}

func (p *LogEntry) writeField1(oprot thrift.TProtocol) (err error) {
	if err := oprot.WriteFieldBegin("category", thrift.STRING, 1); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write field begin error 1:category: ", p), err)
	}
	if err := oprot.WriteString(string(p.Category)); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T.category (1) field write error: ", p), err)
	}
	if err := oprot.WriteFieldEnd(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write field end error 1:category: ", p), err)
	}
	return err
}

func (p *LogEntry) writeField2(oprot thrift.TProtocol) (err error) {
	if err := oprot.WriteFieldBegin("message", thrift.STRING, 2); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write field begin error 2:message: ", p), err)
	}
	if err := oprot.WriteString(string(p.Message)); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T.message (2) field write error: ", p), err)
	}
	if err := oprot.WriteFieldEnd(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write field end error 2:message: ", p), err)
	}
	return err
}

func (p *LogEntry) String() string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("LogEntry(%+v)", *p)
}

type Scribe interface {
	// Parameters:
	//  - Messages
	Log(ctx context.Context, messages []*LogEntry) (r ResultCode, err error)
}

type ScribeClient struct {
	c thrift.TClient
}

func NewScribeClientFactory(t thrift.TTransport, f thrift.TProtocolFactory) *ScribeClient {
	return &ScribeClient{
		c: thrift.NewTStandardClient(f.GetProtocol(t), f.GetProtocol(t)),
	}
}

func NewScribeClientProtocol(t thrift.TTransport, iprot thrift.TProtocol, oprot thrift.TProtocol) *ScribeClient {
	return &ScribeClient{
		c: thrift.NewTStandardClient(iprot, oprot),
	}
}

func NewScribeClient(c thrift.TClient) *ScribeClient {
	return &ScribeClient{
		c: c,
	}
}

func (p *ScribeClient) Client_() thrift.TClient {
	return p.c
}

// Parameters:
//  - Messages
func (p *ScribeClient) Log(ctx context.Context, messages []*LogEntry) (r ResultCode, err error) {
	var _args0 ScribeLogArgs
	_args0.Messages = messages
	var _result1 ScribeLogResult
	if err = p.Client_().Call(ctx, "Log", &_args0, &_result1); err != nil {
		return
	}
	return _result1.GetSuccess(), nil
}

type ScribeProcessor struct {
	processorMap map[string]thrift.TProcessorFunction
	handler      Scribe
}

func (p *ScribeProcessor) AddToProcessorMap(key string, processor thrift.TProcessorFunction) {
	p.processorMap[key] = processor
}

func (p *ScribeProcessor) GetProcessorFunction(key string) (processor thrift.TProcessorFunction, ok bool) {
	processor, ok = p.processorMap[key]
	return processor, ok
}

func (p *ScribeProcessor) ProcessorMap() map[string]thrift.TProcessorFunction {
	return p.processorMap
}

func NewScribeProcessor(handler Scribe) *ScribeProcessor {

	self2 := &ScribeProcessor{handler: handler, processorMap: make(map[string]thrift.TProcessorFunction)}
	self2.processorMap["Log"] = &scribeProcessorLog{handler: handler}
	return self2
}

func (p *ScribeProcessor) Process(ctx context.Context, iprot, oprot thrift.TProtocol) (success bool, err thrift.TException) {
	name, _, seqId, err := iprot.ReadMessageBegin()
	if err != nil {
		return false, err
	}
	if processor, ok := p.GetProcessorFunction(name); ok {
		return processor.Process(ctx, seqId, iprot, oprot)
	}
	iprot.Skip(thrift.STRUCT)
	iprot.ReadMessageEnd()
	x3 := thrift.NewTApplicationException(thrift.UNKNOWN_METHOD, "Unknown function "+name)
	oprot.WriteMessageBegin(name, thrift.EXCEPTION, seqId)
	x3.Write(oprot)
	oprot.WriteMessageEnd()
	oprot.Flush(ctx)
	return false, x3

}

type scribeProcessorLog struct {
	handler Scribe
}

func (p *scribeProcessorLog) Process(ctx context.Context, seqId int32, iprot, oprot thrift.TProtocol) (success bool, err thrift.TException) {
	args := ScribeLogArgs{}
	if err = args.Read(iprot); err != nil {
		iprot.ReadMessageEnd()
		x := thrift.NewTApplicationException(thrift.PROTOCOL_ERROR, err.Error())
		oprot.WriteMessageBegin("Log", thrift.EXCEPTION, seqId)
		x.Write(oprot)
		oprot.WriteMessageEnd()
		oprot.Flush(ctx)
		return false, err
	}

	iprot.ReadMessageEnd()
	result := ScribeLogResult{}
	var retval ResultCode
	var err2 error
	if retval, err2 = p.handler.Log(ctx, args.Messages); err2 != nil {
		x := thrift.NewTApplicationException(thrift.INTERNAL_ERROR, "Internal error processing Log: "+err2.Error())
		oprot.WriteMessageBegin("Log", thrift.EXCEPTION, seqId)
		x.Write(oprot)
		oprot.WriteMessageEnd()
		oprot.Flush(ctx)
		return true, err2
	} else {
		result.Success = &retval
	}
	if err2 = oprot.WriteMessageBegin("Log", thrift.REPLY, seqId); err2 != nil {
		err = err2
	}
	if err2 = result.Write(oprot); err == nil && err2 != nil {
		err = err2
	}
	if err2 = oprot.WriteMessageEnd(); err == nil && err2 != nil {
		err = err2
	}
	if err2 = oprot.Flush(ctx); err == nil && err2 != nil {
		err = err2
	}
	if err != nil {
		return
	}
	return true, err
}

// HELPER FUNCTIONS AND STRUCTURES

// Attributes:
//  - Messages
type ScribeLogArgs struct {
	Messages []*LogEntry `thrift:"messages,1" db:"messages" json:"messages"`
}

func NewScribeLogArgs() *ScribeLogArgs {
	return &ScribeLogArgs{}
}

func (p *ScribeLogArgs) GetMessages() []*LogEntry {
	return p.Messages
}
func (p *ScribeLogArgs) Read(iprot thrift.TProtocol) error {
	if _, err := iprot.ReadStructBegin(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T read error: ", p), err)
	}

	for {
		_, fieldTypeId, fieldId, err := iprot.ReadFieldBegin()
		if err != nil {
			return thrift.PrependError(fmt.Sprintf("%T field %d read error: ", p, fieldId), err)
		}
		if fieldTypeId == thrift.STOP {
			break
		}
		switch fieldId {
		case 1:
			if fieldTypeId == thrift.LIST {
				if err := p.ReadField1(iprot); err != nil {
					return err
				}
			} else {
				if err := iprot.Skip(fieldTypeId); err != nil {
					return err
				}
			}
		default:
			if err := iprot.Skip(fieldTypeId); err != nil {
				return err
			}
		}
		if err := iprot.ReadFieldEnd(); err != nil {
			return err
		}
	}
	if err := iprot.ReadStructEnd(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T read struct end error: ", p), err)
	}
	return nil
}

func (p *ScribeLogArgs) ReadField1(iprot thrift.TProtocol) error {
	_, size, err := iprot.ReadListBegin()
	if err != nil {
		return thrift.PrependError("error reading list begin: ", err)
	}
	tSlice := make([]*LogEntry, 0, size)
	p.Messages = tSlice
	for i := 0; i < size; i++ {
		_elem4 := &LogEntry{}
		if err := _elem4.Read(iprot); err != nil {
			return thrift.PrependError(fmt.Sprintf("%T error reading struct: ", _elem4), err)
		}
		p.Messages = append(p.Messages, _elem4)
	}
	if err := iprot.ReadListEnd(); err != nil {
		return thrift.PrependError("error reading list end: ", err)
	}
	return nil
}

func (p *ScribeLogArgs) Write(oprot thrift.TProtocol) error {
	if err := oprot.WriteStructBegin("Log_args"); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write struct begin error: ", p), err)
	}
	if p != nil {
		if err := p.writeField1(oprot); err != nil {
			return err
		}
	}
	if err := oprot.WriteFieldStop(); err != nil {
		return thrift.PrependError("write field stop error: ", err)
	}
	if err := oprot.WriteStructEnd(); err != nil {
		return thrift.PrependError("write struct stop error: ", err)
	}
	return nil
}

func (p *ScribeLogArgs) writeField1(oprot thrift.TProtocol) (err error) {
	if err := oprot.WriteFieldBegin("messages", thrift.LIST, 1); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write field begin error 1:messages: ", p), err)
	}
	if err := oprot.WriteListBegin(thrift.STRUCT, len(p.Messages)); err != nil {
		return thrift.PrependError("error writing list begin: ", err)
	}
	for _, v := range p.Messages {
		if err := v.Write(oprot); err != nil {
			return thrift.PrependError(fmt.Sprintf("%T error writing struct: ", v), err)
		}
	}
	if err := oprot.WriteListEnd(); err != nil {
		return thrift.PrependError("error writing list end: ", err)
	}
	if err := oprot.WriteFieldEnd(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write field end error 1:messages: ", p), err)
	}
	return err
}

func (p *ScribeLogArgs) String() string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("ScribeLogArgs(%+v)", *p)
}

// Attributes:
//  - Success
type ScribeLogResult struct {
	Success *ResultCode `thrift:"success,0" db:"success" json:"success,omitempty"`
}

func NewScribeLogResult() *ScribeLogResult {
	return &ScribeLogResult{}
}

var ScribeLogResult_Success_DEFAULT ResultCode

func (p *ScribeLogResult) GetSuccess() ResultCode {
	if !p.IsSetSuccess() {
		return ScribeLogResult_Success_DEFAULT
	}
	return *p.Success
}
func (p *ScribeLogResult) IsSetSuccess() bool {
	return p.Success != nil
}

func (p *ScribeLogResult) Read(iprot thrift.TProtocol) error {
	if _, err := iprot.ReadStructBegin(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T read error: ", p), err)
	}

	for {
		_, fieldTypeId, fieldId, err := iprot.ReadFieldBegin()
		if err != nil {
			return thrift.PrependError(fmt.Sprintf("%T field %d read error: ", p, fieldId), err)
		}
		if fieldTypeId == thrift.STOP {
			break
		}
		switch fieldId {
		case 0:
			if fieldTypeId == thrift.I32 {
				if err := p.ReadField0(iprot); err != nil {
					return err
				}
			} else {
				if err := iprot.Skip(fieldTypeId); err != nil {
					return err
				}
			}
		default:
			if err := iprot.Skip(fieldTypeId); err != nil {
				return err
			}
		}
		if err := iprot.ReadFieldEnd(); err != nil {
			return err
		}
	}
	if err := iprot.ReadStructEnd(); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T read struct end error: ", p), err)
	}
	return nil
}

func (p *ScribeLogResult) ReadField0(iprot thrift.TProtocol) error {
	if v, err := iprot.ReadI32(); err != nil {
		return thrift.PrependError("error reading field 0: ", err)
	} else {
		temp := ResultCode(v)
		p.Success = &temp
	}
	return nil
}

func (p *ScribeLogResult) Write(oprot thrift.TProtocol) error {
	if err := oprot.WriteStructBegin("Log_result"); err != nil {
		return thrift.PrependError(fmt.Sprintf("%T write struct begin error: ", p), err)
	}
	if p != nil {
		if err := p.writeField0(oprot); err != nil {
			return err
		}
	}
	if err := oprot.WriteFieldStop(); err != nil {
		return thrift.PrependError("write field stop error: ", err)
	}
	if err := oprot.WriteStructEnd(); err != nil {
		return thrift.PrependError("write struct stop error: ", err)
	}
	return nil
}

func (p *ScribeLogResult) writeField0(oprot thrift.TProtocol) (err error) {
	if p.IsSetSuccess() {
		if err := oprot.WriteFieldBegin("success", thrift.I32, 0); err != nil {
			return thrift.PrependError(fmt.Sprintf("%T write field begin error 0:success: ", p), err)
		}
		if err := oprot.WriteI32(int32(*p.Success)); err != nil {
			return thrift.PrependError(fmt.Sprintf("%T.success (0) field write error: ", p), err)
		}
		if err := oprot.WriteFieldEnd(); err != nil {
			return thrift.PrependError(fmt.Sprintf("%T write field end error 0:success: ", p), err)
		}
	}
	return err
}

func (p *ScribeLogResult) String() string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("ScribeLogResult(%+v)", *p)
}
