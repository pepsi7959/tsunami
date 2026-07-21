package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	tsgrpc "github.com/tsunami/proto"

	tshttp "github.com/tsunami/libs"
	clientv3 "go.etcd.io/etcd/client/v3"
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

// startParams builds the gRPC START request for a job at a given concurrency.
func startParams(job *Job, take int) *tsgrpc.Request {
	return &tsgrpc.Request{
		Command: tsgrpc.Request_START,
		Params: &tsgrpc.Request_Params{
			Name:            job.conf.Name,
			Url:             job.conf.URL,
			Method:          tsgrpc.Request_GET,
			Protocol:        job.conf.Protocol,
			Host:            job.conf.Host,
			Port:            job.conf.Port,
			Path:            job.conf.Path,
			MaxConcurrences: int32(take),
			Body:            job.conf.Body,
		},
	}
}

// stopParams builds the gRPC STOP request for a job.
func stopParams(job *Job) *tsgrpc.Request {
	return &tsgrpc.Request{
		Command: tsgrpc.Request_STOP,
		Params: &tsgrpc.Request_Params{
			Name:   job.conf.Name,
			Url:    job.conf.URL,
			Method: tsgrpc.Request_GET,
			Host:   job.conf.Host,
			Body:   job.conf.Body,
		},
	}
}

type reservation struct {
	worker *Worker
	take   int
}

// resKey is this master's reservation key for (worker, job).
func (oc *Ocean) resKey(workerID, job string) string {
	return oc.viper.GetString("registry.reservation_prefix") + workerID + "/" + oc.masterID + "/" + job
}

func (oc *Ocean) rollbackReservations(job *Job, plan []reservation) {
	for _, p := range plan {
		oc.etcd.Delete(oc.resKey(p.worker.ID, job.conf.Name))
	}
}

// Reserve allocates a job's concurrency across workers using a per-worker etcd
// mutex so that concurrent masters can never oversubscribe a worker's
// MaxConcurrences. A worker's used capacity is the sum of ALL masters'
// reservation keys under it; the reservation this master writes is bound to its
// lease, so it auto-releases if this master dies. Load is only started once the
// reservation is durable.
func (oc *Ocean) Reserve(job *Job) error {
	need := job.pendingConcurrence
	resPrefix := oc.viper.GetString("registry.reservation_prefix")
	lockPrefix := oc.viper.GetString("registry.lock_prefix")

	// snapshot candidate workers in a deterministic order, without holding mu
	// across the etcd/gRPC calls below
	oc.mu.RLock()
	cands := make([]*Worker, 0, len(oc.workers))
	for _, wk := range oc.workers {
		cands = append(cands, wk)
	}
	oc.mu.RUnlock()
	sort.Slice(cands, func(i, j int) bool { return cands[i].ID < cands[j].ID })

	plan := []reservation{}
	for _, wk := range cands {
		if need <= 0 {
			break
		}
		take := 0
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := oc.etcd.WithLock(ctx, lockPrefix+wk.ID, func() error {
			kvs, err := oc.etcd.GetPrefix(resPrefix + wk.ID + "/")
			if err != nil {
				return err
			}
			used := 0
			for _, kv := range kvs {
				n, _ := strconv.Atoi(string(kv.Value))
				used += n
			}
			avail := wk.maxQouta - used
			if avail <= 0 {
				return nil
			}
			if avail < need {
				take = avail
			} else {
				take = need
			}
			return oc.etcd.Put(oc.resKey(wk.ID, job.conf.Name), strconv.Itoa(take), clientv3.WithLease(oc.masterLease))
		})
		cancel()
		if err != nil {
			oc.rollbackReservations(job, plan)
			return err
		}
		if take > 0 {
			plan = append(plan, reservation{worker: wk, take: take})
			need -= take
			fmt.Printf("Reserved %d on worker %s (job %s)\n", take, wk.name, job.conf.Name)
		}
	}

	if need > 0 {
		oc.rollbackReservations(job, plan)
		return errors.New("Insuficient Quata or Worker unavailable")
	}

	// reservation is durable; now start the load
	for i, p := range plan {
		if _, err := p.worker.gRPCClient.Start(startParams(job, p.take)); err != nil {
			for j := 0; j < i; j++ {
				plan[j].worker.gRPCClient.Stop(stopParams(job))
			}
			oc.rollbackReservations(job, plan)
			return err
		}
	}

	// record local assignment cache (used for Stop/metrics on this master)
	oc.mu.Lock()
	wis := make([]*WorkerInfo, 0, len(plan))
	for _, p := range plan {
		wis = append(wis, &WorkerInfo{concurrence: p.take, worker: p.worker})
	}
	oc.jobToWorkers[job.conf.Name] = wis
	oc.mu.Unlock()

	// publish fleet-wide job metadata so any master can list/stop/report on it.
	// Bound to this master's lease → auto-removed if this master dies.
	meta := JobMeta{Name: job.conf.Name, Owner: oc.masterID, URL: job.conf.URL, Host: job.conf.Host}
	for _, p := range plan {
		meta.Assignments = append(meta.Assignments, JobAssign{WorkerID: p.worker.ID, Count: p.take})
	}
	if b, err := json.Marshal(meta); err == nil {
		oc.etcd.Put(oc.viper.GetString("registry.job_prefix")+job.conf.Name, string(b), clientv3.WithLease(oc.masterLease))
	}
	return nil
}

// destroy stops a job on every worker it was assigned to and releases its
// reservation keys (capacity also auto-reclaims via lease). It always clears the
// local job/assignment records so a failed schedule can't leave a zombie.
// Callers must NOT hold oc.mu (destroy takes it briefly itself).
func (oc *Ocean) destroy(job *Job) error {
	name := job.conf.Name

	oc.mu.RLock()
	meta, viaFleet := oc.allJobs[name]
	local := oc.jobToWorkers[name]
	oc.mu.RUnlock()

	// clear local claim/cache
	oc.mu.Lock()
	delete(oc.jobs, name)
	delete(oc.jobToWorkers, name)
	oc.mu.Unlock()

	fmt.Printf("Finding service: %v ...\n", name)
	resPrefix := oc.viper.GetString("registry.reservation_prefix")

	var firstErr error
	if viaFleet {
		// works even if this master isn't the owner (fleet-wide stop)
		for _, a := range meta.Assignments {
			if wk := oc.workerByID(a.WorkerID); wk != nil && wk.gRPCClient != nil {
				if _, err := wk.gRPCClient.Stop(stopParams(job)); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			oc.etcd.Delete(resPrefix + a.WorkerID + "/" + meta.Owner + "/" + name)
		}
		oc.etcd.Delete(oc.viper.GetString("registry.job_prefix") + name)
	} else {
		// fallback: local cache (e.g. Start rollback before meta was published)
		for _, wrkInfo := range local {
			if wrkInfo.worker.gRPCClient != nil {
				wrkInfo.worker.gRPCClient.Stop(stopParams(job))
			}
			oc.etcd.Delete(oc.resKey(wrkInfo.worker.ID, name))
		}
	}
	return firstErr
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

	// Claim the job name locally under a brief lock, then release it before the
	// (slow) reserve/gRPC work so the watch goroutines aren't blocked.
	oc.mu.Lock()
	if oc.jobs[job.conf.Name] != nil {
		oc.mu.Unlock()
		data := make(map[string]string)
		data["url"] = "http://" + tshttp.GetIP().String() + Listen + APIVersion + "/metrics"
		data["name"] = req.Conf.Name
		tshttp.WriteSuccess(&w, &data, nil)
		return
	}
	oc.jobs[job.conf.Name] = &job
	oc.mu.Unlock()

	if err = oc.Reserve(&job); err != nil {

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

	oc.mu.RLock()
	_, inFleet := oc.allJobs[job.conf.Name]
	_, inLocal := oc.jobs[job.conf.Name]
	oc.mu.RUnlock()

	if !inFleet && !inLocal {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: "service not found"})
		return
	}

	// destroy manages oc.mu itself; do not hold it here
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

	// resolve the job's workers from the fleet-wide metadata, so any master can
	// report metrics for a job owned by any master
	oc.mu.RLock()
	meta, ok := oc.allJobs[req.Conf.Name]
	oc.mu.RUnlock()

	if !ok {
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: req.Conf.Name + " not running"})
		return
	}

	var num int
	allMetrics := tshttp.Metric{}

	for _, a := range meta.Assignments {
		wk := oc.workerByID(a.WorkerID)
		if wk == nil || wk.gRPCClient == nil {
			continue
		}
		num++
		fmt.Println("get metrics from " + wk.name)
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
		resp, err := wk.gRPCClient.GetMetrics(&grpcReq)

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
	oc.mu.RLock()
	defer oc.mu.RUnlock()
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

	oc.mu.RLock()
	defer oc.mu.RUnlock()

	for ID, worker := range oc.workers {
		// remaining is fleet-wide: max minus the sum of ALL masters' reservations
		remain := worker.maxQouta - oc.resByWorker[ID]
		if remain < 0 {
			remain = 0
		}
		wrkName := "worker_" + ID + "_"
		data[wrkName+"max_qouta"] = fmt.Sprintf("%d", worker.maxQouta)
		data[wrkName+"remain_qouta"] = fmt.Sprintf("%d", remain)
		maxConcurrent += worker.maxQouta
		remainingQouta += remain
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

	// fleet-wide list of running tests (for the web control's "watching test" dropdown)
	names := make([]string, 0, len(oc.allJobs))
	for n := range oc.allJobs {
		names = append(names, n)
	}
	data["jobs"] = strings.Join(names, ",")

	// topology graph: masters -> workers -> targets (fleet-wide), for the visualizer
	g := topoGraph{}
	for id, m := range oc.masters {
		nm := m.Name
		if nm == "" {
			nm = id
		}
		g.Nodes = append(g.Nodes, topoNode{ID: "master:" + id, Name: nm, Kind: "master", IP: m.IP, Endpoint: m.HTTP})
	}
	for ID, worker := range oc.workers {
		g.Nodes = append(g.Nodes, topoNode{ID: ID, Name: worker.name, Kind: "worker", IP: resolveHost(worker.endpoint), Endpoint: worker.endpoint})
		// active-active: every master can control every worker
		for id := range oc.masters {
			g.Edges = append(g.Edges, topoEdge{From: "master:" + id, To: ID, Kind: "control"})
		}
	}
	seenTarget := map[string]bool{}
	for name, m := range oc.allJobs {
		if m.URL == "" {
			continue
		}
		tid := "target:" + m.URL
		if !seenTarget[tid] {
			seenTarget[tid] = true
			g.Nodes = append(g.Nodes, topoNode{ID: tid, Name: m.URL, Kind: "target", Endpoint: m.URL})
		}
		for _, a := range m.Assignments {
			g.Edges = append(g.Edges, topoEdge{From: a.WorkerID, To: tid, Kind: "load", Job: name})
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

	oc.mu.RLock()
	total := len(oc.workers)
	oc.mu.RUnlock()

	data := map[string]string{
		"added":   fmt.Sprintf("%d", added),
		"workers": fmt.Sprintf("%d", total),
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
