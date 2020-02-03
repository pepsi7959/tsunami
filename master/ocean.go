package main

import (
	tshttp "github.com/tsunami/libs"
)

//APIVersion version of APIs
var APIVersion = "/api/v1"

// AllowOrigin is a header to allow cross domain
var AllowOrigin = "*"

// Listen port
var Listen = ":8080"

// worker state
// Cycle of State
//
// --> stop --> waiting --> Ready --> Busy --|---|
// |                           ^--------------|  |
// |---------------------------------------------|
const (
	//WokerStateWaiting waiting for complete connection
	WokerStateWaiting = 0
	//WokerStateReady ready to serve
	WokerStateReady = 1
	//WokerStateReady full of qouta
	WokerBusy = 2
	//WokerStop inactive worker
	WokerStop = 3
)

//Stat statistic of service
type Stat struct {
	NumofRequests int
	Max           float32
	Min           float32
	ErrorCount    int
	RPS           float32
}

//Job structure
type Job struct {
	conf               tshttp.Conf
	stat               Stat
	pendingConcurrence int
}

//Worker information of worker
type Worker struct {

	//state state of worker
	state int

	//endpoint is "ip:port"
	endpoint string

	//name is worker name
	name string

	//maxQuata maximum concurrent request
	maxQouta int

	//remainingQouta is the left of curcurrent request
	remainingQouta int

	gRPCClient *GRPCClient
}

// Ocean keep all information regarding to user request
type Ocean struct {
	//Workers list of registered worker
	workers map[string]*Worker

	//jobs is map which is asigned from an user
	//key is service name, value is request from user
	jobs map[string]*Job

	//jobToWorkers is mapping between a job and workers
	jobToWorkers map[string][]*Worker
}

func main() {
	ocs := Ocean{
		jobs:    make(map[string]*Job),
		workers: make(map[string]*Worker),
	}
	//Connection between Ocean and Tsunami
	gRPCClient := NewClient()
	gRPCClient.InitClient("127.0.0.1:8050")

	ocs.workers["w1"] = &Worker{
		state:          WokerStateReady,
		endpoint:       "127.0.0.1:8050",
		name:           "w1",
		maxQouta:       100,
		remainingQouta: 100,
		gRPCClient:     gRPCClient,
	}

	app := &tshttp.App{}
	app.Init("8080")
	app.AddAPI(APIVersion+"/start", ocs.Start)
	app.AddAPI(APIVersion+"/stop", ocs.Stop)
	app.AddAPI(APIVersion+"/metrics", ocs.GetMetrics)
	app.Run()

}
