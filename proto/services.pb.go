// Code generated by protoc-gen-go. DO NOT EDIT.
// source: services.proto

package tsgrpc

import (
	context "context"
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type Request_CommandType int32

const (
	Request_START        Request_CommandType = 0
	Request_RESTART      Request_CommandType = 1
	Request_STOP         Request_CommandType = 2
	Request_GET_METRICES Request_CommandType = 3
)

var Request_CommandType_name = map[int32]string{
	0: "START",
	1: "RESTART",
	2: "STOP",
	3: "GET_METRICES",
}

var Request_CommandType_value = map[string]int32{
	"START":        0,
	"RESTART":      1,
	"STOP":         2,
	"GET_METRICES": 3,
}

func (x Request_CommandType) String() string {
	return proto.EnumName(Request_CommandType_name, int32(x))
}

func (Request_CommandType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{0, 0}
}

type Request_HTTPMethod int32

const (
	Request_GET    Request_HTTPMethod = 0
	Request_POST   Request_HTTPMethod = 1
	Request_PUT    Request_HTTPMethod = 2
	Request_DELETE Request_HTTPMethod = 3
	Request_UPDATE Request_HTTPMethod = 4
)

var Request_HTTPMethod_name = map[int32]string{
	0: "GET",
	1: "POST",
	2: "PUT",
	3: "DELETE",
	4: "UPDATE",
}

var Request_HTTPMethod_value = map[string]int32{
	"GET":    0,
	"POST":   1,
	"PUT":    2,
	"DELETE": 3,
	"UPDATE": 4,
}

func (x Request_HTTPMethod) String() string {
	return proto.EnumName(Request_HTTPMethod_name, int32(x))
}

func (Request_HTTPMethod) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{0, 1}
}

type Request struct {
	Command              Request_CommandType  `protobuf:"varint,1,opt,name=command,proto3,enum=tsgrpc.Request_CommandType" json:"command,omitempty"`
	Params               *Request_Params      `protobuf:"bytes,2,opt,name=params,proto3" json:"params,omitempty"`
	Timestamp            *timestamp.Timestamp `protobuf:"bytes,3,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	XXX_NoUnkeyedLiteral struct{}             `json:"-"`
	XXX_unrecognized     []byte               `json:"-"`
	XXX_sizecache        int32                `json:"-"`
}

func (m *Request) Reset()         { *m = Request{} }
func (m *Request) String() string { return proto.CompactTextString(m) }
func (*Request) ProtoMessage()    {}
func (*Request) Descriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{0}
}

func (m *Request) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Request.Unmarshal(m, b)
}
func (m *Request) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Request.Marshal(b, m, deterministic)
}
func (m *Request) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Request.Merge(m, src)
}
func (m *Request) XXX_Size() int {
	return xxx_messageInfo_Request.Size(m)
}
func (m *Request) XXX_DiscardUnknown() {
	xxx_messageInfo_Request.DiscardUnknown(m)
}

var xxx_messageInfo_Request proto.InternalMessageInfo

func (m *Request) GetCommand() Request_CommandType {
	if m != nil {
		return m.Command
	}
	return Request_START
}

func (m *Request) GetParams() *Request_Params {
	if m != nil {
		return m.Params
	}
	return nil
}

func (m *Request) GetTimestamp() *timestamp.Timestamp {
	if m != nil {
		return m.Timestamp
	}
	return nil
}

type Request_HTTPHeader struct {
	Key                  string   `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Value                string   `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Request_HTTPHeader) Reset()         { *m = Request_HTTPHeader{} }
func (m *Request_HTTPHeader) String() string { return proto.CompactTextString(m) }
func (*Request_HTTPHeader) ProtoMessage()    {}
func (*Request_HTTPHeader) Descriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{0, 0}
}

func (m *Request_HTTPHeader) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Request_HTTPHeader.Unmarshal(m, b)
}
func (m *Request_HTTPHeader) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Request_HTTPHeader.Marshal(b, m, deterministic)
}
func (m *Request_HTTPHeader) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Request_HTTPHeader.Merge(m, src)
}
func (m *Request_HTTPHeader) XXX_Size() int {
	return xxx_messageInfo_Request_HTTPHeader.Size(m)
}
func (m *Request_HTTPHeader) XXX_DiscardUnknown() {
	xxx_messageInfo_Request_HTTPHeader.DiscardUnknown(m)
}

var xxx_messageInfo_Request_HTTPHeader proto.InternalMessageInfo

func (m *Request_HTTPHeader) GetKey() string {
	if m != nil {
		return m.Key
	}
	return ""
}

func (m *Request_HTTPHeader) GetValue() string {
	if m != nil {
		return m.Value
	}
	return ""
}

type Request_Params struct {
	Name                 string                `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Url                  string                `protobuf:"bytes,2,opt,name=url,proto3" json:"url,omitempty"`
	Method               Request_HTTPMethod    `protobuf:"varint,3,opt,name=method,proto3,enum=tsgrpc.Request_HTTPMethod" json:"method,omitempty"`
	Protocol             string                `protobuf:"bytes,4,opt,name=protocol,proto3" json:"protocol,omitempty"`
	Host                 string                `protobuf:"bytes,5,opt,name=host,proto3" json:"host,omitempty"`
	Port                 string                `protobuf:"bytes,6,opt,name=port,proto3" json:"port,omitempty"`
	Path                 string                `protobuf:"bytes,7,opt,name=path,proto3" json:"path,omitempty"`
	MaxConcurrences      int32                 `protobuf:"varint,8,opt,name=maxConcurrences,proto3" json:"maxConcurrences,omitempty"`
	Header               []*Request_HTTPHeader `protobuf:"bytes,9,rep,name=header,proto3" json:"header,omitempty"`
	Body                 string                `protobuf:"bytes,10,opt,name=body,proto3" json:"body,omitempty"`
	XXX_NoUnkeyedLiteral struct{}              `json:"-"`
	XXX_unrecognized     []byte                `json:"-"`
	XXX_sizecache        int32                 `json:"-"`
}

func (m *Request_Params) Reset()         { *m = Request_Params{} }
func (m *Request_Params) String() string { return proto.CompactTextString(m) }
func (*Request_Params) ProtoMessage()    {}
func (*Request_Params) Descriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{0, 1}
}

func (m *Request_Params) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Request_Params.Unmarshal(m, b)
}
func (m *Request_Params) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Request_Params.Marshal(b, m, deterministic)
}
func (m *Request_Params) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Request_Params.Merge(m, src)
}
func (m *Request_Params) XXX_Size() int {
	return xxx_messageInfo_Request_Params.Size(m)
}
func (m *Request_Params) XXX_DiscardUnknown() {
	xxx_messageInfo_Request_Params.DiscardUnknown(m)
}

var xxx_messageInfo_Request_Params proto.InternalMessageInfo

func (m *Request_Params) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Request_Params) GetUrl() string {
	if m != nil {
		return m.Url
	}
	return ""
}

func (m *Request_Params) GetMethod() Request_HTTPMethod {
	if m != nil {
		return m.Method
	}
	return Request_GET
}

func (m *Request_Params) GetProtocol() string {
	if m != nil {
		return m.Protocol
	}
	return ""
}

func (m *Request_Params) GetHost() string {
	if m != nil {
		return m.Host
	}
	return ""
}

func (m *Request_Params) GetPort() string {
	if m != nil {
		return m.Port
	}
	return ""
}

func (m *Request_Params) GetPath() string {
	if m != nil {
		return m.Path
	}
	return ""
}

func (m *Request_Params) GetMaxConcurrences() int32 {
	if m != nil {
		return m.MaxConcurrences
	}
	return 0
}

func (m *Request_Params) GetHeader() []*Request_HTTPHeader {
	if m != nil {
		return m.Header
	}
	return nil
}

func (m *Request_Params) GetBody() string {
	if m != nil {
		return m.Body
	}
	return ""
}

type Response struct {
	ErrorCode            int32    `protobuf:"varint,1,opt,name=error_code,json=errorCode,proto3" json:"error_code,omitempty"`
	Data                 string   `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Response) Reset()         { *m = Response{} }
func (m *Response) String() string { return proto.CompactTextString(m) }
func (*Response) ProtoMessage()    {}
func (*Response) Descriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{1}
}

func (m *Response) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Response.Unmarshal(m, b)
}
func (m *Response) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Response.Marshal(b, m, deterministic)
}
func (m *Response) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Response.Merge(m, src)
}
func (m *Response) XXX_Size() int {
	return xxx_messageInfo_Response.Size(m)
}
func (m *Response) XXX_DiscardUnknown() {
	xxx_messageInfo_Response.DiscardUnknown(m)
}

var xxx_messageInfo_Response proto.InternalMessageInfo

func (m *Response) GetErrorCode() int32 {
	if m != nil {
		return m.ErrorCode
	}
	return 0
}

func (m *Response) GetData() string {
	if m != nil {
		return m.Data
	}
	return ""
}

type RegisterRequest struct {
	Id                   int32    `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	Name                 string   `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	MaxConcurrences      int32    `protobuf:"varint,3,opt,name=maxConcurrences,proto3" json:"maxConcurrences,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *RegisterRequest) Reset()         { *m = RegisterRequest{} }
func (m *RegisterRequest) String() string { return proto.CompactTextString(m) }
func (*RegisterRequest) ProtoMessage()    {}
func (*RegisterRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_8e16ccb8c5307b32, []int{2}
}

func (m *RegisterRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RegisterRequest.Unmarshal(m, b)
}
func (m *RegisterRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RegisterRequest.Marshal(b, m, deterministic)
}
func (m *RegisterRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RegisterRequest.Merge(m, src)
}
func (m *RegisterRequest) XXX_Size() int {
	return xxx_messageInfo_RegisterRequest.Size(m)
}
func (m *RegisterRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_RegisterRequest.DiscardUnknown(m)
}

var xxx_messageInfo_RegisterRequest proto.InternalMessageInfo

func (m *RegisterRequest) GetId() int32 {
	if m != nil {
		return m.Id
	}
	return 0
}

func (m *RegisterRequest) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *RegisterRequest) GetMaxConcurrences() int32 {
	if m != nil {
		return m.MaxConcurrences
	}
	return 0
}

func init() {
	proto.RegisterEnum("tsgrpc.Request_CommandType", Request_CommandType_name, Request_CommandType_value)
	proto.RegisterEnum("tsgrpc.Request_HTTPMethod", Request_HTTPMethod_name, Request_HTTPMethod_value)
	proto.RegisterType((*Request)(nil), "tsgrpc.Request")
	proto.RegisterType((*Request_HTTPHeader)(nil), "tsgrpc.Request.HTTPHeader")
	proto.RegisterType((*Request_Params)(nil), "tsgrpc.Request.Params")
	proto.RegisterType((*Response)(nil), "tsgrpc.Response")
	proto.RegisterType((*RegisterRequest)(nil), "tsgrpc.RegisterRequest")
}

func init() { proto.RegisterFile("services.proto", fileDescriptor_8e16ccb8c5307b32) }

var fileDescriptor_8e16ccb8c5307b32 = []byte{
	// 584 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x53, 0x41, 0x6f, 0xd3, 0x30,
	0x18, 0x5d, 0x93, 0x26, 0x6d, 0xbe, 0xa1, 0x2e, 0xb2, 0x10, 0x44, 0x41, 0x88, 0xaa, 0xa7, 0x4a,
	0xa0, 0x4c, 0x14, 0x10, 0x5c, 0x90, 0x98, 0xba, 0x68, 0x43, 0x62, 0x5a, 0xe4, 0x78, 0xe7, 0xc9,
	0x4b, 0x4c, 0x1b, 0xd1, 0xc4, 0xc1, 0x76, 0x27, 0x76, 0xe5, 0x37, 0xf0, 0x33, 0xf8, 0x91, 0xc8,
	0x76, 0xba, 0x8d, 0xaa, 0x87, 0xde, 0x5e, 0x9e, 0xdf, 0xb3, 0x9f, 0x9f, 0xbf, 0xc0, 0x48, 0x32,
	0x71, 0x5b, 0x15, 0x4c, 0x26, 0xad, 0xe0, 0x8a, 0x23, 0x5f, 0xc9, 0x85, 0x68, 0x8b, 0xf8, 0xd5,
	0x82, 0xf3, 0xc5, 0x8a, 0x1d, 0x1b, 0xf6, 0x66, 0xfd, 0xfd, 0x58, 0x55, 0x35, 0x93, 0x8a, 0xd6,
	0xad, 0x15, 0x4e, 0xfe, 0x78, 0x30, 0xc0, 0xec, 0xe7, 0x9a, 0x49, 0x85, 0x3e, 0xc0, 0xa0, 0xe0,
	0x75, 0x4d, 0x9b, 0x32, 0xea, 0x8d, 0x7b, 0xd3, 0xd1, 0xec, 0x45, 0x62, 0xb7, 0x49, 0x3a, 0x45,
	0x32, 0xb7, 0xcb, 0xe4, 0xae, 0x65, 0x78, 0xa3, 0x45, 0x09, 0xf8, 0x2d, 0x15, 0xb4, 0x96, 0x91,
	0x33, 0xee, 0x4d, 0x0f, 0x67, 0xcf, 0xb6, 0x5d, 0x99, 0x59, 0xc5, 0x9d, 0x0a, 0x7d, 0x82, 0xe0,
	0x3e, 0x45, 0xe4, 0x1a, 0x4b, 0x9c, 0xd8, 0x9c, 0xc9, 0x26, 0x67, 0x42, 0x36, 0x0a, 0xfc, 0x20,
	0x8e, 0xdf, 0x03, 0x9c, 0x13, 0x92, 0x9d, 0x33, 0x5a, 0x32, 0x81, 0x42, 0x70, 0x7f, 0xb0, 0x3b,
	0x13, 0x35, 0xc0, 0x1a, 0xa2, 0xa7, 0xe0, 0xdd, 0xd2, 0xd5, 0x9a, 0x99, 0x20, 0x01, 0xb6, 0x1f,
	0xf1, 0x5f, 0x07, 0x7c, 0x1b, 0x01, 0x21, 0xe8, 0x37, 0xb4, 0x66, 0x9d, 0xc7, 0x60, 0xbd, 0xcd,
	0x5a, 0xac, 0x3a, 0x8b, 0x86, 0x68, 0x06, 0x7e, 0xcd, 0xd4, 0x92, 0x97, 0x26, 0xdd, 0x68, 0x16,
	0x6f, 0x5f, 0x48, 0x87, 0xb8, 0x30, 0x0a, 0xdc, 0x29, 0x51, 0x0c, 0x43, 0x93, 0xbd, 0xe0, 0xab,
	0xa8, 0x6f, 0xb6, 0xba, 0xff, 0xd6, 0xa7, 0x2e, 0xb9, 0x54, 0x91, 0x67, 0x4f, 0xd5, 0x58, 0x73,
	0x2d, 0x17, 0x2a, 0xf2, 0x2d, 0xa7, 0xb1, 0xe1, 0xa8, 0x5a, 0x46, 0x83, 0x8e, 0xa3, 0x6a, 0x89,
	0xa6, 0x70, 0x54, 0xd3, 0x5f, 0x73, 0xde, 0x14, 0x6b, 0x21, 0x58, 0x53, 0x30, 0x19, 0x0d, 0xc7,
	0xbd, 0xa9, 0x87, 0xb7, 0x69, 0x9d, 0x7a, 0x69, 0x8a, 0x89, 0x82, 0xb1, 0x6b, 0x3a, 0xdd, 0x91,
	0xda, 0x56, 0x87, 0x3b, 0xa5, 0x3e, 0xf1, 0x86, 0x97, 0x77, 0x11, 0xd8, 0x13, 0x35, 0x9e, 0x9c,
	0xc0, 0xe1, 0xa3, 0x67, 0x46, 0x01, 0x78, 0x39, 0x39, 0xc1, 0x24, 0x3c, 0x40, 0x87, 0x30, 0xc0,
	0xa9, 0xfd, 0xe8, 0xa1, 0x21, 0xf4, 0x73, 0x72, 0x99, 0x85, 0x0e, 0x0a, 0xe1, 0xc9, 0x59, 0x4a,
	0xae, 0x2f, 0x52, 0x82, 0xbf, 0xce, 0xd3, 0x3c, 0x74, 0x27, 0x5f, 0xec, 0x3b, 0xd9, 0x8a, 0xd0,
	0x00, 0xdc, 0xb3, 0x54, 0xfb, 0x87, 0xd0, 0xcf, 0x2e, 0x73, 0x6d, 0x1e, 0x80, 0x9b, 0x5d, 0x91,
	0xd0, 0x41, 0x00, 0xfe, 0x69, 0xfa, 0x2d, 0x25, 0x69, 0xe8, 0x6a, 0x7c, 0x95, 0x9d, 0x9e, 0x90,
	0x34, 0xec, 0x4f, 0x3e, 0xc3, 0x10, 0x33, 0xd9, 0xf2, 0x46, 0x32, 0xf4, 0x12, 0x80, 0x09, 0xc1,
	0xc5, 0x75, 0xc1, 0x4b, 0xfb, 0x74, 0x1e, 0x0e, 0x0c, 0x33, 0xe7, 0x25, 0xd3, 0x77, 0x28, 0xa9,
	0xa2, 0xdd, 0x03, 0x1a, 0x3c, 0xb9, 0x86, 0x23, 0xcc, 0x16, 0x95, 0x54, 0x4c, 0x6c, 0x86, 0x7b,
	0x04, 0x4e, 0x55, 0x76, 0x6e, 0xa7, 0x2a, 0xef, 0x47, 0xc1, 0x79, 0x34, 0x0a, 0x3b, 0xca, 0x76,
	0x77, 0x96, 0x3d, 0xfb, 0xed, 0x40, 0x40, 0xf2, 0x39, 0x6f, 0x94, 0xe0, 0x2b, 0xf4, 0x06, 0xbc,
	0x5c, 0x51, 0xa1, 0xd0, 0xd1, 0x56, 0xe7, 0x71, 0xf8, 0x40, 0xd8, 0xdb, 0x4c, 0x0e, 0xd0, 0x6b,
	0xe8, 0xe7, 0x8a, 0xb7, 0xfb, 0x89, 0x13, 0xfd, 0x7b, 0xca, 0xfd, 0x37, 0x7f, 0x0b, 0x70, 0xc6,
	0xd4, 0x05, 0x53, 0xa2, 0x2a, 0xe4, 0x7e, 0x96, 0x8f, 0xba, 0x6b, 0x5b, 0x16, 0x7a, 0xfe, 0xb0,
	0xfe, 0x5f, 0x7d, 0xbb, 0x8c, 0x37, 0xbe, 0x99, 0xf0, 0x77, 0xff, 0x02, 0x00, 0x00, 0xff, 0xff,
	0x70, 0xdb, 0x04, 0x91, 0x7d, 0x04, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// TSControlClient is the client API for TSControl service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type TSControlClient interface {
	Start(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error)
	Stop(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error)
	Restart(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error)
	GetMetrics(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error)
	Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*Response, error)
}

type tSControlClient struct {
	cc grpc.ClientConnInterface
}

func NewTSControlClient(cc grpc.ClientConnInterface) TSControlClient {
	return &tSControlClient{cc}
}

func (c *tSControlClient) Start(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/tsgrpc.TSControl/Start", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tSControlClient) Stop(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/tsgrpc.TSControl/Stop", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tSControlClient) Restart(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/tsgrpc.TSControl/Restart", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tSControlClient) GetMetrics(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/tsgrpc.TSControl/GetMetrics", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tSControlClient) Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*Response, error) {
	out := new(Response)
	err := c.cc.Invoke(ctx, "/tsgrpc.TSControl/Register", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TSControlServer is the server API for TSControl service.
type TSControlServer interface {
	Start(context.Context, *Request) (*Response, error)
	Stop(context.Context, *Request) (*Response, error)
	Restart(context.Context, *Request) (*Response, error)
	GetMetrics(context.Context, *Request) (*Response, error)
	Register(context.Context, *RegisterRequest) (*Response, error)
}

// UnimplementedTSControlServer can be embedded to have forward compatible implementations.
type UnimplementedTSControlServer struct {
}

func (*UnimplementedTSControlServer) Start(ctx context.Context, req *Request) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Start not implemented")
}
func (*UnimplementedTSControlServer) Stop(ctx context.Context, req *Request) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Stop not implemented")
}
func (*UnimplementedTSControlServer) Restart(ctx context.Context, req *Request) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Restart not implemented")
}
func (*UnimplementedTSControlServer) GetMetrics(ctx context.Context, req *Request) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetMetrics not implemented")
}
func (*UnimplementedTSControlServer) Register(ctx context.Context, req *RegisterRequest) (*Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Register not implemented")
}

func RegisterTSControlServer(s *grpc.Server, srv TSControlServer) {
	s.RegisterService(&_TSControl_serviceDesc, srv)
}

func _TSControl_Start_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TSControlServer).Start(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tsgrpc.TSControl/Start",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TSControlServer).Start(ctx, req.(*Request))
	}
	return interceptor(ctx, in, info, handler)
}

func _TSControl_Stop_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TSControlServer).Stop(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tsgrpc.TSControl/Stop",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TSControlServer).Stop(ctx, req.(*Request))
	}
	return interceptor(ctx, in, info, handler)
}

func _TSControl_Restart_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TSControlServer).Restart(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tsgrpc.TSControl/Restart",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TSControlServer).Restart(ctx, req.(*Request))
	}
	return interceptor(ctx, in, info, handler)
}

func _TSControl_GetMetrics_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Request)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TSControlServer).GetMetrics(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tsgrpc.TSControl/GetMetrics",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TSControlServer).GetMetrics(ctx, req.(*Request))
	}
	return interceptor(ctx, in, info, handler)
}

func _TSControl_Register_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TSControlServer).Register(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/tsgrpc.TSControl/Register",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TSControlServer).Register(ctx, req.(*RegisterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _TSControl_serviceDesc = grpc.ServiceDesc{
	ServiceName: "tsgrpc.TSControl",
	HandlerType: (*TSControlServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Start",
			Handler:    _TSControl_Start_Handler,
		},
		{
			MethodName: "Stop",
			Handler:    _TSControl_Stop_Handler,
		},
		{
			MethodName: "Restart",
			Handler:    _TSControl_Restart_Handler,
		},
		{
			MethodName: "GetMetrics",
			Handler:    _TSControl_GetMetrics_Handler,
		},
		{
			MethodName: "Register",
			Handler:    _TSControl_Register_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "services.proto",
}
