package main

import (
	"encoding/json"
	"time"

	tsetcd "github.com/tsunami/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	oceanPrefix = "/tsunami/oceans/"
	fleetPrefix = "/tsunami/fleet/"
)

// oceanReg is what an ocean publishes to etcd for worker discovery + selection.
type oceanReg struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	GRPC           string `json:"grpc"` // advertise address workers dial
	HTTP           string `json:"http"`
	MaxConnections int    `json:"maxConnections"`
	Workers        int    `json:"workers"`
	MaxCap         int    `json:"maxCap"`
	FreeCap        int    `json:"freeCap"`
}

type fleetJob struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	Concurrency int    `json:"concurrency"`
	Ocean       string `json:"ocean"`
}

type fleetSummary struct {
	Ocean string     `json:"ocean"`
	Jobs  []fleetJob `json:"jobs"`
}

// initDiscovery registers this ocean in etcd under a lease and keeps its
// connection/capacity counters fresh so workers can select the best ocean.
func (oc *Ocean) initDiscovery() error {
	ttl := oc.viper.GetInt("lease.ttl")
	if ttl <= 0 {
		ttl = 10
	}
	cli, err := tsetcd.NewClient(
		oc.viper.GetStringSlice("registry.endpoints"),
		time.Second*time.Duration(oc.viper.GetInt("registry.dial_timeout")),
		time.Second*time.Duration(oc.viper.GetInt("registry.request_timeout")),
		ttl,
	)
	if err != nil {
		return err
	}
	oc.etcd = cli
	lease, err := cli.Grant(int64(ttl))
	if err != nil {
		return err
	}
	oc.lease = lease
	oc.publishOcean()
	if err := cli.KeepAlive(lease); err != nil {
		return err
	}
	// heartbeat refresh of counters + fleet summary
	go func() {
		t := time.NewTicker(time.Duration(ttl/3+1) * time.Second)
		defer t.Stop()
		for range t.C {
			oc.publishOcean()
			oc.publishFleet()
		}
	}()
	return nil
}

// publishOcean writes this ocean's live connection + capacity state to etcd.
func (oc *Ocean) publishOcean() {
	if oc.etcd == nil {
		return
	}
	oc.mu.Lock()
	maxCap, used := 0, 0
	for _, w := range oc.workers {
		maxCap += w.maxCap
		used += w.used
	}
	reg := oceanReg{
		ID: oc.id, Name: oc.name, GRPC: oc.advertiseGRPC, HTTP: oc.httpAddr,
		MaxConnections: oc.maxConnections, Workers: len(oc.workers),
		MaxCap: maxCap, FreeCap: maxCap - used,
	}
	oc.mu.Unlock()
	if b, err := json.Marshal(reg); err == nil {
		oc.etcd.Put(oceanPrefix+oc.id, string(b), clientv3.WithLease(oc.lease))
	}
}

// publishFleet writes this ocean's running-job summary for the fleet dashboard.
func (oc *Ocean) publishFleet() {
	if oc.etcd == nil {
		return
	}
	oc.mu.Lock()
	fs := fleetSummary{Ocean: oc.id}
	for name, j := range oc.jobs {
		total := 0
		for _, c := range j.assignments {
			total += c
		}
		fs.Jobs = append(fs.Jobs, fleetJob{Name: name, Target: j.conf.URL, Concurrency: total, Ocean: oc.id})
	}
	oc.mu.Unlock()
	if b, err := json.Marshal(fs); err == nil {
		oc.etcd.Put(fleetPrefix+oc.id, string(b), clientv3.WithLease(oc.lease))
	}
}

// allOceans returns every ocean currently registered (for the fleet topology).
func (oc *Ocean) allOceans() []oceanReg {
	if oc.etcd == nil {
		return nil
	}
	kvs, err := oc.etcd.GetPrefix(oceanPrefix)
	if err != nil {
		return nil
	}
	out := make([]oceanReg, 0, len(kvs))
	for _, kv := range kvs {
		var r oceanReg
		if json.Unmarshal(kv.Value, &r) == nil && r.ID != "" {
			out = append(out, r)
		}
	}
	return out
}

// allFleetJobs returns every running job across all oceans (for the dashboard).
func (oc *Ocean) allFleetJobs() []fleetJob {
	if oc.etcd == nil {
		return nil
	}
	kvs, err := oc.etcd.GetPrefix(fleetPrefix)
	if err != nil {
		return nil
	}
	out := []fleetJob{}
	for _, kv := range kvs {
		var fs fleetSummary
		if json.Unmarshal(kv.Value, &fs) == nil {
			out = append(out, fs.Jobs...)
		}
	}
	return out
}
