# VQ-RTCPXR SIP QoS Receiver

**Disabled by default.** Set `ingest.vqrtcp.enable` to `true` to start the listener.

Homer can accept SIP messages carrying `application/vq-rtcpxr` bodies on a dedicated SIP listener and store parsed QoS reports in DuckLake table `vqrtcpxr_stats`.

## Configuration

```json
"ingest": {
  "vqrtcp": {
    "enable": true,
    "bind_ip": "0.0.0.0",
    "sip_port": 5063,
    "transports": ["udp", "tcp"],
    "methods": ["PUBLISH", "MESSAGE"],
    "reply_200": true,
    "async_queue_depth": 1024,
    "node_id": "homer"
  }
}
```

Requires the **writer** module (DuckLake). The receiver is started from `main.go` alongside OTLP and Line Protocol receivers.

## Storage

Table: `{lake_name}.vqrtcpxr_stats`

Key columns: `callid`, `mos`, `event` (`VQSessionReport` / `VQIntervalReport`), `source_ip/port`, `destination_ip/port`, `data` (raw body), `message` (parsed JSON summary).

## UI

Transaction modal → **QoS** tab → **VQRTCP** sub-tab (when data exists). API: `POST /api/v4/transactions/qos` returns `data.vqrtcp.data[]` correlated by Call-ID.

## Example body

```
Call-ID: abc@host
VQIntervalReport:
QualityEst:MOSLQ=4.20 MOSCQ=4.10
LocalAddr: 10.0.0.1 5060
RemoteAddr: 10.0.0.2 5070
JitterBuffer:JBA=0 JBR=0 JBN=10 JBM=0 JBX=0
PacketLoss:NLR=0.5 JDR=0.1
```

Point your SBC or media gateway VQ-RTCPXR SIP target at `bind_ip:sip_port`.
