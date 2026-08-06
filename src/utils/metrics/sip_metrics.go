// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package metrics

import (
	"encoding/binary"
	"strconv"
	"strings"
	"sync"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/buger/jsonparser"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SIP Method/Response counters
	SIPMethodResponses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_sip_method_response_total",
			Help: "SIP method and response counter",
		},
		[]string{"target_name", "direction", "node_id", "response", "method"},
	)

	// SIP packets by type
	SIPPacketsByType = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_packets_by_type_total",
			Help: "Total packets by HEP type",
		},
		[]string{"node_id", "type"},
	)

	// SIP packets by size
	SIPPacketsBySize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_packets_size",
			Help: "Packet size by HEP type",
		},
		[]string{"node_id", "type"},
	)

	// Q.850 Reason causes
	SIPReasonCause = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_sip_reason_cause_total",
			Help: "ISUP Q.850 cause from reason header",
		},
		[]string{"target_name", "cause", "method"},
	)

	// Setup Response Delay (INVITE -> 180/183/200)
	SIPSRD = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_sip_srd_seconds",
			Help: "SIP Session Request Delay (INVITE to 180/183/200)",
		},
		[]string{"target_name", "node_id"},
	)

	// Registration Response Delay
	SIPRRD = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_sip_rrd_seconds",
			Help: "SIP Registration Request Delay",
		},
		[]string{"target_name", "node_id"},
	)

	// RTCP Metrics
	RTCPFractionLost = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtcp_fraction_lost",
			Help: "RTCP fraction lost",
		},
		[]string{"target_name", "direction", "node_id"},
	)

	RTCPPacketsLost = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtcp_packets_lost",
			Help: "RTCP packets lost",
		},
		[]string{"target_name", "direction", "node_id"},
	)

	RTCPJitter = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtcp_jitter",
			Help: "RTCP jitter",
		},
		[]string{"target_name", "direction", "node_id"},
	)

	RTCPDLSR = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtcp_dlsr",
			Help: "RTCP DLSR (delay since last sender report)",
		},
		[]string{"target_name", "direction", "node_id"},
	)

	// X-RTP-Stat Metrics
	XRTPCallSetup = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_call_setup_seconds",
			Help: "XRTP call setup time",
		},
		[]string{"target_name"},
	)

	XRTPJitterReceived = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_jitter_received",
			Help: "XRTP received jitter",
		},
		[]string{"target_name"},
	)

	XRTPJitterSent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_jitter_sent",
			Help: "XRTP sent jitter",
		},
		[]string{"target_name"},
	)

	XRTPPacketsLostReceived = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_packets_lost_received",
			Help: "XRTP received packets lost",
		},
		[]string{"target_name"},
	)

	XRTPPacketsLostSent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_packets_lost_sent",
			Help: "XRTP sent packets lost",
		},
		[]string{"target_name"},
	)

	XRTPMOS = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_mos",
			Help: "XRTP MOS score",
		},
		[]string{"target_name"},
	)

	XRTPRTT = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_xrtp_rtt_ms",
			Help: "XRTP mean RTT in milliseconds",
		},
		[]string{"target_name"},
	)

	// RTPAgent Metrics (ProtoType 34)
	RTPAgentDelta = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtpagent_delta",
			Help: "RTPAgent delta",
		},
		[]string{"node_id"},
	)

	RTPAgentJitter = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtpagent_jitter",
			Help: "RTPAgent jitter",
		},
		[]string{"node_id"},
	)

	RTPAgentMOS = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtpagent_mos",
			Help: "RTPAgent MOS score",
		},
		[]string{"node_id"},
	)

	RTPAgentPacketsLost = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_rtpagent_packets_lost",
			Help: "RTPAgent packets lost",
		},
		[]string{"node_id"},
	)
)

// SIPMetricsProcessor handles SIP-specific metrics processing
type SIPMetricsProcessor struct {
	TargetEmpty bool
	TargetIP    []string
	TargetName  []string
	TargetMap   map[string]string
	TargetConf  *sync.RWMutex
	srdCache    *fastcache.Cache
	cacheSize   int
	// agentLabel is "node_id" (HEP 0x000c) or "node_name" (HEP 0x0013).
	// It controls which value fills the Prometheus "node_id" label.
	agentLabel string
}

const defaultCacheSize = 60 * 1024 * 1024

const (
	AgentLabelNodeID   = "node_id"
	AgentLabelNodeName = "node_name"
)

// ResolveAgentLabel picks the Prometheus node_id label value.
// mode "node_name" prefers the HEP NodeName (hostname); otherwise NodeID.
// Empty nodeName falls back to nodeID so RTCP/SIP stay consistent.
func ResolveAgentLabel(mode, nodeID, nodeName string) string {
	if strings.EqualFold(strings.TrimSpace(mode), AgentLabelNodeName) {
		if name := strings.TrimSpace(nodeName); name != "" {
			return name
		}
	}
	if id := strings.TrimSpace(nodeID); id != "" {
		return id
	}
	return strings.TrimSpace(nodeName)
}

// NewSIPMetricsProcessor creates a new SIP metrics processor.
// agentLabel selects "node_id" or "node_name" for the Prometheus node_id label.
func NewSIPMetricsProcessor(targetIP, targetName, agentLabel string) *SIPMetricsProcessor {
	p := &SIPMetricsProcessor{
		TargetConf: new(sync.RWMutex),
		srdCache:   fastcache.New(defaultCacheSize),
		cacheSize:  defaultCacheSize,
		agentLabel: normalizeAgentLabel(agentLabel),
	}

	p.TargetIP = strings.Split(strings.ReplaceAll(targetIP, " ", ""), ",")
	p.TargetName = strings.Split(strings.ReplaceAll(targetName, " ", ""), ",")

	if len(p.TargetIP) == len(p.TargetName) && len(p.TargetIP[0]) > 0 && len(p.TargetName[0]) > 0 {
		p.TargetMap = make(map[string]string)
		for i := 0; i < len(p.TargetName); i++ {
			p.TargetMap[p.TargetIP[i]] = p.TargetName[i]
		}
	} else {
		p.TargetEmpty = true
		p.TargetIP = []string{""}
		p.TargetName = []string{""}
	}

	return p
}

func normalizeAgentLabel(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), AgentLabelNodeName) {
		return AgentLabelNodeName
	}
	return AgentLabelNodeID
}

func (p *SIPMetricsProcessor) agentLabelFor(nodeID, nodeName string) string {
	return ResolveAgentLabel(p.agentLabel, nodeID, nodeName)
}

func (p *SIPMetricsProcessor) agentLabelForPacket(pkt *SIPPacketInfo) string {
	if pkt == nil {
		return ""
	}
	return p.agentLabelFor(pkt.NodeID, pkt.NodeName)
}

// SIPPacketInfo contains information about a SIP packet for metrics
type SIPPacketInfo struct {
	NodeName      string
	NodeID        string
	ProtoType     uint32
	ProtoString   string
	PayloadSize   int
	SrcIP         string
	DstIP         string
	TimestampNano int64
	CallID        string
	Method        string
	Response      string
	CseqMethod    string
	ReasonVal     string
	RTPStatVal    string
}

// ProcessSIPPacket processes a SIP packet and records metrics
func (p *SIPMetricsProcessor) ProcessSIPPacket(pkt *SIPPacketInfo) {
	agent := p.agentLabelForPacket(pkt)

	// Record packets by type
	SIPPacketsByType.WithLabelValues(agent, pkt.ProtoString).Inc()
	SIPPacketsBySize.WithLabelValues(agent, pkt.ProtoString).Set(float64(pkt.PayloadSize))

	var srcTarget, dstTarget string
	var srcHit, dstHit bool

	if !p.TargetEmpty {
		srcTarget, srcHit = p.TargetMap[pkt.SrcIP]
		dstTarget, dstHit = p.TargetMap[pkt.DstIP]
	}

	// SIP message processing
	if pkt.ProtoType == 1 && pkt.Method != "" {
		if !p.TargetEmpty {
			if srcHit {
				SIPMethodResponses.WithLabelValues(srcTarget, "src", agent, pkt.Response, pkt.CseqMethod).Inc()
				if pkt.ReasonVal != "" && strings.Contains(pkt.ReasonVal, "850") {
					SIPReasonCause.WithLabelValues(srcTarget, extractCause(pkt.ReasonVal), pkt.Response).Inc()
				}
			}
			if dstHit {
				SIPMethodResponses.WithLabelValues(dstTarget, "dst", agent, pkt.Response, pkt.CseqMethod).Inc()
			}
			if !srcHit && !dstHit {
				SIPMethodResponses.WithLabelValues("unknown", "", agent, pkt.Response, pkt.CseqMethod).Inc()
			}
		}

		// Calculate SRD/RRD
		skip := dstTarget == "" && srcTarget == "" && !p.TargetEmpty

		callID := pkt.CallID
		for strings.HasSuffix(callID, "_b2b-1") {
			callID = callID[:len(callID)-6]
		}

		// Track INVITE and REGISTER requests
		if !skip && ((pkt.Response == "INVITE" && pkt.CseqMethod == "INVITE") ||
			(pkt.Response == "REGISTER" && pkt.CseqMethod == "REGISTER")) {
			sid := []byte(callID)
			buf := p.srdCache.Get(nil, sid)
			ptn := uint64(pkt.TimestampNano)
			if buf == nil || (buf != nil && ptn < binary.BigEndian.Uint64(buf)) {
				sk := []byte(pkt.SrcIP + callID)
				tb := make([]byte, 8)
				binary.BigEndian.PutUint64(tb, ptn)
				p.srdCache.Set(sid, tb)
				p.srdCache.Set(sk, tb)
			}
		}

		// Track responses for SRD/RRD calculation
		if !skip && ((pkt.CseqMethod == "INVITE" || pkt.CseqMethod == "REGISTER") &&
			(pkt.Response == "180" || pkt.Response == "181" || pkt.Response == "182" ||
				pkt.Response == "183" || pkt.Response == "200")) {
			did := []byte(pkt.DstIP + callID)
			if buf := p.srdCache.Get(nil, did); buf != nil {
				ptn := uint64(pkt.TimestampNano)
				d := ptn - binary.BigEndian.Uint64(buf)

				if dstTarget == "" {
					dstTarget = srcTarget
				}

				if pkt.CseqMethod == "INVITE" {
					SIPSRD.WithLabelValues(dstTarget, agent).Set(float64(d) / 1e9) // Convert to seconds
				} else {
					SIPRRD.WithLabelValues(dstTarget, agent).Set(float64(d) / 1e9)
					p.srdCache.Del([]byte(callID))
				}
				p.srdCache.Del(did)
			}
		}

		// Process X-RTP-Stat header
		if pkt.RTPStatVal != "" {
			target := srcTarget
			if target == "" {
				target = "unknown"
			}
			p.dissectXRTPStats(target, pkt.RTPStatVal)
		}

		// Without targets - use call dedup cache
		if p.TargetEmpty {
			k := []byte(callID + pkt.Response + pkt.CseqMethod)
			if p.srdCache.Has(k) {
				return
			}
			p.srdCache.Set(k, nil)
			SIPMethodResponses.WithLabelValues(agent, "", agent, pkt.Response, pkt.CseqMethod).Inc()

			if pkt.ReasonVal != "" && strings.Contains(pkt.ReasonVal, "850") {
				SIPReasonCause.WithLabelValues("unknown", extractCause(pkt.ReasonVal), pkt.Response).Inc()
			}
		}
	}
}

// ProcessRTCPPacket processes RTCP packet (ProtoType 5).
// nodeID is HEP 0x000c; nodeName is HEP 0x0013 (may be empty).
func (p *SIPMetricsProcessor) ProcessRTCPPacket(nodeID, nodeName, srcIP, dstIP string, payload []byte) {
	agent := p.agentLabelFor(nodeID, nodeName)

	var srcTarget, dstTarget string
	var srcHit, dstHit bool

	if !p.TargetEmpty {
		srcTarget, srcHit = p.TargetMap[srcIP]
		dstTarget, dstHit = p.TargetMap[dstIP]
	}

	if srcHit {
		p.dissectRTCPStats(srcTarget, "src", agent, payload)
	}
	if dstHit {
		p.dissectRTCPStats(dstTarget, "dst", agent, payload)
	}
	if !srcHit && !dstHit {
		p.dissectRTCPStats("unknown", "", agent, payload)
	}
}

// ProcessRTPAgentPacket processes RTPAgent packet (ProtoType 34).
// nodeID is HEP 0x000c; nodeName is HEP 0x0013 (may be empty).
func (p *SIPMetricsProcessor) ProcessRTPAgentPacket(nodeID, nodeName string, payload []byte) {
	p.dissectRTPAgentStats(p.agentLabelFor(nodeID, nodeName), payload)
}

// dissectXRTPStats extracts and records X-RTP-Stat metrics
func (p *SIPMetricsProcessor) dissectXRTPStats(tn, stats string) {
	if cs := extractXR("CS=", stats); cs != "" {
		if val, err := strconv.ParseFloat(cs, 64); err == nil {
			XRTPCallSetup.WithLabelValues(tn).Set(val / 1000)
		}
	}

	if plt := extractXR("PL=", stats); len(plt) > 1 {
		if plr, pls, err := splitCommaInt(plt); err == nil {
			XRTPPacketsLostReceived.WithLabelValues(tn).Set(float64(plr))
			XRTPPacketsLostSent.WithLabelValues(tn).Set(float64(pls))
		}
	}

	if jit := extractXR("JI=", stats); len(jit) > 1 {
		if jir, jis, err := splitCommaInt(jit); err == nil {
			XRTPJitterReceived.WithLabelValues(tn).Set(float64(jir))
			XRTPJitterSent.WithLabelValues(tn).Set(float64(jis))
		}
	}

	if dlt := extractXR("DL=", stats); len(dlt) > 1 {
		if dle, _, err := splitCommaInt(dlt); err == nil && dle > 0 {
			XRTPRTT.WithLabelValues(tn).Set(float64(dle))
		}
	}

	// Calculate MOS
	pr, _ := strconv.Atoi(extractXR("PR=", stats))
	ps, _ := strconv.Atoi(extractXR("PS=", stats))
	if pr == 0 && ps == 0 {
		pr, ps = 1, 1
	}

	jir, _ := strconv.Atoi(extractXR("JI=", stats))
	dle, _ := strconv.Atoi(extractXR("DL=", stats))
	plr, _ := strconv.Atoi(extractXR("PL=", stats))
	pls, _ := strconv.Atoi(extractXR("PL=", stats))

	loss := ((plr + pls) * 100) / (pr + ps)
	el := (jir * 2) + (dle + 10)

	var r float64
	if el < 160 {
		r = 93.2 - (float64(el) / 40)
	} else {
		r = 93.2 - (float64(el-120) / 10)
	}
	r = r - (float64(loss) * 2.5)

	mos := 1 + (0.035)*r + (0.000007)*r*(r-60)*(100-r)
	if mos < 1 || mos > 5 {
		mos = 1
	}
	XRTPMOS.WithLabelValues(tn).Set(mos)
}

// dissectRTCPStats extracts and records RTCP metrics
func (p *SIPMetricsProcessor) dissectRTCPStats(targetName, direction, nodeID string, data []byte) {
	rtcpPaths := [][]string{
		{"report_blocks", "[0]", "fraction_lost"},
		{"report_blocks", "[0]", "packets_lost"},
		{"report_blocks", "[0]", "ia_jitter"},
		{"report_blocks", "[0]", "dlsr"},
	}

	jsonparser.EachKey(data, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		if err != nil {
			return
		}
		switch idx {
		case 0:
			if fractionLost, err := jsonparser.ParseFloat(value); err == nil {
				RTCPFractionLost.WithLabelValues(targetName, direction, nodeID).Set(normMax(fractionLost))
			}
		case 1:
			if packetsLost, err := jsonparser.ParseFloat(value); err == nil {
				RTCPPacketsLost.WithLabelValues(targetName, direction, nodeID).Set(normMax(packetsLost))
			}
		case 2:
			if iaJitter, err := jsonparser.ParseFloat(value); err == nil {
				RTCPJitter.WithLabelValues(targetName, direction, nodeID).Set(normMax(iaJitter))
			}
		case 3:
			if dlsr, err := jsonparser.ParseFloat(value); err == nil {
				RTCPDLSR.WithLabelValues(targetName, direction, nodeID).Set(normMax(dlsr))
			}
		}
	}, rtcpPaths...)
}

// dissectRTPAgentStats extracts and records RTPAgent metrics
func (p *SIPMetricsProcessor) dissectRTPAgentStats(nodeID string, data []byte) {
	rtpPaths := [][]string{
		{"DELTA"},
		{"JITTER"},
		{"MOS"},
		{"PACKET_LOSS"},
	}

	jsonparser.EachKey(data, func(idx int, value []byte, vt jsonparser.ValueType, err error) {
		if err != nil {
			return
		}
		switch idx {
		case 0:
			if delta, err := jsonparser.ParseFloat(value); err == nil {
				RTPAgentDelta.WithLabelValues(nodeID).Set(delta)
			}
		case 1:
			if jitter, err := jsonparser.ParseFloat(value); err == nil {
				RTPAgentJitter.WithLabelValues(nodeID).Set(jitter)
			}
		case 2:
			if mos, err := jsonparser.ParseFloat(value); err == nil {
				RTPAgentMOS.WithLabelValues(nodeID).Set(mos)
			}
		case 3:
			if packetsLost, err := jsonparser.ParseFloat(value); err == nil {
				RTPAgentPacketsLost.WithLabelValues(nodeID).Set(packetsLost)
			}
		}
	}, rtpPaths...)
}

// Helper functions

func extractXR(key, stats string) string {
	idx := strings.Index(stats, key)
	if idx == -1 {
		return ""
	}
	start := idx + len(key)
	end := strings.IndexAny(stats[start:], " ;,\r\n")
	if end == -1 {
		return stats[start:]
	}
	return stats[start : start+end]
}

func extractCause(reason string) string {
	idx := strings.Index(reason, "cause=")
	if idx == -1 {
		return "unknown"
	}
	start := idx + 6
	end := strings.IndexAny(reason[start:], " ;,\r\n\"")
	if end == -1 {
		return reason[start:]
	}
	return reason[start : start+end]
}

func splitCommaInt(str string) (int, int, error) {
	sp := strings.IndexRune(str, ',')
	if sp == -1 {
		val, err := strconv.Atoi(str)
		return val, 0, err
	}
	one, err := strconv.Atoi(str[0:sp])
	if err != nil {
		return 0, 0, err
	}
	two, err := strconv.Atoi(str[sp+1:])
	if err != nil {
		return one, 0, err
	}
	return one, two, nil
}

func normMax(val float64) float64 {
	if val > 10000000 {
		return 0
	}
	return val
}
