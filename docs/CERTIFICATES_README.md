# TLS Certificate Generation for Testing

This document describes how to generate test TLS certificates for homer-core.

## Quick Start

Generate certificates in the default location (`./certs`):

```bash
./generate_test_certs.sh
```

Generate certificates in a custom directory:

```bash
./generate_test_certs.sh /path/to/certs
```

Generate certificates with custom validity period (default is 365 days):

```bash
./generate_test_certs.sh ./certs 730  # 2 years
```

## Generated Files

The script generates the following files:

- **ca-cert.pem** - CA (Certificate Authority) certificate
- **ca-key.pem** - CA private key
- **server-cert.pem** - Server certificate (signed by CA)
- **server-key.pem** - Server private key
- **client-cert.pem** - Client certificate (for mutual TLS testing)
- **client-key.pem** - Client private key

## Configuration Example

After generating certificates, configure homer-core:

```json
{
  "SERVER_SETTINGS": {
    "TLS_SERVER": {
      "Enable": true,
      "Host": "0.0.0.0",
      "Port": 9062,
      "Cert": "./certs/server-cert.pem",
      "Key": "./certs/server-key.pem",
      "CaCert": "./certs/ca-cert.pem",
      "MutualTLS": false,
      "MinTLSVersion": "TLS1.2",
      "MaxTLSVersion": "TLS1.3"
    }
  }
}
```

## Mutual TLS (mTLS)

For mutual TLS testing, set `MutualTLS: true` in the configuration. Clients will need to provide a valid client certificate signed by the CA.

## Testing TLS Connection

### Using the Test Script

The easiest way to test TLS connection:

```bash
# Basic TLS connection test
./scripts/test_tls_connection.sh

# Test with custom host and port
./scripts/test_tls_connection.sh localhost 9062

# Test with custom certificate paths
./scripts/test_tls_connection.sh localhost 9062 ./examples/test_certs/ca-cert.pem
```

### Manual Testing with openssl

Test TLS connection manually:

```bash
# Test server certificate (basic TLS)
openssl s_client -connect localhost:9062 -CAfile ./examples/test_certs/ca-cert.pem

# Test with client certificate (mutual TLS)
openssl s_client -connect localhost:9062 \
  -CAfile ./examples/test_certs/ca-cert.pem \
  -cert ./examples/test_certs/client-cert.pem \
  -key ./examples/test_certs/client-key.pem

# Test specific TLS version
openssl s_client -connect localhost:9062 \
  -CAfile ./examples/test_certs/ca-cert.pem \
  -tls1_2  # or -tls1_3
```

### Sending HEP Packets over TLS

To send a test HEP packet:

```bash
# Send test HEP packet
./scripts/send_hep_over_tls.sh

# With custom parameters
./scripts/send_hep_over_tls.sh localhost 9062 ./examples/test_certs/ca-cert.pem
```

## Security Note

⚠️ **WARNING**: These are self-signed test certificates. **DO NOT** use them in production environments. For production, use certificates from a trusted Certificate Authority (CA) like Let's Encrypt or a commercial CA.

## Certificate Details

- **Key Size**: 4096 bits (strong encryption)
- **Validity**: Configurable (default: 365 days)
- **Subject Alternative Names**: Includes localhost, *.localhost, 127.0.0.1, and 0.0.0.0
- **Key Usage**: Server authentication (server cert), Client authentication (client cert)

