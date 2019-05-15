package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	RESULT_NOT_FOUND = 40400
	RESULT_SUCCESS   = 20000
)

// google style guide
// https://google.github.io/styleguide/

// Data is interface
type Data interface{}

// Error is sturct of error response
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Reponse structure
type Response struct {
	Code  int   `json:"code"`
	Data  Data  `json:"data"`
	Error Error `json:"error"`
}

func WriteSuccess(w *http.ResponseWriter, data *map[string]string, err *Error) {

	(*w).Header().Set("Content-Type", "application/json")

	if err != nil {
		resp := Response{Error: Error{Message: err.Message}}
		j_resp, _ := json.Marshal(&resp)
		fmt.Fprintf(*w, string(j_resp))
	} else {
		resp := Response{Code: RESULT_SUCCESS, Data: data}
		j_resp, _ := json.Marshal(&resp)
		fmt.Fprintf(*w, string(j_resp))
	}
}
