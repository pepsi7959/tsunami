package main

import (
	"fmt"
	"net/http"

	tshttp "github.com/tsunami/libs"
)

// Start begin calling worker to generate load
func (oc *Oceans) Start(w http.ResponseWriter, r *http.Request) {

	var req tshttp.Request
	err := tshttp.Decoder(w, r, req)

	if err != nil {
		fmt.Printf(err.Error())
		return
	}

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
