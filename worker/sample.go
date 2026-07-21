package main

import (
	"sync"
	"time"

	tsgrpc "github.com/tsunami/proto"
	"github.com/valyala/fasthttp"
)

// sampleRec holds the most recent request/response captured for a verbose job.
// It is shared by all workers of a service; last write wins. When verbose is
// off the recorder is nil and nothing is captured (zero hot-path overhead).
type sampleRec struct {
	mu sync.Mutex
	s  *tsgrpc.Sample
}

// capture records one request/response pair. Call before the fasthttp objects
// are released (their bodies are reused after Release).
func (r *sampleRec) capture(req *fasthttp.Request, resp *fasthttp.Response, latencyMs float64, doErr error) {
	s := &tsgrpc.Sample{
		Method:        string(req.Header.Method()),
		Url:           req.URI().String(),
		RequestHeader: reqHeaders(req),
		RequestBody:   string(req.Body()),
		LatencyMs:     latencyMs,
		Ts:            time.Now().UnixMilli(),
	}
	if doErr != nil {
		s.Error = doErr.Error()
	} else {
		s.StatusCode = int32(resp.StatusCode())
		s.ResponseHeader = respHeaders(resp)
		s.ResponseBody = string(resp.Body())
	}
	r.mu.Lock()
	r.s = s
	r.mu.Unlock()
}

func (r *sampleRec) get() *tsgrpc.Sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.s
}

func reqHeaders(req *fasthttp.Request) []*tsgrpc.HTTPHeader {
	var out []*tsgrpc.HTTPHeader
	req.Header.VisitAll(func(k, v []byte) {
		out = append(out, &tsgrpc.HTTPHeader{Key: string(k), Value: string(v)})
	})
	return out
}

func respHeaders(resp *fasthttp.Response) []*tsgrpc.HTTPHeader {
	var out []*tsgrpc.HTTPHeader
	resp.Header.VisitAll(func(k, v []byte) {
		out = append(out, &tsgrpc.HTTPHeader{Key: string(k), Value: string(v)})
	})
	return out
}
