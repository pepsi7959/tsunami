package main

import (
	context "context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

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
		Protocol:    req.Params.Protocol,
		Host:        req.Params.Host,
		Port:        req.Params.Port,
		Path:        req.Params.Path,
		Method:      methodToString(req.Params.Method),
		Body:        req.Params.Body,
		Concurrence: int(req.Params.MaxConcurrences),
	}

	if s.Ctrl.services[tsConf.Name] == nil {
		go StartApp(tsConf.Name, s.Ctrl, tsConf)
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

	return nil, errors.New(tsConf.Name + " already exist")
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

	return nil, errors.New("not found service")
}

//Restart send re-start command
func (s *GRPCServer) Restart(context context.Context, r *tsgrpc.Request) (*tsgrpc.Response, error) {
	fmt.Println("Restart Received")
	return nil, nil
}

//GetMetrics request to metrics
func (s *GRPCServer) GetMetrics(context context.Context, req *tsgrpc.Request) (*tsgrpc.Response, error) {
	fmt.Println("GetMetrics Received")

	ts := s.Ctrl.services[req.Params.Name]

	if ts == nil {
		return nil, errors.New("not found service")
	}

	var avg, min, max float64
	var numRes, numErr int
	//data := make(map[string]string)

	for _, w := range ts.workers {
		numRes += w.GetNumRes()
		numErr += w.GetNumErr()
		avg += w.GetAvgRes()

		if w.GetMaxRes() > max {
			max = w.GetMaxRes()
		}

		if min == 0.0 || w.GetMinRes() < min {
			min = w.GetMinRes()
		}
	}

	workers := len(ts.workers)
	avg = avg / float64(workers)
	// data["name"] = ts.conf.Name
	// data["workers_count"] = fmt.Sprintf("%d", workers)
	// data["errors_count"] = fmt.Sprintf("%d", numErr)
	// data["avg"] = fmt.Sprintf("%f", avg)
	// data["min"] = fmt.Sprintf("%f", min)
	// data["max"] = fmt.Sprintf("%f", max)
	// data["elaped_time"] = fmt.Sprintf("%f", time.Since(ts.start).Seconds())
	// data["requests_count"] = fmt.Sprintf("%f", float64(numRes))
	// data["rps"] = fmt.Sprintf("%f", float64(numRes)/time.Since(ts.start).Seconds())

	jsonStruct := tshttp.Metric{
		Name:         ts.conf.Name,
		WorkerCount:  workers,
		ErrorCount:   numErr,
		Avg:          avg,
		Min:          min,
		Max:          max,
		ElapedTime:   time.Since(ts.start).Seconds(),
		RequestCount: numRes,
		Rps:          (float64(numRes) / time.Since(ts.start).Seconds()),
	}

	JSONData, err := json.Marshal(&jsonStruct)

	fmt.Printf("%v \n", jsonStruct)
	fmt.Printf("%v \n", string(JSONData))

	if err != nil {
		fmt.Println("error marshal: " + err.Error())
		return nil, err
	}

	resp := tsgrpc.Response{
		ErrorCode: 0,
		Data:      string(JSONData),
	}

	return &resp, nil
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
