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

	done bool

	start time.Time
	end   time.Time
}

func (ts *Tsunami) Init(max_queues int) {
	ts.max_queues = max_queues
	fmt.Println("initialize Tsunami")
	fmt.Println("initialize logger")
	ts.logger = log.New(&ts.buf, "Tsunami", log.Lshortfile)
	ts.jobs = make(chan Job, ts.max_queues)
}

func (ts *Tsunami) AddWorker(w Worker) {
	w.jobs = &ts.jobs
	ts.workers = append(ts.workers, w)
}

func (ts *Tsunami) Run() {
	fmt.Println("Start worker")
	ts.start = time.Now()
	for i, _ := range ts.workers {
		go ts.workers[i].Run()
	}
}

func (ts *Tsunami) Stop() {
	fmt.Println("Stoping worker")
	for i, w := range ts.workers {
		fmt.Println("Stoping worker[%d]\n", i)
		w.Done <- true
	}
	ts.done = true
}

func (ts *Tsunami) GenLoad() {
	ts.done = false
	for ts.done != true {
		ts.jobs <- Job{}
	}
}

func (ts *Tsunami) Monitoring(d time.Duration) {
	for ts.done != true {
		c := time.Tick(d)
		<-c

		var numRes int = 0
		var numErr int = 0
		for i, w := range ts.workers {
			fmt.Println("update worker id: ", i)
			fmt.Println("number Of Request: ", w.GetNumRes())
			fmt.Println("Average Response Of Request: ", w.GetAvgRes())
			numRes += w.GetNumRes()
			numErr += w.GetNumErr()
		}

		fmt.Println("----------------------------------------")
		fmt.Println("Number of Error   : ", numErr)
		fmt.Println("Number of Requests: ", numRes)
		fmt.Println("Elaped time       : ", time.Since(ts.start))
		fmt.Println("Request Per Second: ", float64(numRes)/time.Since(ts.start).Seconds())
		fmt.Println("----------------------------------------")

	}

}

func main() {

	// Read user parameter
	conf := ReadConf()

	// Initialize Tsunami
	app := Tsunami{conf: conf}
	app.Init(100)

	for i := 0; i < conf.concurrence; i++ {
		worker := Worker{conf: conf}
		app.AddWorker(worker)
	}
	app.Run()

	go app.GenLoad()

	go app.Monitoring(2 * time.Second)

	c := time.Tick(60 * time.Second)
	<-c

	app.Stop()
}
