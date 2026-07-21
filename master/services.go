package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	tsgrpc "github.com/tsunami/proto"

	tshttp "github.com/tsunami/libs"
)

// topology graph exposed to the web control's visualizer
type topoNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // master | worker | target
	IP       string `json:"ip"`
	Endpoint string `json:"endpoint"`
}

type topoEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // control | load
	Job  string `json:"job,omitempty"`
}

type topoGraph struct {
	Nodes []topoNode `json:"nodes"`
	Edges []topoEdge `json:"edges"`
}

// resolveHost turns "host:port" (or "host") into a best-effort IP string.
func resolveHost(endpoint string) string {
	host := endpoint
	if h, _, err := net.SplitHostPort(endpoint); err == nil {
		host = h
	}
	if addrs, err := net.LookupHost(host); err == nil && len(addrs) > 0 {
		return addrs[0]
	}
	return host
}

func metricxToSlice(m *tshttp.Metric) *map[string]string {
	data := make(map[string]string)
	data["name"] = m.Name
	data["avg"] = fmt.Sprintf("%f", m.Avg)
	data["min"] = fmt.Sprintf("%f", m.Min)
	data["max"] = fmt.Sprintf("%f", m.Max)
	data["error_count"] = fmt.Sprintf("%d", m.ErrorCount)
	data["workerCount"] = fmt.Sprintf("%d", m.WorkerCount)
	data["ElapedTime"] = fmt.Sprintf("%f", m.ElapedTime)
	data["RequestCount"] = fmt.Sprintf("%d", m.RequestCount)
	data["rps"] = fmt.Sprintf("%f", m.Rps)
	return &data
}

func getJob(req *tshttp.Request) Job {

	conf := tshttp.Conf{
		Name:        req.Conf.Name,
		URL:         req.Conf.URL,
		Protocol:    req.Conf.Protocol,
		Host:        req.Conf.Host,
		Port:        req.Conf.Port,
		Path:        req.Conf.Path,
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
			Protocol:        job.conf.Protocol,
			Host:            job.conf.Host,
			Port:            job.conf.Port,
			Path:            job.conf.Path,
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

	// Always drop the job + assignment records, even if scheduling failed and no
	// workers were ever assigned — otherwise the job lingers in oc.jobs forever as
	// an unstoppable "zombie" that clutters the test list and reports no metrics.
	defer delete(oc.jobs, job.conf.Name)
	defer delete(oc.jobToWorkers, job.conf.Name)

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
		return nil
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

	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}

	var req tshttp.Request

	origin := r.Header.Get("Origin")

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}

	err := tshttp.Decoder(w, r, &req)

	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	fmt.Println("cmd: ", req.Cmd)
	fmt.Println("Name: ", req.Conf.Name)
	fmt.Println("Url: ", req.Conf.URL)
	fmt.Println("Protocol: ", req.Conf.Protocol)
	fmt.Println("Host: ", req.Conf.Host)
	fmt.Println("Post: ", req.Conf.Port)
	fmt.Println("Path: ", req.Conf.Path)
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

	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}

	var req tshttp.Request

	origin := r.Header.Get("Origin")

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}

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

	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}

	var req tshttp.Request

	origin := r.Header.Get("Origin")

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}

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

	if oc.jobToWorkers[req.Conf.Name] == nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: req.Conf.Name + " not running"})
		return
	}

	var num int
	allMetrics := tshttp.Metric{}

	for _, workerLists := range oc.jobToWorkers[req.Conf.Name] {
		num++
		fmt.Println("get metrics from " + workerLists.worker.name)
		grpcReq := tsgrpc.Request{
			Command: tsgrpc.Request_GET_METRICES,
			Params: &tsgrpc.Request_Params{
				Name:   req.Conf.Name,
				Url:    req.Conf.URL,
				Method: tsgrpc.Request_GET,
				Host:   req.Conf.Host,
				Body:   req.Conf.Body,
			},
		}
		resp, err := workerLists.worker.gRPCClient.GetMetrics(&grpcReq)

		if err != nil {
			tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: err.Error()})
			return
		}

		metric := tshttp.Metric{}

		err = json.Unmarshal([]byte(resp.GetData()), &metric)

		if err != nil {
			tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: err.Error()})
			return
		}

		if metric.Max > allMetrics.Max {
			allMetrics.Max = metric.Max
		}

		if allMetrics.Min == 0.0 || metric.Min < allMetrics.Min {
			allMetrics.Min = metric.Min
		}

		allMetrics.Avg += metric.Avg
		allMetrics.ElapedTime += metric.ElapedTime
		allMetrics.ErrorCount += metric.ErrorCount
		allMetrics.WorkerCount += metric.WorkerCount
		allMetrics.RequestCount += metric.RequestCount
		fmt.Println(resp.GetData())
	}
	allMetrics.Name = req.Conf.Name
	allMetrics.Avg = (allMetrics.Avg / float64(num))
	allMetrics.ElapedTime = (allMetrics.ElapedTime / float64(num))
	allMetrics.Rps = (float64(allMetrics.RequestCount) / float64(allMetrics.ElapedTime))

	tshttp.WriteSuccess(&w, metricxToSlice(&allMetrics), nil)
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

// GetInfo master configuration
func (oc *Ocean) GetInfo(w http.ResponseWriter, r *http.Request) {

	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}

	var req tshttp.Request

	origin := r.Header.Get("Origin")

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}

	err := tshttp.Decoder(w, r, &req)

	if err != nil {
		fmt.Printf(err.Error())
		return
	}

	maxConcurrent := 0
	remainingQouta := 0
	data := make(map[string]string)

	for ID, worker := range oc.workers {
		wrkName := "worker_" + ID + "_"
		data[wrkName+"max_qouta"] = fmt.Sprintf("%d", worker.maxQouta)
		data[wrkName+"remain_qouta"] = fmt.Sprintf("%d", worker.remainingQouta)
		maxConcurrent += worker.maxQouta
		remainingQouta += worker.remainingQouta
	}

	data["name"] = oc.viper.GetString("name")
	data["id"] = oc.viper.GetString("id")
	// data["registry.endpoints"] = oc.viper.GetStringSlice("registry.endpoints")
	data["registry.request_timeout"] = oc.viper.GetString("registry.request_timeout")
	data["registry.dial_timeout"] = oc.viper.GetString("registry.dial_timeout")
	data["registry.client_config_key"] = oc.viper.GetString("registry.client_config_key")
	data["workers"] = fmt.Sprintf("%d", len(oc.workers))

	data["max_concurrent"] = fmt.Sprintf("%d", maxConcurrent)
	data["remaining_Concurrent"] = fmt.Sprintf("%d", remainingQouta)

	// list of currently running tests (for the web control's "watching test" dropdown)
	names := make([]string, 0, len(oc.jobs))
	for n := range oc.jobs {
		names = append(names, n)
	}
	data["jobs"] = strings.Join(names, ",")

	// topology graph: ocean -> worker(s) -> target(s), for the visualizer
	g := topoGraph{}
	oceanIP := ""
	if ip := tshttp.GetIP(); ip != nil {
		oceanIP = ip.String()
	}
	g.Nodes = append(g.Nodes, topoNode{ID: "ocean", Name: "ocean", Kind: "master", IP: oceanIP, Endpoint: oc.viper.GetString("endpoints.http")})
	for ID, worker := range oc.workers {
		g.Nodes = append(g.Nodes, topoNode{ID: ID, Name: worker.name, Kind: "worker", IP: resolveHost(worker.endpoint), Endpoint: worker.endpoint})
		g.Edges = append(g.Edges, topoEdge{From: "ocean", To: ID, Kind: "control"})
	}
	seenTarget := map[string]bool{}
	for name, job := range oc.jobs {
		url := job.conf.URL
		if url == "" {
			continue
		}
		tid := "target:" + url
		if !seenTarget[tid] {
			seenTarget[tid] = true
			g.Nodes = append(g.Nodes, topoNode{ID: tid, Name: url, Kind: "target", Endpoint: url})
		}
		for _, wi := range oc.jobToWorkers[name] {
			g.Edges = append(g.Edges, topoEdge{From: wi.worker.ID, To: tid, Kind: "load", Job: name})
		}
	}
	if b, err := json.Marshal(g); err == nil {
		data["topology"] = string(b)
	}

	tshttp.WriteSuccess(&w, &data, nil)

}

// ReloadWorkers re-reads the worker registry from etcd and picks up any newly
// registered workers (used by the web control's Workers page "rediscover").
func (oc *Ocean) ReloadWorkers(w http.ResponseWriter, r *http.Request) {

	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}

	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}

	added, err := oc.DiscoverWorkers()
	if err != nil {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 500, Message: err.Error()})
		return
	}

	data := map[string]string{
		"added":   fmt.Sprintf("%d", added),
		"workers": fmt.Sprintf("%d", len(oc.workers)),
	}
	tshttp.WriteSuccess(&w, &data, nil)
}

// Options support preflight
func (oc *Ocean) Options(w http.ResponseWriter, r *http.Request) {

	origin := r.Header.Get("Origin")

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Content-Length, X-Requested-With, userid, token")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	tshttp.WriteSuccess(&w, nil, nil)
}
