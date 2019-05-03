package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type HttpError struct {
	Code    int
	Message string
}

type HttpResponse struct {
	data  map[string]interface{}
	error map[string]interface{}
}

func CreateJsonRes(w *http.ResponseWriter, data *map[string]string, err *HttpError) {
	if err != nil {
		fmt.Fprintf(*w, "{\"error\": {\"code\": %v,\"message\": \"%v\"}", err.Code, err.Message)
	} else {
		var b bytes.Buffer
		for k, v := range *data {
			fmt.Fprintf(&b, "\"%s\":\"%s\",", k, v)
		}
		resp := Response{Data: data}
		j_resp, _ := json.Marshal(&resp)
		fmt.Fprintf(*w, string(j_resp))
	}
}
