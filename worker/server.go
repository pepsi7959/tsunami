package main

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	tshttp "github.com/tsunami/libs"
	tsgrpc "github.com/tsunami/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// OceanClient keeps the worker's single bidirectional stream to an ocean:
// it dials out, registers, runs load on START commands, stops on STOP, reports
// metrics up the stream, and — if the stream drops — stops all load (dead-man's
// switch) before re-attaching.
type OceanClient struct {
	ctrl     *TSControl
	workerID string
	name     string
	maxConc  int32
	report   time.Duration // metrics report interval
	token    string        // optional attach auth token

	mu sync.Mutex // guards ctrl.services
}

func methodName(m tsgrpc.HTTPMethod) string {
	switch m {
	case tsgrpc.HTTPMethod_POST:
		return "POST"
	case tsgrpc.HTTPMethod_PUT:
		return "PUT"
	case tsgrpc.HTTPMethod_DELETE:
		return "DELETE"
	case tsgrpc.HTTPMethod_UPDATE:
		return "UPDATE"
	default:
		return "GET"
	}
}

// headersFromParams converts the proto header list to a map.
func headersFromParams(hs []*tsgrpc.HTTPHeader) map[string]string {
	m := make(map[string]string, len(hs))
	for _, h := range hs {
		m[h.GetKey()] = h.GetValue()
	}
	return m
}

// defaultHeaders sets a sensible Content-Type for body-carrying methods when the
// caller didn't provide one (e.g. POST/PUT/PATCH default to JSON).
func defaultHeaders(method string, headers map[string]string) map[string]string {
	if headers == nil {
		headers = map[string]string{}
	}
	switch method {
	case "POST", "PUT", "PATCH":
		has := false
		for k := range headers {
			if strings.EqualFold(k, "Content-Type") {
				has = true
				break
			}
		}
		if !has {
			headers["Content-Type"] = "application/json; charset=utf-8"
		}
	}
	return headers
}

// Run keeps a worker attached to an ocean. On every (re)connect it calls
// resolve() to pick an ocean endpoint (a static one, or the least-loaded ocean
// from etcd discovery), reconnecting on failure.
func (c *OceanClient) Run(resolve func() string) {
	for {
		endpoint := resolve()
		if endpoint == "" {
			log.Printf("attach: no ocean available; retrying in 3s")
			time.Sleep(3 * time.Second)
			continue
		}
		if err := c.attachOnce(endpoint); err != nil {
			log.Printf("attach: stream to %s ended: %v", endpoint, err)
		}
		// dead-man's switch: whenever the stream is gone, stop all load so a
		// worker can never keep hammering a target without a controlling ocean
		c.stopAll()
		time.Sleep(3 * time.Second)
	}
}

func (c *OceanClient) attachOnce(endpoint string) error {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx := context.Background()
	if c.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
	}
	stream, err := tsgrpc.NewTSAttachClient(conn).Attach(ctx)
	if err != nil {
		return err
	}
	log.Printf("attach: connected to ocean %s as %s (%s), maxConcurrency=%d", endpoint, c.name, c.workerID, c.maxConc)

	// 1) Register
	if err := stream.Send(&tsgrpc.WorkerMessage{Msg: &tsgrpc.WorkerMessage_Register{
		Register: &tsgrpc.Register{WorkerId: c.workerID, Name: c.name, MaxConcurrency: c.maxConc},
	}}); err != nil {
		return err
	}

	// 2) periodic Report + Heartbeat
	done := make(chan struct{})
	defer close(done)
	go c.reportLoop(stream, done)

	// 3) receive Commands until the stream breaks
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if cmd := msg.GetCommand(); cmd != nil {
			c.handleCommand(cmd)
		}
	}
}

func (c *OceanClient) reportLoop(stream grpc.BidiStreamingClient[tsgrpc.WorkerMessage, tsgrpc.OceanMessage], done <-chan struct{}) {
	t := time.NewTicker(c.report)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			c.mu.Lock()
			reports := make([]*tsgrpc.Report, 0, len(c.ctrl.services))
			for _, ts := range c.ctrl.services {
				rep := &tsgrpc.Report{Metric: serviceMetric(ts)}
				if ts.verbose && ts.sample != nil {
					rep.Sample = ts.sample.get() // last request/response for this job
				}
				reports = append(reports, rep)
			}
			c.mu.Unlock()
			for _, rep := range reports {
				if stream.Send(&tsgrpc.WorkerMessage{Msg: &tsgrpc.WorkerMessage_Report{Report: rep}}) != nil {
					return
				}
			}
			if stream.Send(&tsgrpc.WorkerMessage{Msg: &tsgrpc.WorkerMessage_Heartbeat{Heartbeat: &tsgrpc.Heartbeat{}}}) != nil {
				return
			}
		}
	}
}

func (c *OceanClient) handleCommand(cmd *tsgrpc.Command) {
	name := cmd.GetJob()
	switch cmd.GetAction() {
	case tsgrpc.Action_START:
		p := cmd.GetParams()
		method := methodName(p.GetMethod())
		conf := tshttp.Conf{
			Name:        name,
			URL:         p.GetUrl(),
			Protocol:    p.GetProtocol(),
			Host:        p.GetHost(),
			Port:        p.GetPort(),
			Path:        p.GetPath(),
			Method:      method,
			Headers:     defaultHeaders(method, headersFromParams(p.GetHeader())),
			Body:        p.GetBody(),
			Concurrence: int(p.GetConcurrency()),
		}
		c.startService(name, conf, p.GetVerbose())
	case tsgrpc.Action_STOP:
		c.stopService(name)
	}
}

// startService launches a load test (reuses the existing Tsunami engine).
func (c *OceanClient) startService(name string, conf tshttp.Conf, verbose bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.ctrl.services[name]; existing != nil {
		existing.Stop()
		delete(c.ctrl.services, name)
	}
	log.Printf("start: %s -> %s (%d concurrency, verbose=%v)", name, conf.URL, conf.Concurrence, verbose)
	app := &Tsunami{done: false, conf: conf, duration: 3600, refresh: 2, enableReport: false, verbose: verbose}
	app.Init(100000)
	c.ctrl.services[name] = app
	app.Run()
	go app.GenLoad()
	go app.GenLoad()
}

func (c *OceanClient) stopService(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts := c.ctrl.services[name]; ts != nil {
		log.Printf("stop: %s", name)
		ts.Stop()
		delete(c.ctrl.services, name)
	}
}

func (c *OceanClient) stopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, ts := range c.ctrl.services {
		log.Printf("stop (stream lost): %s", name)
		ts.Stop()
		delete(c.ctrl.services, name)
	}
}

// serviceMetric snapshots a running service's live stats into a proto Metric.
func serviceMetric(ts *Tsunami) *tsgrpc.Metric {
	var avg, min, max float64
	var numRes, numErr int
	for _, w := range ts.workers {
		numRes += w.GetNumRes()
		numErr += w.GetNumErr()
		avg += w.GetAvgRes()
		if w.GetMaxRes() > max {
			max = w.GetMaxRes()
		}
		if min == 0.0 || w.GetMinRes() < min {
			min = w.GetMinRes()
		}
	}
	n := len(ts.workers)
	if n > 0 {
		avg = avg / float64(n)
	}
	elapsed := time.Since(ts.start).Seconds()
	rps := 0.0
	if elapsed > 0 {
		rps = float64(numRes) / elapsed
	}
	return &tsgrpc.Metric{
		Job:          ts.conf.Name,
		WorkerCount:  int32(n),
		RequestCount: int64(numRes),
		ErrorCount:   int64(numErr),
		Avg:          avg,
		Min:          min,
		Max:          max,
		Rps:          rps,
		ElapsedTime:  elapsed,
	}
}
