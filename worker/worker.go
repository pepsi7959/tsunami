package main

import (
	"fmt"
	"time"

	tshttp "github.com/tsunami/libs"
	"github.com/valyala/fasthttp"
)

type client interface {
	do() (code int, msTaken uint64, err error)
}

// Stat statistic structure
type Stat struct {
	numReq int
	numRes int

	minResTime int64
	maxResTime int64
	avgResTime float64

	numErr int
}

//Worker structure
type Worker struct {
	conf   tshttp.Conf
	jobs   *chan Job
	Done   *bool
	client *fasthttp.HostClient
	stat   Stat
}

func (w *Worker) url() string {

	protocol := "http"

	if w.conf.Protocol == "http" || w.conf.Protocol == "https" {
		protocol = w.conf.Protocol
	}
	if w.conf.URL == "" {
		url := fmt.Sprintf("%s://%s:%s%s", protocol, w.conf.Host, w.conf.Port, w.conf.Path)
		return url
	}
	return w.conf.URL
}

// UpdateErr update the error value
func (w *Worker) UpdateErr() {
	w.stat.numErr++
}

//GetNumErr get number of errors
func (w Worker) GetNumErr() int {
	return w.stat.numErr
}

//GetNumRes get number of responses
func (w Worker) GetNumRes() int {
	return w.stat.numRes
}

//GetMaxRes get maximum time in micro second
func (w Worker) GetMaxRes() float64 {
	return float64(w.stat.maxResTime) / 1000000.00
}

//GetMinRes get mininum time in micro second
func (w Worker) GetMinRes() float64 {
	return float64(w.stat.minResTime) / 1000000.00
}

//GetAvgRes get average time in micro second
func (w Worker) GetAvgRes() float64 {
	return w.stat.avgResTime / 1000000
}

//UpdateStat update statistic
func (w *Worker) UpdateStat(resTime int64) {
	if resTime < w.stat.minResTime || w.stat.minResTime == 0 {
		w.stat.minResTime = resTime
	}

	if resTime > w.stat.maxResTime {
		w.stat.maxResTime = resTime
	}

	if w.stat.avgResTime == 0 {
		w.stat.avgResTime = float64(resTime)
	} else {
		w.stat.avgResTime = (w.stat.avgResTime + float64(resTime)) / 2
	}

	w.stat.numRes++
}

//Run invoke the worker
func (w *Worker) Run() {
	fmt.Println("Run Worker...")
	for *w.Done != true {
		select {
		case <-*w.jobs:
			w.do()
		}
	}
	fmt.Println("quit worker")
}

func (w *Worker) do() {
	req := fasthttp.AcquireRequest()

	h := &req.Header

	h.SetMethod(w.conf.Method)
	for k, v := range w.conf.Headers {
		h.Add(k, v)
	}

	req.SetRequestURI(w.url())
	req.SetBodyString(w.conf.Body)

	resp := fasthttp.AcquireResponse()
	start := time.Now()
	err := w.client.Do(req, resp)
	if err != nil {
		w.UpdateErr()
		fmt.Println("error client do: " + err.Error())
	} else {
		code := resp.StatusCode()
		if code != 200 {
			w.UpdateErr()
			fmt.Println("code: ", code, "Error: ", string(resp.Body()))
		}
	}

	w.UpdateStat(time.Since(start).Nanoseconds())

	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

}
