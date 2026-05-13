# Homer Search CLI

The `homer search` subcommand lets you search Homer data from the terminal via the coordinator API. It supports multiple output formats including tables, vertical rows, CSV, JSON, ASCII charts, SIP call flow diagrams, and **PCAP** export for SIP payloads.

## Quick Start

```bash
# Search SIP INVITE messages from the last hour
homer search --host 10.0.0.1:8081 --user admin --pass secret --method INVITE

# Interactive TUI mode
homer search --host 10.0.0.1:8081 --user admin --pass secret --interactive
```

## MCP Quick Example (UI Natural Language Search)

If you use the Dashboard UI and want natural-language search, open the `SearchPanel` widget and switch to the `AI` tab.

The UI sends requests to:

- `POST /api/v4/mcp/query`

Example request body:

```json
{
  "query_text": "find all INVITE messages from the last hour",
  "mode": "auto",
  "limit": 100
}
```

### Modes

- `auto` - uses structured search by default, switches to SQL when explicitly requested.
- `structured` - natural language -> `/api/v4/transactions/search`.
- `sql` - natural language -> generated SQL -> `/api/v4/query`.

For full setup and configuration details, see [MCP UI Guide](MCP_UI_GUIDE.md).

## Authentication

On first login, the JWT token is cached in `~/.homer_token` for subsequent requests:

```bash
# First time: login with credentials
homer search --host coordinator:8081 --user admin --pass mypassword

# Subsequent calls: token is reused automatically
homer search --host coordinator:8081 --method INVITE

# Use a pre-existing token directly
homer search --host coordinator:8081 --token "eyJhbGciOiJI..."
```

## Flags Reference

### Connection

| Flag | Description | Default |
|------|-------------|---------|
| `--host <host:port>` | Coordinator address (**required**) | - |
| `--user <name>` | Username for authentication | - |
| `--pass <secret>` | Password for authentication | - |
| `--token <jwt>` | JWT token (skip login) | - |

### Time Range

| Flag | Description | Default |
|------|-------------|---------|
| `--from <spec>` | Start of time range | `1h` |
| `--to <spec>` | End of time range | `now` |

Supported time formats:
- **Duration**: `30m`, `1h`, `2h30m`, `1d`, `7d` (relative to now)
- **ISO 8601**: `2026-02-07T12:00:00Z`
- **Date-time**: `2026-02-07 12:00:00`
- **Date**: `2026-02-07`
- **Epoch ms**: `1707260400000`

### Search Filters

| Flag | Description | Default |
|------|-------------|---------|
| `--proto` | Protocol: name (`sip`, `otlp_traces`, …) or integer `proto_type` | `sip` (same as `1`) |
| `--event-type <type>` | SIP: `call`/`registration`/`default`; OTLP: `default`; LP: `schema__table` | `call` (SIP only) |
| `--method <method>` | SIP method filter | - |
| `--call-id <id>` | Call-ID / session ID (partial match) | - |
| `--from-user <user>` | Caller/from user (partial match) | - |
| `--to-user <user>` | Callee/to user (partial match) | - |
| `--src-ip <ip>` | Source IP (exact match) | - |
| `--dst-ip <ip>` | Destination IP (exact match) | - |
| `--node <name>` | Node name filter | - |
| `--limit <n>` | Maximum results (1-50000) | `50` |

Protocol types (`--proto` accepts the **Name** or the **Proto** integer; hyphens and spaces in names are ignored, case is ignored):

| Name | Proto | Description | event-type |
|------|-------|-------------|------------|
| `sip` | `1` | SIP | `call` (default), `registration`, `default` |
| `rtcp` | `5` | RTCP | `default` |
| `rtp` | `34` | RTP agent reports | `default` |
| `dns` | `35` | DNS | `default` |
| `log` | `100` | Logs | `default` |
| `otlp_traces` | `200` | OTLP Traces | `default` |
| `otlp_metrics` | `201` | OTLP Metrics | `default` |
| `otlp_logs` | `202` | OTLP Logs | `default` |
| `lp` | `300` | Line Protocol | `schema__table` (e.g. `main__cpu`) |

### Output

| Flag | Description | Default |
|------|-------------|---------|
| `--format <fmt>` | Output format (see below) | `table` |
| `--output <path>` | Output file path (**required** with `--format pcap`) | - |
| `-o <path>` | Shorthand for `--output` | - |
| `--fields <list>` | Comma-separated fields to show | all |
| `--grep <pattern>` | Post-filter: include only matching rows | - |
| `--exclude <pattern>` | Post-filter: exclude matching rows | - |
| `--interactive` | Launch interactive TUI | `false` |
| `--debug` | Print request/response to stderr | `false` |

## Post-Filters (--grep / --exclude)

Post-filters are applied client-side after results are returned from the API. They allow you to narrow down results without changing the server query.

**Pattern syntax:**
- **Simple**: `INVITE` -- match if any field contains "INVITE" (case-insensitive)
- **Field-specific**: `event=INVITE` -- match only the "event" field
- **Multiple (OR)**: `INVITE,BYE` -- match if any pattern matches
- **Mixed**: `event=INVITE,event=BYE` -- field-specific with OR logic

```bash
# Show only INVITE messages (any field match)
homer search --host coordinator:8081 --grep INVITE

# Field-specific: only rows where event is INVITE
homer search --host coordinator:8081 --grep event=INVITE

# Show INVITEs and BYEs
homer search --host coordinator:8081 --grep "INVITE,BYE"

# Exclude provisional responses
homer search --host coordinator:8081 --exclude "100,183"

# Combine: INVITEs only, but exclude a specific source IP
homer search --host coordinator:8081 --grep event=INVITE --exclude src_ip=10.0.0.99

# Filter + call flow: show only INVITEs and 200s in the ladder
homer search --host coordinator:8081 --call-id abc123 --grep "INVITE,200" --format callflow

# Filter with field selection
homer search --host coordinator:8081 --grep event=REGISTER --fields timestamp,src_ip,caller --format table
```

A summary is printed to stderr showing how many rows were filtered:

```
Post-filter (grep "INVITE"): 50 → 12 rows
```

## Output Formats

### table (default)

Standard horizontal table:

```bash
homer search --host coordinator:8081 --from 1h --method INVITE
```

```
+---------------------+--------+--------------+--------------+------------------+
| timestamp           | event  | src_ip       | dst_ip       | session_id       |
+---------------------+--------+--------------+--------------+------------------+
| 2026-02-07 20:46:32 | INVITE | 10.1.68.219  | 100.125.0.3  | abc123@host      |
| 2026-02-07 20:46:50 | INVITE | 10.1.68.219  | 100.125.0.3  | def456@host      |
+---------------------+--------+--------------+--------------+------------------+

2 rows in set
```

### vertical (MySQL `\G` style)

Each row displayed vertically, one field per line:

```bash
homer search --host coordinator:8081 --call-id abc123 --format vertical
```

```
*************************** 1. row ***************************
  timestamp: 2026-02-07 20:46:32
      event: INVITE
     src_ip: 10.1.68.219
     dst_ip: 100.125.0.3
 session_id: abc123@host
     caller: +1555123
     callee: +1555456
    payload: INVITE sip:+1555456@example.com SIP/2.0...

1 rows in set
```

### csv

Standard CSV for piping to other tools:

```bash
homer search --host coordinator:8081 --from 3h --format csv > results.csv
homer search --host coordinator:8081 --from 3h --format csv | column -t -s,
```

### json

Full JSON array output:

```bash
homer search --host coordinator:8081 --from 1h --format json | jq '.[0]'
homer search --host coordinator:8081 --from 1h --format json | jq '.[] | .session_id'
```

### chart

ASCII histogram of events over time:

```bash
homer search --host coordinator:8081 --from 24h --limit 5000 --format chart
```

```
 150 ┤                     ╭╮
 140 ┤                    ╭╯│
 130 ┤                   ╭╯ ╰╮
 120 ┤        ╭╮        ╭╯   │
 110 ┤       ╭╯╰╮      ╭╯    ╰╮
 100 ┤      ╭╯  │     ╭╯      │
  90 ┤     ╭╯   ╰╮   ╭╯       ╰╮
  80 ┤    ╭╯     │  ╭╯          │
  70 ┤   ╭╯      ╰╮╭╯           ╰╮
  60 ┤  ╭╯        ╰╯              │
  50 ┤ ╭╯                         ╰╮
  40 ┤╭╯                            ╰╮
  30 ┤╯                               ╰╮
  20 ┤                                  ╰──
       Events per 5m0s  |  2026-02-07 08:00 .. 2026-02-08 08:00  |  Total: 4521
```

### callflow (SIP ladder diagram)

ASCII call flow like sngrep/Homer UI:

```bash
homer search --host coordinator:8081 --call-id abc123 --from 3h --format callflow --limit 500
```

```
                    10.1.68.219:5060      100.125.0.3:5060      10.1.1.100:5060
          ----------+---------------------+---------------------+----------
                    |                     |                     |
20:46:32.140        |    INVITE (SDP)     |                     |
                    |-------------------->|                     |
                    |                     |                     |
20:46:32.145        |    100 Trying       |                     |
                    |<--------------------|                     |
                    |                     |                     |
20:46:32.200        |                     |      INVITE         |
                    |                     |-------------------->|
                    |                     |                     |
20:46:32.350        |                     |     180 Ringing     |
                    |                     |<--------------------|
                    |                     |                     |
20:46:32.351        |    180 Ringing      |                     |
                    |<--------------------|                     |
                    |                     |                     |
20:46:34.100        |                     |      200 OK         |
                    |                     |<--------------------|
                    |                     |                     |
20:46:34.102        |     200 OK          |                     |
                    |<--------------------|                     |
                    |                     |                     |
20:46:34.150        |       ACK           |                     |
                    |-------------------->|                     |
                    |                     |                     |

12 messages, 3 endpoints
Call-IDs: abc123@host
```

Also accepts aliases: `--format flow` or `--format ladder`.

### pcap (SIP capture file)

Writes a **libpcap** file from the search result rows. Framing matches the coordinator API **`POST /api/v4/transactions/export/pcap`** (Ethernet + IPv4/IPv6 + UDP + raw SIP payload per row).

**Requirements:**

- **`--output path`** or **`-o path`** — required (binary output is never written to stdout).
- **SIP only** — use default `--proto` / `sip` / `1`; other protocols are rejected.
- **Not available with `--interactive`** — use a one-shot command.
- Each exported row needs a non-empty **`payload`** plus **`src_ip`** / **`dst_ip`** (and optional **`src_port`** / **`dst_port`**; zero ports default to **5060**). Rows are sorted by **`timestamp`** ascending before writing.

Works with structured search or **`--sql`**, as long as returned columns include those fields.

```bash
# Export one call to Wireshark/tcpdump
homer search --host coordinator:8081 --call-id 'abc123@host' --limit 500 --format pcap -o /tmp/call.pcap

# Same with long flag
homer search --host coordinator:8081 --user admin --pass secret \
  --from 30m --method INVITE --limit 200 --format pcap --output captures/invite-sample.pcap
```

If no row contains a SIP payload after filters, the command fails with a clear error.

## Field Selection

Show only specific columns with `--fields`:

```bash
# Show only key SIP fields
homer search --host coordinator:8081 --fields timestamp,event,src_ip,dst_ip,session_id

# Minimal output: just time and method
homer search --host coordinator:8081 --fields timestamp,event

# Call-ID list only
homer search --host coordinator:8081 --fields session_id --from 1h | sort -u
```

## Search Examples

### Basic SIP Searches

```bash
# All SIP calls in the last hour
homer search --host coordinator:8081

# INVITE messages only
homer search --host coordinator:8081 --method INVITE

# REGISTER messages
homer search --host coordinator:8081 --method REGISTER --event-type registration

# BYE messages in the last 24 hours
homer search --host coordinator:8081 --method BYE --from 24h
```

### Search by Call-ID

```bash
# Find all messages for a specific call (searches session_id and cid)
homer search --host coordinator:8081 --call-id "abc123@10.0.0.1" --limit 500

# Show call as call flow diagram
homer search --host coordinator:8081 --call-id "abc123@10.0.0.1" --format callflow --limit 500

# Show call with full message details
homer search --host coordinator:8081 --call-id "abc123@10.0.0.1" --format vertical
```

### Search by User

```bash
# Find calls from a specific user
homer search --host coordinator:8081 --from-user "+1555123"

# Find calls to a specific user
homer search --host coordinator:8081 --to-user "bob"

# Calls between two users
homer search --host coordinator:8081 --from-user alice --to-user bob
```

### Search by IP Address

```bash
# Traffic from a specific source
homer search --host coordinator:8081 --src-ip 10.1.68.219

# Traffic to a specific destination
homer search --host coordinator:8081 --dst-ip 100.125.0.3

# Traffic between two IPs
homer search --host coordinator:8081 --src-ip 10.1.68.219 --dst-ip 100.125.0.3
```

### Time Range Examples

```bash
# Last 30 minutes
homer search --host coordinator:8081 --from 30m

# Last 3 hours
homer search --host coordinator:8081 --from 3h

# Last 7 days
homer search --host coordinator:8081 --from 7d --limit 1000

# Specific time window
homer search --host coordinator:8081 --from "2026-02-07T08:00:00" --to "2026-02-07T12:00:00"

# Since a specific date
homer search --host coordinator:8081 --from "2026-02-07"
```

### Non-SIP Protocol Searches

```bash
# RTCP quality reports
homer search --host coordinator:8081 --proto rtcp --event-type default --from 1h

# RTP agent reports
homer search --host coordinator:8081 --proto rtp --event-type default

# DNS queries
homer search --host coordinator:8081 --proto dns --event-type default

# Application logs
homer search --host coordinator:8081 --proto log --event-type default
```

### OTLP Searches (Traces / Metrics / Logs)

OTLP signals are stored in dedicated tables (`otlp_traces`, `otlp_metrics`, `otlp_logs`) and accessed via virtual proto types 200-202. The `--event-type` is always `default` for all three (or can be left blank).

```bash
# All traces from the last hour
homer search --host coordinator:8081 --proto otlp_traces --from 1h

# Traces for a specific trace_id  (--call-id maps to trace_id)
homer search --host coordinator:8081 --proto otlp_traces --call-id "a1b2c3d4e5f60718a9b0c1d2e3f40516"

# Traces filtered by service name  (--user-agent maps to service_name)
homer search --host coordinator:8081 --proto otlp_traces --from 2h --user-agent "payment-service"

# Error spans only (use --sql for direct column access)
homer search --host coordinator:8081 --sql "SELECT timestamp,trace_id,name,status_message FROM default.otlp_traces WHERE status_code=2 AND timestamp >= now()-INTERVAL 1 HOUR ORDER BY timestamp DESC LIMIT 50"

# All metrics from the last 30 minutes
homer search --host coordinator:8081 --proto otlp_metrics --from 30m

# Metrics filtered by metric name  (--call-id maps to name LIKE)
homer search --host coordinator:8081 --proto otlp_metrics --from 1h --call-id "http.server.duration"

# Metrics for a specific service
homer search --host coordinator:8081 --proto otlp_metrics --from 1h --user-agent "api-gateway"

# All log records from the last hour
homer search --host coordinator:8081 --proto otlp_logs --from 1h

# Logs containing "error" in body or raw  (--payload maps to body/raw LIKE)
homer search --host coordinator:8081 --proto otlp_logs --from 30m --payload "error"

# Logs linked to a specific trace  (--call-id maps to trace_id)
homer search --host coordinator:8081 --proto otlp_logs --call-id "a1b2c3d4e5f60718a9b0c1d2e3f40516"

# Interactive TUI with OTLP traces pre-selected
homer search --host coordinator:8081 --proto otlp_traces --interactive
```

**OTLP filter mapping** — how generic CLI flags translate to OTLP columns:

| CLI flag | traces | metrics | logs |
|----------|--------|---------|------|
| `--call-id` | `trace_id =` | `name LIKE` | `trace_id =` |
| `--payload` | `name LIKE` + `raw LIKE` | `name LIKE` + `raw LIKE` | `body LIKE` + `raw LIKE` |
| `--user-agent` | `service_name LIKE` | `service_name LIKE` | `service_name LIKE` |
| SIP fields (`--method`, `--src-ip`, …) | ignored | ignored | ignored |

For full column reference see [`OTLP.md`](./OTLP.md#storage-layout).

### Line Protocol (LP) Searches

Line Protocol data uses `--proto lp`. The `--event-type` flag selects the target table using the `schema__table` convention (double underscore separates DuckDB schema from table name).

```bash
# Query a specific LP measurement (schema=main, table=cpu)
homer search --host coordinator:8081 --proto lp --event-type "main__cpu" --from 15m

# Free-text search within any LP table
homer search --host coordinator:8081 --proto lp --event-type "main__mem" --payload "host=web01"

# Interactive TUI for LP
homer search --host coordinator:8081 --proto lp --event-type "main__cpu" --interactive
```

For available LP tables use `/api/v4/line_protocol/tables` or statistics endpoints; with the default (empty) `table_prefix`, table names match measurements (not necessarily `lp_%`).

### Raw SQL Search

Use `--sql` to send an arbitrary SQL query directly to the coordinator. This bypasses all structured filters and gives full control over table names and columns.

```bash
# Query any table directly
homer search --host coordinator:8081 --sql "SELECT * FROM default.otlp_traces LIMIT 10"

# Cross-signal join: errors with linked logs
homer search --host coordinator:8081 --sql "
  SELECT t.trace_id, t.name, l.body
  FROM default.otlp_traces t
  JOIN default.otlp_logs l ON t.trace_id = l.trace_id
  WHERE t.status_code = 2
  ORDER BY t.timestamp DESC LIMIT 20
"

# Aggregation over LP data
homer search --host coordinator:8081 --sql "
  SELECT date_trunc('minute', time) AS minute, AVG(cpu_usage_user) AS avg_cpu
  FROM default.main.cpu
  WHERE time >= now() - INTERVAL 1 HOUR
  GROUP BY 1 ORDER BY 1
" --format chart

# Raw SQL result as JSON for scripting
homer search --host coordinator:8081 --sql "SELECT service_name, COUNT(*) as cnt FROM default.otlp_traces GROUP BY 1 ORDER BY 2 DESC" --format json
```

### Output Piping

```bash
# Export to CSV file
homer search --host coordinator:8081 --from 24h --limit 5000 --format csv > calls.csv

# Extract unique Call-IDs
homer search --host coordinator:8081 --from 1h --format json | jq -r '.[].session_id' | sort -u

# Count calls per source IP
homer search --host coordinator:8081 --from 1h --format csv | awk -F, 'NR>1{print $4}' | sort | uniq -c | sort -rn

# Pipe to less for large results
homer search --host coordinator:8081 --from 24h --limit 5000 --format vertical | less
```

### Debugging

```bash
# Show full HTTP request and response
homer search --host coordinator:8081 --debug

# Debug output goes to stderr, results to stdout
homer search --host coordinator:8081 --debug --format json 2>debug.log > results.json
```

## Interactive TUI Mode

Launch with `--interactive` (or `-interactive`) for a full-screen search form:

```bash
homer search --host coordinator:8081 --interactive
```

### TUI Controls

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate between fields |
| `Enter` | Execute search |
| `Esc` | Back to form / quit |
| `f` | Cycle output format (table -> vertical -> csv -> json -> chart -> callflow) |
| `r` | Re-run search |

All CLI flags are pre-filled in the TUI when provided:

```bash
# Open TUI with pre-filled filters
homer search --host coordinator:8081 --method INVITE --from 3h --format callflow --interactive
```

## Available Data Fields

### SIP (proto=1)

| Field | Description |
|-------|-------------|
| `uuid` | Unique message identifier |
| `timestamp` | Message timestamp |
| `session_id` | SIP Call-ID |
| `caller` | From user |
| `callee` | To user |
| `src_ip` | Source IP address |
| `dst_ip` | Destination IP address |
| `src_port` | Source port |
| `dst_port` | Destination port |
| `event` | SIP method or response (INVITE, 200, BYE, etc.) |
| `proto_type` | HEP protocol type |
| `protocol` | Transport protocol (UDP, TCP, TLS) |
| `node_id` | Capture node identifier |
| `cid` | Correlation ID |
| `payload` | Raw SIP message body |
| `data_extra` | Extra metadata |

### OTLP Traces (proto=200)

| Field | Description |
|-------|-------------|
| `timestamp` | Span start time |
| `end_timestamp` | Span end time |
| `duration_ns` | Span duration in nanoseconds |
| `trace_id` | Hex-encoded 16-byte trace id |
| `span_id` | Hex-encoded 8-byte span id |
| `parent_span_id` | Parent span id (empty for root spans) |
| `name` | Span name |
| `kind` | OTel `SpanKind` integer |
| `status_code` | `0`=unset, `1`=ok, `2`=error |
| `status_message` | Optional status description |
| `service_name` | From resource attribute `service.name` |
| `scope_name` | Instrumentation scope name |
| `resource_attrs` | JSON resource attribute map |
| `span_attrs` | JSON span attribute map |
| `raw` | Full OTLP span document (JSON) |

### OTLP Metrics (proto=201)

| Field | Description |
|-------|-------------|
| `timestamp` | Sample timestamp |
| `name` | Metric / instrument name |
| `description` | Instrument description |
| `unit` | Instrument unit |
| `type` | `Gauge`/`Sum`/`Histogram`/`Summary`/`ExponentialHistogram` |
| `value_double` | Float-valued sample |
| `value_int` | Integer-valued sample |
| `service_name` | From resource attribute `service.name` |
| `scope_name` | Instrumentation scope name |
| `attributes` | JSON per-point attribute map |
| `raw` | Full OTLP metric document (JSON, preserves histogram buckets) |

### OTLP Logs (proto=202)

| Field | Description |
|-------|-------------|
| `timestamp` | Event time |
| `observed_timestamp` | Wall-clock observation time |
| `severity_number` | OTel severity integer (1..24) |
| `severity_text` | `TRACE`/`DEBUG`/`INFO`/`WARN`/`ERROR`/`FATAL` |
| `body` | Stringified log body (fast `LIKE` search) |
| `body_json` | Structured body (map/array) |
| `trace_id` | Linked trace id (hex) |
| `span_id` | Linked span id (hex) |
| `service_name` | From resource attribute `service.name` |
| `attributes` | JSON log record attribute map |
| `raw` | Full OTLP log record (JSON) |

### Line Protocol (proto=300)

Column names depend on the specific measurement schema. Common fields:

| Field | Description |
|-------|-------------|
| `time` | Timestamp (nanosecond epoch) |
| `<tag_name>` | LP tag columns (indexed strings) |
| `<field_name>` | LP field columns (numeric or string values) |

Use `--sql` for direct column introspection: `DESCRIBE main.cpu`.
