package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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

	//ID worker
	ID string

	//worker name
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

	viper *viper.Viper
}

func readConf() *viper.Viper {

	viper := viper.New()

	flag.String("path", ".", "configuration path")
	flag.String("file", "config.yaml", "config file name")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	viper.BindPFlags(pflag.CommandLine)

	confName := viper.GetString("file")
	confName = strings.Split(confName, ".")[0]

	log.Printf("config file: %v\n", confName)

	viper.SetConfigName(confName)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./")
	viper.AddConfigPath(viper.GetString("path"))

	viper.SetDefault("name", "master")
	viper.SetDefault("id", "fed3f59d-9b32-4efe-a372-ca02d7ea9f66")

	//Registry Configuration
	viper.SetDefault("registry.endpoints", []string{"localhost:2379", "localhost:22379", "localhost:32379"})
	viper.SetDefault("registry.request_timeout", 2)
	viper.SetDefault("registry.dial_timeout", 2)

	//Key range of client
	viper.SetDefault("registry.client_config_key", "tsunami_config_client_")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
	return viper
}

//New create Ocean
func New(conf *viper.Viper) *Ocean {
	return &Ocean{
		viper:        conf,
		jobs:         make(map[string]*Job),
		workers:      make(map[string]*Worker),
		jobToWorkers: make(map[string][]*WorkerInfo),
	}
}

//NewEtcdClient create ocean client
func (oc *Ocean) NewEtcdClient() tsetcd.EtcdClient {
	return tsetcd.EtcdClient{
		Conf: clientv3.Config{
			Endpoints:   oc.viper.GetStringSlice("registry.endpoints"),
			DialTimeout: time.Second * time.Duration(oc.viper.GetInt("registry.dial_timeout")),
		},
		RequestTimeOut: time.Second * time.Duration(oc.viper.GetInt("registry.request_timeout")),
		Done:           false,
	}
}

//NewWorker create a worker
func (oc *Ocean) NewWorker(wrkConf *tsregistry.Conf) *Worker {

	//Connection between Ocean and Tsunami
	gRPCClient := NewClient()
	gRPCClient.InitClient(wrkConf.Endpoint)

	return &Worker{
		state:          WokerStateReady,
		endpoint:       wrkConf.Endpoint,
		ID:             wrkConf.ID,
		name:           wrkConf.Name,
		maxQouta:       wrkConf.MaxConcurrences,
		remainingQouta: wrkConf.MaxConcurrences,
		gRPCClient:     gRPCClient,
	}
}

func main() {

	conf := readConf()
	ocs := New(conf)

	log.Println("Master ID: ", ocs.viper.GetString("id"))

	etcdClient := ocs.NewEtcdClient()

	//Get list of woker from registry(Etcd)
	wokerConfLists, err := etcdClient.GetRange(ocs.viper.GetString("client_config_key"))

	if err != nil {
		log.Fatalf("etcd connect failed: %v\n", err.Error())
	}

	for _, v := range wokerConfLists {

		conf := tsregistry.Conf{}
		err = json.Unmarshal(v, &conf)

		if err != nil {
			log.Fatalf(err.Error())
		}

		//log.Println("key: ", k, " value: ", conf)

		ocs.workers[conf.ID] = ocs.NewWorker(&conf)
	}

	go func() {
		for true {
			ocs.Monitoring()
			time.Sleep(1 * time.Second)
		}
	}()

	app := &tshttp.App{}
	app.Init(ocs.viper.GetString("endpoints.http"))
	app.AddAPI(APIVersion+"/start", ocs.Start)
	app.AddAPI(APIVersion+"/stop", ocs.Stop)
	app.AddAPI(APIVersion+"/metrics", ocs.GetMetrics)
	app.AddAPI(APIVersion+"/info", ocs.GetInfo)
	app.Run()

}
