# SIPREC Signaling Server

**Disabled by default.** Set `ingest.siprec.enable` to `true` to start the listener.

Homer includes a **Signaling SRS** (SIPREC Session Recording Server) per RFC 7865/7866. It terminates SIPREC INVITE dialogs, parses `application/rs-metadata+xml`, answers with `recvonly` SDP, and writes signaling to `hep_proto_1_siprec`.

## Configuration

```json
"ingest": {
  "siprec": {
    "enable": true,
    "bind_ip": "0.0.0.0",
    "advertise_ip": "203.0.113.10",
    "sip_port": 5062,
    "transports": ["udp", "tcp"],
    "require_siprec": true,
    "user_agent": "homer-siprec-srs",
    "node_id": "homer"
  }
}
```

Set `advertise_ip` to the address SRC/SBC must reach for SIP and SDP.

## Storage

Table: `{lake_name}.main.hep_proto_1_siprec`

- `session_id`: primary `sipSessionID` from rs-metadata (falls back to SIP Call-ID)
- `payload`: full SIP message text
- `data_extra`: JSON with participants, streams, recording metadata, `sip_call_id`

## Protocol Search

Default mapping profile **siprec** (`hepid` 1, alias `SIPREC`) is seeded for Protocol Search.

## Correlation

SIPREC rows correlate to normal SIP calls when `session_id` matches Call-ID or `sipSessionID` from metadata. Use Protocol Search on profile `siprec` or join on `session_id`.

## Limitations

This SRS does **not** record RTP to disk. It is intended for signaling and metadata capture inside Homer. For full media recording use a dedicated SRS (e.g. [siprec](https://github.com/adubovikov/siprec)) and optionally forward signaling via HEP.
