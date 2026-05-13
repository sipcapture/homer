// Command hepudpload sends minimal HEP3 frames carrying a tiny SIP INVITE over UDP
// for ingest / writer CPU profiling (used by scripts/profile_ingest_load.sh).
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

func chunk(vendor uint16, chunkType uint16, body []byte) []byte {
	length := uint16(6 + len(body))
	out := make([]byte, 6+len(body))
	binary.BigEndian.PutUint16(out[0:2], vendor)
	binary.BigEndian.PutUint16(out[2:4], chunkType)
	binary.BigEndian.PutUint16(out[4:6], length)
	copy(out[6:], body)
	return out
}

func buildHEP3(sip []byte) []byte {
	var chunks []byte
	chunks = append(chunks, chunk(0, 1, []byte{2})...)                 // IPv4
	chunks = append(chunks, chunk(0, 2, []byte{17})...)                // UDP
	chunks = append(chunks, chunk(0, 3, []byte{10, 0, 0, 1})...)       // src IP
	chunks = append(chunks, chunk(0, 4, []byte{10, 0, 0, 2})...)       // dst IP
	chunks = append(chunks, chunk(0, 7, []byte{0x13, 0xc4})...)       // 5060
	chunks = append(chunks, chunk(0, 8, []byte{0x13, 0xc4})...)       // 5060
	now := uint32(time.Now().Unix())
	var ts [4]byte
	binary.BigEndian.PutUint32(ts[:], now)
	chunks = append(chunks, chunk(0, 9, ts[:])...)
	chunks = append(chunks, chunk(0, 10, []byte{0, 0, 0, 0})...) // tmsec
	chunks = append(chunks, chunk(0, 11, []byte{1})...)          // SIP
	var node [4]byte
	binary.BigEndian.PutUint32(node[:], 1)
	chunks = append(chunks, chunk(0, 12, node[:])...)
	chunks = append(chunks, chunk(0, 15, sip)...)

	totalLen := uint16(6 + len(chunks))
	out := make([]byte, 6+len(chunks))
	copy(out[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(out[4:6], totalLen)
	copy(out[6:], chunks)
	return out
}

func main() {
	addr := flag.String("addr", "127.0.0.1:19060", "UDP address (host:port) of HEP ingest")
	pps := flag.Int("pps", 8000, "target datagrams per second (best effort)")
	dur := flag.Duration("duration", 25*time.Second, "how long to send")
	flag.Parse()

	if *pps < 1 {
		fmt.Fprintln(os.Stderr, "pps must be >= 1")
		os.Exit(2)
	}

	sip := []byte(
		"INVITE sip:u@10.0.0.2 SIP/2.0\r\n" +
			"Via: SIP/2.0/UDP 10.0.0.1:5060;branch=z9hG4bKhepudpload\r\n" +
			"From: <sip:a@10.0.0.1>;tag=1\r\n" +
			"To: <sip:u@10.0.0.2>\r\n" +
			"Call-ID: hepudpload@127.0.0.1\r\n" +
			"CSeq: 1 INVITE\r\n" +
			"Content-Length: 0\r\n\r\n",
	)
	pkt := buildHEP3(sip)

	udpAddr, err := net.ResolveUDPAddr("udp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve udp: %v\n", err)
		os.Exit(1)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial udp: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	deadline := time.Now().Add(*dur)
	interval := time.Duration(1e9 / int64(*pps))
	var n int64
	next := time.Now()
	for time.Now().Before(deadline) {
		if _, werr := conn.Write(pkt); werr != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", werr)
			os.Exit(1)
		}
		n++
		next = next.Add(interval)
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		}
	}
	fmt.Printf("hepudpload: sent_udp_datagrams=%d addr=%s duration=%s pps_target=%d\n", n, *addr, dur.String(), *pps)
}
