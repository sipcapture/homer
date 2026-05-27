# Live HEP Stream

homer-core exposes a **WebSocket** feed of decoded HEP events, fed from
an in-process circular buffer that sits on the same hot path as the
storage writer. Clients (the built-in UI, third-party dashboards, game
widgets, ops tools) can subscribe in near real-time without touching
DuckLake.

## Architecture

```
                  ┌──────────────────────────────────────┐
                  │                 node                 │
   HEP pkts  ─▶   │  writer.worker                       │
                  │     │                                │
                  │     ├─▶ broker.Publish (non-blocking)│
                  │     │        │                       │
                  │     │        ▼                       │
                  │     │   ring buffer (N events)       │
                  │     │        │                       │
                  │     ▼        ▼                       │
                  │  DuckLake   subscribers ─── /stream ─┼─WS─┐
                  └──────────────────────────────────────┘    │
                                                              │
                  ┌──────────────────────────────────────┐    │
                  │            coordinator               │    │
   browser ──WS─▶ │  /api/v4/stream/hep (JWT)            │    │
                  │     │                                │    │
                  │     ▼                                │    │
                  │  StreamService ─── local broker*     │    │
                  │     │                                │    │
                  │     └── WS fan-out to every node ────┘◀───┘
                  └──────────────────────────────────────┘

  *used when coordinator and writer run in the same process
```

Two endpoints exist:

| Endpoint                        | Auth            | Purpose                                              |
|--------------------------------|-----------------|------------------------------------------------------|
| `GET /stream` (node port)      | none (firewall) | Node-local fan-out. Intended for coordinators / ops. |
| `GET /api/v4/stream/hep`        | JWT             | UI-facing aggregate; fans in from every node.        |

## Configuration

### Ingest-side (per node running `writer`)

```yaml
ingest:
  hep_stream:
    enable: false            # off by default
    buffer_size: 10000       # ring buffer capacity, events
    max_subscribers: 32      # hard cap per node
    per_sub_queue_len: 256   # backpressure bound per subscriber
    rate_per_sub_pps: 500    # 0 = unlimited; per-subscriber shaping
```

Publish is **non-blocking**: when a subscriber queue is full the event
is dropped for *that* subscriber only, never for the writer/DuckLake
pipeline. Drop counters are exported via metrics.

### Coordinator-side

```yaml
coordinator:
  hep_stream:
    enable: false            # must be on to expose /api/v4/stream/hep
    allow_payload: false     # hard switch for ?include_payload=1
    fan_out_timeout_ms: 2000 # WS handshake timeout per node
    history_limit: 200       # max ?history= clients can ask for
```

When coordinator and writer share a process (single-node deploys) the
coordinator subscribes directly to the in-memory broker and skips the
WebSocket round-trip to localhost.

## Query parameters

Both endpoints accept the same filter. Each param except `history` can
be repeated.

| Param              | Meaning                                                                 | Default        |
|--------------------|-------------------------------------------------------------------------|----------------|
| `proto=N`          | HEP protocol id (1=SIP, 5=RTCP, 6=RTP, 34=RTCPXR, …).                   | all            |
| `method=NAME`      | SIP method filter (INVITE, REGISTER, BYE, …). Ignored for non-SIP.      | all            |
| `only_requests=1`  | Drop SIP responses.                                                     | off            |
| `include_payload=1`| Include the raw HEP payload in each frame (see "payload policy").       | off            |
| `history=N`        | On connect, replay up to N recently buffered events matching the filter.| 0              |

### Payload policy

Metadata is always streamed. The raw payload (`payload` field) is only
included when **all** of the following hold:

1. Client sends `?include_payload=1`.
2. `coordinator.hep_stream.allow_payload` is `true`.
3. The JWT has admin rights (`Admin=true`).

Otherwise the flag is silently stripped and payload is omitted.

## Auth

`/api/v4/stream/hep` is behind the standard v4 JWT middleware. Browsers
cannot set an `Authorization` header on a WebSocket handshake, so the
server additionally accepts `?access_token=<jwt>`. Tokens are still
validated the same way — short-lived, same secret, same claims.

## Frame format

The server streams newline-independent JSON frames. Each frame is a
single decoded HEP event:

```json
{
  "ts": 1713379200123,
  "proto": 1,
  "src_ip": "10.0.0.5",
  "src_port": 5060,
  "dst_ip": "10.0.0.1",
  "dst_port": 5060,
  "node_id": 2001,
  "sip": {
    "method": "INVITE",
    "resp_code": "",
    "resp_text": "",
    "callid": "abc@host",
    "cseq": "1 INVITE",
    "from_user": "alice",
    "to_user": "bob",
    "ruri_user": "bob"
  },
  "payload": "INVITE sip:bob@… SIP/2.0\r\n…"
}
```

- `ts` — milliseconds since epoch, source clock from the HEP header.
- `proto` — HEP protocol type.
- `sip` — present only when proto=1 and the decoder parsed a SIP
  message. Non-SIP events carry metadata only.
- `resp_code` — empty for SIP requests, a 3-digit status for responses.
- `payload` — omitted unless the client + server + JWT all allow it.

Clients should **not** assume fields exist — add fields server-side are
backward compatible, but any given field can be missing on a given
event.

## Backpressure and drops

Per-subscriber shaping happens in three places:

1. **Ring buffer overflow** — the oldest event is dropped before newer
   ones. Never blocks ingest.
2. **Subscriber queue full** — event is dropped *for that subscriber*
   and `droppedBackpressureN` increments. The pipeline keeps flowing.
3. **Rate limiter** — when `rate_per_sub_pps > 0`, over-budget events
   increment `droppedRateN`.

Distributed deploys add one more class: the coordinator-to-node WS may
drop/reconnect; the `StreamService` backs off exponentially and clients
see a brief gap in the stream rather than an error.

## Client SDK (UI)

The UI ships a thin WebSocket client with auto-reconnect:

```ts
import { openHepStream } from '@/api'

const client = openHepStream(
  { proto: 1, method: ['INVITE', 'REGISTER'], history: 20 },
  {
    onOpen: () => console.log('stream open'),
    onEvent: (e) => console.log(e.sip?.method, e.sip?.callid),
    onClose: () => console.log('stream closed, will retry'),
    onError: (err) => console.warn(err),
  },
)

// later:
client.close()
```

Reconnects use exponential backoff up to 30s with small jitter. The
JWT from `sessionStorage` (`homer_v4_token`) is attached as `?access_token=` automatically.

## Example clients

```bash
# From the host running the coordinator:
TOKEN=$(cat ~/.homer/jwt)
wscat -c "ws://localhost:8081/api/v4/stream/hep?proto=1&method=INVITE&access_token=$TOKEN"

# Node-local (no auth):
wscat -c "ws://127.0.0.1:8083/stream?proto=1"
```

## Operational notes

- The feature is off by default on both sides. Enabling just the ingest
  side creates the buffer but no subscribers can reach it — flip the
  coordinator side too for UI access, or leave it off for strictly
  internal taps.
- Restarting a node drops all its subscribers. Coordinators reconnect
  automatically; external clients must reconnect themselves.
- Payload streaming bypasses all downstream redaction/censoring. Only
  turn `allow_payload` on for admin-only dashboards.
