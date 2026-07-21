package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	tsetcd "github.com/tsunami/etcd"
	tshttp "github.com/tsunami/libs"
	"github.com/valyala/fasthttp"
)

// APIVersion is prefix url of api version
var APIVersion = "/api/v1"

// APIAdmin is prefix url of admin api
const APIAdmin = "/api/v1/admin"

// AllowOrigin is a header to allow cross domain
var AllowOrigin = "*"

// version is set at build time via -ldflags "-X main.version=<tag>"
var version = "dev"

const (
	modeStandAlone = "standalone"
	modeCluster    = "cluster"
)

// TSControl holds the worker's running load services and config.
type TSControl struct {
	id       string
	name     string
	mode     string
	services map[string]*Tsunami
	viper    *viper.Viper
}

// Tsunami used to keep important infomation
type Tsunami struct {
	conf tshttp.Conf

	buf    bytes.Buffer
	logger *log.Logger

	// max queues to storing request
	maxQueues int
	jobs      chan Job
	workers   []Worker

	done bool

	start time.Time
	end   time.Time

	// duration to run test in second
	duration int

	// refresh rate
	refresh int

	// Report
	enableReport bool

	// api service
	apiServer *tshttp.App

	// shell service
	shell *Shell

	// verbose records the last request/response for this job (see sample.go)
	verbose bool
	sample  *sampleRec
}

// Init is used to initiaize parameters, logging and workers
func (ts *Tsunami) Init(maxQueues int) {
	ts.maxQueues = maxQueues
	fmt.Println("initialize Tsunami")
	fmt.Println("initialize logger")
	ts.logger = log.New(&ts.buf, "Tsunami", log.Lshortfile)
	ts.jobs = make(chan Job, ts.maxQueues)

	isTLS := false
	var host string

	if ts.conf.URL != "" {
		var u fasthttp.URI
		u.Parse(nil, []byte(ts.conf.URL))
		if string(u.Scheme()) == "https" {
			host = string(u.Host()) + ":443"
			isTLS = true
		} else {
			if ts.conf.Port != "" {
				host = string(u.Host()) + ":" + ts.conf.Port
			} else {
				host = string(u.Host()) + ":80"
			}
		}

	} else {
		if ts.conf.Protocol == "https" {
			isTLS = true
		}
		host = ts.conf.Host + ":" + ts.conf.Port
	}

	c := &fasthttp.HostClient{Addr: host,
		MaxConns:     ts.conf.MaxConns,
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
		IsTLS:        isTLS,
		Dial:         func(addr string) (net.Conn, error) { return fasthttp.DialTimeout(addr, time.Second*60) }}
	if ts.verbose {
		ts.sample = &sampleRec{}
	}
	for i := 0; i < ts.conf.Concurrence; i++ {
		worker := Worker{Done: &ts.done, conf: ts.conf, client: c, sample: ts.sample}
		ts.AddWorker(worker)
	}
}

// AddWorker used for creating a new worker then storing to the Tsunami
func (ts *Tsunami) AddWorker(w Worker) {
	w.jobs = &ts.jobs
	ts.workers = append(ts.workers, w)
}

// Run invoke all workers
func (ts *Tsunami) Run() {
	fmt.Println("Start worker")
	ts.start = time.Now()
	ts.done = false
	for i := range ts.workers {
		go ts.workers[i].Run()
	}
}

// Stop to stop all workers
func (ts *Tsunami) Stop() {
	fmt.Println("Stoping worker")
	ts.done = true
	for i := range ts.workers {
		fmt.Printf("Stoping worker[%d]\n", i)
	}
}

// GenLoad assigning the jobs to workers
func (ts *Tsunami) GenLoad() {
	ts.done = false
	for ts.done != true {
		ts.jobs <- Job{}
	}
}

// Monitoring provide all metrics
func (ts *Tsunami) Monitoring(d time.Duration) {
	for ts.done != true {
		c := time.Tick(d)
		<-c

		var numRes = 0
		var numErr = 0
		var avg float64
		var max float64
		var min float64
		var workers int

		avg = 0.0
		max = 0.0
		min = 9999.99

		workers = len(ts.workers)

		if ts.enableReport == true {
			for _, w := range ts.workers {
				numRes += w.GetNumRes()
				numErr += w.GetNumErr()
				avg += w.GetAvgRes()
				if w.GetMinRes() < min {
					min = w.GetMinRes()
				}

				if w.GetMaxRes() > max {
					max = w.GetMaxRes()
				}
			}
			avg = avg / float64(workers)
			fmt.Println("\033[H\033[2J")
			fmt.Println("----------------------------------------")
			fmt.Println("Service Name           :", ts.conf.Name)
			fmt.Println("Number of Worker       : ", len(ts.workers))
			fmt.Println("Number of Errors       : ", numErr)
			fmt.Println("Number of Requests     : ", numRes)
			fmt.Printf("Average Response(msec) : %.3f\n", avg)
			fmt.Printf("Max Response(msec)     : %.3f\n", max)
			fmt.Printf("Min Response(msec)     : %.3f\n", min)
			fmt.Printf("Elapped time(sec)      : %.0f\n", time.Since(ts.start).Seconds())
			fmt.Printf("Request Per Second     : %.2f\n", float64(numRes)/time.Since(ts.start).Seconds())
			fmt.Println("----------------------------------------")
		}
	}

}

// List of controlling command

// Quit quiting the process
func (ts *Tsunami) Quit(p string) error {
	fmt.Println("Quit!!!")
	os.Exit(0)
	return nil
}

// SetRefresh setting rate to refresh metrics
func (ts *Tsunami) SetRefresh(p string) error {
	fmt.Println("Set Refresh rate: ", p)
	ts.Reload(map[string]string{"refresh": p})
	return nil
}

// SetEnableReport enable or disable to dispaly metrics
func (ts *Tsunami) SetEnableReport(p string) error {
	fmt.Println("Set enableReport: ", p)
	ts.Reload(map[string]string{"enableReport": p})
	return nil
}

// ShowHelp show all available commands
func (ts *Tsunami) ShowHelp(p string) error {
	fmt.Println("\033[H\033[2J")
	fmt.Println("-------------------------------")
	fmt.Println("         Online Command        ")
	fmt.Println("-------------------------------")
	fmt.Println("q               stop and quit  ")
	fmt.Println("q!              force stop     ")
	fmt.Println("refresh <d>     used for refresh report in second")
	fmt.Println("report <true|false> used for enable or disable report")
	fmt.Println("+               Add concurrence")
	fmt.Printf("Enter command: ")
	return nil
}

// AddNewWorker add and start a new worker
func (ts *Tsunami) AddNewWorker(p string) error {
	w := Worker{Done: &ts.done, conf: ts.conf, jobs: &ts.jobs}
	ts.workers = append(ts.workers, w)
	go w.Run()
	return nil
}

// Reload reload the configuration
func (ts *Tsunami) Reload(conf map[string]string) {
	for k, v := range conf {
		switch k {
		case "refresh":
			ts.refresh, _ = strconv.Atoi(v)
		case "duration":
			ts.duration, _ = strconv.Atoi(v)
		case "enableReport":
			ts.enableReport, _ = strconv.ParseBool(v)
		default:
			fmt.Printf("Reload: unknow [%s][%s] \n", k, v)
		}
	}
}

// SetConf setting configuraiton for Tsunami
func (ts *Tsunami) SetConf(c tshttp.Conf) {
	ts.conf = c
}

//StartApp start Tsunami Application
func StartApp(service string, ctrl *TSControl, conf tshttp.Conf) {

	// Read user parameter
	// conf := ReadConf()

	// Initialize Tsunami
	app := Tsunami{done: false, conf: conf, duration: 3600, refresh: 2, enableReport: true}

	// max queues is 1000000
	app.Init(100000)
	ctrl.services[service] = &app
	app.Run()

	app.shell = &Shell{Done: &app.done, enableReport: &app.enableReport}
	app.shell.Init()
	app.shell.AddCmd("+", app.AddNewWorker)
	app.shell.AddCmd("help", app.ShowHelp)
	app.shell.AddCmd("q", app.Quit)
	app.shell.AddCmd("refresh", app.SetRefresh)
	app.shell.AddCmd("report", app.SetEnableReport)
	go app.shell.Run()

	go app.GenLoad()
	go app.GenLoad()

	go app.Monitoring(time.Duration(app.refresh) * time.Second)

	//Start Api Service
	app.apiServer = &tshttp.App{}
	app.apiServer.Init(":8091")
	app.apiServer.AddAPI(APIVersion+"/metrics", app.GetMetrics)
	app.apiServer.Run()

	c := time.Tick(time.Duration(app.duration) * time.Second)
	<-c

	app.Stop()

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

	viper.SetDefault("name", "worker-0")
	viper.SetDefault("id", "8cd50597-39df-473f-af2e-33e116f53105")
	viper.SetDefault("concurence", 10)
	viper.SetDefault("mode", "cluster")

	// Ocean bootstrap: the worker dials one of these ocean gRPC endpoints and
	// keeps an outbound stream. (etcd-based discovery is layered on top later.)
	viper.SetDefault("ocean.endpoints", []string{"127.0.0.1:8050"})

	// etcd endpoints for ocean discovery. Empty by default => use ocean.endpoints
	// directly (static bootstrap); set this to enable discovery/selection.
	viper.SetDefault("registry.endpoints", []string{})
	viper.SetDefault("registry.request_timeout", 2)
	viper.SetDefault("registry.dial_timeout", 2)

	// metrics report interval (seconds) up the attach stream
	viper.SetDefault("report.interval", 2)

	// shared token presented to the ocean on attach (empty = none)
	viper.SetDefault("auth.token", "")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %s ", err))
	}
	return viper

}

//New create TSControl
func New(viper *viper.Viper) *TSControl {
	log.Println("worker id: ", viper.GetString("id"))
	log.Println("worker name: ", viper.GetString("name"))
	return &TSControl{
		id:       viper.GetString("id"),
		name:     viper.GetString("name"),
		mode:     viper.GetString("mode"),
		services: make(map[string]*Tsunami),
		viper:    viper,
	}
}

func main() {

	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			fmt.Println("tsunami", version)
			return
		}
	}

	config := readConf()
	ctrl := New(config)

	interval := config.GetInt("report.interval")
	if interval <= 0 {
		interval = 2
	}

	client := &OceanClient{
		ctrl:     ctrl,
		workerID: ctrl.id,
		name:     ctrl.name,
		maxConc:  int32(config.GetInt("concurence")),
		report:   time.Duration(interval) * time.Second,
		token:    config.GetString("auth.token"),
	}

	// Resolver picks which ocean to attach to on each (re)connect.
	var resolve func() string
	if reg := config.GetStringSlice("registry.endpoints"); len(reg) > 0 {
		// discovery: choose the least-loaded ocean from etcd (by free conn slots)
		etcdCli, err := tsetcd.NewClient(
			reg,
			time.Second*time.Duration(config.GetInt("registry.dial_timeout")),
			time.Second*time.Duration(config.GetInt("registry.request_timeout")),
			10,
		)
		if err != nil {
			log.Fatalf("discovery: connect etcd failed: %v", err)
		}
		log.Printf("worker: discovering oceans via etcd %v", reg)
		resolve = func() string { return selectOcean(etcdCli) }
	} else {
		// static bootstrap: first configured ocean endpoint
		endpoint := "127.0.0.1:8050"
		if oceans := config.GetStringSlice("ocean.endpoints"); len(oceans) > 0 {
			endpoint = oceans[0]
		}
		log.Printf("worker: attaching to ocean %s", endpoint)
		resolve = func() string { return endpoint }
	}

	client.Run(resolve) // blocks: resolve + attach + reconnect loop
}
