package tsgrpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//Client structure
type Client struct {
	ServerEnpoint string
	conn          *grpc.ClientConn
	clnt          TSControlClient
}

//Start send start command to worker
func (c *Client) Start() (*Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	r := Request{
		Command: Request_START,
	}

	res, err := c.clnt.Start(ctx, &r)
	statusCode := status.Code(err)

	if statusCode != codes.OK {
		return nil, err
	}

	return res, nil
}

//Stop send stop command
func (c *Client) Stop(*Request) (*Response, error) {
	return nil, nil
}

//Restart send restart command
func (c *Client) Restart(*Request) (*Response, error) {
	return nil, nil
}

//GetMetrics request the metrics
func (c *Client) GetMetrics(*Request) (*Response, error) {
	return nil, nil
}

//NewClient creat new client
func NewClient() *Client {
	return &Client{}
}

//InitClient initlize client
func (c *Client) InitClient(endpointServer string) {
	opts := []grpc.DialOption{grpc.WithInsecure()}
	conn, err := grpc.Dial(endpointServer, opts...)
	if err != nil {
		fmt.Println("InitClient error : ", err.Error())
	}

	clnt := NewTSControlClient(conn)

	c.conn = conn
	c.clnt = clnt
}

//Close close conection
func (c *Client) Close() {
	c.conn.Close()
}
