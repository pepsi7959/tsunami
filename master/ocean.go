package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	tshttp "github.com/tsunami/libs"
	tsgrpc "github.com/tsunami/proto"
	"google.golang.org/grpc"
)

//APIVersion version of APIs
var APIVersion = "/api/v1"

// AllowOrigin is a header to allow cross domain
var AllowOrigin = "*"

// version is set at build time via -ldflags "-X main.version=<tag>"
var version = "dev"

// attachedWorker is a worker currently holding a stream to this ocean.
type attachedWorker struct {
	id      string
	name    string
	ip      string
	maxCap  int
	used    int                              // concurrency currently assigned
	send    chan *tsgrpc.OceanMessage         // buffered; drained to the stream
	metrics map[string]*tsgrpc.Metric         // latest per-job metric from this worker
}

// job is a running load test owned by this ocean.
type job struct {
	name        string
	conf        tshttp.Conf
	assignments map[string]int // workerID -> concurrency
}

// Ocean is the job distributor: it accepts client requests over HTTP and drives
// its attached workers over their gRPC streams. All state is in memory.
type Ocean struct {
	viper          *viper.Viper
	id             string
	name           string
	maxConnections int

	mu      sync.Mutex
	workers map[string]*attachedWorker
	jobs    map[string]*job
}

func readConf() *viper.Viper {
	v := viper.New()

	flag.String("path", ".", "configuration path")
	flag.String("file", "config.yaml", "config file name")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	v.BindPFlags(pflag.CommandLine)

	confName := strings.Split(v.GetString("file"), ".")[0]
	log.Printf("config file: %v\n", confName)
	v.SetConfigName(confName)
	v.SetConfigType("yaml")
	v.AddConfigPath("./")
	v.AddConfigPath(v.GetString("path"))

	v.SetDefault("name", "ocean")
	v.SetDefault("id", "ocean-0")
	v.SetDefault("endpoints.http", "0.0.0.0:8080") // client HTTP API + web control
	v.SetDefault("endpoints.grpc", "0.0.0.0:8050") // worker attach stream server
	v.SetDefault("max_connections", 1000)          // max attached workers

	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
	return v
}

//New create Ocean
func New(conf *viper.Viper) *Ocean {
	return &Ocean{
		viper:          conf,
		id:             conf.GetString("id"),
		name:           conf.GetString("name"),
		maxConnections: conf.GetInt("max_connections"),
		workers:        make(map[string]*attachedWorker),
		jobs:           make(map[string]*job),
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
	oc := New(conf)
	log.Println("Ocean ID:", oc.id, "name:", oc.name)

	// worker attach gRPC server (workers dial IN and hold a stream)
	grpcAddr := oc.viper.GetString("endpoints.grpc")
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("attach server: listen %s: %v", grpcAddr, err)
	}
	gs := grpc.NewServer()
	tsgrpc.RegisterTSAttachServer(gs, &attachServer{oc: oc})
	go func() {
		log.Println("attach gRPC server:", grpcAddr)
		if err := gs.Serve(lis); err != nil {
			log.Fatalf("attach server: %v", err)
		}
	}()

	// client-facing HTTP API + web control
	app := &tshttp.App{}
	app.Init(oc.viper.GetString("endpoints.http"))
	app.AddAPI(APIVersion+"/start", oc.Start)
	app.AddAPI(APIVersion+"/stop", oc.Stop)
	app.AddAPI(APIVersion+"/metrics", oc.GetMetrics)
	app.AddAPI(APIVersion+"/info", oc.GetInfo)
	log.Println("HTTP API:", oc.viper.GetString("endpoints.http"))
	app.Run()
}
