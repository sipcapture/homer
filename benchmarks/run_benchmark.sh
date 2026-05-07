#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BENCH_DIR="$ROOT_DIR/bechmarks"
RUNS_DIR="$BENCH_DIR/runs"
BIN_DIR="$BENCH_DIR/.bin"
RESULTS_FILE="$BENCH_DIR/results.csv"

# Tunables (can be overridden via env)
HEP_PORT="${HEP_PORT:-19060}"
PROTOCOL="${PROTOCOL:-udp}"
BENCH_DURATION="${BENCH_DURATION:-10s}"
BENCH_WORKERS="${BENCH_WORKERS:-4}"
MIN_STORED_COUNT="${MIN_STORED_COUNT:-1000}"
MIN_STORED_PPS="${MIN_STORED_PPS:-100}"
DEGRADE_THRESHOLD_PCT="${DEGRADE_THRESHOLD_PCT:-5}"
INGEST_WORKER_COUNT="${INGEST_WORKER_COUNT:-8}"
INGEST_QUEUE_SIZE="${INGEST_QUEUE_SIZE:-200000}"
STORAGE_BATCH_SIZE="${STORAGE_BATCH_SIZE:-20000}"
STORAGE_FLUSH_INTERVAL_SEC="${STORAGE_FLUSH_INTERVAL_SEC:-10}"
INGEST_SCRIPTS_ENABLE="${INGEST_SCRIPTS_ENABLE:-true}"
REMOTE_LOGGING_ENABLE="${REMOTE_LOGGING_ENABLE:-false}"
HEP_NO_PARSE="${HEP_NO_PARSE:-0}"
HEP_BENCH_PROTO_TYPE="${HEP_BENCH_PROTO_TYPE:-1}"
BENCH_SCENARIO="${BENCH_SCENARIO:-current}"
BENCH_SCENARIOS="${BENCH_SCENARIOS:-$BENCH_SCENARIO}"
BENCH_UDP_RXBUF_BYTES="${BENCH_UDP_RXBUF_BYTES:-2097152}"
BENCH_UDP_TXBUF_BYTES="${BENCH_UDP_TXBUF_BYTES:-2097152}"
TUNE_UDP_BUFFERS="${TUNE_UDP_BUFFERS:-false}"
STORAGE_SHARD_COUNT="${STORAGE_SHARD_COUNT:-1}"

BASE_INGEST_WORKER_COUNT="$INGEST_WORKER_COUNT"
BASE_INGEST_QUEUE_SIZE="$INGEST_QUEUE_SIZE"
BASE_STORAGE_BATCH_SIZE="$STORAGE_BATCH_SIZE"
BASE_STORAGE_FLUSH_INTERVAL_SEC="$STORAGE_FLUSH_INTERVAL_SEC"
BASE_SCRIPTS_ENABLED="$INGEST_SCRIPTS_ENABLE"
BASE_HEP_BENCH_PROTO_TYPE="$HEP_BENCH_PROTO_TYPE"
BASE_HEP_NO_PARSE="$HEP_NO_PARSE"
BASE_REMOTE_LOGGING_ENABLE="$REMOTE_LOGGING_ENABLE"

mkdir -p "$RUNS_DIR" "$BIN_DIR"

RUN_ID="$(date +%Y%m%d-%H%M%S)"
HOMER_BIN="$BIN_DIR/homer"

SERVER_PID=""

if [[ "$PROTOCOL" != "udp" && "$PROTOCOL" != "tcp" ]]; then
  echo "ERROR: PROTOCOL must be 'udp' or 'tcp' (current: $PROTOCOL)"
  exit 1
fi

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill -TERM "$SERVER_PID" 2>/dev/null || true
    for _ in {1..20}; do
      if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        break
      fi
      sleep 0.25
    done
    if kill -0 "$SERVER_PID" 2>/dev/null; then
      kill -KILL "$SERVER_PID" 2>/dev/null || true
    fi
  fi
}
trap cleanup EXIT INT TERM

write_config() {
  local udp_enable tcp_enable
  if [[ "$PROTOCOL" == "tcp" ]]; then
    udp_enable="false"
    tcp_enable="true"
  else
    udp_enable="true"
    tcp_enable="false"
  fi

  cat > "$CFG_PATH" <<JSON
{
  "ingest": {
    "enable": true,
    "worker_count": $INGEST_WORKER_COUNT,
    "queue_size": $INGEST_QUEUE_SIZE,
    "udp": {
      "enable": $udp_enable,
      "host": "127.0.0.1",
      "port": $HEP_PORT,
      "multicore": true
    },
    "tcp": {
      "enable": $tcp_enable,
      "host": "127.0.0.1",
      "port": $HEP_PORT,
      "multicore": true
    },
    "tls": {"enable": false},
    "http": {"enable": false},
    "https": {"enable": false},
    "hep": {
      "hepv2_enable": true,
      "hepv3_enable": true,
      "protobuf_enable": false,
      "deduplicate": false
    },
    "scripting": {
      "enable": ${INGEST_SCRIPTS_ENABLE}
    },
    "sip": {
      "censored_methods": [],
      "discard_methods": [],
      "aleg_ids": ["X-CID", "X-Session-ID"],
      "custom_headers": [],
      "force_aleg_id": false
    }
  },
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "sqlite",
      "catalog_path": "$RUN_DIR/data/homer_catalog.sqlite",
      "data_path": "$RUN_DIR/data/parquet",
      "lake_name": "homer_lake",
      "batch_size": $STORAGE_BATCH_SIZE,
      "flush_interval_sec": $STORAGE_FLUSH_INTERVAL_SEC,
      "shard_count": $STORAGE_SHARD_COUNT,
      "compaction": {"enable": false}
    }
  },
  "node": {"enable": false},
  "coordinator": {"enable": false},
  "prometheus": {"enable": false},
  "remote_logging": {
    "loki": {"enable": ${REMOTE_LOGGING_ENABLE}},
    "elasticsearch": {"enable": false},
    "lineproto": {"enable": false}
  },
  "log": {
    "level": "error",
    "json": false,
    "output": ["stdout"],
    "stdout": {"colored": false}
  }
}
JSON
}

wait_for_udp_port() {
  local retries=40

  for _ in $(seq 1 "$retries"); do
    if [[ -n "$SERVER_PID" ]] && ! kill -0 "$SERVER_PID" 2>/dev/null; then
      return 1
    fi
    if grep -q "UDP server error" "$SERVER_LOG" 2>/dev/null; then
      return 1
    fi
    sleep 0.25
  done

  return 0
}

reset_benchmark_tunables() {
  INGEST_WORKER_COUNT="$BASE_INGEST_WORKER_COUNT"
  INGEST_QUEUE_SIZE="$BASE_INGEST_QUEUE_SIZE"
  STORAGE_BATCH_SIZE="$BASE_STORAGE_BATCH_SIZE"
  STORAGE_FLUSH_INTERVAL_SEC="$BASE_STORAGE_FLUSH_INTERVAL_SEC"
  INGEST_SCRIPTS_ENABLE="$BASE_SCRIPTS_ENABLED"
  HEP_BENCH_PROTO_TYPE="$BASE_HEP_BENCH_PROTO_TYPE"
  HEP_NO_PARSE="$BASE_HEP_NO_PARSE"
  REMOTE_LOGGING_ENABLE="$BASE_REMOTE_LOGGING_ENABLE"
}

apply_benchmark_scenario() {
  local scenario="$1"

  reset_benchmark_tunables

  case "$scenario" in
    current|base|default)
      ;;
    batch_flush_only_up)
      INGEST_WORKER_COUNT="${SCENARIO_WORKER_COUNT:-16}"
      INGEST_QUEUE_SIZE="${SCENARIO_QUEUE_SIZE:-80000}"
      STORAGE_BATCH_SIZE="${SCENARIO_BATCH_SIZE:-50000}"
      STORAGE_FLUSH_INTERVAL_SEC="${SCENARIO_FLUSH_SEC:-30}"
      ;;
    minimal_payload|no_parse)
      HEP_NO_PARSE=1
      HEP_BENCH_PROTO_TYPE="${SCENARIO_PROTO_TYPE:-34}"
      ;;
    no_scripts_remote)
      INGEST_SCRIPTS_ENABLE=false
      REMOTE_LOGGING_ENABLE=false
      ;;
    *)
      echo "ERROR: unknown BENCH_SCENARIO '$scenario'"
      exit 1
      ;;
  esac
}

validate_udp_buffers() {
  if ! command -v sysctl >/dev/null 2>&1; then
    echo "INFO: sysctl not found, skipping UDP buffer validation"
    return
  fi

  local rmem_max rmem_def wmem_max wmem_def
  rmem_max=$(sysctl -n net.core.rmem_max 2>/dev/null || echo "0")
  rmem_def=$(sysctl -n net.core.rmem_default 2>/dev/null || echo "0")
  wmem_max=$(sysctl -n net.core.wmem_max 2>/dev/null || echo "0")
  wmem_def=$(sysctl -n net.core.wmem_default 2>/dev/null || echo "0")

  echo "INFO: current UDP sysctl rmem_max=$rmem_max rmem_default=$rmem_def wmem_max=$wmem_max wmem_default=$wmem_def"

  if [[ "$rmem_max" != "0" && "$rmem_max" -lt "$BENCH_UDP_RXBUF_BYTES" ]]; then
    echo "WARN: net.core.rmem_max (${rmem_max}) is below target (${BENCH_UDP_RXBUF_BYTES})"
  fi
  if [[ "$wmem_max" != "0" && "$wmem_max" -lt "$BENCH_UDP_TXBUF_BYTES" ]]; then
    echo "WARN: net.core.wmem_max (${wmem_max}) is below target (${BENCH_UDP_TXBUF_BYTES})"
  fi

  if [[ "${TUNE_UDP_BUFFERS}" == "true" ]]; then
    if [[ "$EUID" -ne 0 ]]; then
      echo "WARN: TUNE_UDP_BUFFERS=true but not root; cannot apply sysctl changes"
      return
    fi
    sysctl -w net.core.rmem_max="$BENCH_UDP_RXBUF_BYTES" >/dev/null || true
    sysctl -w net.core.rmem_default="$BENCH_UDP_RXBUF_BYTES" >/dev/null || true
    sysctl -w net.core.wmem_max="$BENCH_UDP_TXBUF_BYTES" >/dev/null || true
    sysctl -w net.core.wmem_default="$BENCH_UDP_TXBUF_BYTES" >/dev/null || true
    echo "INFO: applied UDP socket buffers via sysctl"
  fi
}

get_udp_snmp_counter() {
  local counter_name="$1"
  local block
  local header_line
  local value_line

  block=$(grep -A1 -m1 '^Udp:' /proc/net/snmp | tr -s ' ')
  if [[ -z "$block" ]]; then
    echo 0
    return
  fi

  header_line=$(printf "%s\n" "$block" | head -1)
  value_line=$(printf "%s\n" "$block" | tail -1)

  awk -v h="$header_line" -v v="$value_line" -v key="$counter_name" '
    BEGIN {
      n = split(h, keys, " ");
      m = split(v, vals, " ");
      idx = 0;
      for (i = 1; i <= n; i++) {
        if (keys[i] == key) {
          idx = i;
          break;
        }
      }
      if (idx == 0 || idx > m) {
        print 0;
      } else {
        print vals[idx];
      }
    }'
}

snapshot_udp_snmp() {
  local rcvbuf in_datagrams out_datagrams
  rcvbuf=$(get_udp_snmp_counter "RcvbufErrors")
  in_datagrams=$(get_udp_snmp_counter "InDatagrams")
  out_datagrams=$(get_udp_snmp_counter "OutDatagrams")
  echo "$rcvbuf $in_datagrams $out_datagrams"
}

build_binaries() {
  echo "[1/6] Building homer binary..."
  (cd "$ROOT_DIR/src" && go build -o "$HOMER_BIN" .)
  if [[ "$PROTOCOL" == "tcp" ]]; then
    build_tcp_sender
  fi
}

start_server() {
  echo "[2/6] Starting homer server..."
  "$HOMER_BIN" --config-path "$CFG_PATH" --syslog-disable --log-level error >"$SERVER_LOG" 2>&1 &
  SERVER_PID="$!"

  if ! wait_for_udp_port; then
    echo "ERROR: server did not start correctly. See $SERVER_LOG"
    exit 1
  fi
}

run_sender() {
  echo "[3/6] Sending HEP packets (protocol=$PROTOCOL duration=$BENCH_DURATION workers=$BENCH_WORKERS)..."
  local sender_flags=()
  sender_flags+=("-proto-type" "$HEP_BENCH_PROTO_TYPE")
  if [[ "$HEP_NO_PARSE" == "1" ]]; then
    sender_flags+=("-compact-payload")
  fi

  if [[ "$PROTOCOL" == "tcp" ]]; then
    "$SENDER_BIN" \
      -addr "127.0.0.1:$HEP_PORT" \
      -duration "$BENCH_DURATION" \
      -workers "$BENCH_WORKERS" \
      | tee "$SENDER_LOG"
  else
    go run "$ROOT_DIR/scripts/hep_benchmark.go" \
      -addr "127.0.0.1:$HEP_PORT" \
      -duration "$BENCH_DURATION" \
      -workers "$BENCH_WORKERS" \
      "${sender_flags[@]}" | tee "$SENDER_LOG"
  fi
}

build_tcp_sender() {
  local src="$RUN_DIR/tcp_sender.go"
  cat > "$src" <<'GO'
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func makeChunk(vendorID, chunkType uint16, body []byte) []byte {
	chunkLen := uint16(6 + len(body))
	buf := make([]byte, chunkLen)
	binary.BigEndian.PutUint16(buf[0:2], vendorID)
	binary.BigEndian.PutUint16(buf[2:4], chunkType)
	binary.BigEndian.PutUint16(buf[4:6], chunkLen)
	copy(buf[6:], body)
	return buf
}

func makeU8Chunk(chunkType uint16, val byte) []byte {
	return makeChunk(0x0000, chunkType, []byte{val})
}

func makeU16Chunk(chunkType uint16, val uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, val)
	return makeChunk(0x0000, chunkType, b)
}

func makeU32Chunk(chunkType uint16, val uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, val)
	return makeChunk(0x0000, chunkType, b)
}

func makeStrChunk(chunkType uint16, s string) []byte {
	return makeChunk(0x0000, chunkType, []byte(s))
}

func makeIP4Chunk(chunkType uint16, ip net.IP) []byte {
	return makeChunk(0x0000, chunkType, ip.To4())
}

func buildSIPInvite(callID string, seq int) string {
	return fmt.Sprintf(
		"INVITE sip:bob@biloxi.example.com SIP/2.0\r\n"+
			"Via: SIP/2.0/TCP pc33.atlanta.example.com;branch=z9hG4bK776asdhds\r\n"+
			"Max-Forwards: 70\r\n"+
			"To: Bob <sip:bob@biloxi.example.com>\r\n"+
			"From: Alice <sip:alice@atlanta.example.com>;tag=1928301774\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: %d INVITE\r\n"+
			"Contact: <sip:alice@pc33.atlanta.example.com>\r\n"+
			"Content-Length: 0\r\n"+
			"\r\n",
		callID, seq)
}

func buildMinimalInvite(callID string) string {
	return fmt.Sprintf(
		"INVITE sip:bench@localhost SIP/2.0\r\n"+
			"Max-Forwards: 70\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: 1 INVITE\r\n"+
			"Content-Length: 0\r\n"+
			"\r\n",
		callID)
}

func buildHEP3Packet(callID string, seq int, protoType uint16, compactPayload bool) []byte {
	const (
		chunkIPFamily = 0x0001
		chunkIPProto  = 0x0002
		chunkIP4Src   = 0x0003
		chunkIP4Dst   = 0x0004
		chunkSrcPort  = 0x0007
		chunkDstPort  = 0x0008
		chunkTsec     = 0x0009
		chunkTmsec    = 0x000a
		chunkProtoT   = 0x000b
		chunkNodeID   = 0x000c
		chunkNodePW   = 0x000e
		chunkPayload  = 0x000f
		chunkCID      = 0x0011
	)
	now := time.Now()
	var sipPayload string
	if compactPayload {
		sipPayload = buildMinimalInvite(callID)
	} else {
		sipPayload = buildSIPInvite(callID, seq)
	}
	var chunks []byte
	chunks = append(chunks, makeU8Chunk(chunkIPFamily, 0x02)...)
	chunks = append(chunks, makeU8Chunk(chunkIPProto, 0x06)...)
	chunks = append(chunks, makeIP4Chunk(chunkIP4Src, net.ParseIP("10.0.0.1"))...)
	chunks = append(chunks, makeIP4Chunk(chunkIP4Dst, net.ParseIP("10.0.0.2"))...)
	chunks = append(chunks, makeU16Chunk(chunkSrcPort, 5060)...)
	chunks = append(chunks, makeU16Chunk(chunkDstPort, 5060)...)
	chunks = append(chunks, makeU32Chunk(chunkTsec, uint32(now.Unix()))...)
	chunks = append(chunks, makeU32Chunk(chunkTmsec, uint32(now.UnixMicro()%1_000_000))...)
	chunks = append(chunks, makeU8Chunk(chunkProtoT, byte(protoType))...)
	chunks = append(chunks, makeU32Chunk(chunkNodeID, 2001)...)
	chunks = append(chunks, makeStrChunk(chunkNodePW, "myHep")...)
	chunks = append(chunks, makeStrChunk(chunkPayload, sipPayload)...)
	chunks = append(chunks, makeStrChunk(chunkCID, callID)...)
	totalLen := 4 + 2 + len(chunks)
	packet := make([]byte, totalLen)
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(totalLen))
	copy(packet[6:], chunks)
	return packet
}

func main() {
	addr := flag.String("addr", "127.0.0.1:19060", "homer-core HEP TCP address")
	protoType := flag.Uint("proto-type", 1, "transport protocol type for chunkProtoT")
	compactPayload := flag.Bool("compact-payload", false, "send minimal SIP payload")
	duration := flag.Duration("duration", 10*time.Second, "benchmark duration")
	workers := flag.Int("workers", 1, "number of parallel sender goroutines")
	flag.Parse()

	fmt.Printf("HEP3 Benchmark\n")
	fmt.Printf("  Target:   %s\n", *addr)
	fmt.Printf("  Duration: %s\n", *duration)
	fmt.Printf("  Workers:  %d\n\n", *workers)

	samplePacket := buildHEP3Packet("bench-sample-callid@localhost", 1, uint16(*protoType), *compactPayload)
	fmt.Printf("  Packet size: %d bytes\n\n", len(samplePacket))

	var totalSent int64
	var totalErrors int64
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})
	start := time.Now()

	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			conn, err := net.Dial("tcp", *addr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker %d: dial error: %v\n", workerID, err)
				return
			}
			defer conn.Close()

			batchSize := 100
			packets := make([][]byte, batchSize)
			for i := 0; i < batchSize; i++ {
				callID := fmt.Sprintf("bench-%d-%d-%d@localhost", workerID, i, rand.Int63())
				packets[i] = buildHEP3Packet(callID, i+1, uint16(*protoType), *compactPayload)
			}

			<-startBarrier
			endTime := time.Now().Add(*duration)
			conn.SetDeadline(endTime.Add(3 * time.Second))
			var localSent, localErr int64
			idx := 0
			for time.Now().Before(endTime) {
				if _, err := conn.Write(packets[idx%batchSize]); err != nil {
					localErr++
					break
				}
				localSent++
				idx++
			}
			atomic.AddInt64(&totalSent, localSent)
			atomic.AddInt64(&totalErrors, localErr)
		}(w)
	}

	close(startBarrier)
	start = time.Now()
	wg.Wait()
	elapsed := time.Since(start)

	sent := atomic.LoadInt64(&totalSent)
	errors := atomic.LoadInt64(&totalErrors)
	pps := float64(sent) / elapsed.Seconds()
	mbps := float64(sent) * float64(len(samplePacket)) * 8 / elapsed.Seconds() / 1_000_000
	fmt.Printf("Results:\n")
	fmt.Printf("  Duration:    %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Packets:     %d sent, %d errors\n", sent, errors)
	fmt.Printf("  Throughput:  %.0f packets/sec\n", pps)
	fmt.Printf("  Bandwidth:   %.1f Mbps\n", mbps)
	fmt.Printf("  Per worker:  %.0f packets/sec\n", pps/float64(*workers))
}
GO
  go build -o "$SENDER_BIN" "$src"
}

stop_server() {
  echo "[4/6] Stopping server (flush data)..."
  # Give the server time to flush pending memory tables to parquet
  sleep 15
  cleanup
  SERVER_PID=""
}

query_scalar() {
  local sql="$1"
  local out
  local rc
  out=$("$HOMER_BIN" cli --config-path "$CFG_PATH" --query "$sql" 2>&1)
  rc=$?
  if [[ $rc -ne 0 ]]; then
    echo "SQL query failed:"
    echo "$out"
    return $rc
  fi

  printf "%s\n" "$out" \
    | awk -F'|' '/^\|[[:space:]]*[0-9]+/{gsub(/^[ \t]+|[ \t]+$/, "", $2); gsub(/,/, "", $2); print $2; exit}'
}

validate_results() {
  echo "[5/6] Validating stored data..."

  local parquet_count
  local proto_parquet_count
  parquet_count=$(find "$RUN_DIR/data/parquet" -name '*.parquet' 2>/dev/null | wc -l | tr -d ' ')
  if [[ "$parquet_count" == "0" ]]; then
    echo "ERROR: no parquet files were written"
    return 1
  fi

  proto_parquet_count=$(find "$RUN_DIR/data/parquet" -type f -path "*/hep_proto_${HEP_BENCH_PROTO_TYPE}_*/*/*.parquet" 2>/dev/null | wc -l | tr -d ' ')
  if [[ "$proto_parquet_count" == "0" ]]; then
    echo "ERROR: no parquet files for proto_type=$HEP_BENCH_PROTO_TYPE"
    return 1
  fi

  local call_glob
  call_glob="$RUN_DIR/data/parquet/**/hep_proto_${HEP_BENCH_PROTO_TYPE}_*/*/*.parquet"

  STORED_COUNT=$(query_scalar "SELECT COUNT(*) FROM read_parquet('$call_glob', hive_partitioning=true, union_by_name=true) WHERE session_id LIKE 'bench-%@localhost';")
  if ! INVITE_COUNT=$(
    query_scalar "SELECT COUNT(*) FROM read_parquet('$call_glob', hive_partitioning=true, union_by_name=true) WHERE session_id LIKE 'bench-%@localhost' AND method = 'INVITE';"
  ); then
    INVITE_COUNT=$STORED_COUNT
  fi

  if [[ -z "${STORED_COUNT:-}" ]] || [[ -z "${INVITE_COUNT:-}" ]]; then
    echo "ERROR: failed to query validation metrics via homer cli"
    return 1
  fi

  if ! [[ "$STORED_COUNT" =~ ^[0-9]+$ ]] || ! [[ "$INVITE_COUNT" =~ ^[0-9]+$ ]]; then
    echo "ERROR: validation query returned non-numeric result"
    return 1
  fi

  if [[ "$STORED_COUNT" -eq 0 ]]; then
    echo "ERROR: zero benchmark records found in storage"
    return 1
  fi

  return 0
}

extract_sender_metrics() {
  SENDER_DURATION_SEC=$(sed -n 's/.*Duration:[[:space:]]*\([0-9.]\+\)s.*/\1/p' "$SENDER_LOG" | tail -1)
  SENT_PACKETS=$(sed -n 's/.*Packets:[[:space:]]*\([0-9]\+\) sent, \([0-9]\+\) errors.*/\1/p' "$SENDER_LOG" | tail -1)
  SEND_ERRORS=$(sed -n 's/.*Packets:[[:space:]]*\([0-9]\+\) sent, \([0-9]\+\) errors.*/\2/p' "$SENDER_LOG" | tail -1)
  SENDER_PPS=$(sed -n 's/.*Throughput:[[:space:]]*\([0-9.]\+\) packets\/sec.*/\1/p' "$SENDER_LOG" | tail -1)

  if [[ -z "${SENDER_DURATION_SEC:-}" ]] || [[ -z "${SENT_PACKETS:-}" ]] || [[ -z "${SEND_ERRORS:-}" ]] || [[ -z "${SENDER_PPS:-}" ]]; then
    echo "ERROR: failed to parse sender benchmark metrics"
    exit 1
  fi
}

append_results() {
  local git_commit
  local expected_header
  git_commit=$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")

  STORE_RATIO=$(awk -v s="$STORED_COUNT" -v n="$SENT_PACKETS" 'BEGIN { if (n == 0) {print "0.0000"} else {printf "%.4f", s/n} }')
  STORED_PPS=$(awk -v s="$STORED_COUNT" -v d="$SENDER_DURATION_SEC" 'BEGIN { if (d == 0) {print "0.00"} else {printf "%.2f", s/d} }')
  expected_header="timestamp,git_commit,scenario,protocol,duration,workers,sent_packets,send_errors,sender_pps,sender_duration_sec,stored_count,stored_pps,store_ratio,invite_count,status,run_dir"

  if [[ -f "$RESULTS_FILE" ]]; then
    local current_header
    current_header=$(head -n 1 "$RESULTS_FILE")
    if [[ "$current_header" != "$expected_header" ]]; then
      local legacy_file
      legacy_file="$BENCH_DIR/results.legacy.$(date +%Y%m%d-%H%M%S).csv"
      mv "$RESULTS_FILE" "$legacy_file"
      echo "INFO: moved legacy results format to $legacy_file"
    fi
  fi

  if [[ ! -f "$RESULTS_FILE" ]]; then
    echo "$expected_header" > "$RESULTS_FILE"
  fi

  PREV_LINE=""
  if [[ -s "$RESULTS_FILE" ]] && [[ $(wc -l < "$RESULTS_FILE") -gt 1 ]]; then
    PREV_LINE=$(tail -n 1 "$RESULTS_FILE")
  fi

  echo "$(date -Iseconds),$git_commit,$BENCH_SCENARIO,$PROTOCOL,$BENCH_DURATION,$BENCH_WORKERS,$SENT_PACKETS,$SEND_ERRORS,$SENDER_PPS,$SENDER_DURATION_SEC,$STORED_COUNT,$STORED_PPS,$STORE_RATIO,$INVITE_COUNT,OK,$RUN_DIR" >> "$RESULTS_FILE"
}

print_summary() {
  echo "[6/6] Summary"
  echo "Run directory:    $RUN_DIR"
  echo "Protocol:         $PROTOCOL"
  echo "Sent packets:     $SENT_PACKETS"
  echo "Send errors:      $SEND_ERRORS"
  echo "Sender throughput:$SENDER_PPS pps"
  echo "Sender duration:  $SENDER_DURATION_SEC s"
  echo "Stored records:   $STORED_COUNT"
  echo "Stored throughput:$STORED_PPS rec/s"
  echo "INVITE records:   $INVITE_COUNT"
  echo "Store ratio:      $STORE_RATIO (info only)"

  if [[ "$STORED_COUNT" -lt "$MIN_STORED_COUNT" ]]; then
    echo "ERROR: stored_count ($STORED_COUNT) is below threshold ($MIN_STORED_COUNT)"
    exit 1
  fi

  local stored_pps_ok
  stored_pps_ok=$(awk -v p="$STORED_PPS" -v m="$MIN_STORED_PPS" 'BEGIN { if (p >= m) print "1"; else print "0" }')
  if [[ "$stored_pps_ok" != "1" ]]; then
    echo "ERROR: stored_pps ($STORED_PPS) is below threshold ($MIN_STORED_PPS)"
    exit 1
  fi

  if [[ -n "${PREV_LINE:-}" ]]; then
    local prev_sender_pps prev_stored_pps sender_delta stored_delta
    IFS=',' read -r _ _ _ prev_protocol _ _ _ _ prev_sender_pps _ _ prev_stored_pps _ _ _ _ <<< "$PREV_LINE"
    if [[ "$prev_protocol" != "$PROTOCOL" ]]; then
      echo "Delta vs previous: skipped (previous protocol=$prev_protocol, current=$PROTOCOL)"
      echo "Results CSV:      $RESULTS_FILE"
      return
    fi

    sender_delta=$(awk -v c="$SENDER_PPS" -v p="$prev_sender_pps" 'BEGIN { if (p == 0) print "0.00"; else printf "%.2f", ((c-p)/p)*100 }')
    stored_delta=$(awk -v c="$STORED_PPS" -v p="$prev_stored_pps" 'BEGIN { if (p == 0) print "0.00"; else printf "%.2f", ((c-p)/p)*100 }')

    echo "Delta vs previous: sender_pps=${sender_delta}% stored_pps=${stored_delta}%"

    local sender_degraded stored_degraded
    sender_degraded=$(awk -v d="$sender_delta" -v t="$DEGRADE_THRESHOLD_PCT" 'BEGIN { if (d < -t) print "1"; else print "0" }')
    stored_degraded=$(awk -v d="$stored_delta" -v t="$DEGRADE_THRESHOLD_PCT" 'BEGIN { if (d < -t) print "1"; else print "0" }')
    if [[ "$sender_degraded" == "1" ]]; then
      echo "WARN: sender throughput degraded by more than ${DEGRADE_THRESHOLD_PCT}%"
    fi
    if [[ "$stored_degraded" == "1" ]]; then
      echo "WARN: storage throughput degraded by more than ${DEGRADE_THRESHOLD_PCT}%"
    fi
  else
    echo "Delta vs previous: n/a (first run)"
  fi

  echo "Results CSV:      $RESULTS_FILE"
}

run_benchmark_once() {
  local scenario="$1"
  local pre_udp_stats post_udp_stats
  local pre_rcvbuf pre_in pre_out
  local post_rcvbuf post_in post_out
  local delta_rcvbuf delta_in delta_out

  BENCH_SCENARIO="$scenario"
  apply_benchmark_scenario "$scenario"
  validate_udp_buffers
  if [[ "$scenario" == "current" ]]; then
    RUN_ID="$(date +%Y%m%d-%H%M%S)"
  else
    RUN_ID="$(date +%Y%m%d-%H%M%S)-$scenario"
  fi

  RUN_DIR="$RUNS_DIR/$RUN_ID"
  mkdir -p "$RUN_DIR/data"

  CFG_PATH="$RUN_DIR/homer-bench.json"
  SERVER_LOG="$RUN_DIR/server.log"
  SENDER_LOG="$RUN_DIR/sender.log"
  SENDER_BIN="$RUN_DIR/hep_sender"

  echo "[0/6] Scenario: $scenario (workers=$INGEST_WORKER_COUNT queue=$INGEST_QUEUE_SIZE batch=$STORAGE_BATCH_SIZE interval=${STORAGE_FLUSH_INTERVAL_SEC}s scripts=$INGEST_SCRIPTS_ENABLE proto=$HEP_BENCH_PROTO_TYPE no_parse=$HEP_NO_PARSE)"
  write_config
  build_binaries
  start_server

  if [[ "$PROTOCOL" == "udp" ]]; then
    pre_udp_stats=$(snapshot_udp_snmp)
    read -r pre_rcvbuf pre_in pre_out <<< "$pre_udp_stats"
  fi

  run_sender
  extract_sender_metrics
  stop_server

  if [[ "$PROTOCOL" == "udp" ]]; then
    post_udp_stats=$(snapshot_udp_snmp)
    read -r post_rcvbuf post_in post_out <<< "$post_udp_stats"
    delta_rcvbuf=$(awk -v p="$post_rcvbuf" -v c="$pre_rcvbuf" 'BEGIN { printf "%d", (p - c) }')
    delta_in=$(awk -v p="$post_in" -v c="$pre_in" 'BEGIN { printf "%d", (p - c) }')
    delta_out=$(awk -v p="$post_out" -v c="$pre_out" 'BEGIN { printf "%d", (p - c) }')

    echo "INFO: UDP SNMP delta: RcvbufErrors=$delta_rcvbuf InDatagrams=$delta_in OutDatagrams=$delta_out"
    if (( delta_rcvbuf > 0 )); then
      echo "WARN: UDP RcvbufErrors increased during run; consider increasing receive buffers"
    else
      echo "INFO: UDP receive buffers did not drop packets due to rcvbuf pressure in this run"
    fi
  fi

  validate_results
  append_results
  print_summary
}

mkdir -p "$RUNS_DIR" "$BIN_DIR"
IFS=',' read -r -a BENCH_SCENARIO_LIST <<< "$BENCH_SCENARIOS"
for scenario in "${BENCH_SCENARIO_LIST[@]}"; do
  scenario="${scenario//[[:space:]]/}"
  [[ -z "$scenario" ]] && continue
  run_benchmark_once "$scenario"
done
