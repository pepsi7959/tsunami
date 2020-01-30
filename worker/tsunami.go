package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	tshttp "github.com/tsunami/libs"
	"github.com/valyala/fasthttp"
)

// APIVersion is prefix url of api version
var APIVersion = "/api/v1"

// APIAdmin is prefix url of admin api
const APIAdmin = "/api/v1/admin"

// AllowOrigin is a header to allow cross domain
var AllowOrigin = "*"

// TSControl used for storing tsunami service
type TSControl struct {
	services map[string]*Tsunami
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
}

// Init is used to initiaize parameters, logging and workers
func (ts *Tsunami) Init(maxQueues int) {
	ts.maxQueues = maxQueues
	fmt.Println("initialize Tsunami")
	fmt.Println("initialize logger")
	ts.logger = log.New(&ts.buf, "Tsunami", log.Lshortfile)
	ts.jobs = make(chan Job, ts.maxQueues)

	c := &fasthttp.HostClient{Addr: ts.conf.Host,
		MaxConns:     ts.conf.MaxConns,
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
		Dial:         func(addr string) (net.Conn, error) { return fasthttp.DialTimeout(addr, time.Second*60) }}
	for i := 0; i < ts.conf.Concurrence; i++ {
		worker := Worker{Done: &ts.done, conf: ts.conf, client: c}
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

// GetMetrics getting all metrics including workers, errors,
// avg(average), elaped time, requests, rps(request per second)
func (ts *Tsunami) GetMetrics(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	defer r.Body.Close()
	var avg, min, max float64
	var numRes, numErr int
	data := make(map[string]string)

	for _, w := range ts.workers {
		numRes += w.GetNumRes()
		numErr += w.GetNumErr()
		avg += w.GetAvgRes()

		if w.GetMaxRes() > max {
			max = w.GetMaxRes()
		}

		if min == 0.0 || min > w.GetMinRes() {
			min = w.GetMinRes()
		}
	}

	workers := len(ts.workers)
	avg = avg / float64(workers)
	data["name"] = ts.conf.Name
	data["workers_count"] = fmt.Sprintf("%d", workers)
	data["errors_count"] = fmt.Sprintf("%d", numErr)
	data["avg"] = fmt.Sprintf("%f", avg)
	data["min"] = fmt.Sprintf("%f", min)
	data["max"] = fmt.Sprintf("%f", max)
	data["elaped_time"] = fmt.Sprintf("%f", time.Since(ts.start).Seconds())
	data["requests_count"] = fmt.Sprintf("%f", float64(numRes))
	data["rps"] = fmt.Sprintf("%f", float64(numRes)/time.Since(ts.start).Seconds())

	tshttp.WriteSuccess(&w, &data, nil)
	return
}

// Decoder decode request to json structure
func (ctrl *TSControl) Decoder(w http.ResponseWriter, r *http.Request, v interface{}) error {
	defer r.Body.Close()

	d := json.NewDecoder(r.Body)
	err := d.Decode(v)

	if err == io.EOF {
		//do nothing
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return errors.New("JSON decode error: " + err.Error())
	}
	return nil
}

//CmdStart command to start worker
func (ctrl *TSControl) CmdStart(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	err := ctrl.Decoder(w, r, &req)

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("cmd: ", req.Cmd)
	fmt.Println("Name: ", req.CmdConf.Name)
	fmt.Println("Url: ", req.CmdConf.URL)
	fmt.Println("Host: ", req.CmdConf.Host)
	fmt.Println("Concurrence: ", req.CmdConf.Concurrence)
	fmt.Println("Method: ", req.CmdConf.Method)
	fmt.Println("Headers ", req.CmdConf.Headers)
	fmt.Println("Body: ", req.CmdConf.Body)

	if req.CmdConf.Method == "" {
		req.CmdConf.Method = "GET"
	}

	tsConf := tshttp.Conf{
		Name:        req.CmdConf.Name,
		URL:         req.CmdConf.URL,
		Host:        req.CmdConf.Host,
		Method:      req.CmdConf.Method,
		Headers:     req.CmdConf.Headers,
		Body:        req.CmdConf.Body,
		Concurrence: req.CmdConf.Concurrence,
	}

	if ctrl.services[req.CmdConf.Name] == nil {
		go StartApp(req.CmdConf.Name, ctrl, tsConf)
	}

	data := make(map[string]string)
	data["url"] = "http://" + GetIP().String() + ":8091" + APIVersion
	data["name"] = req.CmdConf.Name

	tshttp.WriteSuccess(&w, &data, nil)
}

// CmdStop command to stop running workers
func (ctrl *TSControl) CmdStop(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	//decode request
	ctrl.Decoder(w, r, &req)

	fmt.Println("Cmd: ", req.Cmd)
	fmt.Println("Name: ", req.CmdConf.Name)

	t := ctrl.services[req.CmdConf.Name]
	if t != nil {
		t.Stop()
		t.apiServer.Stop()
		delete(ctrl.services, req.CmdConf.Name)
	} else {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultNotFound, Message: "service not found"})
		return
	}
	tshttp.WriteSuccess(&w, nil, nil)
}

//CmdMetrics get all information of worker
func (ctrl *TSControl) CmdMetrics(w http.ResponseWriter, r *http.Request) {
	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	//decode request
	ctrl.Decoder(w, r, &req)

	fmt.Println("Cmd: ", req.Cmd)
	fmt.Println("Name: ", req.CmdConf.Name)

	if req.Cmd != "metrics" {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultInvalidCMD, Message: "invalid command"})
		return
	}

	app := ctrl.services[req.CmdConf.Name]

	if app == nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultNotFound, Message: "service not found"})
		return
	}
	app.GetMetrics(w, r)
}

// CmdRestart stop and start workers
func CmdRestart() {}

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
	app.apiServer = &tshttp.App{}
	app.apiServer.Init("8091")
	app.apiServer.AddAPI(APIVersion+"/metrics", app.GetMetrics)
	app.apiServer.Run()

	c := time.Tick(time.Duration(app.duration) * time.Second)
	<-c

	app.Stop()

}

func main() {

	ctrl := TSControl{services: make(map[string]*Tsunami)}

	// Start deamon service
	api := &tshttp.App{}
	api.Init("8090")
	api.AddAPI(APIAdmin+"/start", ctrl.CmdStart)
	api.AddAPI(APIAdmin+"/stop", ctrl.CmdStop)
	api.AddAPI(APIAdmin+"/metrics", ctrl.CmdMetrics)
	api.Run()

}
