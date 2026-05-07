#!/bin/bash
#
# Quick test script for HEP → Arrow/Parquet pipeline
# Tests the complete flow: send HEP packets → write to Parquet → read via DuckDB
#

set -e

# Configuration
OUTPUT_DIR="/tmp/homer_parquet"
HEP_PORT=9060
NUM_PACKETS=100

echo "=== HEP → Arrow/Parquet Pipeline Test ==="
echo ""

# Clean up previous test data
echo "[1/5] Cleaning up previous test data..."
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# Check if homer-core is running
echo "[2/5] Checking homer-core..."
if ! nc -z localhost $HEP_PORT 2>/dev/null; then
    echo "ERROR: homer-core is not running on port $HEP_PORT"
    echo ""
    echo "Start it with:"
    echo "  cd /home/shurik/Projects/homer-core/src"
    echo "  go build -o homer-core . && ./homer-core -config-path ../examples/homer-core-arrow.json"
    exit 1
fi
echo "  ✓ Server is running on port $HEP_PORT"

# Send HEP packets using embedded Python script
echo "[3/5] Sending $NUM_PACKETS HEP packets..."

python3 << 'PYTHON_SCRIPT'
import socket
import struct
import time
import sys

def create_hep3_packet(src_ip, dst_ip, src_port, dst_port, call_id, method):
    """Create a minimal HEP3 packet with SIP payload"""
    hep_id = b'HEP3'
    chunks = b''
    
    # Protocol Family - IPv4
    chunks += struct.pack('>HHB', 0x0001, 1, 0x02)
    # Protocol ID - UDP
    chunks += struct.pack('>HHB', 0x0002, 1, 17)
    # Source IPv4
    chunks += struct.pack('>HH', 0x0003, 4) + socket.inet_aton(src_ip)
    # Destination IPv4
    chunks += struct.pack('>HH', 0x0004, 4) + socket.inet_aton(dst_ip)
    # Source Port
    chunks += struct.pack('>HHH', 0x0007, 2, src_port)
    # Destination Port
    chunks += struct.pack('>HHH', 0x0008, 2, dst_port)
    # Timestamp (seconds)
    chunks += struct.pack('>HHI', 0x0009, 4, int(time.time()))
    # Timestamp (microseconds)
    chunks += struct.pack('>HHI', 0x000a, 4, int((time.time() % 1) * 1000000))
    # Protocol Type - SIP = 1
    chunks += struct.pack('>HHB', 0x000b, 1, 1)
    # Capture Agent ID
    chunks += struct.pack('>HHI', 0x000c, 4, 2001)
    
    # SIP Payload
    payload = f"""{method} sip:user@example.com SIP/2.0\r
Via: SIP/2.0/UDP {src_ip}:{src_port};branch=z9hG4bK{call_id[:8]}\r
From: <sip:caller@{src_ip}>;tag={call_id[:12]}\r
To: <sip:callee@{dst_ip}>\r
Call-ID: {call_id}\r
CSeq: 1 {method}\r
User-Agent: HomerTestScript/1.0\r
Content-Length: 0\r
\r
"""
    payload_bytes = payload.encode('utf-8')
    chunks += struct.pack('>HH', 0x000f, len(payload_bytes)) + payload_bytes
    
    # Build complete packet
    total_len = 4 + 2 + len(chunks)
    return hep_id + struct.pack('>H', total_len) + chunks

# Create UDP socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

# SIP methods to cycle through
methods = ['INVITE', 'ACK', 'BYE', 'CANCEL', 'REGISTER', 'OPTIONS']

# Send packets
for i in range(100):
    call_id = f"test-{i:04d}-{int(time.time())}@homer.local"
    method = methods[i % len(methods)]
    src_ip = f"192.168.{(i // 256) % 256}.{i % 256}"
    
    packet = create_hep3_packet(src_ip, "10.0.0.1", 5060, 5060, call_id, method)
    sock.sendto(packet, ("127.0.0.1", 9060))
    
    # Progress indicator
    if (i + 1) % 20 == 0:
        sys.stdout.write(f"\r  Sent {i + 1}/100 packets")
        sys.stdout.flush()
    
    time.sleep(0.01)

sock.close()
print(f"\r  ✓ Sent 100 packets                    ")
PYTHON_SCRIPT

# Wait for data to be flushed to parquet files
echo "[4/5] Waiting for data flush (15 seconds)..."
sleep 15

# Check results
echo "[5/5] Checking results..."
echo ""

# Count parquet files
FILE_COUNT=$(find "$OUTPUT_DIR" -name "*.parquet" 2>/dev/null | wc -l)
echo "Parquet files created: $FILE_COUNT"

if [ "$FILE_COUNT" -eq 0 ]; then
    echo ""
    echo "ERROR: No parquet files created!"
    echo "Check homer-core logs for errors."
    exit 1
fi

# Show directory structure
echo ""
echo "Directory structure:"
tree "$OUTPUT_DIR" 2>/dev/null || find "$OUTPUT_DIR" -type f -name "*.parquet"

# Query data with DuckDB if available
echo ""
echo "=== Data Analysis with DuckDB ==="

if command -v duckdb &> /dev/null; then
    echo ""
    echo "Total records:"
    duckdb -c "SELECT COUNT(*) as total FROM read_parquet('$OUTPUT_DIR/**/*.parquet', hive_partitioning=true);"
    
    echo ""
    echo "Records by SIP method:"
    duckdb -c "SELECT event, COUNT(*) as count FROM read_parquet('$OUTPUT_DIR/**/*.parquet', hive_partitioning=true) GROUP BY event ORDER BY count DESC;"
    
    echo ""
    echo "Sample data:"
    duckdb -c "SELECT timestamp, src_ip, dst_ip, event, session_id FROM read_parquet('$OUTPUT_DIR/**/*.parquet', hive_partitioning=true) LIMIT 5;"
else
    echo ""
    echo "DuckDB CLI not found. Install it to query data:"
    echo "  pip install duckdb-cli"
    echo ""
    echo "Or use Python:"
    echo "  pip install duckdb"
    echo "  python3 -c \"import duckdb; print(duckdb.query(\\\"SELECT COUNT(*) FROM read_parquet('$OUTPUT_DIR/**/*.parquet', hive_partitioning=true)\\\").fetchone())\""
fi

echo ""
echo "=== Test Complete ==="
echo ""
echo "Files location: $OUTPUT_DIR"
echo ""
echo "To explore data manually:"
echo "  duckdb -c \"SELECT * FROM read_parquet('$OUTPUT_DIR/**/*.parquet', hive_partitioning=true) LIMIT 100;\""
