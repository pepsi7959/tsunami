package main

import (
	"net/http"

	tshttp "github.com/tsunami/libs"
)

// Start begin calling worker to generate load
func (oc *Oceans) Start(w http.ResponseWriter, r *http.Request) {
	tshttp.WriteSuccess(&w, nil, nil)
}

// Stop stop generating load to target
func (oc *Oceans) Stop(w http.ResponseWriter, r *http.Request) {
	tshttp.WriteSuccess(&w, nil, nil)
}

// GetMetrics get all worker information
func (oc *Oceans) GetMetrics(w http.ResponseWriter, r *http.Request) {
	tshttp.WriteSuccess(&w, nil, nil)
}
