#!/usr/bin/env bash
#
# One-command installer for the Tsunami CONTROL PLANE (etcd + ocean + web control)
# on a single Linux host. Docker-less and downloads prebuilt binaries, so it fits
# a small box (e.g. a 512 MB droplet). Idempotent: re-run any time to UPDATE to
# the newest release — it keeps your token and skips the etcd/nginx reinstall.
#
#   curl -fsSL https://raw.githubusercontent.com/pepsi7959/tsunami/streaming-control-plane/install-control-plane.sh \
#     | sudo PUBLIC_IP=165.22.103.216 TOKEN=<shared-secret> bash
#
# Env overrides: TSUNAMI_VERSION (release tag), MAX_CONNECTIONS, ETCD_VER.
set -euo pipefail

REPO="pepsi7959/tsunami"
ETCD_VER="${ETCD_VER:-v3.5.16}"
VERSION="${TSUNAMI_VERSION:-}"                 # release tag; default = newest (incl prerelease)
PUBLIC_IP="${PUBLIC_IP:-}"
TOKEN="${TOKEN:-}"
MAX_CONNECTIONS="${MAX_CONNECTIONS:-2000}"
ADMIN_USER="${ADMIN_USER:-admin}"   # web-control login (seeded first run only)
ADMIN_PASS="${ADMIN_PASS:-}"        # empty => ocean seeds default 'admin' (with a warning)
BIN=/usr/local/bin
CFG=/etc/tsunami
WWW=/var/www/tsunami
DATA=/var/lib/tsunami                # SQLite auth db (users + sessions)

log(){ printf '\033[1;32m==>\033[0m %s\n' "$*"; }
err(){ printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ]            || err "run as root (sudo)"
[ "$(uname -s)" = "Linux" ]    || err "Linux only (use docker-compose.droplet.yml on bigger/other hosts)"
command -v systemctl >/dev/null 2>&1 || err "systemd is required"
command -v curl >/dev/null 2>&1      || err "curl is required"

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) err "unsupported arch $(uname -m)" ;;
esac

# public IP: env, else auto-detect
[ -z "$PUBLIC_IP" ] && PUBLIC_IP="$(curl -fsSL https://ifconfig.me 2>/dev/null || true)"
[ -z "$PUBLIC_IP" ] && err "set PUBLIC_IP=<this host's public ip>"

# token: env > keep existing config's token > required on first install
if [ -z "$TOKEN" ] && [ -f "$CFG/config.yaml" ]; then
  TOKEN="$(sed -n 's/.*token:[[:space:]]*"\{0,1\}\([^"[:space:]]*\).*/\1/p' "$CFG/config.yaml" | head -n1 || true)"
fi
[ -z "$TOKEN" ] && err "set TOKEN=<shared worker token> on first install"

# resolve newest release (list is newest-first; includes prereleases). Download
# the JSON fully first so no early-closing pipe trips pipefail.
if [ -z "$VERSION" ]; then
  rels="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" 2>/dev/null || true)"
  VERSION="$(printf '%s\n' "$rels" | grep '"tag_name":' | head -n1 | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/' || true)"
  [ -z "$VERSION" ] && VERSION="v0.2.0"   # fallback if the API is unavailable/rate-limited
fi
log "control plane ${VERSION} (${ARCH}); advertising ${PUBLIC_IP}"

mkdir -p "$CFG" "$WWW" "$DATA" /var/lib/etcd

# ---- ocean binary (from the release tarball) ----
tmp="$(mktemp -d)"
asset="tsunami_${VERSION}_linux_${ARCH}.tar.gz"
log "downloading ${asset}"
curl -fsSL "https://github.com/${REPO}/releases/download/${VERSION}/${asset}" -o "$tmp/t.tgz" \
  || err "download failed: ${asset}"
tar -C "$tmp" -xzf "$tmp/t.tgz" ocean || err "ocean binary not found in ${asset}"
install "$tmp/ocean" "$BIN/ocean"
rm -rf "$tmp"

# ---- etcd (install once) ----
if ! command -v etcd >/dev/null 2>&1; then
  log "installing etcd ${ETCD_VER}"
  t2="$(mktemp -d)"; ( cd "$t2"
    curl -fsSL -o e.tgz "https://github.com/etcd-io/etcd/releases/download/${ETCD_VER}/etcd-${ETCD_VER}-linux-${ARCH}.tar.gz"
    tar xzf e.tgz
    install "etcd-${ETCD_VER}-linux-${ARCH}/etcd" "etcd-${ETCD_VER}-linux-${ARCH}/etcdctl" "$BIN/" )
  rm -rf "$t2"
else
  log "etcd present, skipping install"
fi

# ---- web control UI (from the tagged source) ----
log "fetching web control"
curl -fsSL "https://raw.githubusercontent.com/${REPO}/${VERSION}/webcontrol/index.html" -o "$WWW/index.html" \
  || err "could not fetch webcontrol/index.html for ${VERSION}"

# ---- nginx (install once) ----
if ! command -v nginx >/dev/null 2>&1; then
  log "installing nginx"
  apt-get update -y >/dev/null
  DEBIAN_FRONTEND=noninteractive apt-get install -y nginx >/dev/null
else
  log "nginx present, skipping install"
fi
cat >/etc/nginx/sites-available/tsunami <<'NG'
server {
  listen 8082 default_server;
  root /var/www/tsunami;
  index index.html;
  # same-origin API proxy so the session cookie is first-party (works in every browser)
  location /api/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 65s;
  }
  location / { try_files $uri $uri/ /index.html; add_header Cache-Control "no-store"; }
}
NG
ln -sf /etc/nginx/sites-available/tsunami /etc/nginx/sites-enabled/tsunami
rm -f /etc/nginx/sites-enabled/default

# ---- config + systemd units (rewritten every run) ----
cat >"$CFG/config.yaml" <<EOF
name: ocean-1
id: ocean-1
endpoints: { http: 0.0.0.0:8080, grpc: 0.0.0.0:8050 }
advertise: { grpc: ${PUBLIC_IP}:8050 }
max_connections: ${MAX_CONNECTIONS}
registry: { endpoints: ["127.0.0.1:2379"] }
lease: { ttl: 10 }
auth:
  token: "${TOKEN}"
  db_path: ${DATA}/auth.db
  session_ttl: 3600
  cookie_secure: false            # set true once this ocean is served over HTTPS
  bootstrap_admin: { username: "${ADMIN_USER}" }   # password via ADMIN_PASS env (systemd unit below)
EOF
cat >/etc/systemd/system/etcd.service <<EOF
[Unit]
Description=etcd (tsunami registry)
After=network-online.target
[Service]
ExecStart=${BIN}/etcd --name etcd0 --data-dir /var/lib/etcd \\
  --listen-client-urls http://0.0.0.0:2379 --advertise-client-urls http://${PUBLIC_IP}:2379 \\
  --listen-peer-urls http://127.0.0.1:2380 --initial-advertise-peer-urls http://127.0.0.1:2380 \\
  --initial-cluster etcd0=http://127.0.0.1:2380 --initial-cluster-state new --initial-cluster-token tsunami
Restart=always
LimitNOFILE=65536
[Install]
WantedBy=multi-user.target
EOF
cat >/etc/systemd/system/ocean.service <<EOF
[Unit]
Description=tsunami ocean
After=etcd.service
Requires=etcd.service
[Service]
Environment=ADMIN_USER=${ADMIN_USER}
Environment=ADMIN_PASS=${ADMIN_PASS}
ExecStart=${BIN}/ocean --path ${CFG} --file config.yaml
Restart=always
LimitNOFILE=65536
[Install]
WantedBy=multi-user.target
EOF

# ---- start / update ----
systemctl daemon-reload
systemctl enable etcd ocean >/dev/null 2>&1 || true
systemctl start etcd 2>/dev/null || true   # started once; not bounced on updates
sleep 2
systemctl restart ocean                    # pick up the new binary/config
nginx -t >/dev/null 2>&1 && systemctl restart nginx || true

sleep 3
log "services: $(systemctl is-active etcd ocean nginx 2>/dev/null | tr '\n' ' ' || true)"
if curl -fsS -X POST http://127.0.0.1:8080/api/v1/info -d '{"cmd":"info","conf":{}}' >/dev/null 2>&1; then
  log "ocean API responding"
else
  err "ocean not responding yet — check: journalctl -u ocean -n 50 --no-pager"
fi
cat <<EOF

Control plane ${VERSION} is up.
  Web control : http://${PUBLIC_IP}:8082   (set Master URL to http://${PUBLIC_IP}:8080)
  Ocean API   : http://${PUBLIC_IP}:8080
  Workers use : ${PUBLIC_IP}:2379  (etcd)  + the same token

Firewall: allow 8080/8082 from your IP; 2379/8050 from your worker IPs.
Update: re-run this exact command any time (it keeps your token).
EOF
