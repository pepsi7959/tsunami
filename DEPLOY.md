# Deploying tsunami — control plane on one droplet, workers anywhere

Architecture: **etcd + ocean(s) + web control on one droplet**; **workers run
anywhere** and only need the etcd endpoint. Workers dial *out* (NAT-friendly):
they read the ocean list from etcd, pick the ocean with the most free connection
slots, and hold one bidirectional gRPC stream to it. The ocean pushes start/stop
commands down and receives metrics up — it never dials the worker.

```
  operator browser ──► web control :8082  +  ocean API :8080/:8081   (PUBLIC — protect!)
        droplet:  etcd :2379   ocean1 :8050(grpc)   ocean2 :8060(grpc)
                     ▲ discover           ▲ attach stream (workers dial out)
   worker (anywhere) ┘────────────────────┘   knows only: etcd :2379  + shared token
```

## 1. Control-plane droplet
1. Edit `deploy/droplet/ocean1.yaml` and `ocean2.yaml`:
   - `advertise.grpc` → your droplet's **public** host/IP + that ocean's published
     grpc port (`<host>:8050` for ocean1, `<host>:8060` for ocean2). Workers dial this.
   - `auth.token` → a strong shared secret (same in both oceans and all workers).
2. `docker compose -f docker-compose.droplet.yml up --build -d`
3. Check: `curl -s http://<host>:8080/api/v1/info -d '{}'` → `workers: 0` initially.

Scale the control plane by adding more `oceanN` services (unique id + published
ports); workers auto-balance across them by free connection slots.

## 2. Workers (any machine, anywhere)
Fill `deploy/worker.remote.yaml`:
- `id` → unique per worker
- `concurence` → this worker's max load capacity
- `registry.endpoints` → `<droplet-public-host>:2379`
- `auth.token` → the shared token

Run the worker (Docker):
```
docker run -d --name tsunami-worker \
  -v $PWD/deploy/worker.remote.yaml:/etc/tsunami/config.yaml:ro \
  <your-tsunami-image>   # built from `docker build --target tsunami`
```
Within ~seconds the ocean's `/api/v1/info` shows the worker; the topology shows
it attached under the ocean it picked. Start a test from the web control or:
```
curl -X POST http://<host>:8080/api/v1/start \
  -d '{"cmd":"start","conf":{"name":"t1","url":"https://target/","method":"GET","concurrence":500}}'
```
Insufficient capacity ⇒ `503` (rejected, nothing partial starts).

## 3. Security (do this before any public exposure)
- **Shared token** (implemented): `auth.token` gates the attach stream — a worker
  can't attach without it. Set it in the oceans and all workers.
- **etcd (2379) is public here** so remote workers can reach it. It has no auth by
  default — restrict it with a cloud firewall to known worker IPs, and terminate
  **TLS** in front of it (or enable etcd client-cert auth) before real exposure.
- **Ocean attach ports (8050/8060)** are public — firewall to worker IPs; front
  with TLS in production.
- **The client API (8080/8081) + web control (8082) can command the whole fleet**
  (an open load cannon). Put them behind a reverse proxy with auth (basic/OAuth)
  or a VPN / IP allowlist — never expose them openly.

## 4. Failure behavior
- **Ocean dies:** its workers detect the dropped stream, **stop their load**
  (dead-man's switch), then re-discover and re-attach to a surviving ocean idle.
  Re-submit the interrupted test. (Reclaim is gated by the ocean's ~`lease.ttl`.)
- **Worker dies:** its ocean drops it and frees capacity; a job that spanned it
  continues on the survivors at reduced concurrency.
- **etcd down:** running streams keep working; new workers can't discover until
  it's back. (Single etcd is a SPOF — run a 3-node cluster for HA.)
