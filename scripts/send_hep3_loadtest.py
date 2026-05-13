#!/usr/bin/env python3
"""Send synthetic HEP3 (SIP) packets to homer-core for load / CPU testing."""
from __future__ import annotations

import argparse
import socket
import struct
import time


def chunk(vendor: int, type_id: int, body: bytes) -> bytes:
    total = 6 + len(body)
    return struct.pack(">HHH", vendor, type_id, total) + body


def build_hep3(
    sip: str,
    *,
    src_ip: str,
    dst_ip: str,
    src_port: int,
    dst_port: int,
    capture_id: int,
    correlation_id: str,
) -> bytes:
    body = b""
    body += chunk(0, 0x0001, struct.pack("B", 2))  # IPv4
    body += chunk(0, 0x0002, struct.pack("B", 17))  # UDP
    body += chunk(0, 0x0003, socket.inet_aton(src_ip))
    body += chunk(0, 0x0004, socket.inet_aton(dst_ip))
    body += chunk(0, 0x0007, struct.pack(">H", src_port))
    body += chunk(0, 0x0008, struct.pack(">H", dst_port))
    t = time.time()
    body += chunk(0, 0x0009, struct.pack(">I", int(t)))
    body += chunk(0, 0x000A, struct.pack(">I", int((t % 1) * 1_000_000)))
    body += chunk(0, 0x000B, struct.pack("B", 1))  # SIP
    body += chunk(0, 0x000C, struct.pack(">I", capture_id & 0xFFFFFFFF))
    body += chunk(0, 0x000F, sip.encode("utf-8", errors="replace"))
    if correlation_id:
        body += chunk(0, 0x0011, correlation_id.encode("utf-8", errors="replace"))
    total = 6 + len(body)
    return b"HEP3" + struct.pack(">H", total) + body


def send_udp(host: str, port: int, pkt: bytes, count: int) -> None:
    addr = (host, port)
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
        for i in range(count):
            s.sendto(pkt, addr)


def send_tcp(host: str, port: int, pkt: bytes, count: int) -> None:
    with socket.create_connection((host, port), timeout=30) as s:
        for _ in range(count):
            s.sendall(pkt)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--udp", metavar="HOST:PORT", help="UDP HEP target")
    ap.add_argument("--tcp", metavar="HOST:PORT", help="TCP HEP target")
    ap.add_argument("--count", type=int, default=10_000)
    ap.add_argument("--burst", type=int, default=0, help="repeat same packet (0 = unique CID per packet)")
    args = ap.parse_args()

    if not args.udp and not args.tcp:
        ap.error("need --udp and/or --tcp")

    base_sip = (
        "INVITE sip:bob@example.com SIP/2.0\r\n"
        "Via: SIP/2.0/UDP 10.0.0.1:5060;branch=z9hG4bK-load\r\n"
        "From: <sip:alice@example.com>;tag=load\r\n"
        "To: <sip:bob@example.com>\r\n"
        "Call-ID: {cid}\r\n"
        "CSeq: 1 INVITE\r\n"
        "Contact: <sip:alice@10.0.0.1:5060>\r\n"
        "Content-Length: 0\r\n\r\n"
    )

    def one_pkt(seq: int) -> bytes:
        cid = f"load-{seq}" if args.burst <= 0 else f"load-burst-{args.burst}"
        sip = base_sip.format(cid=cid)
        return build_hep3(
            sip,
            src_ip="10.0.0.1",
            dst_ip="10.0.0.2",
            src_port=5060,
            dst_port=5060,
            capture_id=seq,
            correlation_id=cid,
        )

    t0 = time.perf_counter()
    if args.udp:
        h, p = args.udp.rsplit(":", 1)
        pkt0 = one_pkt(0)
        for i in range(args.count):
            send_udp(h, int(p), pkt0 if args.burst else one_pkt(i), 1)
    if args.tcp:
        h, p = args.tcp.rsplit(":", 1)
        for i in range(args.count):
            pkt = one_pkt(i) if args.burst <= 0 else one_pkt(0)
            send_tcp(h, int(p), pkt, 1)
    dt = time.perf_counter() - t0
    print(f"sent {args.count} pkt(s) per protocol in {dt:.3f}s ({args.count / max(dt, 1e-9):.0f} pkt/s combined)")


if __name__ == "__main__":
    main()
