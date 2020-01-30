package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	tshttp "github.com/tsunami/libs"
)

// HTTPError http error structure
type HTTPError struct {
	Code    int
	Message string
}

// HTTPResponse http response
type HTTPResponse struct {
	data  map[string]interface{}
	error map[string]interface{}
}

//CreateJSONRes creat response as json format
func CreateJSONRes(w *http.ResponseWriter, data *map[string]string, err *HTTPError) {
	if err != nil {
		fmt.Fprintf(*w, "{\"error\": {\"code\": %v,\"message\": \"%v\"}", err.Code, err.Message)
	} else {
		var b bytes.Buffer
		for k, v := range *data {
			fmt.Fprintf(&b, "\"%s\":\"%s\",", k, v)
		}
		resp := tshttp.Response{Data: data}
		JSONResp, _ := json.Marshal(&resp)
		fmt.Fprintf(*w, string(JSONResp))
	}
}
