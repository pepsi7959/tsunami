#!/usr/bin/env bash
#
# One-command installer for the Tsunami worker node (Linux + macOS).
#
#   curl -sSL https://raw.githubusercontent.com/pepsi7959/tsunami/master/install.sh \
#     | sudo bash -s -- --etcd HOST:2379
#
# Downloads a prebuilt static binary from GitHub Releases, writes a cluster-mode
# config, installs a service (systemd on Linux, launchd on macOS), and starts it.
# Re-running is safe (idempotent).
#
set -euo pipefail

REPO="pepsi7959/tsunami"
SERVICE="tsunami-worker"        # systemd unit name (Linux)
LABEL="com.tsunami.worker"      # launchd label (macOS)

# ---- defaults (each overridable by flag or TSUNAMI_* env) ----
ETCD="${TSUNAMI_ETCD:-}"
ADVERTISE="${TSUNAMI_ADVERTISE:-}"
NAME="${TSUNAMI_NAME:-$(hostname -s 2>/dev/null || hostname)}"
CONCURRENCE="${TSUNAMI_CONCURRENCE:-10}"
VERSION="${TSUNAMI_VERSION:-}"
HTTP_PORT="${TSUNAMI_HTTP_PORT:-8090}"
GRPC_PORT="${TSUNAMI_GRPC_PORT:-8050}"
INSTALL_DIR="${TSUNAMI_INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${TSUNAMI_CONFIG_DIR:-/etc/tsunami}"
WORKER_PREFIX="${TSUNAMI_WORKER_PREFIX:-/tsunami/workers/}"
LEASE_TTL="${TSUNAMI_LEASE_TTL:-10}"
RUN_USER="${TSUNAMI_USER:-tsunami}"
DO_UNINSTALL=0
DO_PURGE=0
DRY_RUN=0

# ---- output helpers ----
log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m warning:\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m error:\033[0m %s\n' "$*" >&2; exit 1; }
run()  { if [ "$DRY_RUN" -eq 1 ]; then printf '   [dry-run] %s\n' "$*"; else eval "$*"; fi; }

usage() {
  cat <<'EOF'
Tsunami worker installer (Linux + macOS)

Usage:
  sudo bash install.sh --etcd HOST:2379[,HOST:2379...] [options]

Required:
  --etcd LIST          Comma-separated etcd endpoints the worker registers with

Options:
  --advertise IP       Address the master dials for gRPC (default: auto-detected)
  --name NAME          Worker name (default: hostname)
  --concurrence N      Default client concurrency (default: 10)
  --version vX.Y.Z     Release to install (default: latest)
  --http-port PORT     Admin HTTP port, binds 0.0.0.0 (default: 8090)
  --grpc-port PORT     gRPC control port (default: 8050)
  --install-dir DIR    Binary directory (default: /usr/local/bin)
  --config-dir DIR     Config directory (default: /etc/tsunami)
  --worker-prefix P    etcd key prefix (default: /tsunami/workers/)
  --lease-ttl N        etcd lease TTL seconds (default: 10)
  --user USER          service user. Linux: a dedicated 'tsunami' user is created
                       by default (use 'root' to skip). macOS: defaults to root
                       (no user is created; pass an existing user to override).
  --uninstall          Stop and remove the service + binary (keeps config)
  --purge              With --uninstall, also remove the config dir and user
  --dry-run            Print actions without changing anything
  -h, --help           Show this help

Every flag can also be set via its TSUNAMI_* environment variable.
EOF
}

# ---- argument parsing ----
while [ $# -gt 0 ]; do
  case "$1" in
    --etcd)          ETCD="$2"; shift 2;;
    --advertise)     ADVERTISE="$2"; shift 2;;
    --name)          NAME="$2"; shift 2;;
    --concurrence)   CONCURRENCE="$2"; shift 2;;
    --version)       VERSION="$2"; shift 2;;
    --http-port)     HTTP_PORT="$2"; shift 2;;
    --grpc-port)     GRPC_PORT="$2"; shift 2;;
    --install-dir)   INSTALL_DIR="$2"; shift 2;;
    --config-dir)    CONFIG_DIR="$2"; shift 2;;
    --worker-prefix) WORKER_PREFIX="$2"; shift 2;;
    --lease-ttl)     LEASE_TTL="$2"; shift 2;;
    --user)          RUN_USER="$2"; shift 2;;
    --uninstall)     DO_UNINSTALL=1; shift;;
    --purge)         DO_PURGE=1; shift;;
    --dry-run)       DRY_RUN=1; shift;;
    -h|--help)       usage; exit 0;;
    *) err "unknown argument: $1 (see --help)";;
  esac
done

UNIT_PATH="/etc/systemd/system/${SERVICE}.service"
PLIST_PATH="/Library/LaunchDaemons/${LABEL}.plist"
LOG_PATH="/var/log/tsunami-worker.log"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"

# ---- must run as root ----
[ "$(id -u)" -eq 0 ] || err "must run as root (use: sudo bash install.sh ...)"

# ---- OS detection + service manager ----
case "$(uname -s)" in
  Linux)  OS="linux";  SVC="systemd";;
  Darwin) OS="darwin"; SVC="launchd";;
  *) err "unsupported OS '$(uname -s)'. This installer supports Linux and macOS.";;
esac
if [ "$SVC" = "systemd" ]; then
  command -v systemctl >/dev/null 2>&1 || \
    err "systemd (systemctl) is required. Run the binary manually instead: tsunami --path ${CONFIG_DIR} --file config.yaml"
else
  command -v launchctl >/dev/null 2>&1 || err "launchd (launchctl) is required on macOS"
  # macOS has no useradd; default to running as root unless an existing user is given.
  [ "$RUN_USER" = "tsunami" ] && RUN_USER="root"
fi

# ---- service abstraction ----------------------------------------------------
svc_is_active() {
  if [ "$SVC" = "systemd" ]; then
    systemctl is-active --quiet "$SERVICE"
  else
    launchctl list "$LABEL" >/dev/null 2>&1
  fi
}

svc_stop_if_present() {
  if [ "$SVC" = "systemd" ]; then
    if systemctl list-unit-files 2>/dev/null | grep -q "^${SERVICE}.service"; then
      run "systemctl stop ${SERVICE} 2>/dev/null || true"
    fi
  else
    [ -f "$PLIST_PATH" ] && run "launchctl unload '${PLIST_PATH}' 2>/dev/null || true"
  fi
}

svc_write_and_start() {
  if [ "$SVC" = "systemd" ]; then
    if [ "$DRY_RUN" -eq 1 ]; then
      printf '   [dry-run] write %s\n' "$UNIT_PATH"
    else
      cat > "$UNIT_PATH" <<EOF
[Unit]
Description=Tsunami load-generation worker
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${RUN_USER}
Group=${RUN_USER}
WorkingDirectory=${CONFIG_DIR}
ExecStart=${INSTALL_DIR}/tsunami --path ${CONFIG_DIR} --file config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
    fi
    run "systemctl daemon-reload"
    run "systemctl enable ${SERVICE} >/dev/null 2>&1 || true"
    run "systemctl restart ${SERVICE}"
  else
    if [ "$DRY_RUN" -eq 1 ]; then
      printf '   [dry-run] write %s\n' "$PLIST_PATH"
    else
      cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/tsunami</string>
    <string>--path</string>
    <string>${CONFIG_DIR}</string>
    <string>--file</string>
    <string>config.yaml</string>
  </array>
  <key>WorkingDirectory</key>
  <string>${CONFIG_DIR}</string>
  <key>UserName</key>
  <string>${RUN_USER}</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>
  <key>SoftResourceLimits</key>
  <dict>
    <key>NumberOfFiles</key>
    <integer>65536</integer>
  </dict>
  <key>StandardOutPath</key>
  <string>${LOG_PATH}</string>
  <key>StandardErrorPath</key>
  <string>${LOG_PATH}</string>
</dict>
</plist>
EOF
      chmod 0644 "$PLIST_PATH"
    fi
    # reload: unload (ignore if not loaded) then load -w to enable + start
    run "launchctl unload '${PLIST_PATH}' 2>/dev/null || true"
    run "launchctl load -w '${PLIST_PATH}'"
  fi
}

svc_remove() {
  if [ "$SVC" = "systemd" ]; then
    run "systemctl disable --now ${SERVICE} 2>/dev/null || true"
    run "rm -f ${UNIT_PATH}"
    run "systemctl daemon-reload"
  else
    run "launchctl unload '${PLIST_PATH}' 2>/dev/null || true"
    run "rm -f '${PLIST_PATH}'"
  fi
}

detect_ip() {
  local ip=""
  if [ "$OS" = "linux" ]; then
    ip="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}' || true)"
    [ -n "$ip" ] || ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  else
    # macOS: find the interface for the default route, then its IPv4 address
    local iface
    iface="$(route -n get default 2>/dev/null | awk '/interface:/{print $2; exit}' || true)"
    [ -n "$iface" ] && ip="$(ipconfig getifaddr "$iface" 2>/dev/null || true)"
    [ -n "$ip" ] || ip="$(ipconfig getifaddr en0 2>/dev/null || true)"
  fi
  printf '%s' "$ip"
}
# ----------------------------------------------------------------------------

# ---- uninstall path ----
if [ "$DO_UNINSTALL" -eq 1 ]; then
  log "Uninstalling tsunami worker (${SVC})"
  svc_remove
  run "rm -f ${INSTALL_DIR}/tsunami"
  if [ "$DO_PURGE" -eq 1 ]; then
    run "rm -rf ${CONFIG_DIR}"
    if [ "$OS" = "linux" ] && [ "$RUN_USER" != "root" ] && id "$RUN_USER" >/dev/null 2>&1; then
      run "userdel ${RUN_USER} 2>/dev/null || true"
    fi
    log "Purged config and user. The etcd registration expires within its lease TTL."
  else
    log "Removed service and binary. Kept ${CONFIG_DIR} (use --purge to remove)."
  fi
  exit 0
fi

# ---- install preflight ----
[ -n "$ETCD" ] || { usage; err "--etcd is required"; }
for c in curl tar; do command -v "$c" >/dev/null 2>&1 || err "missing required command: $c"; done
if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD="shasum -a 256"
else
  err "need sha256sum or shasum to verify the download"
fi

# ---- arch detection ----
case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64";;
  aarch64|arm64) ARCH="arm64";;
  *) err "unsupported architecture '$(uname -m)'. Only amd64/arm64 are published; build from source instead.";;
esac

# ---- version resolution ----
if [ -z "$VERSION" ]; then
  log "Resolving latest release"
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/' || true)"
  if [ -z "$VERSION" ]; then
    err "no published release found for ${REPO}.
    The first release must be cut before this installer can download a binary:
      1) merge install.sh + .github/workflows/release.yml to master
      2) push a tag:  git tag v0.1.0 && git push origin v0.1.0
      3) the release workflow publishes the binaries, then re-run this command.
    Or pass an explicit --version once a release exists."
  fi
fi

ARCHIVE="tsunami_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

# ---- advertise IP ----
[ -n "$ADVERTISE" ] || ADVERTISE="$(detect_ip)"
[ -n "$ADVERTISE" ] || err "could not auto-detect an IP for --advertise; pass it explicitly"
case "$ADVERTISE" in
  127.*|localhost) err "advertise address '$ADVERTISE' is loopback; the master could not reach it. Pass --advertise <routable-ip>.";;
esac

log "Installing tsunami worker ${VERSION} (${OS}/${ARCH})"
log "  name=${NAME}  advertise=${ADVERTISE}:${GRPC_PORT}  etcd=${ETCD}  user=${RUN_USER}"

# ---- download + verify ----
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
log "Downloading ${ARCHIVE}"
run "curl -fSL -o '${WORKDIR}/${ARCHIVE}' '${BASE_URL}/${ARCHIVE}'" \
  || err "download failed: ${BASE_URL}/${ARCHIVE}"
run "curl -fSL -o '${WORKDIR}/SHA256SUMS' '${BASE_URL}/SHA256SUMS'" \
  || err "download failed: ${BASE_URL}/SHA256SUMS"

if [ "$DRY_RUN" -eq 0 ]; then
  expected="$(grep " ${ARCHIVE}\$" "${WORKDIR}/SHA256SUMS" | awk '{print $1}')"
  [ -n "$expected" ] || err "checksum for ${ARCHIVE} not found in SHA256SUMS"
  actual="$($SHA_CMD "${WORKDIR}/${ARCHIVE}" | awk '{print $1}')"
  [ "$expected" = "$actual" ] || err "checksum mismatch for ${ARCHIVE} (expected ${expected}, got ${actual})"
  log "Checksum verified"
fi

# ---- stop existing service before replacing the binary ----
svc_stop_if_present

# ---- install binary ----
run "tar -C '${WORKDIR}' -xzf '${WORKDIR}/${ARCHIVE}' tsunami"
run "install -m 0755 '${WORKDIR}/tsunami' '${INSTALL_DIR}/tsunami'"

# ---- service user (Linux only; macOS runs as root/existing user) ----
if [ "$OS" = "linux" ] && [ "$RUN_USER" != "root" ]; then
  if ! id "$RUN_USER" >/dev/null 2>&1; then
    log "Creating system user ${RUN_USER}"
    run "useradd --system --no-create-home --shell /usr/sbin/nologin ${RUN_USER}"
  fi
fi

# ---- worker id: reuse existing to stay idempotent, else generate ----
WORKER_ID=""
if [ -f "$CONFIG_FILE" ]; then
  WORKER_ID="$(grep -E '^id:' "$CONFIG_FILE" | head -1 | sed -E 's/^id:[[:space:]]*//; s/[[:space:]]*$//' || true)"
fi
if [ -z "$WORKER_ID" ]; then
  if [ -r /proc/sys/kernel/random/uuid ]; then
    WORKER_ID="$(cat /proc/sys/kernel/random/uuid)"
  elif command -v uuidgen >/dev/null 2>&1; then
    WORKER_ID="$(uuidgen)"
  else
    err "cannot generate a UUID (no /proc/sys/kernel/random/uuid or uuidgen)"
  fi
fi
log "Worker id: ${WORKER_ID}"

# ---- write config (exactly the keys worker/tsunami.go readConf() reads) ----
run "mkdir -p '${CONFIG_DIR}'"
if [ "$DRY_RUN" -eq 1 ]; then
  printf '   [dry-run] write %s\n' "$CONFIG_FILE"
else
  {
    echo "name: ${NAME}"
    echo "id: ${WORKER_ID}"
    echo "concurence: ${CONCURRENCE}"
    echo "mode: cluster"
    echo "endpoints:"
    echo "  grpc: ${ADVERTISE}:${GRPC_PORT}"
    echo "  http: 0.0.0.0:${HTTP_PORT}"
    echo "registry:"
    echo "  endpoints:"
    IFS=',' read -ra _eps <<< "$ETCD"
    for ep in "${_eps[@]}"; do
      ep="$(echo "$ep" | tr -d '[:space:]')"
      [ -n "$ep" ] && echo "    - ${ep}"
    done
    echo "  request_timeout: 2"
    echo "  dial_timeout: 2"
    echo "  worker_prefix: ${WORKER_PREFIX}"
    echo "lease:"
    echo "  ttl: ${LEASE_TTL}"
  } > "$CONFIG_FILE"
  chmod 0640 "$CONFIG_FILE"
fi

# ---- ownership (Linux non-root user) ----
if [ "$OS" = "linux" ] && [ "$RUN_USER" != "root" ] && [ "$DRY_RUN" -eq 0 ]; then
  run "chown -R ${RUN_USER}:${RUN_USER} '${CONFIG_DIR}'"
fi

# ---- write service + activate ----
svc_write_and_start

# ---- verify + summary ----
if [ "$DRY_RUN" -eq 1 ]; then
  log "Dry run complete — no changes made."
  exit 0
fi

sleep 1
if svc_is_active; then
  log "Service is active."
else
  warn "Service is not active yet — check the logs (see below)."
fi

if [ "$OS" = "linux" ]; then
  INSPECT="journalctl -u ${SERVICE} -f"
  STATUS="systemctl status ${SERVICE}"
else
  INSPECT="tail -f ${LOG_PATH}"
  STATUS="launchctl list ${LABEL}"
fi

cat <<EOF

$(log "Done.")
  Worker:     ${NAME} (id ${WORKER_ID})
  gRPC (advertised to master): ${ADVERTISE}:${GRPC_PORT}
  Admin HTTP: 0.0.0.0:${HTTP_PORT}   (per-test metrics: :8091)
  etcd:       ${ETCD}

  Inspect:    ${INSPECT}
  Status:     ${STATUS}
  Registered: etcdctl --endpoints=${ETCD%%,*} get --prefix ${WORKER_PREFIX}

  NOTE: ensure the master can reach ports ${GRPC_PORT} (gRPC) and ${HTTP_PORT} (HTTP)
        on ${ADVERTISE}. Open your firewall if needed:
          Linux: firewall-cmd --add-port=${GRPC_PORT}/tcp --add-port=${HTTP_PORT}/tcp --permanent && firewall-cmd --reload
                 # or: ufw allow ${GRPC_PORT}/tcp && ufw allow ${HTTP_PORT}/tcp
          macOS: allow '${INSTALL_DIR}/tsunami' to accept incoming connections in
                 System Settings > Network > Firewall (if the firewall is on)
EOF
