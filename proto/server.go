package tsgrpc

import (
	context "context"
	"fmt"
	"net"

	"google.golang.org/grpc"
)

//GRPCServer server for gRPC connecion
type GRPCServer struct {
	Endpoint   string
	Transport  net.Listener
	GrpcServer *grpc.Server
}

//Start send start command
func (s *GRPCServer) Start(context.Context, *Request) (*Response, error) {
	fmt.Println("Start Received")
	r := Response{
		ErrorCode: 0,
		Data:      "pepsi",
	}
	return &r, nil
}

//Stop send stop command
func (s *GRPCServer) Stop(context.Context, *Request) (*Response, error) {
	return nil, nil
}

//Restart send re-start command
func (s *GRPCServer) Restart(context.Context, *Request) (*Response, error) {
	return nil, nil
}

//GetMetrics request to metrics
func (s *GRPCServer) GetMetrics(context.Context, *Request) (*Response, error) {
	return nil, nil
}

//NewServer create server instance
func NewServer(endpoint string) *GRPCServer {
	return &GRPCServer{Endpoint: endpoint}
}

//InitServer initilize gRPC server
func (s *GRPCServer) InitServer() {
	transp, err := net.Listen("tcp", s.Endpoint)

	if err != nil {
		panic(err)
	}

	s.GrpcServer = grpc.NewServer()
	s.Transport = transp
}

//StartServer start gRPC server
func (s *GRPCServer) StartServer() {

	fmt.Println("gRPC listen: ", s.Endpoint)

	RegisterTSControlServer(s.GrpcServer, s)

	if err := s.GrpcServer.Serve(s.Transport); err != nil {
		panic(err)
	}

}

//StopServer stop gRPC server
func (s *GRPCServer) StopServer() {
	s.GrpcServer.GracefulStop()
}
