package main

import (
	"testing"
	"time"
)

const ms = int64(1_000_000) // nanoseconds per millisecond

// A worker's avg must be a true mean (sum/count), not the old (avg+new)/2 EMA.
func TestWorkerTrueMean(t *testing.T) {
	w := &Worker{}
	w.record(true, 1*ms)
	w.record(true, 3*ms)
	if got := w.GetAvgRes(); got < 1.99 || got > 2.01 {
		t.Fatalf("avg = %v ms, want 2", got)
	}
	if w.GetNumReq() != 2 || w.GetNumRes() != 2 || w.GetNumErr() != 0 {
		t.Fatalf("counts req=%d res=%d err=%d", w.GetNumReq(), w.GetNumRes(), w.GetNumErr())
	}
}

// Failures must not pollute latency (min/max/avg), only bump the error count.
func TestFailuresExcludedFromLatency(t *testing.T) {
	w := &Worker{}
	w.record(true, 2*ms)
	w.record(false, 60_000*ms) // a 60s timeout must NOT touch latency
	if got := w.GetAvgRes(); got < 1.99 || got > 2.01 {
		t.Fatalf("avg polluted by failure: %v ms", got)
	}
	if got := w.GetMaxRes(); got > 2.01 {
		t.Fatalf("max polluted by failure: %v ms", got)
	}
	if w.GetNumReq() != 2 || w.GetNumErr() != 1 || w.GetNumRes() != 1 {
		t.Fatalf("counts req=%d err=%d res=%d", w.GetNumReq(), w.GetNumErr(), w.GetNumRes())
	}
}

// serviceMetric must produce a request-weighted mean, not a mean of means:
// 100 reqs @1ms + 1 req @50ms -> ~1.49ms, NOT (1+50)/2 = 25.5ms.
func TestServiceMetricWeightedAvg(t *testing.T) {
	ts := &Tsunami{start: time.Now()}
	ts.conf.Name = "t"
	a := Worker{}
	for i := 0; i < 100; i++ {
		a.record(true, 1*ms)
	}
	b := Worker{}
	b.record(true, 50*ms)
	ts.workers = []Worker{a, b}

	m := serviceMetric(ts)
	if m.GetAvg() > 2.0 {
		t.Fatalf("avg not request-weighted: %v ms (mean-of-means bug)", m.GetAvg())
	}
	if m.GetRequestCount() != 101 {
		t.Fatalf("request count = %d, want 101", m.GetRequestCount())
	}
}

// Concurrent writer + reader must be data-race free (run with -race).
func TestStatConcurrentRaceFree(t *testing.T) {
	w := &Worker{}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200000; i++ {
			w.record(i%7 == 0, int64(i%5+1)*ms)
		}
		close(done)
	}()
	for {
		_ = w.GetAvgRes()
		_ = w.GetNumReq()
		_ = w.GetMaxRes()
		_ = w.GetMinRes()
		select {
		case <-done:
			return
		default:
		}
	}
}
