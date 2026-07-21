# Tsunami

Tsunami is a distributed HTTP/HTTPS **load-generation platform**, written in Go
(low memory footprint via [fasthttp](https://github.com/valyala/fasthttp)). You
drive tests from a web console or a simple HTTP API; the load itself is produced
by a fleet of workers that can scale out across many machines.

---

## Architecture

Three components plus a small registry:

1. **Web Control** — a static single-page console (start/stop tests, live charts,
   an animated topology view, per-worker status). Talks to an ocean's HTTP API.
2. **Ocean** (`master/`) — the **job distributor / scheduler**. It takes a test
   request, splits the requested concurrency across the workers attached to it
   (balanced, capped per worker), and **rejects** a request the fleet can't cover.
3. **Worker** (`worker/`, binary `tsunami`) — an **outbound-only agent** that
   generates the load. It dials an ocean and holds one bidirectional gRPC stream:
   the ocean pushes start/stop commands down, the worker streams metrics up.
4. **etcd** — a small **discovery registry**. Oceans register themselves (with
   their connection/capacity counts); workers read the list and pick an ocean.

```
   operator ─http─► web control ──► ocean(s) ──etcd── discovery + fleet view
                                      ▲
                     bidirectional gRPC stream (workers dial OUT — NAT-friendly)
                     ┌────────────────┴───────────────┐
                  worker ...(scale out; add oceans for more)... worker
   each: Register(maxConcurrency) → Report(metrics)/Heartbeat ↑ ; Command(start/stop) ↓
```

**Why this shape:** workers only ever make outbound connections and need to know
only an endpoint, so they run anywhere (including behind NAT). etcd stays tiny
(only oceans connect to it); you scale the data plane by adding oceans. If a
worker loses its ocean it **stops its load** (dead-man's switch) and re-attaches
to a surviving one; if a worker dies, its ocean frees the capacity.

---

## Quick start (Docker Compose)

Requires Docker + Docker Compose.

```bash
docker compose up --build -d          # etcd + ocean + worker + web control
```

Open the console at **http://localhost:8082**, or drive the API directly:

```bash
# start a test: 50 concurrent GETs against a target
curl -X POST http://localhost:8080/api/v1/start \
  -d '{"cmd":"start","conf":{"name":"t1","url":"https://example.com/","method":"GET","concurrence":50}}'

curl -X POST http://localhost:8080/api/v1/metrics -d '{"cmd":"metrics","conf":{"name":"t1"}}'
curl -X POST http://localhost:8080/api/v1/stop    -d '{"cmd":"stop","conf":{"name":"t1"}}'
```

Ports: web control `8082`, ocean HTTP API `8080`, ocean attach gRPC `8050`, etcd `2379`.

### HTTP API (ocean)
`POST /api/v1/{start,stop,metrics,info}` with a JSON body
`{"cmd":"…","conf":{ name, url, method, concurrence, host, port, path, protocol, body, headers }}`.
`start` returns `503` if the attached workers can't cover the requested concurrency.

---

## Web control

Served at `:8082` (nginx). Pages:

- **Dashboard** — live KPIs and hand-built SVG charts (throughput, latency,
  error rate, errors), an animated **ocean → worker → target** topology, and
  duplicate-name notifications.
- **Ocean** — manage the master connections the console talks to.
- **Workers** — a table of attached workers (name, IP, endpoint, capacity, status).

---

## Deploy (control plane + remote workers)

Run **etcd + ocean(s) + web control on one host**, and workers **anywhere** —
each worker needs only the etcd endpoint (and a shared token). Workers discover
the oceans, pick the one with the most free connection slots, and attach.

See **[DEPLOY.md](./DEPLOY.md)** for the full runbook (`docker-compose.droplet.yml`,
`deploy/worker.remote.yaml`, firewall/TLS/auth checklist). A one-command worker
installer is described in [`worker/WORKERS.md`](./worker/WORKERS.md).

---

## Build from source

Go modules, Go 1.23+ (module `github.com/tsunami`). gRPC stubs are generated with
`protoc` + `protoc-gen-go`/`protoc-gen-go-grpc`.

```bash
go build ./...                 # build all packages
go build -o ocean   ./master   # the ocean (scheduler) binary
go build -o tsunami ./worker   # the worker binary
make -C proto gen              # regenerate gRPC stubs from proto/services.proto
```

Each binary reads a YAML config selected with `--path <dir> --file <name>`
(see `deploy/` for examples). `ocean --version` / `tsunami --version` print the build tag.

---

## Client / OS tuning (Linux)

High-concurrency load needs raised limits on the machine(s) running workers:

- **Open files:** `ulimit -n 65536` (persist in `/etc/security/limits.conf`:
  `* soft nofile 65536` / `* hard nofile 65536`).
- **TCP** (in `/etc/sysctl.conf`, client side only):
  ```
  net.ipv4.tcp_fin_timeout = 5
  net.ipv4.tcp_tw_reuse    = 1
  ```

---

## Features

- **Distributed & horizontally scalable** — add workers for more load, add oceans
  to hold more workers.
- **Capacity-aware scheduling** — balanced split across workers; over-capacity
  requests are rejected, not silently truncated.
- **Workers anywhere** — outbound-only, NAT-friendly; discover oceans via etcd.
- **Resilient** — dead-man's switch stops orphaned load; workers auto re-attach.
- **Templated request params** — generate dynamic values per request in bodies/URLs.
- **Live web console** — charts, topology, and metrics in real time.
- **HTTP and HTTPS** targets.
```
