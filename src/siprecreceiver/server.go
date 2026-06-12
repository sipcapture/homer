// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package siprecreceiver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/siprecreceiver/metadata"
	"github.com/sipcapture/homer-core/src/siprecreceiver/sdpx"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

const allowMethods = "INVITE, ACK, BYE, CANCEL, OPTIONS, UPDATE"
const signalingOnlyRTPPort = 9

type dialogState struct {
	toTag   string
	answers []sdpx.AnswerMedia
}

type sessionState struct {
	fromTag      string
	metadata     *metadata.Recording
	metadataXML  []byte
	recordingID  string
}

// Server is a signaling-only SIPREC SRS.
type Server struct {
	cfg     *config.SiprecConfig
	storage *ducklake.Manager
	log     *slog.Logger

	ua  *sipgo.UserAgent
	srv *sipgo.Server

	mu       sync.Mutex
	dialogs  map[string]*dialogState
	sessions map[string]*sessionState
}

// NewServer creates the SIPREC signaling server.
func NewServer(cfg *config.SiprecConfig, storage *ducklake.Manager, log *slog.Logger) (*Server, error) {
	if cfg == nil || storage == nil {
		return nil, fmt.Errorf("siprec: config and storage required")
	}
	if log == nil {
		log = slog.Default()
	}
	if cfg.AdvertiseIP == "" {
		cfg.AdvertiseIP = cfg.BindIP
	}
	uaName := cfg.UserAgent
	if uaName == "" {
		uaName = "homer-siprec-srs"
	}
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(uaName))
	if err != nil {
		return nil, fmt.Errorf("siprec: create UA: %w", err)
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("siprec: create server: %w", err)
	}
	s := &Server{
		cfg:      cfg,
		storage:  storage,
		log:      log,
		ua:       ua,
		srv:      srv,
		dialogs:  make(map[string]*dialogState),
		sessions: make(map[string]*sessionState),
	}
	srv.OnInvite(s.onInvite)
	srv.OnAck(s.onAck)
	srv.OnBye(s.onBye)
	srv.OnCancel(s.onCancel)
	srv.OnOptions(s.onOptions)
	return s, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := net.JoinHostPort(s.cfg.BindIP, fmt.Sprintf("%d", s.cfg.SIPPort))
	errCh := make(chan error, len(s.cfg.Transports))
	var closers []func()
	for _, tp := range s.cfg.Transports {
		switch strings.ToLower(strings.TrimSpace(tp)) {
		case "udp":
			pc, err := net.ListenPacket("udp", addr)
			if err != nil {
				return fmt.Errorf("siprec: bind udp %s: %w", addr, err)
			}
			closers = append(closers, func() { pc.Close() })
			go func() {
				if err := s.srv.ServeUDP(pc); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("siprec: udp: %w", err)
				}
			}()
		case "tcp":
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("siprec: bind tcp %s: %w", addr, err)
			}
			closers = append(closers, func() { l.Close() })
			go func() {
				if err := s.srv.ServeTCP(l); err != nil && ctx.Err() == nil {
					errCh <- fmt.Errorf("siprec: tcp: %w", err)
				}
			}()
		default:
			return fmt.Errorf("siprec: unsupported transport %q", tp)
		}
		s.log.Info("SIPREC listener started", "transport", tp, "addr", addr)
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

func (s *Server) Close() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

func (s *Server) onInvite(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	log := s.log.With("call_id", callID, "source", req.Source())

	contentType := ""
	if ct := req.ContentType(); ct != nil {
		contentType = ct.Value()
	}
	body, err := parseInviteBody(contentType, req.Body())
	if err != nil {
		log.Warn("rejecting INVITE", "reason", err)
		s.respondError(req, tx, 400, "Bad Request")
		return
	}
	offer, err := sdpx.Parse(body.SDP)
	if err != nil {
		log.Warn("rejecting INVITE: bad SDP", "error", err)
		s.respondError(req, tx, 488, "Not Acceptable Here")
		return
	}
	var rec *metadata.Recording
	if body.MetadataXML != nil {
		rec, err = metadata.Parse(body.MetadataXML)
		if err != nil {
			log.Warn("rejecting INVITE: bad metadata", "error", err)
			s.respondError(req, tx, 400, "Bad SIPREC Metadata")
			return
		}
	}
	s.mu.Lock()
	sess := s.sessions[callID]
	exists := sess != nil
	if !exists {
		if s.cfg.RequireSiprec && rec == nil && !hasSiprecRequire(req) {
			s.mu.Unlock()
			log.Warn("rejecting INVITE: not SIPREC")
			s.respondError(req, tx, 488, "Not Acceptable Here")
			return
		}
		fromTag, _ := req.From().Params.Get("tag")
		sess = &sessionState{fromTag: fromTag}
		s.sessions[callID] = sess
		s.dialogs[callID] = &dialogState{}
	}
	if rec != nil {
		sess.metadata = rec
		sess.metadataXML = append([]byte(nil), body.MetadataXML...)
		if len(rec.Sessions) > 0 {
			sess.recordingID = rec.Sessions[0].ID
		}
	}
	dlg := s.dialogs[callID]
	s.mu.Unlock()

	answers := negotiateSignalingOnly(offer)
	dlg.answers = answers
	answerSDP := sdpx.BuildAnswer(offer, s.cfg.AdvertiseIP, answers, time.Now().Unix())
	res := sip.NewResponseFromRequest(req, 200, "OK", []byte(answerSDP))
	res.AppendHeader(s.contactHeader(req.Transport()))
	ct := sip.ContentTypeHeader(sdpx.ContentType)
	res.AppendHeader(&ct)
	res.AppendHeader(sip.NewHeader("Allow", allowMethods))
	if err := tx.Respond(res); err != nil {
		log.Error("failed to send 200 OK", "error", err)
		return
	}
	if to := res.To(); to != nil {
		if tag, ok := to.Params.Get("tag"); ok {
			dlg.toTag = tag
		}
	}
	s.mu.Lock()
	s.dialogs[callID] = dlg
	s.mu.Unlock()

	s.persistRequest(req, "INVITE", sess, rec)
	s.persistResponse(req, res, "INVITE", sess)
}

func (s *Server) onAck(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	s.mu.Lock()
	sess := s.sessions[callID]
	s.mu.Unlock()
	s.persistRequest(req, "ACK", sess, nil)
}

func (s *Server) onBye(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	s.mu.Lock()
	sess := s.sessions[callID]
	s.mu.Unlock()
	s.persistRequest(req, "BYE", sess, nil)
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	_ = tx.Respond(res)
	s.persistResponse(req, res, "BYE", sess)
	s.mu.Lock()
	delete(s.sessions, callID)
	delete(s.dialogs, callID)
	s.mu.Unlock()
}

func (s *Server) onCancel(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	s.mu.Lock()
	sess := s.sessions[callID]
	s.mu.Unlock()
	s.persistRequest(req, "CANCEL", sess, nil)
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	_ = tx.Respond(res)
}

func (s *Server) onOptions(req *sip.Request, tx sip.ServerTransaction) {
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	res.AppendHeader(sip.NewHeader("Allow", allowMethods))
	_ = tx.Respond(res)
}

func negotiateSignalingOnly(offer *sdpx.SessionDescription) []sdpx.AnswerMedia {
	out := make([]sdpx.AnswerMedia, len(offer.Media))
	for i, m := range offer.Media {
		if m.Type != "audio" || m.Port == 0 {
			out[i] = sdpx.AnswerMedia{Accepted: false}
			continue
		}
		out[i] = sdpx.AnswerMedia{
			Accepted: true,
			Port:     signalingOnlyRTPPort,
			Codec:    sdpx.SelectCodec(&m),
			Label:    m.Label,
		}
	}
	return out
}

func (s *Server) persistRequest(req *sip.Request, method string, sess *sessionState, rec *metadata.Recording) {
	if rec == nil && sess != nil {
		rec = sess.metadata
	}
	row := s.buildRow(req, method, "", string(req.Body()), sess, rec)
	if err := s.storage.WriteSiprecRow(row); err != nil {
		s.log.Error("siprec write failed", "error", err, "method", method)
	}
}

func (s *Server) persistResponse(req *sip.Request, res *sip.Response, cseqMethod string, sess *sessionState) {
	row := s.buildRowFromResponse(req, res, cseqMethod, sess)
	if err := s.storage.WriteSiprecRow(row); err != nil {
		s.log.Error("siprec write failed", "error", err, "method", cseqMethod)
	}
}

func (s *Server) buildRow(req *sip.Request, method, responseCode, payload string, sess *sessionState, rec *metadata.Recording) ducklake.SiprecRow {
	callID := ""
	if req.CallID() != nil {
		callID = req.CallID().Value()
	}
	sessionID := callID
	caller, callee := sipParties(req)
	dataExtra := map[string]any{
		"sip_call_id": callID,
		"direction":   "in",
	}
	if sess != nil && len(sess.metadataXML) > 0 {
		dataExtra["raw_metadata_xml"] = string(sess.metadataXML)
	}
	if rec != nil {
		dataExtra["datamode"] = rec.DataMode
		dataExtra["participants"] = rec.Participants
		dataExtra["streams"] = rec.Streams
		dataExtra["sessions"] = rec.Sessions
		if id := rec.PrimarySIPSessionID(); id != "" {
			sessionID = id
		}
		if sess != nil && sess.recordingID != "" {
			dataExtra["recording_id"] = sess.recordingID
		}
	}
	srcIP, srcPort := splitHostPort(req.Source())
	dstIP, dstPort := splitHostPort(req.Destination())
	return ducklake.SiprecRow{
		SessionID:    sessionID,
		Caller:       caller,
		Callee:       callee,
		SrcIP:        srcIP,
		DstIP:        dstIP,
		SrcPort:      srcPort,
		DstPort:      dstPort,
		Method:       method,
		ResponseCode: responseCode,
		CseqMethod:   method,
		CID:          callID,
		NodeID:       s.cfg.NodeID,
		Payload:      payload,
		DataExtra:    dataExtra,
		Timestamp:    time.Now().UTC(),
	}
}

func (s *Server) buildRowFromResponse(req *sip.Request, res *sip.Response, cseqMethod string, sess *sessionState) ducklake.SiprecRow {
	payload := fmt.Sprintf("SIP/2.0 %d %s\r\n", res.StatusCode, res.Reason)
	if body := res.Body(); len(body) > 0 {
		payload += "\r\n" + string(body)
	}
	row := s.buildRow(req, cseqMethod, fmt.Sprintf("%d", res.StatusCode), payload, sess, nil)
	row.SrcIP, row.DstIP = row.DstIP, row.SrcIP
	row.SrcPort, row.DstPort = row.DstPort, row.SrcPort
	dataExtra := row.DataExtra
	if dataExtra == nil {
		dataExtra = map[string]any{}
	}
	dataExtra["direction"] = "out"
	row.DataExtra = dataExtra
	return row
}

func (s *Server) respondError(req *sip.Request, tx sip.ServerTransaction, code int, reason string) {
	res := sip.NewResponseFromRequest(req, code, reason, nil)
	_ = tx.Respond(res)
}

func (s *Server) contactHeader(transport string) *sip.ContactHeader {
	h := &sip.ContactHeader{
		Address: sip.Uri{
			Scheme: "sip",
			User:   "siprec",
			Host:   s.cfg.AdvertiseIP,
			Port:   s.cfg.SIPPort,
		},
		Params: sip.NewParams(),
	}
	h.Params.Add("+sip.srs", "")
	if transport != "" && !strings.EqualFold(transport, "udp") {
		h.Address.UriParams = sip.NewParams()
		h.Address.UriParams.Add("transport", strings.ToLower(transport))
	}
	return h
}

func hasSiprecRequire(req *sip.Request) bool {
	for _, h := range req.GetHeaders("Require") {
		if strings.Contains(strings.ToLower(h.Value()), "siprec") {
			return true
		}
	}
	return false
}

func sipParties(req *sip.Request) (caller, callee string) {
	if from := req.From(); from != nil {
		caller = from.Address.User
	}
	if to := req.To(); to != nil {
		callee = to.Address.User
	}
	return caller, callee
}

func splitHostPort(addr string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}
