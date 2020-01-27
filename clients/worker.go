package main

import (
	"fmt"
	"time"

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
	conf   Conf
	jobs   *chan Job
	Done   *bool
	client *fasthttp.HostClient
	stat   Stat
}

func (w *Worker) url() string {
	if w.conf.url == "" {
		return fmt.Sprintf("http://%s:%s%s", w.conf.host, w.conf.port, w.conf.path)
	}
	return w.conf.url
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

	h.SetMethod(w.conf.method)
	for k, v := range w.conf.headers {
		h.Add(k, v)
	}

	req.SetRequestURI(w.url())
	req.SetBodyString(w.conf.body)

	resp := fasthttp.AcquireResponse()
	start := time.Now()
	err := w.client.Do(req, resp)
	if err != nil {
		w.UpdateErr()
		fmt.Println(err)
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
