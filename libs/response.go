package tshttp

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	//ResultNotFound not found error code
	ResultNotFound = 40400
	//ResultSuccess success code
	ResultSuccess = 20000
	//ResultBadRequest user input invalid syntax
	ResultBadRequest = 40000
	//ResultInvalidCMD invalid command
	ResultInvalidCMD = 40001
	//ResultServiceUnavailable unavailable service
	ResultServiceUnavailable = 50300
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

//Response structure
type Response struct {
	Code  int   `json:"code"`
	Data  Data  `json:"data"`
	Error Error `json:"error"`
}

//WriteSuccess send success response
func WriteSuccess(w *http.ResponseWriter, data *map[string]string, err *Error) {

	(*w).Header().Set("Content-Type", "application/json")

	if err != nil {
		resp := Response{Error: Error{Code: err.Code, Message: err.Message}}
		JSONResp, _ := json.Marshal(&resp)
		fmt.Fprintf(*w, string(JSONResp))
	} else {
		resp := Response{Code: ResultSuccess, Data: data}
		JSONResp, _ := json.Marshal(&resp)
		fmt.Fprintf(*w, string(JSONResp))
	}
}
