package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	tsetcd "github.com/tsunami/etcd"
	tshttp "github.com/tsunami/libs"
	tsregistry "github.com/tsunami/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

//APIVersion version of APIs
var APIVersion = "/api/v1"

// AllowOrigin is a header to allow cross domain
var AllowOrigin = "*"

// Listen port
var Listen = ":8080"

// version is set at build time via -ldflags "-X main.version=<tag>"
var version = "dev"

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

	//etcd is the long-lived registry/coordination client
	etcd *tsetcd.Client
	//masterID uniquely identifies this ocean replica
	masterID string
	//masterLease binds this master's reservations; expires if the master dies
	masterLease clientv3.LeaseID

	//resKeys mirrors every reservation key -> count (from watchReservations)
	resKeys map[string]int
	//resByWorker is the per-worker sum of reservations (used capacity)
	resByWorker map[string]int

	//allJobs mirrors fleet-wide job metadata (from watchJobs) so any master can
	//list/stop/report on jobs owned by any master
	allJobs map[string]JobMeta
	//masters mirrors the live master set (from watchMasters) for the topology view
	masters map[string]MasterMeta

	//mu guards workers/jobs/jobToWorkers/resKeys/resByWorker/allJobs/masters
	mu sync.RWMutex
}

//JobAssign is one worker's share of a job
type JobAssign struct {
	WorkerID string `json:"workerID"`
	Count    int    `json:"count"`
}

//JobMeta is the fleet-wide record of a running job, stored in etcd
type JobMeta struct {
	Name        string      `json:"name"`
	Owner       string      `json:"owner"`
	URL         string      `json:"url"`
	Host        string      `json:"host"`
	Assignments []JobAssign `json:"assignments"`
}

//MasterMeta is a master's registry record
type MasterMeta struct {
	Name string `json:"name"`
	HTTP string `json:"http"`
	IP   string `json:"ip"`
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

	//Coordination key prefixes + lease TTL (active-active multi-master)
	viper.SetDefault("registry.worker_prefix", "/tsunami/workers/")
	viper.SetDefault("registry.master_prefix", "/tsunami/masters/")
	viper.SetDefault("registry.reservation_prefix", "/tsunami/reservations/")
	viper.SetDefault("registry.job_prefix", "/tsunami/jobs/")
	viper.SetDefault("registry.lock_prefix", "/tsunami/locks/worker/")
	viper.SetDefault("lease.ttl", 10)

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
		resKeys:      make(map[string]int),
		resByWorker:  make(map[string]int),
		allJobs:      make(map[string]JobMeta),
		masters:      make(map[string]MasterMeta),
	}
}

//workerByID returns a worker by id (nil if unknown). Takes the read lock.
func (oc *Ocean) workerByID(id string) *Worker {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	return oc.workers[id]
}

//workerIDFromResKey extracts the worker id from a reservation key
// "<reservation_prefix><workerID>/<masterID>/<job>".
func workerIDFromResKey(key, prefix string) string {
	rest := strings.TrimPrefix(key, prefix)
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

//recomputeReservationsLocked rebuilds resByWorker from resKeys. Caller holds mu.
func (oc *Ocean) recomputeReservationsLocked() {
	prefix := oc.viper.GetString("registry.reservation_prefix")
	sum := make(map[string]int)
	for key, n := range oc.resKeys {
		sum[workerIDFromResKey(key, prefix)] += n
	}
	oc.resByWorker = sum
}

//watchReservations keeps a live mirror of all reservation keys so every master
//sees the fleet-wide used capacity (dead masters' reservations expire via lease).
func (oc *Ocean) watchReservations() {
	prefix := oc.viper.GetString("registry.reservation_prefix")

	if kvs, err := oc.etcd.GetPrefix(prefix); err == nil {
		oc.mu.Lock()
		for _, kv := range kvs {
			n, _ := strconv.Atoi(string(kv.Value))
			oc.resKeys[kv.Key] = n
		}
		oc.recomputeReservationsLocked()
		oc.mu.Unlock()
	}

	for resp := range oc.etcd.WatchPrefix(context.Background(), prefix) {
		oc.mu.Lock()
		for _, ev := range resp.Events {
			key := string(ev.Kv.Key)
			if ev.Type == clientv3.EventTypeDelete {
				delete(oc.resKeys, key)
			} else {
				n, _ := strconv.Atoi(string(ev.Kv.Value))
				oc.resKeys[key] = n
			}
		}
		oc.recomputeReservationsLocked()
		oc.mu.Unlock()
	}
}

//initEtcd opens the long-lived registry client and registers this master under
//a lease so peers can see it and its reservations auto-expire if it dies.
func (oc *Ocean) initEtcd() error {
	ttl := int64(oc.viper.GetInt("lease.ttl"))
	if ttl <= 0 {
		ttl = 10
	}
	cli, err := tsetcd.NewClient(
		oc.viper.GetStringSlice("registry.endpoints"),
		time.Second*time.Duration(oc.viper.GetInt("registry.dial_timeout")),
		time.Second*time.Duration(oc.viper.GetInt("registry.request_timeout")),
		int(ttl),
	)
	if err != nil {
		return err
	}
	oc.etcd = cli
	oc.masterID = oc.viper.GetString("id")

	lease, err := cli.Grant(ttl)
	if err != nil {
		return err
	}
	oc.masterLease = lease
	ip := ""
	if p := tshttp.GetIP(); p != nil {
		ip = p.String()
	}
	meta, _ := json.Marshal(MasterMeta{
		Name: oc.viper.GetString("name"),
		HTTP: oc.viper.GetString("endpoints.http"),
		IP:   ip,
	})
	if err := cli.Put(oc.viper.GetString("registry.master_prefix")+oc.masterID, string(meta), clientv3.WithLease(lease)); err != nil {
		return err
	}
	return cli.KeepAlive(lease)
}

//watchJobs keeps a live fleet-wide mirror of job metadata (job_prefix).
func (oc *Ocean) watchJobs() {
	prefix := oc.viper.GetString("registry.job_prefix")
	load := func(kvKey string, val []byte, del bool) {
		name := strings.TrimPrefix(kvKey, prefix)
		oc.mu.Lock()
		if del {
			delete(oc.allJobs, name)
		} else {
			var m JobMeta
			if json.Unmarshal(val, &m) == nil {
				oc.allJobs[name] = m
			}
		}
		oc.mu.Unlock()
	}
	if kvs, err := oc.etcd.GetPrefix(prefix); err == nil {
		for _, kv := range kvs {
			load(kv.Key, kv.Value, false)
		}
	}
	for resp := range oc.etcd.WatchPrefix(context.Background(), prefix) {
		for _, ev := range resp.Events {
			load(string(ev.Kv.Key), ev.Kv.Value, ev.Type == clientv3.EventTypeDelete)
		}
	}
}

//watchMasters keeps a live mirror of the master set (master_prefix).
func (oc *Ocean) watchMasters() {
	prefix := oc.viper.GetString("registry.master_prefix")
	load := func(kvKey string, val []byte, del bool) {
		id := strings.TrimPrefix(kvKey, prefix)
		oc.mu.Lock()
		if del {
			delete(oc.masters, id)
		} else {
			var m MasterMeta
			if json.Unmarshal(val, &m) == nil {
				oc.masters[id] = m
			}
		}
		oc.mu.Unlock()
	}
	if kvs, err := oc.etcd.GetPrefix(prefix); err == nil {
		for _, kv := range kvs {
			load(kv.Key, kv.Value, false)
		}
	}
	for resp := range oc.etcd.WatchPrefix(context.Background(), prefix) {
		for _, ev := range resp.Events {
			load(string(ev.Kv.Key), ev.Kv.Value, ev.Type == clientv3.EventTypeDelete)
		}
	}
}

//DiscoverWorkers (re)seeds oc.workers from the registry prefix — used at boot
//and by the /workers/reload endpoint. Returns the number of newly added workers.
func (oc *Ocean) DiscoverWorkers() (int, error) {
	list, err := oc.etcd.GetPrefix(oc.viper.GetString("registry.worker_prefix"))
	if err != nil {
		return 0, err
	}
	added := 0
	oc.mu.Lock()
	defer oc.mu.Unlock()
	for _, kv := range list {
		conf := tsregistry.Conf{}
		if err := json.Unmarshal(kv.Value, &conf); err != nil {
			continue
		}
		if conf.ID == "" || oc.workers[conf.ID] != nil {
			continue
		}
		oc.workers[conf.ID] = oc.NewWorker(&conf)
		added++
	}
	return added, nil
}

//watchWorkers reacts to registry changes in real time: PUT adds/refreshes a
//worker (dialing its gRPC endpoint), DELETE (or lease expiry) prunes it.
func (oc *Ocean) watchWorkers() {
	prefix := oc.viper.GetString("registry.worker_prefix")
	for resp := range oc.etcd.WatchPrefix(context.Background(), prefix) {
		for _, ev := range resp.Events {
			id := strings.TrimPrefix(string(ev.Kv.Key), prefix)
			if ev.Type == clientv3.EventTypeDelete {
				oc.mu.Lock()
				if w := oc.workers[id]; w != nil {
					if w.gRPCClient != nil {
						w.gRPCClient.Close()
					}
					delete(oc.workers, id)
					log.Printf("registry: worker %s left", id)
				}
				oc.mu.Unlock()
				continue
			}
			conf := tsregistry.Conf{}
			if err := json.Unmarshal(ev.Kv.Value, &conf); err != nil || conf.ID == "" {
				continue
			}
			oc.mu.Lock()
			if oc.workers[conf.ID] == nil {
				oc.workers[conf.ID] = oc.NewWorker(&conf)
				log.Printf("registry: worker %s joined (%s)", conf.ID, conf.Endpoint)
			}
			oc.mu.Unlock()
		}
	}
}

//NewWorker create a worker
func (oc *Ocean) NewWorker(wrkConf *tsregistry.Conf) *Worker {

	//Connection between Ocean and Tsunami
	gRPCClient := NewClient()
	gRPCClient.InitClient(wrkConf.Endpoint)

	return &Worker{
		state:      WokerStateReady,
		endpoint:   wrkConf.Endpoint,
		ID:         wrkConf.ID,
		name:       wrkConf.Name,
		maxQouta:   wrkConf.MaxConcurrences,
		gRPCClient: gRPCClient,
	}
}

func main() {

	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			fmt.Println("ocean", version)
			return
		}
	}

	conf := readConf()
	ocs := New(conf)

	log.Println("Master ID: ", ocs.viper.GetString("id"))

	//Connect to the registry and register this master under a lease
	if err := ocs.initEtcd(); err != nil {
		log.Fatalf("registry: init failed: %v\n", err.Error())
	}

	//Seed the current worker set, then react to joins/leaves in real time
	if _, err := ocs.DiscoverWorkers(); err != nil {
		log.Fatalf("registry: seed workers failed: %v\n", err.Error())
	}
	go ocs.watchWorkers()
	go ocs.watchReservations()
	go ocs.watchJobs()
	go ocs.watchMasters()

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
	app.AddAPI(APIVersion+"/workers/reload", ocs.ReloadWorkers)
	app.Run()

}
