package main

import (
	"errors"
	"log"

	tsgrpc "github.com/tsunami/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// attachServer implements the TSAttach gRPC service. Each worker holds one
// bidirectional stream: it Registers, then sends Reports/Heartbeats up while the
// ocean pushes Commands down (via the worker's buffered send channel).
type attachServer struct {
	tsgrpc.UnimplementedTSAttachServer
	oc *Ocean
}

func (s *attachServer) Attach(stream grpc.BidiStreamingServer[tsgrpc.WorkerMessage, tsgrpc.OceanMessage]) error {
	// optional shared-token auth
	if tok := s.oc.viper.GetString("auth.token"); tok != "" {
		md, _ := metadata.FromIncomingContext(stream.Context())
		vals := md.Get("authorization")
		if len(vals) == 0 || vals[0] != "Bearer "+tok {
			return errors.New("unauthorized: missing or invalid attach token")
		}
	}

	// first message must be Register
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	reg := first.GetRegister()
	if reg == nil || reg.GetWorkerId() == "" {
		return errors.New("first message must be Register with a worker id")
	}

	ip := ""
	if p, ok := peer.FromContext(stream.Context()); ok && p.Addr != nil {
		ip = p.Addr.String()
	}

	w := &attachedWorker{
		id:      reg.GetWorkerId(),
		name:    reg.GetName(),
		ip:      ip,
		maxCap:  int(reg.GetMaxConcurrency()),
		send:    make(chan *tsgrpc.OceanMessage, 32),
		metrics: make(map[string]*tsgrpc.Metric),
	}

	if !s.oc.addWorker(w) {
		return errors.New("ocean is at max_connections")
	}
	log.Printf("attach: worker %s (%s) joined from %s, maxCap=%d", w.name, w.id, w.ip, w.maxCap)
	defer func() {
		s.oc.removeWorker(w.id)
		log.Printf("attach: worker %s (%s) left", w.name, w.id)
	}()

	// sender: drain the worker's command channel to the stream
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for msg := range w.send {
			if err := stream.Send(msg); err != nil {
				return
			}
		}
	}()

	// receiver: Reports / Heartbeats until the stream breaks
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if r := msg.GetReport(); r != nil && r.GetMetric() != nil {
			s.oc.onReport(w.id, r.GetMetric())
		}
	}
}

// addWorker registers an attached worker, enforcing max_connections.
func (oc *Ocean) addWorker(w *attachedWorker) bool {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	if oc.maxConnections > 0 && len(oc.workers) >= oc.maxConnections {
		return false
	}
	// replace any stale entry with the same id
	oc.workers[w.id] = w
	go oc.publishOcean() // refresh etcd counters (no-op if discovery off)
	return true
}

// removeWorker drops a worker on disconnect: free its capacity and detach it
// from any jobs (the job keeps running on survivors at reduced concurrency).
func (oc *Ocean) removeWorker(id string) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	w := oc.workers[id]
	if w == nil {
		return
	}
	close(w.send)
	delete(oc.workers, id)
	for _, j := range oc.jobs {
		delete(j.assignments, id)
	}
	go oc.publishOcean() // refresh etcd counters (no-op if discovery off)
}

// onReport stores the latest per-job metric from a worker.
func (oc *Ocean) onReport(id string, m *tsgrpc.Metric) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	if w := oc.workers[id]; w != nil {
		w.metrics[m.GetJob()] = m
	}
}

// command pushes a command to a worker's stream (non-blocking-ish via buffer).
func (w *attachedWorker) command(msg *tsgrpc.OceanMessage) {
	select {
	case w.send <- msg:
	default:
		// buffer full: drop (worker is unhealthy); it will be pruned on stream error
		log.Printf("attach: worker %s send buffer full, dropping command", w.id)
	}
}
