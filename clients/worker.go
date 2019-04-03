package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type Stat struct {
	numReq int
	numRes int

	minResTime int64
	maxResTime int64
	avgResTime int64

	numErr int
}

type Worker struct {
	conf Conf
	jobs *chan Job
	Done chan bool

	stat Stat
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

func (w Worker) GetNumRes() int {
	return w.stat.numRes
}

// Average time in micro second
func (w Worker) GetAvgRes() int64 {
	return w.stat.avgResTime / 1000
}

func (w *Worker) UpdateStat(resTime int64) {
	if resTime > w.stat.minResTime {
		w.stat.minResTime = resTime
	}

	if resTime < w.stat.maxResTime {
		w.stat.maxResTime = resTime
	}
	w.stat.avgResTime = (w.stat.avgResTime + resTime) / 2
	w.stat.numRes += 1
}

func (w *Worker) Run() {
	fmt.Println("Run Worker...")
	for {
		select {
		case <-*w.jobs:
			w.do_job()
		case <-w.Done:
			fmt.Println("quit worker")
		}
	}
}

func (w *Worker) do_job() {
	//fmt.Printf("Do job: Calling %s\n", w.url())
	clnt := &http.Client{}
	start := time.Now()
	resp, err := clnt.Get(w.url())
	if err == nil {
		_, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			defer resp.Body.Close()
			//fmt.Printf("resposne body : %s\n", body)
			w.UpdateStat(time.Since(start).Nanoseconds())
		} else {
			w.UpdateErr()
			fmt.Printf("ioutil.ReadAll err: %s\n", err)
		}
	} else {
		w.UpdateErr()
		fmt.Printf("http.Client err: %s\n", err)
	}

}
