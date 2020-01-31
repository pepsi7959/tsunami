package main

import (
	"fmt"
	"net/http"

	tshttp "github.com/tsunami/libs"
)

func (oc *Ocean) JobMatching(job *Job) bool {

	pendingCon := job.pendingConcurrence

	for name, worker := range oc.workers {
		if worker.remainingQouta > 0 {
			if worker.remainingQouta > pendingCon {
				//Asigns the job to this worker
				workers := oc.jobToWorkers[name]
				workers = append(workers, worker)
				worker.remainingQouta = worker.remainingQouta - pendingCon
			}
		}

		if pendingCon <= 0 {
			return true
		}
	}

	return false
}

// Start begin calling worker to generate load
func (oc *Ocean) Start(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request
	err := tshttp.Decoder(w, r, req)

	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	tshttp.WriteSuccess(&w, nil, nil)
}

// Stop stop generating load to target
func (oc *Ocean) Stop(w http.ResponseWriter, r *http.Request) {
	tshttp.WriteSuccess(&w, nil, nil)
}

// GetMetrics get all worker information
func (oc *Ocean) GetMetrics(w http.ResponseWriter, r *http.Request) {
	tshttp.WriteSuccess(&w, nil, nil)
}
