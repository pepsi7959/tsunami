#!/usr/bin/env bash
#
# One-command installer for a Tsunami WORKER (streaming model) as a systemd
# service. Downloads the prebuilt binary, writes config, installs + starts the
# service. Idempotent: re-run any time to UPDATE (reuses etcd/token from the
# existing config unless overridden).
#
#   curl -fsSL https://raw.githubusercontent.com/pepsi7959/tsunami/streaming-control-plane/install-worker.sh \
#     | sudo ETCD=ocean.nineplusplus.com:2379 TOKEN=<shared-secret> bash
#
# Env: ETCD (required, host:2379), TOKEN (required), NAME (default hostname),
#      ID (default hostname), CONCURRENCE (default 300), TSUNAMI_VERSION (newest).
set -euo pipefail

REPO="pepsi7959/tsunami"
VERSION="${TSUNAMI_VERSION:-}"
ETCD="${ETCD:-}"
TOKEN="${TOKEN:-}"
NAME="${NAME:-$(hostname -s 2>/dev/null || hostname)}"
ID="${ID:-$NAME}"
CONCURRENCE="${CONCURRENCE:-300}"
BIN=/usr/local/bin
CFG=/etc/tsunami
SVC=tsunami-worker

log(){ printf '\033[1;32m==>\033[0m %s\n' "$*"; }
err(){ printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ]           || err "run as root (sudo)"
command -v systemctl >/dev/null 2>&1 || err "systemd is required"
command -v curl >/dev/null 2>&1      || err "curl is required"
case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) err "unsupported arch $(uname -m)" ;;
esac

# reuse existing config values on update
if [ -f "$CFG/config.yaml" ]; then
  [ -z "$ETCD" ]  && ETCD="$(sed -n 's/.*endpoints:[[:space:]]*\[[[:space:]]*"\([^"]*\)".*/\1/p' "$CFG/config.yaml" | head -n1 || true)"
  [ -z "$TOKEN" ] && TOKEN="$(sed -n 's/.*token:[[:space:]]*"\{0,1\}\([^"[:space:]]*\).*/\1/p' "$CFG/config.yaml" | head -n1 || true)"
fi
[ -z "$ETCD" ]  && err "set ETCD=<host:2379> (control-plane etcd endpoint)"
[ -z "$TOKEN" ] && err "set TOKEN=<shared worker token>"

# newest release (incl prerelease); fallback if the API is unavailable
if [ -z "$VERSION" ]; then
  rels="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" 2>/dev/null || true)"
  VERSION="$(printf '%s\n' "$rels" | grep '"tag_name":' | head -n1 | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/' || true)"
  [ -z "$VERSION" ] && VERSION="v0.2.0"
fi
log "worker '${NAME}' ${VERSION} (${ARCH}) -> etcd ${ETCD}"

# binary
tmp="$(mktemp -d)"
curl -fsSL "https://github.com/${REPO}/releases/download/${VERSION}/tsunami_${VERSION}_linux_${ARCH}.tar.gz" -o "$tmp/t.tgz" \
  || err "download failed"
tar -C "$tmp" -xzf "$tmp/t.tgz" tsunami || err "tsunami binary not in tarball"
install "$tmp/tsunami" "$BIN/tsunami"
rm -rf "$tmp"

# config (only the etcd endpoint + token are needed; the ocean is discovered)
mkdir -p "$CFG"
cat >"$CFG/config.yaml" <<EOF
name: ${NAME}
id: ${ID}
concurence: ${CONCURRENCE}
registry: { endpoints: ["${ETCD}"] }
auth: { token: "${TOKEN}" }
report: { interval: 2 }
EOF

# service
cat >/etc/systemd/system/${SVC}.service <<EOF
[Unit]
Description=Tsunami worker
After=network-online.target
Wants=network-online.target
[Service]
ExecStart=${BIN}/tsunami --path ${CFG} --file config.yaml
Restart=always
RestartSec=3
LimitNOFILE=65536
[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${SVC}" >/dev/null 2>&1 || true
systemctl restart "${SVC}"
sleep 2
log "worker service: $(systemctl is-active ${SVC} 2>/dev/null || true)"
echo "  follow logs:  journalctl -u ${SVC} -f"
echo "  it will attach once the ocean's :8050 is reachable from here."
