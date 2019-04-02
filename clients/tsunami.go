package main

import (
	"bytes"
	"fmt"
	"log"
	"time"
)

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
