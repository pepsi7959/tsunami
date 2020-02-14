package main

import (
	"encoding/json"
	"log"
	"time"

	tsetcd "github.com/tsunami/etcd"
	tshttp "github.com/tsunami/libs"
	tsregistry "github.com/tsunami/registry"
	clientv3 "go.etcd.io/etcd/clientv3"
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

//WorkerInfo keep assignement and worker information
type WorkerInfo struct {
	//number of concurrence have been assigned to the worker
	concurrence int

	//The worker that was assigned a job.
	worker *Worker
}

// Ocean keep all information regarding to user request
type Ocean struct {
	//Workers list of registered worker
	workers map[string]*Worker

	//jobs is map which is assigned from an user
	//key is service name, value is request from user
	jobs map[string]*Job

	//jobToWorkers is mapping between a job and workers
	jobToWorkers map[string][]*WorkerInfo
}

func main() {

	ocs := Ocean{
		jobs:         make(map[string]*Job),
		workers:      make(map[string]*Worker),
		jobToWorkers: make(map[string][]*WorkerInfo),
	}

	etcdClient := tsetcd.EtcdClient{
		Conf: clientv3.Config{
			Endpoints:   []string{"localhost:2379", "localhost:22379", "localhost:32379"},
			DialTimeout: 5 * time.Second,
		},
		RequestTimeOut: time.Second * 5,
		Done:           false,
	}

	key := "tsunami_config_client_"
	workerConfs, err := etcdClient.GetRange(key)

	if err != nil {
		log.Fatalf("etcd connect failed: %v\n", err.Error())
	}

	for k, v := range workerConfs {

		wkConf := tsregistry.Conf{}
		err = json.Unmarshal(v, &wkConf)

		if err != nil {
			log.Fatalf(err.Error())
		}

		log.Println(k, " : ", wkConf)

		//Connection between Ocean and Tsunami
		gRPCClient := NewClient()
		gRPCClient.InitClient(wkConf.Endpoint)

		ocs.workers[wkConf.ID] = &Worker{
			state:          WokerStateReady,
			endpoint:       wkConf.Endpoint,
			name:           wkConf.ID,
			maxQouta:       wkConf.MaxConcurrences,
			remainingQouta: wkConf.MaxConcurrences,
			gRPCClient:     gRPCClient,
		}
	}

	go func() {
		for true {
			ocs.Monitoring()
			time.Sleep(1 * time.Second)
		}
	}()

	app := &tshttp.App{}
	app.Init("8080")
	app.AddAPI(APIVersion+"/start", ocs.Start)
	app.AddAPI(APIVersion+"/stop", ocs.Stop)
	app.AddAPI(APIVersion+"/metrics", ocs.GetMetrics)
	app.Run()

}
