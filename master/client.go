package main

import (
	"context"
	"fmt"
	"time"

	tsgrpc "github.com/tsunami/proto"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//GRPCClient structure
type GRPCClient struct {
	ServerEnpoint string
	conn          *grpc.ClientConn
	clnt          tsgrpc.TSControlClient
}

//Start send start command to worker
func (c *GRPCClient) Start(r *tsgrpc.Request) (*tsgrpc.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	res, err := c.clnt.Start(ctx, r)
	statusCode := status.Code(err)

	if statusCode != codes.OK {
		return nil, err
	}

	return res, nil
}

//Stop send stop command
func (c *GRPCClient) Stop(r *tsgrpc.Request) (*tsgrpc.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	res, err := c.clnt.Stop(ctx, r)
	statusCode := status.Code(err)

	if statusCode != codes.OK {
		return nil, err
	}

	return res, nil
}

//Restart send restart command
func (c *GRPCClient) Restart(*tsgrpc.Request) (*tsgrpc.Response, error) {
	return nil, nil
}

//GetMetrics request the metrics
func (c *GRPCClient) GetMetrics(req *tsgrpc.Request) (*tsgrpc.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	res, err := c.clnt.GetMetrics(ctx, req)
	statusCode := status.Code(err)

	if statusCode != codes.OK {
		return nil, err
	}

	return res, nil
}

//Register register to master node
func (c *GRPCClient) Register(id int32, name string, maxCon int32) (*tsgrpc.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := tsgrpc.RegisterRequest{
		Id:              id,
		Name:            name,
		MaxConcurrences: maxCon,
	}

	res, err := c.clnt.Register(ctx, &r)
	statusCode := status.Code(err)

	if statusCode != codes.OK {
		return nil, err
	}

	return res, nil

}

//NewClient creat new client
func NewClient() *GRPCClient {
	return &GRPCClient{}
}

//InitClient initlize client
func (c *GRPCClient) InitClient(endpointServer string) {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(endpointServer, opts...)
	if err != nil {
		fmt.Println("InitClient error : ", err.Error())
	}

	clnt := tsgrpc.NewTSControlClient(conn)

	c.conn = conn
	c.clnt = clnt
}

//Close close conection
func (c *GRPCClient) Close() {
	c.conn.Close()
}
