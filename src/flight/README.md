# Homer Server Flight Integration

Arrow Flight server for realtime queries to HEP data via DuckDB.

## Overview

The Flight server enables SQL queries to HEP packets in real-time through DuckDB using the [Airport Extension](https://github.com/Query-farm/duckdb-airport-extension).

## Architecture

```
HEP packets (UDP/TCP/HTTP)
        │
        ▼
   Homer Server
        │
        ├──► Flight Server (gRPC :50051)
        │         │
        │         └──► DuckDB (Airport Extension)
        │                  │
        │                  └──► SQL Queries
        │
        └──► Parquet Files (archive)
```

## Configuration

```json
{
  "server_settings": {
    "flight_server": {
      "enable": true,
      "host": "0.0.0.0",
      "port": 50051,
      "auth_token": "",
      "buffer_size": 100000,
      "max_message_size": 16777216
    }
  }
}
```

| Parameter | Description | Default |
|-----------|-------------|---------|
| `enable` | Enable Flight server | `false` |
| `host` | Listen address | `0.0.0.0` |
| `port` | gRPC port | `50051` |
| `auth_token` | Bearer token for authentication (empty = no auth) | `""` |
| `buffer_size` | Ring buffer size (number of packets in memory) | `100000` |
| `max_message_size` | Maximum gRPC message size (bytes) | `16777216` (16MB) |

## Usage with DuckDB

### Installing Airport Extension

```sql
INSTALL airport FROM community;
LOAD airport;
```

### Connecting to Homer Server

```sql
-- Without authentication
ATTACH '' AS homer (TYPE AIRPORT, LOCATION 'grpc://localhost:50051');

-- With authentication
CREATE PERSISTENT SECRET homer_auth (
    TYPE airport,
    auth_token 'your-secret-token',
    scope 'grpc://localhost:50051'
);
ATTACH '' AS homer (TYPE AIRPORT, LOCATION 'grpc://localhost:50051');
```

### Available Tables

| Table | Description |
|-------|-------------|
| `homer.hep.packets` | All HEP packets |
| `homer.hep.sip` | SIP packets only (proto_type=1) |
| `homer.hep.rtcp` | RTCP packets only (proto_type=5) |
| `homer.hep.logs` | Log packets only (proto_type=100) |

### Example Queries

```sql
-- Last 100 SIP packets
SELECT * FROM homer.hep.sip LIMIT 100;

-- Packet count by type
SELECT proto_type, COUNT(*) as cnt 
FROM homer.hep.packets 
GROUP BY proto_type;

-- SIP calls from specific user
SELECT sip_call_id, sip_method, src_ip, dst_ip, timestamp
FROM homer.hep.sip
WHERE sip_from_user = 'alice'
ORDER BY timestamp DESC;

-- Statistics by node_id
SELECT node_id, node_name, COUNT(*) as packets
FROM homer.hep.packets
GROUP BY node_id, node_name
ORDER BY packets DESC;

-- Search by Call-ID
SELECT * FROM homer.hep.sip
WHERE sip_call_id LIKE 'abc123%';
```

## Data Schema

Each table has the following columns:

### Basic HEP Fields
| Column | Type | Description |
|--------|------|-------------|
| `version` | UINT32 | HEP protocol version |
| `protocol` | UINT32 | IP protocol (17=UDP, 6=TCP) |
| `src_ip` | STRING | Source IP address |
| `dst_ip` | STRING | Destination IP address |
| `src_port` | UINT32 | Source port |
| `dst_port` | UINT32 | Destination port |
| `tsec` | UINT32 | Unix timestamp (seconds) |
| `tmsec` | UINT32 | Microseconds |
| `proto_type` | UINT32 | Protocol type (1=SIP, 5=RTCP, 100=LOG) |
| `node_id` | UINT32 | Capture node ID |
| `node_pw` | STRING | Node password |
| `payload` | STRING | Packet body |
| `cid` | STRING | Correlation ID |
| `vlan` | UINT32 | VLAN ID |
| `node_name` | STRING | Node name |
| `target_name` | STRING | Target name |
| `sid` | STRING | Session ID |
| `timestamp` | INT64 | Timestamp in nanoseconds |

### SIP Fields (nullable)
| Column | Type | Description |
|--------|------|-------------|
| `sip_call_id` | STRING | Call-ID |
| `sip_method` | STRING | SIP method (INVITE, BYE, etc.) |
| `sip_from_user` | STRING | From user |
| `sip_from_host` | STRING | From host |
| `sip_to_user` | STRING | To user |
| `sip_to_host` | STRING | To host |
| `sip_cseq` | STRING | CSeq |
| `sip_user_agent` | STRING | User-Agent |

## Performance

- **Ring buffer**: Data stored in memory (default 100k packets)
- **Lock-free reads**: Uses RWMutex for efficient access
- **Zero-copy**: Arrow format minimizes data copying
- **gRPC streaming**: Efficient transfer of large data volumes

## Requirements

- DuckDB v1.1.0+ with Airport Extension
- Go 1.25+
- github.com/hugr-lab/airport-go v0.1.3+
