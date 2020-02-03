package main

import (
	context "context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	tshttp "github.com/tsunami/libs"
	tsgrpc "github.com/tsunami/proto"
	"google.golang.org/grpc"
)

//GRPCServer server for gRPC connecion
type GRPCServer struct {
	Endpoint   string
	Transport  net.Listener
	GrpcServer *grpc.Server
	Ctrl       *TSControl
}

func methodToString(method tsgrpc.Request_HTTPMethod) string {

	if method == 0 {
		return "GET"
	} else if method == 1 {
		return "POST"
	} else if method == 2 {
		return "PUT"
	} else if method == 3 {
		return "DELETE"
	} else if method == 4 {
		return "UPDATE"
	}

	return "Unknown Method"
}

//Start send start command
func (s *GRPCServer) Start(context context.Context, req *tsgrpc.Request) (*tsgrpc.Response, error) {
	fmt.Println("Start Received")

	tsConf := tshttp.Conf{
		Name:        req.Params.Name,
		URL:         req.Params.Url,
		Host:        req.Params.Host,
		Method:      methodToString(req.Params.Method),
		Body:        req.Params.Body,
		Concurrence: int(req.Params.MaxConcurrences),
	}

	if s.Ctrl.services[tsConf.Name] == nil {
		go StartApp(tsConf.Name, s.Ctrl, tsConf)
	}

	data := struct {
		url  string
		name string
	}{
		url:  "http://" + tshttp.GetIP().String() + ":8091" + APIVersion,
		name: tsConf.Name,
	}
	jdata, _ := json.Marshal(data)

	res := tsgrpc.Response{
		ErrorCode: 0,
		Data:      string(jdata),
	}
	return &res, nil
}

//Stop send stop command
func (s *GRPCServer) Stop(context context.Context, req *tsgrpc.Request) (*tsgrpc.Response, error) {
	fmt.Println("Stop Received")

	tsConf := tshttp.Conf{
		Name:        req.Params.Name,
		URL:         req.Params.Url,
		Host:        req.Params.Host,
		Method:      methodToString(req.Params.Method),
		Body:        req.Params.Body,
		Concurrence: int(req.Params.MaxConcurrences),
	}

	t := s.Ctrl.services[tsConf.Name]
	if t != nil {
		fmt.Println("Stoping service name : ", tsConf.Name)
		t.Stop()
		t.apiServer.Stop()
		delete(s.Ctrl.services, tsConf.Name)

		res := tsgrpc.Response{
			ErrorCode: 0,
			Data:      "",
		}

		return &res, nil
	}

	return nil, errors.New("Not found service")
}

//Restart send re-start command
func (s *GRPCServer) Restart(context context.Context, r *tsgrpc.Request) (*tsgrpc.Response, error) {
	fmt.Println("Restart Received")
	return nil, nil
}

//GetMetrics request to metrics
func (s *GRPCServer) GetMetrics(context context.Context, r *tsgrpc.Request) (*tsgrpc.Response, error) {
	fmt.Println("GetMetrics Received")
	return nil, nil
}

//Register request to metrics
func (s *GRPCServer) Register(context context.Context, r *tsgrpc.RegisterRequest) (*tsgrpc.Response, error) {
	return nil, nil
}

//NewServer create server instance
func NewServer(endpoint string) *GRPCServer {
	gRPCServer := GRPCServer{
		Endpoint: endpoint,
	}
	return &gRPCServer
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

	tsgrpc.RegisterTSControlServer(s.GrpcServer, s)

	if err := s.GrpcServer.Serve(s.Transport); err != nil {
		panic(err)
	}

}

//StopServer stop gRPC server
func (s *GRPCServer) StopServer() {
	s.GrpcServer.GracefulStop()
}
