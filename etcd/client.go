package tsetcd

import (
	"context"
	"log"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// KV is a single key/value returned from a prefix read.
type KV struct {
	Key   string
	Value []byte
}

// Client is a long-lived etcd client used for the tsunami registry and for
// active-active coordination (leases, watches, per-worker mutexes). Unlike the
// old wrapper it keeps a single connection open and returns errors instead of
// calling log.Fatalf, so transient etcd blips don't kill the process.
type Client struct {
	cli     *clientv3.Client
	sess    *concurrency.Session
	timeout time.Duration
}

// NewClient opens one etcd connection and a concurrency session (for mutexes).
// sessTTL is the session/lease TTL in seconds used by mutexes created from it.
func NewClient(endpoints []string, dialTimeout, requestTimeout time.Duration, sessTTL int) (*Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		return nil, err
	}
	sess, err := concurrency.NewSession(cli, concurrency.WithTTL(sessTTL))
	if err != nil {
		cli.Close()
		return nil, err
	}
	return &Client{cli: cli, sess: sess, timeout: requestTimeout}, nil
}

func (c *Client) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), c.timeout)
}

// Raw exposes the underlying client for advanced callers (watch, lease).
func (c *Client) Raw() *clientv3.Client { return c.cli }

// Session exposes the concurrency session for building mutexes.
func (c *Client) Session() *concurrency.Session { return c.sess }

// Put writes a key. Pass opts (e.g. clientv3.WithLease) to bind a lease.
func (c *Client) Put(key, val string, opts ...clientv3.OpOption) error {
	ctx, cancel := c.ctx()
	defer cancel()
	_, err := c.cli.Put(ctx, key, val, opts...)
	return err
}

// Get reads a single key; returns (nil, nil) when absent.
func (c *Client) Get(key string) ([]byte, error) {
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.cli.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, nil
	}
	return resp.Kvs[0].Value, nil
}

// GetPrefix reads all keys under a prefix.
func (c *Client) GetPrefix(prefix string) ([]KV, error) {
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.cli.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]KV, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		out = append(out, KV{Key: string(kv.Key), Value: kv.Value})
	}
	return out, nil
}

// Delete removes a single key.
func (c *Client) Delete(key string) error {
	ctx, cancel := c.ctx()
	defer cancel()
	_, err := c.cli.Delete(ctx, key)
	return err
}

// DeletePrefix removes all keys under a prefix.
func (c *Client) DeletePrefix(prefix string) error {
	ctx, cancel := c.ctx()
	defer cancel()
	_, err := c.cli.Delete(ctx, prefix, clientv3.WithPrefix())
	return err
}

// Grant creates a lease with the given TTL (seconds).
func (c *Client) Grant(ttl int64) (clientv3.LeaseID, error) {
	ctx, cancel := c.ctx()
	defer cancel()
	resp, err := c.cli.Grant(ctx, ttl)
	if err != nil {
		return 0, err
	}
	return resp.ID, nil
}

// KeepAlive keeps a lease alive for the life of the process. It drains the
// keepalive channel in a goroutine and logs if the stream closes (lease lost).
func (c *Client) KeepAlive(lease clientv3.LeaseID) error {
	ch, err := c.cli.KeepAlive(context.Background(), lease)
	if err != nil {
		return err
	}
	go func() {
		for range ch {
			// drain; keepalive acks arrive here
		}
		log.Printf("etcd: keepalive channel for lease %x closed (lease may be lost)", lease)
	}()
	return nil
}

// Revoke releases a lease (and all keys bound to it).
func (c *Client) Revoke(lease clientv3.LeaseID) error {
	ctx, cancel := c.ctx()
	defer cancel()
	_, err := c.cli.Revoke(ctx, lease)
	return err
}

// WatchPrefix atomically seeds and watches a prefix: it first emits the current
// keys as synthetic PUT events, then watches from that snapshot's revision + 1,
// so there is NO gap between the seed and the watch (a key written between a
// separate list and a naive Watch would otherwise be missed forever). On
// compaction/cancel it re-watches from the last seen revision. The channel is
// closed when ctx is cancelled.
func (c *Client) WatchPrefix(ctx context.Context, prefix string) clientv3.WatchChan {
	out := make(chan clientv3.WatchResponse)
	go func() {
		defer close(out)

		var startRev int64
		if gresp, err := c.cli.Get(ctx, prefix, clientv3.WithPrefix()); err == nil {
			evs := make([]*clientv3.Event, 0, len(gresp.Kvs))
			for _, kv := range gresp.Kvs {
				evs = append(evs, &clientv3.Event{Type: clientv3.EventTypePut, Kv: kv})
			}
			select {
			case out <- clientv3.WatchResponse{Events: evs}:
			case <-ctx.Done():
				return
			}
			startRev = gresp.Header.Revision + 1
		}

		for {
			opts := []clientv3.OpOption{clientv3.WithPrefix()}
			if startRev > 0 {
				opts = append(opts, clientv3.WithRev(startRev))
			}
			wch := c.cli.Watch(ctx, prefix, opts...)
			for resp := range wch {
				if resp.Canceled {
					break // re-establish below
				}
				select {
				case out <- resp:
				case <-ctx.Done():
					return
				}
				if resp.Header.Revision > 0 {
					startRev = resp.Header.Revision + 1
				}
			}
			if ctx.Err() != nil {
				return
			}
			time.Sleep(500 * time.Millisecond) // brief backoff before re-watch
		}
	}()
	return out
}

// WithLock acquires a distributed mutex on key, runs fn, then releases it.
// Used to serialize per-worker capacity reservations across masters so two
// masters can't both reserve the same free quota. ctx bounds the Lock wait.
func (c *Client) WithLock(ctx context.Context, key string, fn func() error) error {
	mu := concurrency.NewMutex(c.sess, key)
	if err := mu.Lock(ctx); err != nil {
		return err
	}
	defer mu.Unlock(context.Background())
	return fn()
}

// Close tears down the session and connection.
func (c *Client) Close() error {
	if c.sess != nil {
		c.sess.Close()
	}
	return c.cli.Close()
}
