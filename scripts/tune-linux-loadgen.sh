#!/usr/bin/env bash
#
# tune-linux-loadgen.sh — แก้ปัญหา open port / fd หมด บนเครื่องที่รัน Tsunami worker
#
#   1) ulimit -n  1024 -> 65536 (soft+hard, ทั้ง login shell และ systemd service)
#   2) TCP TIME_WAIT ค้างเยอะ + ephemeral port หมด (sysctl)
#
# ทดสอบบน: Ubuntu 20.04 / kernel 5.4 (x86_64)
#
# Usage:
#   sudo ./tune-linux-loadgen.sh status     # ดูค่าปัจจุบัน (ไม่แก้อะไร)
#   sudo ./tune-linux-loadgen.sh apply      # เขียน config + apply
#   sudo ./tune-linux-loadgen.sh revert     # ลบ config ที่สคริปต์นี้เขียน
#
# ปรับค่าได้ด้วย env: NOFILE=131072 sudo -E ./tune-linux-loadgen.sh apply

set -euo pipefail

NOFILE="${NOFILE:-65536}"
PORT_RANGE="${PORT_RANGE:-10240 65535}"

case "$NOFILE" in ''|*[!0-9]*) echo "NOFILE ต้องเป็นตัวเลข: '$NOFILE'" >&2; exit 1;; esac

# nr_open เป็นเพดานสูงสุดของ ulimit -n ทั้งระบบ ต้องยกก่อนไม่งั้น limit ใหม่ถูกตัด
NR_OPEN=1048576
if [ "$NOFILE" -gt "$NR_OPEN" ]; then NR_OPEN="$NOFILE"; fi

LIMITS_FILE=/etc/security/limits.d/99-tsunami-nofile.conf
SYSTEMD_SYSTEM_FILE=/etc/systemd/system.conf.d/99-tsunami-nofile.conf
SYSTEMD_USER_FILE=/etc/systemd/user.conf.d/99-tsunami-nofile.conf
SYSCTL_FILE=/etc/sysctl.d/99-tsunami-net.conf

log()  { printf '\033[0;36m==>\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[0;33m[warn]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[0;31m[err]\033[0m %s\n' "$*" >&2; exit 1; }

need_root() { [ "$(id -u)" -eq 0 ] || err "ต้องรันด้วย root (sudo)"; }

# อ่าน sysctl แบบไม่ล้มถ้า key ไม่มีบน kernel นี้
sq() { sysctl -n "$1" 2>/dev/null || echo '-'; }

# ---------------------------------------------------------------- status

status() {
  log "file descriptor limits"
  printf '  ulimit -n (soft) : %s\n' "$(ulimit -Sn)"
  printf '  ulimit -n (hard) : %s\n' "$(ulimit -Hn)"
  printf '  fs.file-max      : %s\n' "$(sq fs.file-max)"
  printf '  fs.nr_open       : %s\n' "$(sq fs.nr_open)"
  if systemctl cat tsunami-worker >/dev/null 2>&1; then
    printf '  tsunami-worker unit : LimitNOFILE=%s\n' \
      "$(systemctl show -p LimitNOFILE --value tsunami-worker 2>/dev/null || echo '?')"
    # limit ของ unit ไม่การันตีว่า process ที่รันอยู่ได้ค่านั้น (ถ้ายังไม่ restart)
    local pid
    pid="$(systemctl show -p MainPID --value tsunami-worker 2>/dev/null || echo 0)"
    if [ "${pid:-0}" -gt 0 ] && [ -r "/proc/$pid/limits" ]; then
      printf '  tsunami-worker pid %s : %s\n' "$pid" \
        "$(awk '/open files/ {printf "soft=%s hard=%s", $4, $5}' "/proc/$pid/limits")"
      printf '  tsunami-worker pid %s : fd ใช้อยู่ %s\n' "$pid" \
        "$(ls "/proc/$pid/fd" 2>/dev/null | wc -l)"
    fi
  fi

  log "TCP / ephemeral ports"
  printf '  ip_local_port_range : %s\n' "$(sq net.ipv4.ip_local_port_range)"
  printf '  tcp_tw_reuse        : %s\n' "$(sq net.ipv4.tcp_tw_reuse)"
  printf '  tcp_fin_timeout     : %s\n' "$(sq net.ipv4.tcp_fin_timeout)"
  printf '  tcp_max_tw_buckets  : %s\n' "$(sq net.ipv4.tcp_max_tw_buckets)"
  printf '  somaxconn           : %s\n' "$(sq net.core.somaxconn)"

  log "socket ที่ใช้อยู่จริงตอนนี้"
  if command -v ss >/dev/null 2>&1; then
    ss -ant 2>/dev/null | awk 'NR>1 {c[$1]++} END {for (s in c) printf "  %-12s %s\n", s, c[s]}' | sort -k2 -rn
  else
    warn "ไม่มีคำสั่ง ss (ติดตั้ง iproute2)"
  fi
}

# ---------------------------------------------------------------- apply

apply_nofile() {
  log "เขียน $LIMITS_FILE (login shell / PAM)"
  # soft = ค่าที่ได้ทันทีเมื่อ login, hard = เพดานที่ process ยกเองได้ด้วย ulimit -n
  # ตั้ง hard สูงกว่า soft ไว้ ไม่งั้นจะกลายเป็นการ "ลด" hard limit เดิมของระบบลง
  cat >"$LIMITS_FILE" <<EOF
# managed by tsunami scripts/tune-linux-loadgen.sh
*    soft  nofile  ${NOFILE}
*    hard  nofile  ${NR_OPEN}
root soft  nofile  ${NOFILE}
root hard  nofile  ${NR_OPEN}
EOF

  log "เขียน systemd drop-in (DefaultLimitNOFILE=${NOFILE})"
  mkdir -p "$(dirname "$SYSTEMD_SYSTEM_FILE")" "$(dirname "$SYSTEMD_USER_FILE")"
  printf '[Manager]\nDefaultLimitNOFILE=%s:%s\n' "$NOFILE" "$NOFILE" >"$SYSTEMD_SYSTEM_FILE"
  printf '[Manager]\nDefaultLimitNOFILE=%s:%s\n' "$NOFILE" "$NOFILE" >"$SYSTEMD_USER_FILE"

  # PAM ต้องโหลด pam_limits ไม่งั้น limits.d ไม่มีผลกับ ssh login
  # (Ubuntu วางไว้ได้หลายที่: common-session, sshd, login — ต้องสแกนทั้ง dir)
  local pam_hits
  pam_hits="$(grep -rls 'pam_limits.so' /etc/pam.d/ 2>/dev/null | tr '\n' ' ' || true)"
  if [ -n "$pam_hits" ]; then
    log "pam_limits.so พบที่: ${pam_hits}"
  else
    warn "ไม่พบ pam_limits.so ใน /etc/pam.d/ — limits.d จะไม่มีผลกับ ssh login"
    warn "  แก้ด้วย: echo 'session required pam_limits.so' >> /etc/pam.d/common-session"
  fi
}

apply_sysctl() {
  log "เขียน $SYSCTL_FILE"
  cat >"$SYSCTL_FILE" <<EOF
# managed by tsunami scripts/tune-linux-loadgen.sh
# ---- ephemeral port / TIME_WAIT (ฝั่ง client ที่ยิงโหลดออก) ----
# ขยายช่วง port ที่ใช้ต่อออก: ~55k port ต่อ destination (ip:port) หนึ่งชุด
net.ipv4.ip_local_port_range = ${PORT_RANGE}
# ให้ reuse socket ที่อยู่ใน TIME_WAIT สำหรับ outbound connection ใหม่ได้
# (ปลอดภัยฝั่ง client, ใช้ TCP timestamps ตัดสิน — ไม่ใช่ tcp_tw_recycle ที่ถูกถอดไปแล้ว)
net.ipv4.tcp_tw_reuse = 1
# ลดเวลาค้างใน FIN_WAIT_2 จาก 60s
net.ipv4.tcp_fin_timeout = 15
# เพดานจำนวน TIME_WAIT socket ที่เก็บไว้ เกินนี้จะถูกทิ้งทันที (default 32768 ต่ำกว่า
# จำนวน ephemeral port ที่มี จึงเจอ "time wait bucket table overflow" ใน dmesg ได้ง่าย)
net.ipv4.tcp_max_tw_buckets = 262144
# TCP timestamps ต้องเปิด ไม่งั้น tcp_tw_reuse ไม่ทำงาน
net.ipv4.tcp_timestamps = 1

# ---- accept queue / backlog (กรณีเครื่องนี้รับ connection ด้วย) ----
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_max_syn_backlog = 65535
EOF

  # fs.* บน kernel ใหม่ default สูงมากอยู่แล้ว (file-max อาจเป็น 2^63-1) — เขียนทับด้วยค่า
  # คงที่จะกลายเป็นการ "ลด" เพดานลง จึงแตะเฉพาะตัวที่ค่าปัจจุบันต่ำกว่าเป้า
  local cur
  {
    echo
    echo "# ---- file descriptors ----"
    for key in fs.file-max fs.nr_open; do
      cur="$(sq "$key")"
      case "$cur" in
        ''|-|*[!0-9]*) warn "อ่าน $key ไม่ได้ — ข้าม" ;;
        *) if [ "$cur" -lt "$NR_OPEN" ]; then
             echo "${key} = ${NR_OPEN}"
           else
             log "$key = $cur สูงกว่าเป้า ($NR_OPEN) แล้ว — ไม่แตะ"
             echo "# ${key} = ${cur} (default สูงกว่าเป้าแล้ว ไม่ต้องตั้ง)"
           fi ;;
      esac
    done
  } >>"$SYSCTL_FILE"

  # conntrack เต็มก็ทำให้ connection ใหม่ถูก drop เหมือนกัน — tune เฉพาะเมื่อโมดูลถูกโหลด
  if [ -f /proc/sys/net/netfilter/nf_conntrack_max ]; then
    log "พบ nf_conntrack — เพิ่ม conntrack table + ลด timeout ของ TIME_WAIT"
    cat >>"$SYSCTL_FILE" <<'EOF'

# ---- conntrack (โหลดอยู่บนเครื่องนี้) ----
net.netfilter.nf_conntrack_max = 1048576
net.netfilter.nf_conntrack_tcp_timeout_time_wait = 30
EOF
  fi

  log "apply sysctl"
  sysctl --system >/dev/null
}

apply() {
  need_root
  apply_nofile
  apply_sysctl

  log "reload systemd (ค่า DefaultLimitNOFILE ใหม่จะมีผลกับ service ที่ restart หลังจากนี้)"
  systemctl daemon-reexec

  if systemctl is-active --quiet tsunami-worker 2>/dev/null; then
    log "restart tsunami-worker เพื่อรับ limit ใหม่"
    systemctl restart tsunami-worker
  fi

  echo
  status
  echo
  warn "ulimit ของ shell ปัจจุบันยังเป็นค่าเก่า — ต้อง logout/login ใหม่ (หรือ 'ulimit -n ${NOFILE}' ใน shell นี้)"
}

# ---------------------------------------------------------------- revert

revert() {
  need_root
  log "ลบ config ที่สคริปต์นี้เขียน"
  rm -fv "$LIMITS_FILE" "$SYSTEMD_SYSTEM_FILE" "$SYSTEMD_USER_FILE" "$SYSCTL_FILE"
  sysctl --system >/dev/null
  systemctl daemon-reexec
  warn "ค่า sysctl บางตัวที่เขียนลง kernel ไปแล้วจะกลับเป็น default หลัง reboot"
}

case "${1:-status}" in
  status) status ;;
  apply)  apply  ;;
  revert) revert ;;
  *) err "usage: $0 {status|apply|revert}" ;;
esac
