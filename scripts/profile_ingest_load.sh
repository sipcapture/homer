#!/usr/bin/env bash
# Repeatable ingest CPU profile: optional port cleanup, start homer with --pprof,
# UDP HEP3+SIP flood (hepudpload), fetch pprof CPU profile + optional perf flamegraph.
#
# Usage (from repo root):
#   ./scripts/profile_ingest_load.sh
#   ./scripts/profile_ingest_load.sh --kill-ports --config ./homer-check.json
#   PERF=1 ./scripts/profile_ingest_load.sh --kill-ports   # also generate perf flamegraph
#
# Env overrides:
#   HOMER          path to homer binary (default: ./homer under repo root)
#   CONFIG         modular JSON config (default: ./homer-check.json)
#   PPROF_ADDR     pprof listen address (default: 127.0.0.1:6066)
#   UDP_ADDR       HEP UDP target (default: 127.0.0.1:19060)
#   PPS            target packets/sec for hepudpload (default: 12000)
#   PROFILE_SEC    CPU profile duration (default: 22)
#   WARMUP_SEC     seconds of load before profiling starts (default: 12)
#                  homer does one-time init (yaml, pgx, gopacket, template) during startup;
#                  increase if profile still shows yaml/pgx/gopacket noise at top.
#   LOAD_SEC       hepudpload duration (default: WARMUP_SEC+PROFILE_SEC+2, auto-computed)
#   OUT_DIR        where to write profile + logs (default: /tmp/homer-profile-ingest)
#   SKIP_HOMER     if set to 1, do not start/kill homer (you run it yourself with same --pprof)
#   PERF           if set to 1, also run `perf record` + generate flamegraph SVG (requires sudo)
#   FLAMEGRAPH_DIR path to brendangregg/FlameGraph scripts (default: /tmp/FlameGraph)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

HOMER="${HOMER:-$REPO_ROOT/homer}"
CONFIG="${CONFIG:-$REPO_ROOT/homer-check.json}"
PPROF_ADDR="${PPROF_ADDR:-127.0.0.1:6066}"
UDP_ADDR="${UDP_ADDR:-127.0.0.1:19060}"
PPS="${PPS:-12000}"
PROFILE_SEC="${PROFILE_SEC:-22}"
WARMUP_SEC="${WARMUP_SEC:-12}"
LOAD_SEC="${LOAD_SEC:-$((WARMUP_SEC + PROFILE_SEC + 2))}"
OUT_DIR="${OUT_DIR:-/tmp/homer-profile-ingest}"
SKIP_HOMER="${SKIP_HOMER:-0}"
PERF="${PERF:-0}"
FLAMEGRAPH_DIR="${FLAMEGRAPH_DIR:-/tmp/FlameGraph}"

KILL_PORTS=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --kill-ports)    KILL_PORTS=1; shift ;;
    --config)        CONFIG="$2"; shift 2 ;;
    --homer)         HOMER="$2"; shift 2 ;;
    --pprof)         PPROF_ADDR="$2"; shift 2 ;;
    --udp)           UDP_ADDR="$2"; shift 2 ;;
    --pps)           PPS="$2"; shift 2 ;;
    --profile-sec)   PROFILE_SEC="$2"; shift 2 ;;
    --warmup-sec)    WARMUP_SEC="$2"; shift 2 ;;
    --load-sec)      LOAD_SEC="$2"; shift 2 ;;
    --out)           OUT_DIR="$2"; shift 2 ;;
    --perf)          PERF=1; shift ;;
    -h|--help)
      sed -n '1,40p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

mkdir -p "$OUT_DIR"
mkdir -p /tmp/homer-check/parquet 2>/dev/null || true

free_tcp_port() {
  local port="$1"
  for p in $(lsof -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true); do
    kill -9 "$p" 2>/dev/null || true
  done
}

free_udp_port() {
  local port="$1"
  for p in $(lsof -t -iUDP:"$port" 2>/dev/null || true); do
    kill -9 "$p" 2>/dev/null || true
  done
}

if [[ "$KILL_PORTS" -eq 1 ]]; then
  echo "[profile] --kill-ports: freeing default homer-check / pprof sockets"
  free_tcp_port 6066
  free_tcp_port 19061
  free_tcp_port 15051
  free_tcp_port 19190
  free_udp_port 19060
  sleep 1
fi

if [[ ! -x "$HOMER" ]] && [[ -f "$HOMER" ]]; then
  chmod +x "$HOMER" 2>/dev/null || true
fi
if [[ ! -f "$HOMER" ]]; then
  echo "homer binary not found: $HOMER (build with: make -C $REPO_ROOT)" >&2
  exit 1
fi
if [[ ! -f "$CONFIG" ]]; then
  echo "config not found: $CONFIG" >&2
  exit 1
fi

LOG="$OUT_DIR/homer.log"
PROF="$OUT_DIR/cpu.pb.gz"
TOP="$OUT_DIR/pprof-top.txt"
SVG="$OUT_DIR/flamegraph.svg"
PERF_DATA="$OUT_DIR/perf.data"
HPID=""

cleanup() {
  if [[ -n "$HPID" ]] && kill -0 "$HPID" 2>/dev/null; then
    kill "$HPID" 2>/dev/null || true
    wait "$HPID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [[ "$SKIP_HOMER" != "1" ]]; then
  echo "[profile] starting homer: $HOMER --config-path $CONFIG --pprof=$PPROF_ADDR"
  "$HOMER" --config-path "$CONFIG" --pprof="$PPROF_ADDR" >"$LOG" 2>&1 &
  HPID=$!
  for _ in $(seq 1 40); do
    if curl -sfS --max-time 1 "http://${PPROF_ADDR}/debug/pprof/" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
  if ! curl -sfS --max-time 2 "http://${PPROF_ADDR}/debug/pprof/" >/dev/null; then
    echo "[profile] FAIL: pprof not responding at http://${PPROF_ADDR}/debug/pprof/" >&2
    tail -40 "$LOG" >&2 || true
    exit 1
  fi
  echo "[profile] pprof OK at http://${PPROF_ADDR}/debug/pprof/"
else
  echo "[profile] SKIP_HOMER=1 — ensure homer is already running with --pprof=$PPROF_ADDR"
  if ! curl -sfS --max-time 2 "http://${PPROF_ADDR}/debug/pprof/" >/dev/null; then
    echo "[profile] FAIL: pprof not reachable" >&2
    exit 1
  fi
fi

# Resolve the real homer process PID (not the parent bash shell).
# pidof searches by executable name; fallback to pgrep -nx as secondary.
resolve_homer_pid() {
  local bin
  bin="$(basename "$HOMER")"
  local pid
  # pidof returns space-separated list; pick the one whose cmdline matches our binary path
  for pid in $(pidof "$bin" 2>/dev/null); do
    if grep -q "$HOMER" /proc/"$pid"/cmdline 2>/dev/null; then
      echo "$pid"
      return 0
    fi
  done
  # fallback: pgrep exact name
  pgrep -nx "$bin" 2>/dev/null | head -1 || true
}

echo "[profile] load: go run hepudpload -> $UDP_ADDR pps=$PPS duration=${LOAD_SEC}s (warmup=${WARMUP_SEC}s then profile=${PROFILE_SEC}s)"
(
  cd "$REPO_ROOT/src"
  go run ./cmd/hepudpload -addr="$UDP_ADDR" -pps="$PPS" -duration="${LOAD_SEC}s"
) &
LOADPID=$!

echo "[profile] warming up for ${WARMUP_SEC}s …"
sleep "$WARMUP_SEC"

# ── perf flamegraph (optional, requires sudo) ───────────────────────────────
if [[ "$PERF" -eq 1 ]]; then
  REAL_PID="$(resolve_homer_pid)"
  if [[ -z "$REAL_PID" ]]; then
    echo "[profile] WARN: could not resolve homer PID for perf; skipping flamegraph" >&2
  elif ! command -v perf >/dev/null 2>&1; then
    echo "[profile] WARN: perf not found; skipping flamegraph" >&2
  else
    echo "[profile] perf record -F 99 -e cycles -p $REAL_PID --call-graph fp ${PROFILE_SEC}s -> $PERF_DATA"
    # perf needs sudo if perf_event_paranoid > 0; run with sudo -n (non-interactive).
    # If sudo requires a password, this step is skipped with a warning.
    if sudo -n perf record -F 99 -e cycles -p "$REAL_PID" --call-graph fp \
          -o "$PERF_DATA" -- sleep "$PROFILE_SEC" 2>/dev/null; then
      # make perf.data readable by current user
      sudo -n chown "$(id -u):$(id -g)" "$PERF_DATA" 2>/dev/null || true

      # generate flamegraph if FlameGraph scripts exist
      if [[ -f "$FLAMEGRAPH_DIR/stackcollapse-perf.pl" ]]; then
        echo "[profile] generating flamegraph SVG -> $SVG"
        perf script -i "$PERF_DATA" 2>/dev/null \
          | "$FLAMEGRAPH_DIR/stackcollapse-perf.pl" \
          | "$FLAMEGRAPH_DIR/flamegraph.pl" --title "homer ingest (cycles)" \
          > "$SVG"
        echo "[profile] flamegraph: $SVG"
      else
        echo "[profile] FlameGraph not found at $FLAMEGRAPH_DIR (clone with: git clone --depth=1 https://github.com/brendangregg/FlameGraph /tmp/FlameGraph)"
      fi
    else
      echo "[profile] WARN: sudo perf failed (password required or perf_event_paranoid too high)" >&2
      echo "[profile] Run manually: sudo perf record -F 99 -e cycles -p $(resolve_homer_pid) --call-graph fp -o $PERF_DATA -- sleep $PROFILE_SEC"
    fi
  fi
fi

# ── Go pprof CPU profile ─────────────────────────────────────────────────────
echo "[profile] fetching CPU profile ${PROFILE_SEC}s -> $PROF"
curl -sfS --max-time "$((PROFILE_SEC + 8))" "http://${PPROF_ADDR}/debug/pprof/profile?seconds=${PROFILE_SEC}" -o "$PROF"
wait "$LOADPID" || true

{
  echo "========== go tool pprof -top (flat) =========="
  go tool pprof -top -nodecount=40 "$PROF" 2>&1
  echo ""
  echo "========== go tool pprof -top -cum =========="
  go tool pprof -top -cum -nodecount=40 "$PROF" 2>&1
} | tee "$TOP"

echo ""
echo "[profile] done."
echo "  go pprof:  $PROF"
echo "  top/cum:   $TOP"
echo "  homer log: $LOG"
[[ -f "$SVG" ]] && echo "  flamegraph: $SVG  (xdg-open $SVG)"
echo "Interactive: go tool pprof -http=:0 $PROF"
