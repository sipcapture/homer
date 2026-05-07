#!/bin/bash

# Script to send a test HEP packet over TLS connection

set -e

HOST="${1:-localhost}"
PORT="${2:-9062}"
CA_CERT="${3:-./examples/test_certs/ca-cert.pem}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Sending HEP Packet over TLS ===${NC}"
echo "Host: $HOST"
echo "Port: $PORT"
echo ""

# Create a simple HEP packet (version 2, protocol UDP, minimal size)
# HEP header: 6 bytes
# Bytes 0-1: Protocol Family (IPv4 = 2)
# Bytes 2-3: Protocol ID (UDP = 17)
# Bytes 4-5: Length (big endian)

HEP_HEADER="\x02\x00\x00\x11\x00\x06"  # Version 2, IPv4, UDP, length 6
HEP_PACKET="${HEP_HEADER}"

echo -e "${YELLOW}Sending HEP packet over TLS...${NC}"

# Send packet using openssl s_client
echo -n "$HEP_PACKET" | openssl s_client -connect "$HOST:$PORT" -CAfile "$CA_CERT" -quiet 2>/dev/null

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Packet sent successfully${NC}"
else
    echo -e "${YELLOW}⚠ Connection closed (this is normal for HEP)${NC}"
fi

echo ""
echo "Check server logs to verify packet was received"

