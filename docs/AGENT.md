# Homer Agent CLI

The `homer agent` subcommand lets you query a running **heplify** instance remotely from the terminal — display packet capture statistics, check health, or watch metrics in real time.

## Quick Start

```bash
# Show capture statistics (default action)
homer agent --host 127.0.0.1:8008

# Show health status
homer agent health --host 127.0.0.1:8008

# Continuously refresh stats every 3 seconds
homer agent watch --host 127.0.0.1:8008
```

## Actions

| Action   | Description                                      |
|----------|--------------------------------------------------|
| `stats`  | Show packet capture statistics (default)         |
| `health` | Show agent health status                         |
| `watch`  | Continuously refresh stats (Ctrl+C to stop)      |

## Flags

| Flag           | Default           | Description                             |
|----------------|-------------------|-----------------------------------------|
| `--host`       | `127.0.0.1:8008`  | heplify API address (`host:port`)       |
| `--user`       |                   | Username for Basic Auth                 |
| `--pass`       |                   | Password for Basic Auth                 |
| `--interval`   | `3s`              | Refresh interval for `watch` mode       |
| `--format`     | `table`           | Output format: `table` or `json`        |

## Output Examples

### `homer agent stats`

```
heplify @ 127.0.0.1:8008

-- Node
  node_name   prod-sbc-01
  node_id     0
  interfaces  eth0, eth1
  uptime      2h 14m 37s

-- Packets
  total       1423018
  sip         142301
  rtcp        8210
  rtcp_fail   0
  rtp         1272507
  dns         0
  log         0
  hep_sent    142301
  duplicates  0
  unknown     0

-- Transport
+--------------------------+-------+-----------+------------+--------+--------+
| addr                     | proto | connected | reconnects | sent   | errors |
+--------------------------+-------+-----------+------------+--------+--------+
| 10.0.0.1:9060            | tcp   | yes       | 0          | 142301 | 0      |
+--------------------------+-------+-----------+------------+--------+--------+
```

### `homer agent health`

```
heplify @ 127.0.0.1:8008  status=ok        connected_transports=1  queue=0  buffer=0 B
```

If heplify has no connected HEP transports, status changes to `DEGRADED`:

```
heplify @ 127.0.0.1:8008  status=DEGRADED  connected_transports=0  queue=512  buffer=4.2 MB
```

### `homer agent watch`

Clears the terminal and refreshes stats on every interval tick:

```
heplify watch @ 127.0.0.1:8008  [14:32:07]  (Ctrl+C to stop)

heplify @ 127.0.0.1:8008

-- Node
  ...

-- Packets
  total       1428419
  ...
```

### `homer agent stats --format json`

Returns the raw JSON response from heplify's `/api/stats` endpoint — useful for scripting:

```bash
homer agent stats --format json --host 10.0.0.5:8008 | jq '.packets.sip'
142301
```

## Authentication

heplify's `/api/stats` endpoint can be protected with HTTP Basic Auth via the `api_settings` config block:

```json
{
  "api_settings": {
    "addr": "0.0.0.0:8008",
    "user": "admin",
    "pass": "secret"
  }
}
```

Pass credentials to `homer agent`:

```bash
homer agent stats --host 10.0.0.5:8008 --user admin --pass secret
homer agent watch --host 10.0.0.5:8008 --user admin --pass secret --interval 5s
```

**HTTPS / self-signed TLS:** same as `homer search` — certificate verification is on by default. For self-signed heplify or coordinator certs, set **`HOMER_INSECURE_TLS=1`** before running the command. See [SEARCH.md — TLS to the coordinator](SEARCH.md#tls-to-the-coordinator).

Note: the `/health` endpoint is always open (no auth required).

## heplify API Endpoints

| Endpoint     | Auth     | Description                         |
|--------------|----------|-------------------------------------|
| `GET /health`     | none | Health status, connected transports |
| `GET /api/stats`  | optional Basic Auth | Full capture statistics |

The API port is configured in heplify via the `-api` flag or `api_settings.addr` in the config file (default: `0.0.0.0:8008`).

## Multi-node Monitoring

Query multiple heplify instances in a shell loop:

```bash
for host in 10.0.0.1:8008 10.0.0.2:8008 10.0.0.3:8008; do
  echo "=== $host ==="
  homer agent stats --host "$host" --format json | jq '{
    node: .node_name,
    uptime: .uptime,
    total: .packets.total,
    hep_sent: .packets.hep_sent
  }'
done
```
