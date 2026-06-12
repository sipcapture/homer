// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package vqrtcpreceiver

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

const vqContentType = "application/vq-rtcpxr"

// Server accepts SIP requests carrying VQ-RTCPXR bodies.
type Server struct {
	cfg     *config.VqrtcpConfig
	storage *ducklake.VqrtcpStorage
	log     *slog.Logger

	ua  *sipgo.UserAgent
	srv *sipgo.Server

	queue chan ducklake.VqrtcpRow
	wg    sync.WaitGroup
}

// NewServer builds the SIP listener stack.
func NewServer(cfg *config.VqrtcpConfig, storage *ducklake.VqrtcpStorage, log *slog.Logger) (*Server, error) {
	if cfg == nil || storage == nil {
		return nil, fmt.Errorf("vqrtcp: config and storage required")
	}
	if log == nil {
		log = slog.Default()
	}
	depth := cfg.AsyncQueueDepth
	if depth < 1 {
		depth = 1024
	}
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("homer-vqrtcp"))
	if err != nil {
		return nil, fmt.Errorf("vqrtcp: create UA: %w", err)
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("vqrtcp: create server: %w", err)
	}
	s := &Server{
		cfg:     cfg,
		storage: storage,
		log:     log,
		ua:      ua,
		srv:     srv,
		queue:   make(chan ducklake.VqrtcpRow, depth),
	}
	methods := cfg.Methods
	if len(methods) == 0 {
		methods = []string{"PUBLISH", "MESSAGE"}
	}
	for _, m := range methods {
		switch strings.ToUpper(strings.TrimSpace(m)) {
		case "PUBLISH":
			srv.OnPublish(s.onRequest)
		case "MESSAGE":
			srv.OnMessage(s.onRequest)
		case "NOTIFY":
			srv.OnNotify(s.onRequest)
		default:
			return nil, fmt.Errorf("vqrtcp: unsupported method %q", m)
		}
	}
	workers := 2
	if depth >= 512 {
		workers = 4
	}
	for i := 0; i < workers; i++ {
		s.wg.Add(1)
		go s.writeWorker()
	}
	return s, nil
}

func (s *Server) writeWorker() {
	defer s.wg.Done()
	ctx := context.Background()
	for row := range s.queue {
		if err := s.storage.WriteRow(ctx, row); err != nil {
			s.log.Error("vqrtcp write failed", "error", err, "callid", row.CallID)
		}
	}
}

// ListenAndServe binds UDP/TCP and blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := net.JoinHostPort(s.cfg.BindIP, fmt.Sprintf("%d", s.cfg.SIPPort))
	errCh := make(chan error, len(s.cfg.Transports))
	var closers []func()

	for _, tp := range s.cfg.Transports {
		switch strings.ToLower(strings.TrimSpace(tp)) {
		case "udp":
			pc, err := net.ListenPacket("udp", addr)
			if err != nil {
				return fmt.Errorf("vqrtcp: bind udp %s: %w", addr, err)
			}
			closers = append(closers, func() { pc.Close() })
			go func() {
				if err := s.srv.ServeUDP(pc); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("vqrtcp: udp: %w", err)
				}
			}()
		case "tcp":
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("vqrtcp: bind tcp %s: %w", addr, err)
			}
			closers = append(closers, func() { l.Close() })
			go func() {
				if err := s.srv.ServeTCP(l); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("vqrtcp: tcp: %w", err)
				}
			}()
		default:
			return fmt.Errorf("vqrtcp: unsupported transport %q", tp)
		}
		s.log.Info("VQRTCP SIP listener started", "transport", tp, "addr", addr)
	}
	defer func() {
		for _, c := range closers {
			c()
		}
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

// Close stops workers and the SIP stack.
func (s *Server) Close() {
	close(s.queue)
	s.wg.Wait()
	if s.srv != nil {
		_ = s.srv.Close()
	}
}

func (s *Server) onRequest(req *sip.Request, tx sip.ServerTransaction) {
	method := req.Method.String()
	log := s.log.With("method", method, "source", req.Source())

	body, ok := extractVqBody(req)
	if !ok {
		log.Debug("ignoring SIP request without vq-rtcpxr body")
		if s.cfg.Reply200 {
			_ = tx.Respond(sip.NewResponseFromRequest(req, 200, "OK", nil))
		}
		return
	}
	report := ParseBody(body)
	callID := report.CallID
	if callID == "" && req.CallID() != nil {
		callID = req.CallID().Value()
	}
	srcIP, srcPort := splitSIPSource(req.Source())
	localIP, localPort := splitSIPSource(req.Destination())
	if report.LocalAddr != "" {
		localIP, localPort = SplitHostPort(report.LocalAddr)
	}
	if report.RemoteAddr != "" {
		srcIP, srcPort = SplitHostPort(report.RemoteAddr)
	}
	row := ducklake.VqrtcpRow{
		CallID:          callID,
		MosLQ:           report.MosLQ,
		MosCQ:           report.MosCQ,
		Event:           report.EventType(),
		SourceIP:        localIP,
		SourcePort:      localPort,
		DestinationIP:   srcIP,
		DestinationPort: srcPort,
		Node:            s.cfg.NodeID,
		SIPMethod:       method,
		RawData:         report.Raw,
		Timestamp:       time.Now().UTC(),
		Message: map[string]any{
			"moslq":          report.MosLQ,
			"moscq":          report.MosCQ,
			"jitter_buffer":  report.JitterBuffer,
			"packet_loss":    report.PacketLoss,
			"local_addr":     report.LocalAddr,
			"remote_addr":    report.RemoteAddr,
			"session_report": report.HasSessionReport,
			"interval_report": report.HasIntervalReport,
		},
	}
	select {
	case s.queue <- row:
	default:
		log.Warn("vqrtcp async queue full, dropping report", "callid", callID)
	}
	if s.cfg.Reply200 {
		if err := tx.Respond(sip.NewResponseFromRequest(req, 200, "OK", nil)); err != nil {
			log.Error("failed to send 200 OK", "error", err)
		}
	}
}

func extractVqBody(req *sip.Request) ([]byte, bool) {
	if ct := req.ContentType(); ct != nil {
		mediaType, _, _ := mime.ParseMediaType(ct.Value())
		if strings.EqualFold(mediaType, vqContentType) {
			return req.Body(), true
		}
		if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
			return extractVqFromMultipart(req.Body(), ct.Value())
		}
	}
	// Some SBCs send without proper Content-Type but body looks like VQ-RTCPXR.
	body := req.Body()
	if len(body) > 0 && (strings.Contains(string(body), "VQSessionReport:") || strings.Contains(string(body), "VQIntervalReport:")) {
		return body, true
	}
	return nil, false
}

func extractVqFromMultipart(body []byte, contentType string) ([]byte, bool) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, false
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, false
	}
	parts := strings.Split(string(body), "--"+boundary)
	for _, part := range parts {
		if !strings.Contains(part, vqContentType) {
			continue
		}
		idx := strings.Index(part, "\r\n\r\n")
		if idx < 0 {
			idx = strings.Index(part, "\n\n")
		}
		if idx < 0 {
			continue
		}
		payload := part[idx+2:]
		payload = strings.TrimSuffix(payload, "\r\n")
		payload = strings.TrimSuffix(payload, "--")
		return []byte(strings.TrimSpace(payload)), true
	}
	return nil, false
}

func splitSIPSource(src string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(src)
	if err != nil {
		return src, 0
	}
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}
