package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

/* Configuration */
type Conf struct {
	url  string
	host string "localhost"
	port string "80"
	path string "/"
}

type Worker struct {
	conf Conf
	jobs *chan Job
	Done chan bool
}

func (w Worker) url() string {
	if w.url == nil {
		return fmt.Sprintf("http://%s:%s%s", w.conf.host, w.conf.port, w.conf.path)
	} else {
		return w.conf.url
	}
}

func (w Worker) Run() {
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

func (w Worker) do_job() {
	fmt.Printf("Do job: Calling %s\n", w.url())
	clnt := &http.Client{}
	resp, err := clnt.Get(w.url())
	if err == nil {
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			defer resp.Body.Close()
			fmt.Printf("resposne body : %s\n", body)
		} else {
			fmt.Printf("ioutil.ReadAll err: %s\n", err)
		}
	} else {
		fmt.Printf("http.Client err: %s\n", err)
	}

}

type Job struct{}

type Tsunami struct {
	conf Conf

	buf    bytes.Buffer
	logger *log.Logger

	max_queues int
	jobs       chan Job
	workers    []Worker
}

func (ts *Tsunami) Init(max_queues int) {
	ts.max_queues = max_queues
	fmt.Println("initialize Tsunami")
	fmt.Println("initialize logger")
	ts.logger = log.New(&ts.buf, "Tsunami", log.Lshortfile)
	ts.jobs = make(chan Job, ts.max_queues)
}

func (ts *Tsunami) Add_worker(w Worker) {
	w.jobs = &ts.jobs
	ts.workers = append(ts.workers, w)
}

func (ts *Tsunami) Run() {
	fmt.Println("Start worker")
	for _, w := range ts.workers {
		go w.Run()
	}
}

func (ts *Tsunami) Stop() {
	fmt.Println("Stoping worker")
	for i, w := range ts.workers {
		fmt.Println("Stoping worker[%d]\n", i)
		w.Done <- true
	}
}

func main() {
	app := Tsunami{}
	app.Init(100)
	worker := Worker{conf: Conf{url: "http://164.115.28.52/test.php", host: "localhost", port: "80"}}
	//worker := Worker{conf: Conf{url: "http://localhost:8080/solarlaa-admin/api/v1/real-mon/m/users/123/devices/123", host: "localhost", port: "80"}}
	app.Add_worker(worker)

	app.Run()

	go func() {
		for {
			fmt.Println("gen work")
			app.jobs <- Job{}
		}
	}()

	c := time.Tick(30 * time.Second)
	<-c
}
