package main

import (
	"errors"
	"fmt"
	"net/http"

	tsgrpc "github.com/tsunami/proto"

	tshttp "github.com/tsunami/libs"
)

func getJob(req *tshttp.Request) Job {

	conf := tshttp.Conf{
		Name:        req.Conf.Name,
		URL:         req.Conf.URL,
		Host:        req.Conf.Host,
		Method:      req.Conf.Method,
		Headers:     req.Conf.Headers,
		Body:        req.Conf.Body,
		Concurrence: req.Conf.Concurrence,
	}
	return Job{conf: conf, pendingConcurrence: req.Conf.Concurrence}
}

// JobMatching delegate a job to workers
//
// Algorithm:
// First, assigns all concurrences to a worker having available concurrences and more than
// the concurrences of a job
//
// Second, assigns the remainnging to each workers
func (oc *Ocean) JobMatching(job *Job) error {

	pc := job.pendingConcurrence
	req := tsgrpc.Request{
		Command: tsgrpc.Request_START,
		Params: &tsgrpc.Request_Params{
			Name:            job.conf.Name,
			Url:             job.conf.URL,
			Method:          tsgrpc.Request_GET,
			Host:            job.conf.Host,
			MaxConcurrences: int32(job.conf.MaxConns),
			Body:            job.conf.Body,
		},
	}
	for name, worker := range oc.workers {
		if worker.remainingQouta >= pc {

			//send command to a worker
			req.Params.MaxConcurrences = int32(pc)
			_, err := worker.gRPCClient.Start(&req)

			if err != nil {
				return err
			}

			//Assigns the job to this worker
			oc.jobToWorkers[job.conf.Name] = append(oc.jobToWorkers[job.conf.Name], &WorkerInfo{concurrence: pc, worker: worker})

			fmt.Printf("Assigns all %v concurrences to worker %v\n", pc, name)
			worker.remainingQouta = worker.remainingQouta - pc
			pc = 0
			fmt.Printf("Left queues: %v\n", pc)
		}

		if pc <= 0 {
			return nil
		}
	}

	for name, worker := range oc.workers {
		if worker.remainingQouta > 0 {
			if worker.remainingQouta >= pc {

				//send command to a worker
				req.Params.MaxConcurrences = int32(pc)
				_, err := worker.gRPCClient.Start(&req)

				if err != nil {
					return err
				}

				//Assigns the job to this worker
				workers := oc.jobToWorkers[job.conf.Name]
				workers = append(workers, &WorkerInfo{concurrence: pc, worker: worker})
				oc.jobToWorkers[job.conf.Name] = workers

				fmt.Printf("Assigns %v concurrences to worker %v\n", pc, name)
				worker.remainingQouta = worker.remainingQouta - pc
				pc = 0
				fmt.Printf("Left queues: %v\n", pc)

			} else {

				//send command to a worker
				req.Params.MaxConcurrences = int32(worker.remainingQouta)
				_, err := worker.gRPCClient.Start(&req)

				if err != nil {
					return err
				}

				workers := oc.jobToWorkers[job.conf.Name]
				workers = append(workers, &WorkerInfo{concurrence: worker.remainingQouta, worker: worker})
				oc.jobToWorkers[job.conf.Name] = workers

				fmt.Printf("Assigns %v concurrences to worker %v\n", worker.remainingQouta, name)
				pc -= worker.remainingQouta
				worker.remainingQouta = 0
				fmt.Printf("Left queues: %v\n", pc)
			}
		}

		if pc <= 0 {
			return nil
		}
	}

	return errors.New("Insuficient Quata or Worker unavailable")
}

func (oc *Ocean) destroy(job *Job) error {

	reqGRPC := tsgrpc.Request{
		Command: tsgrpc.Request_STOP,
		Params: &tsgrpc.Request_Params{
			Name:            job.conf.Name,
			Url:             job.conf.URL,
			Method:          tsgrpc.Request_GET,
			Host:            job.conf.Host,
			MaxConcurrences: int32(job.conf.MaxConns),
			Body:            job.conf.Body,
		},
	}

	fmt.Printf("Finding service: %v ...\n", job.conf.Name)
	workers := oc.jobToWorkers[job.conf.Name]

	if workers == nil {
		return errors.New("Not found service : " + job.conf.Name)
	}

	for _, wrkInfo := range workers {
		fmt.Println("grpc: stoping service : ", job.conf.Name, " for worker: ", wrkInfo.worker.name)
		wrkInfo.worker.remainingQouta += wrkInfo.concurrence
		fmt.Printf("Recaim %v concurrences to worker : %v\n", wrkInfo.concurrence, wrkInfo.worker.name)
		_, err := wrkInfo.worker.gRPCClient.Stop(&reqGRPC)
		if err != nil {
			fmt.Println("destroy error: " + err.Error())
			return err
		}
	}

	defer delete(oc.jobs, job.conf.Name)
	defer delete(oc.jobToWorkers, job.conf.Name)

	return nil
}

// Start begin calling worker to generate load
func (oc *Ocean) Start(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	err := tshttp.Decoder(w, r, &req)

	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	fmt.Println("cmd: ", req.Cmd)
	fmt.Println("Name: ", req.Conf.Name)
	fmt.Println("Url: ", req.Conf.URL)
	fmt.Println("Host: ", req.Conf.Host)
	fmt.Println("Concurrence: ", req.Conf.Concurrence)
	fmt.Println("Method: ", req.Conf.Method)
	fmt.Println("Headers ", req.Conf.Headers)
	fmt.Println("Body: ", req.Conf.Body)

	job := getJob(&req)

	if oc.jobs[job.conf.Name] != nil {
		data := make(map[string]string)
		data["url"] = "http://" + tshttp.GetIP().String() + Listen + APIVersion + "/metrics"
		data["name"] = req.Conf.Name
		tshttp.WriteSuccess(&w, &data, nil)
		return
	}

	oc.jobs[job.conf.Name] = &job

	if err = oc.JobMatching(&job); err != nil {

		fmt.Println("Warning: " + err.Error())

		if e := oc.destroy(&job); e != nil {
			fmt.Println("Error: " + e.Error())
		}

		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 503, Message: err.Error()})
	} else {
		tshttp.WriteSuccess(&w, nil, nil)
	}

}

// Stop stop generating load to target
func (oc *Ocean) Stop(w http.ResponseWriter, r *http.Request) {
	var req tshttp.Request

	w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)

	err := tshttp.Decoder(w, r, &req)

	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	fmt.Println("cmd: ", req.Cmd)
	fmt.Println("Name: ", req.Conf.Name)
	fmt.Println("Url: ", req.Conf.URL)
	fmt.Println("Host: ", req.Conf.Host)
	fmt.Println("Concurrence: ", req.Conf.Concurrence)
	fmt.Println("Method: ", req.Conf.Method)
	fmt.Println("Headers ", req.Conf.Headers)
	fmt.Println("Body: ", req.Conf.Body)

	job := getJob(&req)

	if oc.jobs[job.conf.Name] == nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: "service not found"})
		return
	}

	if err := oc.destroy(&job); err != nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: err.Error()})
		return
	}

	tshttp.WriteSuccess(&w, nil, nil)
}

// GetMetrics get all worker information
func (oc *Ocean) GetMetrics(w http.ResponseWriter, r *http.Request) {
	tshttp.WriteSuccess(&w, nil, nil)
}

// Register get all worker information
func (oc *Ocean) Register(r *tsgrpc.RegisterRequest) (*tsgrpc.Response, error) {
	fmt.Println("Worker ID               : ", r.Id)
	fmt.Println("Worker Name             : ", r.Name)
	fmt.Println("Worker Max Concurrrencs : ", r.MaxConcurrences)

	return &tsgrpc.Response{
		ErrorCode: 200,
		Data:      "OK",
	}, nil
}

// Monitoring monitor all status of workers.
func (oc *Ocean) Monitoring() {
	for jobname, wrkLists := range oc.jobToWorkers {
		for i := 0; i < len(wrkLists); i++ {
			wrkInfo := wrkLists[i]
			fmt.Println("Monitor job: " + jobname + ", worker: " + wrkInfo.worker.name)
		}
	}
}