package main

import (
	tshttp "github.com/tsunami/libs"
)

//APIVersion version of APIs
var APIVersion = "/api/v1"

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
	//endpoint is "ip:port"
	endpoint string

	//name is worker name
	name string

	//maxQuata maximum concurrent request
	maxQouta int

	//remainingQouta is the left of curcurrent request
	remainingQouta int
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
	ocs := Ocean{jobs: make(map[string]*Job)}

	app := &tshttp.App{}
	app.Init("8080")
	app.AddAPI(APIVersion+"/start", ocs.Start)
	app.AddAPI(APIVersion+"/stop", ocs.Stop)
	app.AddAPI(APIVersion+"/metrics", ocs.GetMetrics)
	app.Run()

}
