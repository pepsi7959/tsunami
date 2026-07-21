#!/usr/bin/env bash
# Idempotent Docker-less install/UPDATE of the tsunami control plane
# (etcd + ocean + web control) for a small host (e.g. a 512 MB droplet).
#
# First run installs everything. Re-running just refreshes the ocean binary,
# web UI and config and restarts the services (etcd/nginx install are skipped if
# already present; the existing auth token is kept unless TOKEN is provided).
#
#   TOKEN=<secret> PUBLIC_IP=<ip> ./install-native.sh     # first time
#   ./install-native.sh                                    # updates (reuses token)
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"

PUBLIC_IP="${PUBLIC_IP:-165.22.103.216}"
ETCD_VER="${ETCD_VER:-v3.5.16}"

# token precedence: env > existing config > prompt
TOKEN="${TOKEN:-}"
if [ -z "$TOKEN" ] && [ -f /etc/tsunami/config.yaml ]; then
  TOKEN="$(sed -n 's/.*token:[[:space:]]*"\{0,1\}\([^"[:space:]]*\).*/\1/p' /etc/tsunami/config.yaml | head -1)"
fi
[ -z "$TOKEN" ] && read -rp "Shared worker token: " TOKEN

mkdir -p /var/lib/etcd /etc/tsunami /var/www/tsunami

# ---------- etcd (install once) ----------
if ! command -v etcd >/dev/null 2>&1; then
  echo ">> installing etcd ${ETCD_VER}"
  tmp="$(mktemp -d)"; ( cd "$tmp"
    curl -fsSL -o e.tgz "https://github.com/etcd-io/etcd/releases/download/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz"
    tar xzf e.tgz
    install "etcd-${ETCD_VER}-linux-amd64/etcd" "etcd-${ETCD_VER}-linux-amd64/etcdctl" /usr/local/bin/ )
  rm -rf "$tmp"
else
  echo ">> etcd present, skipping install"
fi
cat >/etc/systemd/system/etcd.service <<EOF
[Unit]
Description=etcd (tsunami registry)
After=network-online.target
[Service]
ExecStart=/usr/local/bin/etcd --name etcd0 --data-dir /var/lib/etcd \\
  --listen-client-urls http://0.0.0.0:2379 --advertise-client-urls http://${PUBLIC_IP}:2379 \\
  --listen-peer-urls http://127.0.0.1:2380 --initial-advertise-peer-urls http://127.0.0.1:2380 \\
  --initial-cluster etcd0=http://127.0.0.1:2380 --initial-cluster-state new --initial-cluster-token tsunami
Restart=always
LimitNOFILE=65536
[Install]
WantedBy=multi-user.target
EOF

# ---------- ocean binary + config (updated every run) ----------
echo ">> updating ocean binary + config"
install "${HERE}/ocean" /usr/local/bin/ocean
cat >/etc/tsunami/config.yaml <<EOF
name: ocean-1
id: ocean-1
endpoints: { http: 0.0.0.0:8080, grpc: 0.0.0.0:8050 }
advertise: { grpc: ${PUBLIC_IP}:8050 }
max_connections: 2000
registry: { endpoints: ["127.0.0.1:2379"] }
lease: { ttl: 10 }
auth: { token: "${TOKEN}" }
EOF
cat >/etc/systemd/system/ocean.service <<EOF
[Unit]
Description=tsunami ocean
After=etcd.service
Requires=etcd.service
[Service]
ExecStart=/usr/local/bin/ocean --path /etc/tsunami --file config.yaml
Restart=always
LimitNOFILE=65536
[Install]
WantedBy=multi-user.target
EOF

# ---------- web control (updated every run) ----------
if ! command -v nginx >/dev/null 2>&1; then
  echo ">> installing nginx"; apt-get update -y >/dev/null; DEBIAN_FRONTEND=noninteractive apt-get install -y nginx >/dev/null
else
  echo ">> nginx present, skipping install"
fi
rm -rf /var/www/tsunami/*; cp -r "${HERE}/webcontrol/." /var/www/tsunami/
cat >/etc/nginx/sites-available/tsunami <<'EOF'
server {
  listen 8082 default_server;
  root /var/www/tsunami;
  index index.html;
  location / { try_files $uri $uri/ /index.html; add_header Cache-Control "no-store"; }
}
EOF
ln -sf /etc/nginx/sites-available/tsunami /etc/nginx/sites-enabled/tsunami
rm -f /etc/nginx/sites-enabled/default

# ---------- (re)start ----------
systemctl daemon-reload
systemctl enable etcd ocean >/dev/null 2>&1 || true
systemctl start etcd 2>/dev/null || true   # start once; not bounced on updates
sleep 2
systemctl restart ocean                    # pick up the new binary/config
nginx -t >/dev/null 2>&1 && systemctl restart nginx || systemctl reload nginx || true

sleep 3
echo "== services ==";  systemctl is-active etcd ocean nginx || true
echo "== ocean /info =="; curl -s -X POST http://127.0.0.1:8080/api/v1/info -d '{"cmd":"info","conf":{}}' || true; echo
echo
echo "Control plane ready."
echo "  Web control : http://${PUBLIC_IP}:8082   (set Master URL to http://${PUBLIC_IP}:8080)"
echo "  Ocean API   : http://${PUBLIC_IP}:8080"
echo "  etcd (workers): ${PUBLIC_IP}:2379   token: (the one you set)"
