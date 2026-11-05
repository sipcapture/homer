# tshark decode (`decoder_shark`)

Available since **11.0.377**. Off by default.

HOMER can hand one stored message to [tshark](https://www.wireshark.org/docs/man-pages/tshark.html) and show the JSON dissection in the message window. This is the HOMER 7 `decoder_shark` setting. The coordinator writes a one-packet PCAP and runs tshark itself. No separate decode service is required.

The built-in ITU-T ISUP summary does not use tshark. It is returned on the same Decode call whenever the payload is `application/isup`.

## Install tshark

tshark must be on the host that runs the coordinator. The official Homer image does not include it. `-r` reads a file, so capture privileges (`dumpcap`, `cap_net_raw`) are not required.

```bash
# Debian / Ubuntu
sudo apt-get update && sudo apt-get install -y tshark
tshark -v
```

In Docker, add the package to an image based on Homer and leave the process as `homer` (uid 1000):

```dockerfile
FROM ghcr.io/sipcapture/homer:11.0.377
USER root
RUN apt-get update && apt-get install -y --no-install-recommends tshark \
    && rm -rf /var/lib/apt/lists/*
USER homer
```

## Configuration

Put the block under `coordinator` in `homer.json`. A full sample is in [`examples/homer-coordinator.json`](../examples/homer-coordinator.json).

```json
"decoder_shark": {
  "active": true,
  "bin": "/usr/bin/tshark",
  "param": "",
  "protocols": ["1_call", "1_registration", "1_default"],
  "uid": 0,
  "gid": 0,
  "timeout_sec": 15
}
```

Restart the coordinator after changing it.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `active` | bool | `false` | Run tshark on Decode. |
| `bin` | string | `/usr/bin/tshark` | Executable path. Looked up on `PATH` when it contains no slash. |
| `param` | string | empty | Extra tshark arguments, split on whitespace. Example: `"-d udp.port==5060,sip"`. This is not a shell command. |
| `protocols` | string[] | empty | Which messages to send to tshark. A key is `proto_event`, the same form as HOMER 7: `1_call` is SIP (`proto_type` 1) with event `call`. An empty list matches every protocol. `*` matches every protocol too. |
| `uid` | uint | `0` | Applied together with `gid` only when the coordinator process is root. Both must be non-zero. |
| `gid` | uint | `0` | Group paired with `uid`. |
| `timeout_sec` | int | `15` | Kills tshark and its child processes when the limit is reached. |

The same fields can be set with environment variables:

```bash
HOMER_COORDINATOR_DECODER_SHARK_ACTIVE=true
HOMER_COORDINATOR_DECODER_SHARK_BIN=/usr/bin/tshark
HOMER_COORDINATOR_DECODER_SHARK_PARAM="-d udp.port==5060,sip"
HOMER_COORDINATOR_DECODER_SHARK_PROTOCOLS_0=1_call
HOMER_COORDINATOR_DECODER_SHARK_PROTOCOLS_1=1_registration
HOMER_COORDINATOR_DECODER_SHARK_UID=0
HOMER_COORDINATOR_DECODER_SHARK_GID=0
HOMER_COORDINATOR_DECODER_SHARK_TIMEOUT_SEC=15
```

If the coordinator runs as root, set `uid` and `gid` to an unprivileged user. With the official image the process is already `homer`, so leave both at `0`.

## What Decode does

The message window calls `POST /api/v4/messages/decoded`. A SIP body whose `Content-Type` contains `application/isup` is decoded as soon as the window opens. The Decode button sends the same request for any message.

The request carries the stored `uuid`, `proto_type`, and `event_type` (defaults `1` and `call`). Those two fields build the `protocols` key.

When `active` is true and the key matches:

1. The stored row becomes one Ethernet / IPv4 or IPv6 / UDP packet. `src_ip`, `dst_ip`, `src_port`, and `dst_port` come from the row. A missing port becomes `5060`.
2. A SIP-I body stored in `isup_body` is put back into the message before the PCAP is written. Rows from 11.0.376 that still carry `X-Homer-Binary-Body: hex` inside the payload are restored the same way, so tshark sees the original bytes.
3. The coordinator runs `tshark -Q -T json -r -` and writes the PCAP to tshark's stdin. `param` is appended as separate arguments.

tshark JSON is returned as `shark`. A failure is returned as `shark_error`. The rest of the response is still returned, including the built-in `isup` object.

```json
{
  "data": {
    "data": [ { "uuid": "...", "payload": "..." } ],
    "isup": {
      "messages": [
        { "summary": "IAM — Initial address\nCalled party number: 632107608" }
      ]
    },
    "shark": [
      { "_source": { "layers": { "sip": {}, "isup": {} } } }
    ],
    "shark_error": ""
  }
}
```

`shark` is omitted when the decoder is off or the protocol key is not in `protocols`. The message window prints the ISUP summary first and the tshark JSON under it.

Output is capped at 4 MiB. Larger output becomes `shark_error`.

## SIP-I

For `application/isup;version=itu-t92+` the built-in decoder reads ITU-T Q.763 and fills `isup` without tshark: message type, called and calling numbers, and the Q.850 cause. ANSI (`version` containing `ansi`) is left to tshark.

Messages captured before 11.0.376 may have a damaged binary body. Decode cannot repair those rows. From 11.0.376 onward the body is stored reversibly, and PCAP export writes the original bytes as well.

## Checks

```bash
command -v tshark
tshark -v
```

Then open a message and press Decode with `active: true`. `shark_error` of `tshark: executable file not found` means `bin` is wrong or tshark is not installed in that container. `tshark timed out` means the dissection exceeded `timeout_sec`. A protocol that is absent from `protocols` produces neither `shark` nor `shark_error`.
