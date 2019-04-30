package main

import (
	"bytes"
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
		fmt.Fprintf(*w, "{\"data\": { %s }", b)
	}
}
