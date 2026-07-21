package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	tshttp "github.com/tsunami/libs"
	tsgrpc "github.com/tsunami/proto"
)

// ---- topology graph for the web-control visualizer ----
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

func methodEnum(s string) tsgrpc.HTTPMethod {
	switch strings.ToUpper(s) {
	case "POST":
		return tsgrpc.HTTPMethod_POST
	case "PUT":
		return tsgrpc.HTTPMethod_PUT
	case "DELETE":
		return tsgrpc.HTTPMethod_DELETE
	case "UPDATE":
		return tsgrpc.HTTPMethod_UPDATE
	default:
		return tsgrpc.HTTPMethod_GET
	}
}

func confFromReq(c tshttp.CmdConf) tshttp.Conf {
	return tshttp.Conf{
		Name:        c.Name,
		URL:         c.URL,
		Protocol:    c.Protocol,
		Host:        c.Host,
		Port:        c.Port,
		Path:        c.Path,
		Method:      c.Method,
		Headers:     c.Headers,
		Body:        c.Body,
		Concurrence: c.Concurrence,
	}
}

func startCommand(name string, conf tshttp.Conf, take int) *tsgrpc.OceanMessage {
	return &tsgrpc.OceanMessage{Msg: &tsgrpc.OceanMessage_Command{Command: &tsgrpc.Command{
		Action: tsgrpc.Action_START,
		Job:    name,
		Params: &tsgrpc.Params{
			Name:        name,
			Url:         conf.URL,
			Method:      methodEnum(conf.Method),
			Protocol:    conf.Protocol,
			Host:        conf.Host,
			Port:        conf.Port,
			Path:        conf.Path,
			Concurrency: int32(take),
			Body:        conf.Body,
		},
	}}}
}

func stopCommand(name string) *tsgrpc.OceanMessage {
	return &tsgrpc.OceanMessage{Msg: &tsgrpc.OceanMessage_Command{Command: &tsgrpc.Command{
		Action: tsgrpc.Action_STOP, Job: name,
	}}}
}

// allocate distributes `requested` concurrency across attached workers, balanced
// and capped per worker. Returns nil,false if the fleet can't cover it (reject).
// Caller holds oc.mu.
func (oc *Ocean) allocate(requested int) (map[string]int, bool) {
	ids := make([]string, 0, len(oc.workers))
	avail := make(map[string]int, len(oc.workers))
	total := 0
	for id, w := range oc.workers {
		free := w.maxCap - w.used
		if free < 0 {
			free = 0
		}
		ids = append(ids, id)
		avail[id] = free
		total += free
	}
	if requested > total {
		return nil, false
	}
	sort.Strings(ids) // deterministic

	alloc := make(map[string]int, len(ids))
	remaining := requested
	for remaining > 0 {
		active := make([]string, 0, len(ids))
		for _, id := range ids {
			if avail[id]-alloc[id] > 0 {
				active = append(active, id)
			}
		}
		if len(active) == 0 {
			break
		}
		share := remaining / len(active)
		if share < 1 {
			share = 1 // distribute the remainder one at a time
		}
		for _, id := range active {
			if remaining <= 0 {
				break
			}
			give := share
			if room := avail[id] - alloc[id]; give > room {
				give = room
			}
			if give > remaining {
				give = remaining
			}
			alloc[id] += give
			remaining -= give
		}
	}
	if remaining > 0 {
		return nil, false
	}
	for id, c := range alloc {
		if c == 0 {
			delete(alloc, id)
		}
	}
	return alloc, true
}

func cors(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", AllowOrigin)
	}
}

// Options handles CORS preflight.
func (oc *Ocean) Options(w http.ResponseWriter, r *http.Request) {
	cors(w, r)
	w.Header().Set("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE,OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Content-Length, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	tshttp.WriteSuccess(&w, nil, nil)
}

// Start splits a load test across attached workers (reject if capacity short).
func (oc *Ocean) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name
	conf := confFromReq(req.Conf)
	requested := req.Conf.Concurrence

	oc.mu.Lock()
	if oc.jobs[name] != nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 409, Message: "a test named \"" + name + "\" is already running"})
		return
	}
	alloc, ok := oc.allocate(requested)
	if !ok {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 503, Message: "insufficient worker capacity for the requested concurrency"})
		return
	}
	for id, take := range alloc {
		wk := oc.workers[id]
		wk.used += take
		wk.command(startCommand(name, conf, take))
		fmt.Printf("assign %d to worker %s (job %s)\n", take, wk.name, name)
	}
	oc.jobs[name] = &job{name: name, conf: conf, assignments: alloc}
	oc.mu.Unlock()

	tshttp.WriteSuccess(&w, nil, nil)
}

// Stop stops a running test on all its workers and frees capacity.
func (oc *Ocean) Stop(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name

	oc.mu.Lock()
	j := oc.jobs[name]
	if j == nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: "service not found"})
		return
	}
	for id, take := range j.assignments {
		if wk := oc.workers[id]; wk != nil {
			wk.used -= take
			if wk.used < 0 {
				wk.used = 0
			}
			wk.command(stopCommand(name))
		}
	}
	delete(oc.jobs, name)
	oc.mu.Unlock()

	tshttp.WriteSuccess(&w, nil, nil)
}

// GetMetrics aggregates the latest per-worker metrics for a running test.
func (oc *Ocean) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}
	name := req.Conf.Name

	oc.mu.Lock()
	j := oc.jobs[name]
	if j == nil {
		oc.mu.Unlock()
		tshttp.WriteSuccess(&w, nil, &tshttp.Error{Code: 404, Message: name + " not running"})
		return
	}
	agg := tshttp.Metric{Name: name}
	var num int
	for id := range j.assignments {
		wk := oc.workers[id]
		if wk == nil {
			continue
		}
		m := wk.metrics[name]
		if m == nil {
			continue
		}
		num++
		agg.WorkerCount += int(m.GetWorkerCount())
		agg.RequestCount += int(m.GetRequestCount())
		agg.ErrorCount += int(m.GetErrorCount())
		agg.Avg += m.GetAvg()
		agg.Rps += m.GetRps()
		if m.GetMax() > agg.Max {
			agg.Max = m.GetMax()
		}
		if agg.Min == 0.0 || (m.GetMin() > 0 && m.GetMin() < agg.Min) {
			agg.Min = m.GetMin()
		}
		if m.GetElapsedTime() > agg.ElapedTime {
			agg.ElapedTime = m.GetElapsedTime()
		}
	}
	oc.mu.Unlock()
	if num > 0 {
		agg.Avg = agg.Avg / float64(num)
	}
	tshttp.WriteSuccess(&w, metricxToSlice(&agg), nil)
}

func metricxToSlice(m *tshttp.Metric) *map[string]string {
	data := map[string]string{
		"name":         m.Name,
		"avg":          fmt.Sprintf("%f", m.Avg),
		"min":          fmt.Sprintf("%f", m.Min),
		"max":          fmt.Sprintf("%f", m.Max),
		"error_count":  fmt.Sprintf("%d", m.ErrorCount),
		"workerCount":  fmt.Sprintf("%d", m.WorkerCount),
		"ElapedTime":   fmt.Sprintf("%f", m.ElapedTime),
		"RequestCount": fmt.Sprintf("%d", m.RequestCount),
		"rps":          fmt.Sprintf("%f", m.Rps),
	}
	return &data
}

// GetInfo reports cluster state + topology for the web control.
func (oc *Ocean) GetInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		oc.Options(w, r)
		return
	}
	cors(w, r)
	var req tshttp.Request
	if err := tshttp.Decoder(w, r, &req); err != nil {
		return
	}

	data := make(map[string]string)
	oc.mu.Lock()

	maxConcurrent, used := 0, 0
	oceanIP := ""
	if ip := tshttp.GetIP(); ip != nil {
		oceanIP = ip.String()
	}
	g := topoGraph{}
	g.Nodes = append(g.Nodes, topoNode{ID: "master:" + oc.id, Name: oc.name, Kind: "master", IP: oceanIP, Endpoint: oc.viper.GetString("endpoints.http")})

	for id, wk := range oc.workers {
		maxConcurrent += wk.maxCap
		used += wk.used
		data["worker_"+id+"_max_qouta"] = fmt.Sprintf("%d", wk.maxCap)
		data["worker_"+id+"_remain_qouta"] = fmt.Sprintf("%d", wk.maxCap-wk.used)
		g.Nodes = append(g.Nodes, topoNode{ID: id, Name: wk.name, Kind: "worker", IP: wk.ip, Endpoint: wk.ip})
		g.Edges = append(g.Edges, topoEdge{From: "master:" + oc.id, To: id, Kind: "control"})
	}

	names := make([]string, 0, len(oc.jobs))
	seenTarget := map[string]bool{}
	for name, j := range oc.jobs {
		names = append(names, name)
		url := j.conf.URL
		if url != "" {
			tid := "target:" + url
			if !seenTarget[tid] {
				seenTarget[tid] = true
				g.Nodes = append(g.Nodes, topoNode{ID: tid, Name: url, Kind: "target", Endpoint: url})
			}
			for id := range j.assignments {
				g.Edges = append(g.Edges, topoEdge{From: id, To: tid, Kind: "load", Job: name})
			}
		}
	}

	data["name"] = oc.name
	data["id"] = oc.id
	data["workers"] = fmt.Sprintf("%d", len(oc.workers))
	data["max_connections"] = fmt.Sprintf("%d", oc.maxConnections)
	data["max_concurrent"] = fmt.Sprintf("%d", maxConcurrent)
	data["remaining_Concurrent"] = fmt.Sprintf("%d", maxConcurrent-used)
	data["jobs"] = strings.Join(names, ",")
	oc.mu.Unlock()

	if b, err := json.Marshal(g); err == nil {
		data["topology"] = string(b)
	}
	tshttp.WriteSuccess(&w, &data, nil)
}
