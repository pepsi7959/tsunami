package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

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
