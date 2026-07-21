# Tsunami Worker Node (`tsunami`)

The **worker node** — the binary is called `tsunami` — is the component that actually
generates load. The master ([Ocean](../master)) never sends traffic to your target
itself; it discovers workers through etcd and instructs them, over gRPC, to start,
stop, and report on load tests. Each worker can run many independent load tests
(called *services*) at the same time, each with its own pool of concurrent clients.

```
                 ┌──────────────┐        gRPC          ┌───────────────────────────┐
   user ───────► │  Ocean       │ ───────────────────► │  tsunami (worker)         │
  (Web/HTTP)     │  (master)    │  Start/Stop/Metrics   │                           │
                 └──────┬───────┘                       │   ┌───────────────────┐   │
                        │ reads worker list             │   │ Tsunami service A │   │
                        │                               │   │  ├ worker[0..N]   │───┼──► target
                        ▼                               │   │  ├ jobs channel   │   │
                 ┌──────────────┐   register (Put)      │   │  └ metrics API    │   │
                 │  etcd        │ ◄─────────────────────┤   └───────────────────┘   │
                 │  (registry)  │   key = prefix+id     │   ┌───────────────────┐   │
                 └──────────────┘                       │   │ Tsunami service B │   │
                                                        │   └───────────────────┘   │
                                                        └───────────────────────────┘
```

---

## 0. One-command install (agent-style)

Install the worker as a persistent systemd service on any Linux (amd64/arm64) machine
with a single command. It downloads a prebuilt binary from GitHub Releases, writes a
cluster-mode config (auto-generating a unique `id` and detecting this host's routable IP),
and starts the service:

```bash
curl -sSL https://raw.githubusercontent.com/pepsi7959/tsunami/master/install.sh \
  | sudo bash -s -- --etcd 10.0.0.5:2379
```

Common flags (all also settable via `TSUNAMI_*` env; see `install.sh --help`):

| Flag | Default | Meaning |
|------|---------|---------|
| `--etcd LIST` | *(required)* | Comma-separated etcd endpoints to register with |
| `--advertise IP` | auto-detected | Address the master dials for gRPC (override on NAT/multi-homed hosts) |
| `--name NAME` | hostname | Worker name |
| `--version vX.Y.Z` | latest release | Release to install |

The worker binds **gRPC `:8050`** (dialed by the master), **admin HTTP `:8090`**, and a
per-test metrics server on **`:8091`** — open `8050`/`8090` to the master through any firewall.

Verify and operate:
```bash
systemctl status tsunami-worker
journalctl -u tsunami-worker -f
etcdctl --endpoints=10.0.0.5:2379 get --prefix /tsunami/workers/   # confirm registration
sudo bash install.sh --uninstall                                   # remove (add --purge for config)
```

Re-running the installer is idempotent — it reuses the existing `id` and restarts the service.
Config keys are documented in §4; for manual/Docker installs see §3.

---

## 1. What's inside — the source files

| File | Responsibility |
|------|----------------|
| `tsunami.go` | Entry point (`main`). Reads config, starts the gRPC server, registers the worker in etcd (cluster mode), and starts the admin HTTP API. Defines `TSControl` (the worker daemon) and `Tsunami` (one load test), plus `StartApp` which wires a load test together. |
| `server.go` | The **gRPC** server (`GRPCServer`). Implements the `TSControl` service the master calls: `Start`, `Stop`, `Restart`, `GetMetrics`, `Register`. |
| `services.go` | The **admin HTTP** handlers (`CmdStart`, `CmdStop`, `CmdMetrics`) and the per-service `GetMetrics` HTTP handler. A REST-style alternative to the gRPC control path. |
| `worker.go` | A single `Worker`: pulls a job off the channel, fires one HTTP request with `fasthttp`, and records latency/error stats (`Stat`). |
| `job.go` | The `Job` unit of work (currently an empty struct — one job = "send one request"). |
| `shell.go` | The interactive `Shell` — lets an operator type commands into a running worker's stdin (`+`, `help`, `q`, `refresh`, `report`). |
| `help.go` | Legacy stand-alone CLI flag parser (`ReadConf`, `Usage`). Not used by the daemon `main`; kept for the old single-shot mode. |
| `json.go` | Small helper for hand-building JSON HTTP responses (`CreateJSONRes`). |

Shared types live in [`../libs`](../libs) (`Conf`, `Request`, `Metric`, the HTTP `App`)
and [`../registry`](../registry) (`Conf` used for etcd registration). The gRPC contract
is in [`../proto/services.proto`](../proto/services.proto).

---

## 2. How it works

### 2.1 Two nested concepts

- **`TSControl`** — the long-lived *worker daemon*. One per process. Holds the gRPC
  server, the etcd config, and a map of running load tests: `services map[string]*Tsunami`.
- **`Tsunami`** — one *load test*, keyed by name. Holds its own `fasthttp.HostClient`,
  a `jobs` channel, a slice of `Worker`s, a monitor, an interactive shell, and a
  dedicated metrics HTTP server.

### 2.2 Startup sequence (`main` in `tsunami.go`)

1. `readConf()` loads the YAML config via [viper](https://github.com/spf13/viper).
2. `New()` builds the `TSControl` daemon from that config.
3. The gRPC server starts on `endpoints.grpc` (`go StartServer()`).
4. **If `mode: cluster`** → the worker marshals its `registry.Conf`
   (`ID`, `Name`, `Endpoint`, `MaxConcurrences`) to JSON and `Put`s it into etcd under
   the key `registry.client_config_key + id`. This is how Ocean finds the worker.
   In `standalone` mode this step is skipped.
5. The admin HTTP API starts on `endpoints.http` and **blocks** (`api.Run()`), keeping
   the process alive.

### 2.3 Starting a load test (`StartApp`)

When the master calls `Start` (gRPC) or a client hits `POST /api/v1/admin/start` (HTTP),
the worker runs `StartApp`, which:

1. Builds a `Tsunami` with defaults `duration=3600s`, `refresh=2s`, `enableReport=true`.
2. `Init(100000)` — creates the job channel (buffer 100000) and **one shared
   `fasthttp.HostClient`**, then spawns `Concurrence` `Worker`s. HTTPS is auto-detected
   from the URL scheme (or `protocol: https`).
3. Registers the test in `ctrl.services[name]`.
4. `Run()` — launches every worker goroutine; each loops on the job channel.
5. Two `GenLoad()` goroutines continuously push empty `Job`s into the channel → workers
   pick them up and fire requests as fast as the target responds.
6. Starts the interactive shell, the monitor, and a per-service **metrics HTTP server on
   `:8091`** (`GET /api/v1/metrics`).
7. After `duration` seconds it calls `Stop()`, which flips a shared `done` flag that
   makes all workers exit their loops.

### 2.4 The request loop (`worker.go`)

Each `Worker.do()`:
- Builds a `fasthttp` request (method, headers, body, URL).
- Sends it and measures wall-clock latency in nanoseconds.
- Counts an **error** if the transport fails **or** the status code is not `200`.
- Updates running min / max / average latency and the response count.

---

## 3. Installation

### Prerequisites
- **Go 1.26+** (see `go.mod`) to build from source.
- An **etcd** endpoint if you run in `cluster` mode (the default).
- OS tuning on the load-generating host — raise open-file limits and TCP reuse
  (see the [top-level README](../README.md#prerequisite-for-a-client): `ulimit -n 65536`,
  `net.ipv4.tcp_fin_timeout`, `tcp_tw_reuse`, etc.).

### Option A — build from source
```bash
# from the repository root
go build -o tsunami ./worker

# run it with a config file
./tsunami --path ./conf/tsunami --file config-1.yaml
```

### Option B — Docker
The repo's [`Dockerfile`](../Dockerfile) has a `tsunami` build target:
```bash
docker build --target tsunami -t tsunami-worker .
docker run --rm -p 8090:8090 -p 8091:8091 tsunami-worker
```
Or bring up the whole stack (etcd + worker + master + web UI) with compose. Per the
project rule, after any rebuild recreate the containers:
```bash
docker compose build
docker compose up -d --force-recreate
```
The compose file exposes the worker's admin HTTP (`8090`) and metrics (`8091`); the gRPC
port (`8050`) stays on the internal network for the master to reach.

### Option C — process manager (pm2)
See [`scripts/pm2.start.sh`](../scripts/pm2.start.sh) for running multiple workers on one
host, each with its own config file:
```bash
pm2 start /usr/local/bin/tsunami --name tsunami-1 --namespace tsunami \
    -- --path /etc/tsunami/conf/tsunami --file config-1.yaml
```

---

## 4. Configuration

The daemon is configured with a **YAML file** selected by two CLI flags:

| Flag | Default | Meaning |
|------|---------|---------|
| `--path` | `.` | Directory to search for the config file (`./` is always searched too). |
| `--file` | `config.yaml` | Config file name (the extension is stripped; type is always YAML). |

### Config keys

```yaml
name: worker-1                       # human-readable worker name (default: worker-0)
id: 8cd50597-39df-473f-af2e-...      # unique worker id; used as the etcd key suffix
concurence: 300                      # default client concurrency (note: spelled "concurence")
mode: cluster                        # "cluster" (register in etcd) or "standalone"

endpoints:
  grpc: 127.0.0.1:8050               # gRPC control port the master dials
  http: 127.0.0.1:8090               # admin HTTP API port

registry:                            # only used when mode: cluster
  request_timeout: 2                 # etcd request timeout (seconds)
  dial_timeout: 2                    # etcd dial timeout (seconds)
  client_config_key: tsunami_config_client_   # key prefix; final key = prefix + id
  endpoints:                         # etcd cluster endpoints
    - localhost:2379
    - localhost:22379
    - localhost:32379
```

**Defaults** (applied by `readConf()` when a key is missing): `name=worker-0`,
`concurence=10`, `mode=cluster`, `endpoints.grpc=127.0.0.1:8050`,
`endpoints.http=127.0.0.1:8090`, `client_config_key=tsunami_config_client_`, and the
three local etcd endpoints above.

### Cluster vs standalone

| | `cluster` | `standalone` |
|-|-----------|--------------|
| Registers in etcd | ✅ (so Ocean can discover it) | ❌ |
| Needs etcd running | ✅ | ❌ |
| How you drive it | via the master (Ocean) | directly via the admin HTTP API |

> **Networking note (Docker):** `endpoints.grpc` is stored in etcd and dialed *verbatim*
> by the master. In a compose network it must be the service hostname
> (e.g. `tsunami:8050`), **not** `127.0.0.1`. Bind admin HTTP to `0.0.0.0:8090` so it is
> reachable from outside the container. See [`deploy/tsunami.yaml`](../deploy/tsunami.yaml).

### Per-test parameters
The load-test parameters (target URL/host, method, headers, body, concurrency) are **not**
in the config file — they come per request in the `Start` call from the master or the
admin HTTP API (see §5).

---

## 5. Control APIs

### 5.1 gRPC (master → worker) — port `endpoints.grpc`
Service `TSControl` (`proto/services.proto`):

| RPC | Effect |
|-----|--------|
| `Start(Request)` | Start a load test named `params.name`. If one with that name already exists it is **replaced**. |
| `Stop(Request)` | Stop and remove the named test. |
| `GetMetrics(Request)` | Return aggregated metrics for the named test as a JSON `Metric`. |
| `Restart(Request)` | Not implemented (no-op). |
| `Register(RegisterRequest)` | Not implemented (no-op). |

### 5.2 Admin HTTP (direct control) — port `endpoints.http`
Base path `/api/v1/admin`. Body is a JSON `Request` (`libs/request.go`):

```jsonc
// POST /api/v1/admin/start
{
  "cmd": "start",
  "conf": {
    "name": "test-a",
    "url": "https://example.com/health",   // or host/port/path/protocol
    "method": "GET",                         // defaults to GET if empty
    "headers": { "Authorization": "Bearer …" },
    "body": "",
    "concurrence": 300
  }
}
```

| Endpoint | Purpose |
|----------|---------|
| `POST /api/v1/admin/start` | Start a load test (spawns `StartApp`). |
| `POST /api/v1/admin/stop` | Stop the named test (`{"conf":{"name":"test-a"}}`). |
| `POST /api/v1/admin/metrics` | Aggregated metrics; requires `"cmd":"metrics"`. |
| `GET  /help` | Health/aliveness (`{}`). |

### 5.3 Per-test metrics HTTP — port `:8091`
Started per load test: `GET /api/v1/metrics` returns
`name`, `workers_count`, `errors_count`, `avg`, `min`, `max`, `elaped_time`,
`requests_count`, `rps`.

### Ports summary

| Port | Source | Protocol | Purpose |
|------|--------|----------|---------|
| `endpoints.grpc` (default 8050) | config | gRPC | Master → worker control |
| `endpoints.http` (default 8090) | config | HTTP | Admin control API |
| `8091` | hard-coded | HTTP | Per-test metrics |

---

## 6. Interactive shell

If the worker runs in a foreground terminal, each running test attaches a shell to stdin:

| Command | Action |
|---------|--------|
| *(Enter on empty line)* | Toggle the help menu (pauses the live report while shown). |
| `+` | Add one more concurrent worker to the running test. |
| `refresh <seconds>` | Change the live-report refresh interval. |
| `report <true\|false>` | Enable/disable the live metrics printout. |
| `help` | Print the command list. |
| `q` | Stop and exit the process (`os.Exit(0)`). |

---

## 7. Known limitations

Documented so operators aren't surprised — these are current behaviours, not
recommendations:

- **One metrics port per process.** The per-test metrics server is hard-coded to `:8091`,
  so running **more than one test at a time on a single worker** means the second test's
  metrics server can't bind (its `Run()` fails silently in a goroutine). Aggregated
  metrics are still available via the admin/gRPC `metrics` calls.
- **gRPC `Start` response body.** The `Start` RPC currently returns an empty JSON object
  in `data` (the response struct's fields are unexported), so callers should not rely on
  the returned `url`/`name`.
- **`GetMetrics` with zero workers** divides by the worker count; a test with `concurrence: 0`
  yields `NaN` for the average.
- **Success = HTTP 200 only.** Any non-200 status (including 2xx/3xx) is counted as an error.
- `Restart` and `Register` RPCs are stubs.

---

## 8. Quick start (standalone, no etcd)

```bash
# 1. build
go build -o tsunami ./worker

# 2. minimal standalone config
cat > /tmp/worker.yaml <<'EOF'
name: worker-local
id: local-1
concurence: 50
mode: standalone
endpoints:
  grpc: 127.0.0.1:8050
  http: 127.0.0.1:8090
EOF

# 3. run
./tsunami --path /tmp --file worker.yaml

# 4. in another shell, start a load test against a target
curl -s -X POST http://127.0.0.1:8090/api/v1/admin/start \
  -H 'Content-Type: application/json' \
  -d '{"cmd":"start","conf":{"name":"t1","url":"http://localhost:8081/","method":"GET","concurrence":50}}'

# 5. read metrics
curl -s http://127.0.0.1:8091/api/v1/metrics

# 6. stop
curl -s -X POST http://127.0.0.1:8090/api/v1/admin/stop \
  -d '{"cmd":"stop","conf":{"name":"t1"}}'
```
