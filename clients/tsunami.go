package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/tsunami/clients/api"
	"github.com/valyala/fasthttp"
)

// Route URL
var APIV1 = "/api/v1"
var APIV1_ADMIN = "/api/v1/admin"

// Http configuration
var AllowOrigin string = "*"

// TSControl
// Used for storing tsunami service
type TSControl struct {
	services map[string]*Tsunami
}

// Tsunami
type Tsunami struct {
	conf Conf

	buf    bytes.Buffer
	logger *log.Logger

	// max queues to storing request
	max_queues int
	jobs       chan Job
	workers    []Worker

	done bool

	start time.Time
	end   time.Time

	// duration to run test in second
	duration int

	// refresh rate
	refresh int

	// Report
	enableReport bool
}

// Init
// initial parameters, logging and workers
func (ts *Tsunami) Init(max_queues int) {
	ts.max_queues = max_queues
	fmt.Println("initialize Tsunami")
	fmt.Println("initialize logger")
	ts.logger = log.New(&ts.buf, "Tsunami", log.Lshortfile)
	ts.jobs = make(chan Job, ts.max_queues)

	c := &fasthttp.HostClient{Addr: ts.conf.host, MaxConns: ts.conf.maxConns, ReadTimeout: time.Second * 30, WriteTimeout: time.Second * 30, Dial: func(addr string) (net.Conn, error) { return fasthttp.DialTimeout(addr, time.Second*60) }}
	for i := 0; i < ts.conf.concurrence; i++ {
		worker := Worker{Done: &ts.done, conf: ts.conf, client: c}
		ts.AddWorker(worker)
	}
}

// AddWorker
// create a new worker then storing to the Tsunami
func (ts *Tsunami) AddWorker(w Worker) {
	w.jobs = &ts.jobs
	ts.workers = append(ts.workers, w)
}

// Run
// Invoke all workers
func (ts *Tsunami) Run() {
	fmt.Println("Start worker")
	ts.start = time.Now()
	ts.done = false
	for i, _ := range ts.workers {
		go ts.workers[i].Run()
	}
}

// Stop
// Stop all workers
func (ts *Tsunami) Stop() {
	fmt.Println("Stoping worker")
	ts.done = true
	for i, _ := range ts.workers {
		fmt.Printf("Stoping worker[%d]\n", i)
	}
}

// Genload
// Assign jobs to workers
func (ts *Tsunami) GenLoad() {
	ts.done = false
	for ts.done != true {
		ts.jobs <- Job{}
	}
}

// Monitoring
// provide all metrics
func (ts *Tsunami) Monitoring(d time.Duration) {
	for ts.done != true {
		c := time.Tick(d)
		<-c

		var numRes int = 0
		var numErr int = 0
		var avg float64 = 0.0
		var max float64 = 0.0
		var min float64 = 9999.99
		var workers int = len(ts.workers)

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

// Quit
// Quit the process
func (ts *Tsunami) Quit(p string) error {
	fmt.Println("Quit!!!")
	os.Exit(0)
	return nil
}

// SetRefresh
// Rate to refresh metrics
func (ts *Tsunami) SetRefresh(p string) error {
	fmt.Println("Set Refresh rate: ", p)
	ts.Reload(map[string]string{"refresh": p})
	return nil
}

// SetEnableReport
// Enable or disable to dispaly metrics
func (ts *Tsunami) SetEnableReport(p string) error {
	fmt.Println("Set enableReport: ", p)
	ts.Reload(map[string]string{"enableReport": p})
	return nil
}

// ShowHelp
// Show all available commands
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

// AddNewWorker
// Add and start a new worker
func (ts *Tsunami) AddNewWorker(p string) error {
	w := Worker{Done: &ts.done, conf: ts.conf, jobs: &ts.jobs}
	ts.workers = append(ts.workers, w)
	go w.Run()
	return nil
}

// Reload
// Reload configuration
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
			fmt.Printf("Reload: unknow [%s][%s] \n")
		}
	}
}

// GetMetrics
// Get all metrics including workers, errors,
// avg(average), elaped time, requests, rps(request per second)
func (ts *Tsunami) GetMetrics(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	defer r.Body.Close()
	var avg float64
	var numRes, numErr int
	data := make(map[string]string)

	for _, w := range ts.workers {
		numRes += w.GetNumRes()
		numErr += w.GetNumErr()
		avg += w.GetAvgRes()
	}

	workers := len(ts.workers)
	avg = avg / float64(workers)

	data["workers_count"] = fmt.Sprintf("%d", workers)
	data["errors_count"] = fmt.Sprintf("%d", numErr)
	data["avg"] = fmt.Sprintf("%f", avg)
	data["elaped_time"] = fmt.Sprintf("%f", time.Since(ts.start).Seconds())
	data["requests_count"] = fmt.Sprintf("%f", float64(numRes))
	data["rps"] = fmt.Sprintf("%f", float64(numRes)/time.Since(ts.start).Seconds())

	WriteSuccess(&w, &data, nil)
	return
}

// Decoder

func (ctrl *TSControl) Decoder(w http.ResponseWriter, r *http.Request, v interface{}) {
	defer r.Body.Close()

	d := json.NewDecoder(r.Body)
	err := d.Decode(v)

	if err == io.EOF {
		//do nothing
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (ctrl *TSControl) CmdStart(w http.ResponseWriter, r *http.Request) {

	var req Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	//decode request
	ctrl.Decoder(w, r, &req)

	fmt.Println("Name: ", req.CmdConf.Name)
	fmt.Println("Url: ", req.CmdConf.Url)
	fmt.Println("Host: ", req.CmdConf.Host)
	fmt.Println("Concurrence: ", req.CmdConf.Concurrence)

	ts_conf := Conf{url: req.CmdConf.Url, host: req.CmdConf.Host, concurrence: req.CmdConf.Concurrence}
	if ctrl.services[req.CmdConf.Name] == nil {
		go StartApp(req.CmdConf.Name, ctrl, ts_conf)
	}
	data := make(map[string]string)
	data["url"] = "http://" + GetIP().String() + ":8091" + APIV1
	data["name"] = req.CmdConf.Name

	WriteSuccess(&w, &data, nil)
}

func (ctrl *TSControl) CmdStop(w http.ResponseWriter, r *http.Request) {

	var req Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	//decode request
	ctrl.Decoder(w, r, &req)

	fmt.Println("Cmd: ", req.Cmd)
	fmt.Println("Name: ", req.CmdConf.Name)

	t := ctrl.services[req.CmdConf.Name]
	if t != nil {
		t.Stop()
		delete(ctrl.services, req.CmdConf.Name)
	} else {
		WriteSuccess(&w, nil, &Error{Code: RESULT_NOT_FOUND, Message: "service not found"})
		return
	}
	WriteSuccess(&w, nil, nil)
}
func CmdRestart() {}

func (ts *Tsunami) SetConf(c Conf) {
	ts.conf = c
}

func StartApp(service string, ctrl *TSControl, conf Conf) {

	// Read user parameter
	// conf := ReadConf()

	// Initialize Tsunami
	app := Tsunami{done: false, conf: conf, duration: 3600, refresh: 2, enableReport: true}
	app.Init(100000)
	ctrl.services[service] = &app
	app.Run()

	shell := Shell{Done: &app.done, enableReport: &app.enableReport}
	shell.Init()
	shell.AddCmd("+", app.AddNewWorker)
	shell.AddCmd("help", app.ShowHelp)
	shell.AddCmd("q", app.Quit)
	shell.AddCmd("refresh", app.SetRefresh)
	shell.AddCmd("report", app.SetEnableReport)
	go shell.Run()

	go app.GenLoad()
	go app.GenLoad()

	go app.Monitoring(time.Duration(app.refresh) * time.Second)

	// Start Api Service
	api := &api.App{}
	api.Init("8091")
	api.AddApi(APIV1+"/metrics", app.GetMetrics)
	api.Run()

	c := time.Tick(time.Duration(app.duration) * time.Second)
	<-c

	app.Stop()

}

func main() {

	// TS control
	ctrl := TSControl{services: make(map[string]*Tsunami)}

	// Start deamon service
	api := &api.App{}
	api.Init("8090")
	api.AddApi(APIV1_ADMIN+"/start", ctrl.CmdStart)
	api.AddApi(APIV1_ADMIN+"/stop", ctrl.CmdStop)
	api.Run()

}
