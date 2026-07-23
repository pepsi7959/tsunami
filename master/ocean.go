package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	tsetcd "github.com/tsunami/etcd"
	tshttp "github.com/tsunami/libs"
	tsauth "github.com/tsunami/master/auth"
	tsgrpc "github.com/tsunami/proto"
	clientv3 "go.etcd.io/etcd/client/v3"
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
	samples map[string]*tsgrpc.Sample         // latest per-job request/response (verbose)
}

// job is a running load test owned by this ocean.
type job struct {
	name        string
	conf        tshttp.Conf    // effective conf sent to workers (env {{vars}} substituted)
	rawConf     tshttp.Conf    // original conf with {{tokens}} intact, for re-applying an env on resume
	assignments map[string]int // workerID -> concurrency
	paused      bool           // traffic halted but capacity still reserved
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

	// user login/logout for the HTTP API (SQLite-backed sessions)
	auth *tsauth.Auth

	// discovery (optional): register this ocean in etcd so workers can find it
	discovery     bool
	advertiseGRPC string // address workers should dial (published to etcd)
	httpAddr      string
	etcd          *tsetcd.Client
	lease         clientv3.LeaseID
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

	// discovery (optional): if registry.endpoints is set, register in etcd so
	// workers discover + select oceans. advertise.grpc is the address workers
	// dial (must be reachable by them); empty => auto from this host's IP.
	v.SetDefault("registry.endpoints", []string{})
	v.SetDefault("registry.request_timeout", 2)
	v.SetDefault("registry.dial_timeout", 2)
	v.SetDefault("advertise.grpc", "")
	v.SetDefault("lease.ttl", 10)

	// shared token required from workers on the attach stream (empty = open)
	v.SetDefault("auth.token", "")

	// user login/logout for the HTTP API (separate from the worker attach token above)
	v.SetDefault("auth.db_path", "/var/lib/tsunami/auth.db")
	v.SetDefault("auth.session_ttl", 3600) // seconds; hard session lifetime
	v.SetDefault("auth.cookie_secure", false)
	v.SetDefault("auth.bootstrap_admin.username", "admin")
	v.SetDefault("auth.bootstrap_admin.password", "admin")
	// allow the admin password (and user) via env so prod doesn't write it to config
	_ = v.BindEnv("auth.bootstrap_admin.username", "ADMIN_USER")
	_ = v.BindEnv("auth.bootstrap_admin.password", "ADMIN_PASS")

	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
	return v
}

//New create Ocean
func New(conf *viper.Viper) *Ocean {
	oc := &Ocean{
		viper:          conf,
		id:             conf.GetString("id"),
		name:           conf.GetString("name"),
		maxConnections: conf.GetInt("max_connections"),
		httpAddr:       conf.GetString("endpoints.http"),
		workers:        make(map[string]*attachedWorker),
		jobs:           make(map[string]*job),
		discovery:      len(conf.GetStringSlice("registry.endpoints")) > 0,
	}

	// advertise address workers dial: explicit config, else this host's IP + the
	// port of endpoints.grpc (0.0.0.0:8050 is not dialable by remote workers).
	adv := conf.GetString("advertise.grpc")
	if adv == "" {
		port := "8050"
		if _, p, err := net.SplitHostPort(conf.GetString("endpoints.grpc")); err == nil {
			port = p
		}
		ip := "127.0.0.1"
		if p := tshttp.GetIP(); p != nil {
			ip = p.String()
		}
		adv = ip + ":" + port
	}
	oc.advertiseGRPC = adv

	// user auth: open the SQLite store, register providers, seed the admin
	store, err := tsauth.Open(conf.GetString("auth.db_path"))
	if err != nil {
		log.Fatalf("auth: open db %s: %v", conf.GetString("auth.db_path"), err)
	}
	ttl := time.Duration(conf.GetInt("auth.session_ttl")) * time.Second
	oc.auth = tsauth.New(store, ttl, conf.GetBool("auth.cookie_secure"))
	if err := oc.auth.SeedAdmin(
		conf.GetString("auth.bootstrap_admin.username"),
		conf.GetString("auth.bootstrap_admin.password"),
	); err != nil {
		log.Fatalf("auth: seed admin: %v", err)
	}

	return oc
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

	// optional: register in etcd for worker discovery + selection
	if oc.discovery {
		if err := oc.initDiscovery(); err != nil {
			log.Fatalf("discovery: %v", err)
		}
		log.Println("discovery: registered in etcd, advertising", oc.advertiseGRPC)
	}

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
	// auth (login is public; everything else is gated by the middleware below)
	app.AddAPI(APIVersion+"/login", oc.Login)
	app.AddAPI(APIVersion+"/logout", oc.Logout)
	app.AddAPI(APIVersion+"/session", oc.Session)
	app.AddAPI(APIVersion+"/start", oc.Start)
	app.AddAPI(APIVersion+"/stop", oc.Stop)
	app.AddAPI(APIVersion+"/pause", oc.Pause)
	app.AddAPI(APIVersion+"/resume", oc.Resume)
	app.AddAPI(APIVersion+"/metrics", oc.GetMetrics)
	app.AddAPI(APIVersion+"/info", oc.GetInfo)
	app.AddAPI(APIVersion+"/sample", oc.GetSample)
	// saved tests (favorites)
	app.AddAPI(APIVersion+"/bookmarks", oc.GetBookmarks)
	app.AddAPI(APIVersion+"/bookmark", oc.SaveBookmark)
	app.AddAPI(APIVersion+"/unbookmark", oc.DeleteBookmark)
	app.AddAPI(APIVersion+"/envs", oc.GetEnvs)
	app.AddAPI(APIVersion+"/env", oc.SaveEnv)
	app.AddAPI(APIVersion+"/env-delete", oc.DeleteEnv)
	app.AddAPI(APIVersion+"/env-active", oc.SetActiveEnv)
	app.Use(oc.auth.Middleware) // gate all routes except /login (+ CORS/preflight)
	log.Println("HTTP API:", oc.viper.GetString("endpoints.http"))
	app.Run()
}
