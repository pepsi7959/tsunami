package main

import (
	"fmt"
	"log"
	"time"

	"github.com/valyala/fasthttp"
)

type client interface {
	do() (code int, msTaken uint64, err error)
}

type Stat struct {
	numReq int
	numRes int

	minResTime int64
	maxResTime int64
	avgResTime float64

	numErr int
}

type Worker struct {
	conf   Conf
	jobs   *chan Job
	Done   chan bool
	client *fasthttp.HostClient
	stat   Stat
}

func (w *Worker) url() string {
	if w.url == nil {
		return fmt.Sprintf("http://%s:%s%s", w.conf.host, w.conf.port, w.conf.path)
	} else {
		return w.conf.url
	}
}

func (w *Worker) UpdateErr() {
	w.stat.numErr += 1
}

func (w Worker) GetNumErr() int {
	return w.stat.numErr
}

func (w Worker) GetNumRes() int {
	return w.stat.numRes
}

// Average time in micro second
func (w Worker) GetAvgRes() float64 {
	return w.stat.avgResTime / 1000000
}

func (w *Worker) UpdateStat(resTime int64) {
	if resTime > w.stat.minResTime {
		w.stat.minResTime = resTime
	}

	if resTime < w.stat.maxResTime {
		w.stat.maxResTime = resTime
	}
	w.stat.avgResTime = (w.stat.avgResTime + float64(resTime)) / 2
	w.stat.numRes += 1
}

func (w *Worker) Run() {
	fmt.Println("Run Worker...")
	for {
		//select {
		//case <-*w.jobs:
		w.do()
		//case <-w.Done:
		//		fmt.Println("quit worker")
		//	}
	}
}

func (w *Worker) do() {
	//fmt.Printf("Do job: Calling %s\n", w.url())
	//tr := &http.Transport{}
	//defer tr.CloseIdleConnections()
	//clnt := &http.Client{Transport: tr}
	//clnt := &http.Client{Transport: tr}
	start := time.Now()
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(w.url())
	resp := fasthttp.AcquireResponse()
	err := w.client.Do(req, resp)
	if err != nil {
		w.UpdateErr()
		log.Fatal(err)
	} /*else {
		code = resp.StatusCode()
	}*/

	w.UpdateStat(time.Since(start).Nanoseconds())

	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

}
