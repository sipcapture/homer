#!/bin/bash

# Script to test TLS connection to homer-core TLS endpoint

set -e

HOST="${1:-localhost}"
PORT="${2:-9062}"
CA_CERT="${3:-./examples/test_certs/ca-cert.pem}"
CLIENT_CERT="${4:-./examples/test_certs/client-cert.pem}"
CLIENT_KEY="${5:-./examples/test_certs/client-key.pem}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Testing TLS Connection ===${NC}"
echo "Host: $HOST"
echo "Port: $PORT"
echo ""

# Check if CA cert exists
if [ ! -f "$CA_CERT" ]; then
    echo -e "${RED}Error: CA certificate not found: $CA_CERT${NC}"
    echo "Generate certificates first: ./generate_test_certs.sh"
    exit 1
fi

# Test 1: Basic TLS connection (without client certificate)
echo -e "${YELLOW}Test 1: Basic TLS connection (server certificate only)${NC}"
echo "Command: openssl s_client -connect $HOST:$PORT -CAfile $CA_CERT"
echo ""

if openssl s_client -connect "$HOST:$PORT" -CAfile "$CA_CERT" </dev/null 2>&1 | grep -q "Verify return code: 0"; then
    echo -e "${GREEN}✓ TLS connection successful${NC}"
else
    echo -e "${RED}✗ TLS connection failed${NC}"
    echo ""
    echo "Trying without CA verification..."
    openssl s_client -connect "$HOST:$PORT" -verify_return_error </dev/null 2>&1 | head -20
fi

echo ""
echo ""

# Test 2: TLS connection with client certificate (mutual TLS)
if [ -f "$CLIENT_CERT" ] && [ -f "$CLIENT_KEY" ]; then
    echo -e "${YELLOW}Test 2: TLS connection with client certificate (mutual TLS)${NC}"
    echo "Command: openssl s_client -connect $HOST:$PORT -CAfile $CA_CERT -cert $CLIENT_CERT -key $CLIENT_KEY"
    echo ""
    
    if openssl s_client -connect "$HOST:$PORT" -CAfile "$CA_CERT" -cert "$CLIENT_CERT" -key "$CLIENT_KEY" </dev/null 2>&1 | grep -q "Verify return code: 0"; then
        echo -e "${GREEN}✓ Mutual TLS connection successful${NC}"
    else
        echo -e "${YELLOW}⚠ Mutual TLS connection test (server may not require client cert)${NC}"
    fi
else
    echo -e "${YELLOW}Test 2: Skipped (client certificate not found)${NC}"
fi

echo ""
echo ""

# Test 3: Check TLS version support
echo -e "${YELLOW}Test 3: Checking TLS version support${NC}"
for version in "-tls1_2" "-tls1_3"; do
    echo -n "Testing TLS version $version: "
    if openssl s_client -connect "$HOST:$PORT" -CAfile "$CA_CERT" $version </dev/null 2>&1 | grep -q "Protocol.*TLS"; then
        echo -e "${GREEN}Supported${NC}"
    else
        echo -e "${RED}Not supported${NC}"
    fi
done

echo ""
echo -e "${GREEN}=== TLS Connection Test Complete ===${NC}"

