package main

import (
	"fmt"
	"net/http"
	"time"

	tshttp "github.com/tsunami/libs"
)

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

//CmdStart command to start worker
func (ctrl *TSControl) CmdStart(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	//decode request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultBadRequest, Message: err.Error()})
		return
	}

	fmt.Println("cmd: ", req.Cmd)
	fmt.Println("Name: ", req.Conf.Name)
	fmt.Println("Url: ", req.Conf.URL)
	fmt.Println("Protocol: ", req.Conf.Protocol)
	fmt.Println("Host: ", req.Conf.Host)
	fmt.Println("Post: ", req.Conf.Port)
	fmt.Println("Path: ", req.Conf.Path)
	fmt.Println("Concurrence: ", req.Conf.Concurrence)
	fmt.Println("Method: ", req.Conf.Method)
	fmt.Println("Headers ", req.Conf.Headers)
	fmt.Println("Body: ", req.Conf.Body)

	if req.Conf.Method == "" {
		req.Conf.Method = "GET"
	}

	tsConf := tshttp.Conf{
		Name:        req.Conf.Name,
		URL:         req.Conf.URL,
		Protocol:    req.Conf.Protocol,
		Host:        req.Conf.Host,
		Port:        req.Conf.Port,
		Path:        req.Conf.Path,
		Method:      req.Conf.Method,
		Headers:     req.Conf.Headers,
		Body:        req.Conf.Body,
		Concurrence: req.Conf.Concurrence,
	}

	if ctrl.services[req.Conf.Name] == nil {
		go StartApp(req.Conf.Name, ctrl, tsConf)
	}

	data := make(map[string]string)
	data["url"] = "http://" + tshttp.GetIP().String() + ":8091" + APIVersion
	data["name"] = req.Conf.Name

	tshttp.WriteSuccess(&w, &data, nil)
}

// CmdStop command to stop running workers
func (ctrl *TSControl) CmdStop(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	//decode request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultBadRequest, Message: err.Error()})
		return
	}

	fmt.Println("Cmd: ", req.Cmd)
	fmt.Println("Name: ", req.Conf.Name)

	t := ctrl.services[req.Conf.Name]
	if t != nil {
		t.shell.Stop()
		t.Stop()
		t.apiServer.Stop()
		delete(ctrl.services, req.Conf.Name)
		time.Sleep(time.Second * 1)
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
	if err := tshttp.Decoder(w, r, &req); err != nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultBadRequest, Message: err.Error()})
		return
	}

	fmt.Println("Cmd: ", req.Cmd)
	fmt.Println("Name: ", req.Conf.Name)

	if req.Cmd != "metrics" {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultInvalidCMD, Message: "invalid command"})
		return
	}

	app := ctrl.services[req.Conf.Name]

	if app == nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: tshttp.ResultNotFound, Message: "service not found"})
		return
	}
	app.GetMetrics(w, r)
}

// CmdRestart stop and start workers
func CmdRestart() {}
