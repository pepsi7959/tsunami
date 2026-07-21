package main

import (
	"encoding/json"
	"log"

	tsetcd "github.com/tsunami/etcd"
)

const oceanPrefix = "/tsunami/oceans/"

// oceanReg mirrors what an ocean publishes to etcd (see master/discovery.go).
type oceanReg struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	GRPC           string `json:"grpc"`
	MaxConnections int    `json:"maxConnections"`
	Workers        int    `json:"workers"`
	MaxCap         int    `json:"maxCap"`
	FreeCap        int    `json:"freeCap"`
}

// selectOcean picks the ocean with the most free connection slots
// (maxConnections - workers), skipping full ones, tie-breaking by free load
// capacity. Returns "" when none is available.
func selectOcean(cli *tsetcd.Client) string {
	kvs, err := cli.GetPrefix(oceanPrefix)
	if err != nil {
		log.Printf("discovery: list oceans: %v", err)
		return ""
	}
	best := ""
	bestSlots, bestFree := -1, -1
	for _, kv := range kvs {
		var o oceanReg
		if json.Unmarshal(kv.Value, &o) != nil || o.GRPC == "" {
			continue
		}
		slots := o.MaxConnections - o.Workers
		if o.MaxConnections > 0 && slots <= 0 {
			continue // ocean is full on connections
		}
		if slots > bestSlots || (slots == bestSlots && o.FreeCap > bestFree) {
			best, bestSlots, bestFree = o.GRPC, slots, o.FreeCap
		}
	}
	return best
}
