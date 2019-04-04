package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
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

	// duration to run test in second
	duration int

	// refresh rate
	refresh int

	// Report
	enableReport bool
}

func (ts *Tsunami) Init(max_queues int) {
	ts.max_queues = max_queues
	fmt.Println("initialize Tsunami")
	fmt.Println("initialize logger")
	ts.logger = log.New(&ts.buf, "Tsunami", log.Lshortfile)
	ts.jobs = make(chan Job, ts.max_queues)

	for i := 0; i < ts.conf.concurrence; i++ {
		worker := Worker{conf: ts.conf}
		ts.AddWorker(worker)
	}
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
		fmt.Printf("Stoping worker[%d]\n", i)
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

		if ts.enableReport == true {
			for i, w := range ts.workers {
				fmt.Println("update worker id: ", i)
				fmt.Println("number Of Request: ", w.GetNumRes())
				fmt.Println("Average Response Of Request: ", w.GetAvgRes())
				numRes += w.GetNumRes()
				numErr += w.GetNumErr()
			}
			fmt.Println("\033[H\033[2J")
			fmt.Println("----------------------------------------")
			fmt.Println("Number of Worker  : ", len(ts.workers))
			fmt.Println("Number of Errors  : ", numErr)
			fmt.Println("Number of Requests: ", numRes)
			fmt.Println("Elapped time      : ", time.Since(ts.start))
			fmt.Println("Request Per Second: ", float64(numRes)/time.Since(ts.start).Seconds())
			fmt.Println("----------------------------------------")
		}
	}

}

func Help(p string) error {
	fmt.Println("Test")
	return nil
}

func (ts *Tsunami) Quit(p string) error {
	fmt.Println("Quit!!!")
	os.Exit(0)
	return nil
}

func (ts *Tsunami) SetRefresh(p string) error {
	fmt.Println("Set Refresh rate: ", p)
	ts.Reload(map[string]string{"refresh": p})
	return nil
}

func (ts *Tsunami) SetEnableReport(p string) error {
	fmt.Println("Set enableReport: ", p)
	ts.Reload(map[string]string{"enableReport": p})
	return nil
}

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

func (ts *Tsunami) AddNewWorker(p string) error {
	w := Worker{conf: ts.conf, jobs: &ts.jobs}
	ts.workers = append(ts.workers, w)
	go w.Run()
	return nil
}

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
			fmt.Printf("Reload: unknow [%s][%s] \n")
		}
	}
}

func main() {

	// Read user parameter
	conf := ReadConf()

	// Initialize Tsunami
	app := Tsunami{conf: conf, duration: 3600, refresh: 2, enableReport: true}
	app.Init(100)

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

	go app.Monitoring(time.Duration(app.refresh) * time.Second)

	c := time.Tick(time.Duration(app.duration) * time.Second)
	<-c

	app.Stop()
}
