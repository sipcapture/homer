#!/bin/bash

# Script to generate test TLS certificates for homer-core
# Generates self-signed certificates for testing purposes

set -e

CERT_DIR="${1:-./certs}"
DAYS_VALID="${2:-365}"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Generating test TLS certificates...${NC}"
echo "Certificate directory: $CERT_DIR"
echo "Validity period: $DAYS_VALID days"
echo ""

# Create certificate directory if it doesn't exist
mkdir -p "$CERT_DIR"

# Generate private key for CA
echo -e "${YELLOW}Step 1: Generating CA private key...${NC}"
openssl genrsa -out "$CERT_DIR/ca-key.pem" 4096

# Generate CA certificate
echo -e "${YELLOW}Step 2: Generating CA certificate...${NC}"
openssl req -new -x509 -days "$DAYS_VALID" -key "$CERT_DIR/ca-key.pem" \
    -out "$CERT_DIR/ca-cert.pem" \
    -subj "/C=US/ST=Test/L=Test/O=Homer Test CA/CN=Homer Test CA"

# Generate server private key
echo -e "${YELLOW}Step 3: Generating server private key...${NC}"
openssl genrsa -out "$CERT_DIR/server-key.pem" 4096

# Generate server certificate signing request
echo -e "${YELLOW}Step 4: Generating server certificate signing request...${NC}"
openssl req -new -key "$CERT_DIR/server-key.pem" \
    -out "$CERT_DIR/server.csr" \
    -subj "/C=US/ST=Test/L=Test/O=Homer Test Server/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,DNS:*.localhost,IP:127.0.0.1,IP:0.0.0.0"

# Generate server certificate signed by CA
echo -e "${YELLOW}Step 5: Generating server certificate...${NC}"
openssl x509 -req -days "$DAYS_VALID" \
    -in "$CERT_DIR/server.csr" \
    -CA "$CERT_DIR/ca-cert.pem" \
    -CAkey "$CERT_DIR/ca-key.pem" \
    -CAcreateserial \
    -out "$CERT_DIR/server-cert.pem" \
    -extensions v3_req \
    -extfile <(cat <<EOF
[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = *.localhost
IP.1 = 127.0.0.1
IP.2 = 0.0.0.0
EOF
)

# Generate client private key (for mutual TLS testing)
echo -e "${YELLOW}Step 6: Generating client private key...${NC}"
openssl genrsa -out "$CERT_DIR/client-key.pem" 4096

# Generate client certificate signing request
echo -e "${YELLOW}Step 7: Generating client certificate signing request...${NC}"
openssl req -new -key "$CERT_DIR/client-key.pem" \
    -out "$CERT_DIR/client.csr" \
    -subj "/C=US/ST=Test/L=Test/O=Homer Test Client/CN=test-client"

# Generate client certificate signed by CA
echo -e "${YELLOW}Step 8: Generating client certificate...${NC}"
openssl x509 -req -days "$DAYS_VALID" \
    -in "$CERT_DIR/client.csr" \
    -CA "$CERT_DIR/ca-cert.pem" \
    -CAkey "$CERT_DIR/ca-key.pem" \
    -CAcreateserial \
    -out "$CERT_DIR/client-cert.pem" \
    -extensions v3_req \
    -extfile <(cat <<EOF
[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
EOF
)

# Clean up CSR files
rm -f "$CERT_DIR/server.csr" "$CERT_DIR/client.csr"

# Set proper permissions
chmod 600 "$CERT_DIR"/*.pem
chmod 644 "$CERT_DIR"/*.pem 2>/dev/null || true

echo ""
echo -e "${GREEN}✓ Certificates generated successfully!${NC}"
echo ""
echo "Generated files:"
echo "  - CA Certificate:      $CERT_DIR/ca-cert.pem"
echo "  - CA Private Key:       $CERT_DIR/ca-key.pem"
echo "  - Server Certificate:   $CERT_DIR/server-cert.pem"
echo "  - Server Private Key:   $CERT_DIR/server-key.pem"
echo "  - Client Certificate:  $CERT_DIR/client-cert.pem"
echo "  - Client Private Key:  $CERT_DIR/client-key.pem"
echo ""
echo "Example configuration for homer-core:"
echo "  SERVER_SETTINGS:"
echo "    TLS_SERVER:"
echo "      Enable: true"
echo "      Host: 0.0.0.0"
echo "      Port: 9062"
echo "      Cert: $CERT_DIR/server-cert.pem"
echo "      Key: $CERT_DIR/server-key.pem"
echo "      CaCert: $CERT_DIR/ca-cert.pem"
echo "      MutualTLS: false  # Set to true for mutual TLS"
echo ""
echo "For mutual TLS testing, set MutualTLS: true"
echo ""

