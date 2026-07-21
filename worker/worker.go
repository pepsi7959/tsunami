package main

import (
	"fmt"
	"sync/atomic"
	"time"

	tshttp "github.com/tsunami/libs"
	"github.com/valyala/fasthttp"
)

type client interface {
	do() (code int, msTaken uint64, err error)
}

// Stat holds one worker's counters. Each field is written only by that worker's
// own request goroutine and read by the reporter goroutine, so all access is
// atomic (single writer, single reader) to stay data-race free.
type Stat struct {
	numReq int64 // total requests attempted (success + error)
	numErr int64 // failed attempts (transport error or non-2xx)

	// latency is tracked over SUCCESSFUL responses only, in nanoseconds
	numRes     int64 // count of successful responses (avg denominator)
	sumResTime int64 // sum of successful response times
	minResTime int64
	maxResTime int64
}

//Worker structure
type Worker struct {
	conf   tshttp.Conf
	jobs   *chan Job
	Done   *bool
	client *fasthttp.HostClient
	stat   Stat

	// compiled request templates (URL, body, header values); see tmpl.go
	urlTmpl     *Template
	bodyTmpl    *Template
	headerTmpls map[string]*Template

	// sample recorder, shared across a service's workers; nil unless verbose
	sample *sampleRec
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

//GetNumReq total requests attempted (success + error)
func (w *Worker) GetNumReq() int { return int(atomic.LoadInt64(&w.stat.numReq)) }

//GetNumErr number of failed requests
func (w *Worker) GetNumErr() int { return int(atomic.LoadInt64(&w.stat.numErr)) }

//GetNumRes number of successful responses (latency samples)
func (w *Worker) GetNumRes() int { return int(atomic.LoadInt64(&w.stat.numRes)) }

//GetSumNanos sum of successful response times, in nanoseconds
func (w *Worker) GetSumNanos() int64 { return atomic.LoadInt64(&w.stat.sumResTime) }

//GetMaxRes maximum successful response time, in milliseconds
func (w *Worker) GetMaxRes() float64 { return float64(atomic.LoadInt64(&w.stat.maxResTime)) / 1e6 }

//GetMinRes minimum successful response time, in milliseconds
func (w *Worker) GetMinRes() float64 { return float64(atomic.LoadInt64(&w.stat.minResTime)) / 1e6 }

//GetAvgRes mean successful response time, in milliseconds (true mean = sum/count)
func (w *Worker) GetAvgRes() float64 {
	n := atomic.LoadInt64(&w.stat.numRes)
	if n == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&w.stat.sumResTime)) / float64(n) / 1e6
}

// record folds one completed attempt into the stats. Latency (min/max/avg) is
// tracked over successful responses only, so a transport failure or timeout
// never pollutes the latency numbers; failures only bump numErr.
func (w *Worker) record(ok bool, resTime int64) {
	atomic.AddInt64(&w.stat.numReq, 1)
	if !ok {
		atomic.AddInt64(&w.stat.numErr, 1)
		return
	}
	atomic.AddInt64(&w.stat.numRes, 1)
	atomic.AddInt64(&w.stat.sumResTime, resTime)
	// single writer per worker: load/compare/store is safe, and atomics keep
	// the reporter goroutine's reads race-free.
	if mn := atomic.LoadInt64(&w.stat.minResTime); mn == 0 || resTime < mn {
		atomic.StoreInt64(&w.stat.minResTime, resTime)
	}
	if mx := atomic.LoadInt64(&w.stat.maxResTime); resTime > mx {
		atomic.StoreInt64(&w.stat.maxResTime, resTime)
	}
}

//Run invoke the worker
func (w *Worker) Run() {
	fmt.Println("Run Worker...")
	w.prepare()
	for *w.Done != true {
		select {
		case <-*w.jobs:
			w.do()
		}
	}
	fmt.Println("quit worker")
}

// prepare compiles the request templates once, before the request loop starts.
func (w *Worker) prepare() {
	w.urlTmpl = Compile(w.url())
	w.bodyTmpl = Compile(w.conf.Body)
	if len(w.conf.Headers) > 0 {
		w.headerTmpls = make(map[string]*Template, len(w.conf.Headers))
		for k, v := range w.conf.Headers {
			w.headerTmpls[k] = Compile(v)
		}
	}
}

func (w *Worker) do() {
	req := fasthttp.AcquireRequest()

	h := &req.Header

	h.SetMethod(w.conf.Method)
	for k, t := range w.headerTmpls {
		h.Add(k, t.Render())
	}

	req.SetRequestURI(w.urlTmpl.Render())
	req.SetBodyString(w.bodyTmpl.Render())

	resp := fasthttp.AcquireResponse()
	start := time.Now()
	err := w.client.Do(req, resp)
	ok := false
	if err != nil {
		fmt.Println("error client do: " + err.Error())
	} else if code := resp.StatusCode(); code < 200 || code >= 300 {
		fmt.Println("code: ", code, "Error: ", string(resp.Body()))
	} else {
		ok = true
	}

	elapsed := time.Since(start).Nanoseconds()
	if w.sample != nil {
		w.sample.capture(req, resp, float64(elapsed)/1e6, err)
	}
	w.record(ok, elapsed)

	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

}
