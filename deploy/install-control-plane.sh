#!/usr/bin/env bash
# Install the tsunami control plane (etcd + one ocean + web control) on a single
# host (e.g. a DigitalOcean droplet). Run this from the repo root on the droplet:
#
#   PUBLIC_IP=<droplet-public-ip> TOKEN=<shared-secret> ./deploy/install-control-plane.sh
#
# Workers elsewhere then attach knowing only  <PUBLIC_IP>:2379  and the token.
set -euo pipefail

PUBLIC_IP="${PUBLIC_IP:-}"
TOKEN="${TOKEN:-}"
[ -z "$PUBLIC_IP" ] && read -rp "Droplet public IP: " PUBLIC_IP
[ -z "$TOKEN" ]     && read -rp "Shared worker token: " TOKEN

if [ ! -f docker-compose.droplet.yml ]; then
  echo "run this from the repo root (docker-compose.droplet.yml not found)" >&2
  exit 1
fi

# 1. Docker
if ! command -v docker >/dev/null 2>&1; then
  echo ">> installing Docker..."
  curl -fsSL https://get.docker.com | sh
fi

# 2. configure ocean1 (advertise the public gRPC address + shared token)
cfg=deploy/droplet/ocean1.yaml
sed -i "s#CHANGE_ME_PUBLIC_HOST:8050#${PUBLIC_IP}:8050#g" "$cfg"
sed -i "s#CHANGE_ME_SHARED_TOKEN#${TOKEN}#g" "$cfg"
echo ">> configured $cfg (advertise ${PUBLIC_IP}:8050)"

# 3. bring up etcd + one ocean + web control
echo ">> starting etcd + ocean1 + webcontrol..."
docker compose -f docker-compose.droplet.yml up --build -d etcd ocean1 webcontrol

# 4. verify
sleep 8
docker compose -f docker-compose.droplet.yml ps
echo ">> ocean /info:"
curl -s -X POST http://localhost:8080/api/v1/info -d '{"cmd":"info","conf":{}}' || true
echo
cat <<EOF

Control plane up.
  Web control : http://${PUBLIC_IP}:8082   (set the Master URL to http://${PUBLIC_IP}:8080)
  Ocean API   : http://${PUBLIC_IP}:8080
  etcd        : ${PUBLIC_IP}:2379   (workers use this)

On each worker machine, put this in config.yaml and run the tsunami image:
  name: worker-1
  id: <unique-id>
  concurence: 300
  registry: { endpoints: ["${PUBLIC_IP}:2379"] }
  auth: { token: "${TOKEN}" }

SECURITY: firewall 2379 + 8050 to your worker IPs only, and 8080/8082 to your own
IP; add TLS before real exposure (see DEPLOY.md).
EOF
