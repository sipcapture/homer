// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
)

type ModulesHandler struct {
	lokiEnabled     bool
	lokiTemplate    string
	lokiExternalURL string
}

func NewModulesHandler() *ModulesHandler {
	return &ModulesHandler{}
}

type ModulesStatusResponseV4 struct {
	Data struct {
		Loki struct {
			Enable      bool   `json:"enable"`
			Template    string `json:"template,omitempty"`
			ExternalURL string `json:"external_url,omitempty"`
		} `json:"loki"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *ModulesHandler) V4ModulesStatus(c echo.Context) error {
	resp := ModulesStatusResponseV4{}
	resp.Data.Loki.Enable = h.lokiEnabled
	resp.Data.Loki.Template = h.lokiTemplate
	resp.Data.Loki.ExternalURL = h.lokiExternalURL
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// StreamService abstracts services.StreamService so tests can stub it
// without pulling the services package into the handlers test scope.
// The only method V4HepStream needs is Subscribe plus a probe for
// "Configured" used by the 503 fast path.
type StreamService interface {
	Configured() bool
	Subscribe(ctx context.Context, f hepstream.Filter, history int) (<-chan hepstream.Event, func(), error)
}

// StreamHandler owns the live HEP WebSocket endpoint. Construction
// leaves service/config nil so coordinator.go can inject them after
// wiring the downstream pieces; when service is nil the endpoint
// returns 503 and the UI knows to hide its "Use live traffic" toggle.
type StreamHandler struct {
	service      StreamService
	allowPayload bool
	historyLimit int
}

func NewStreamHandler() *StreamHandler {
	return &StreamHandler{}
}

// SetService wires the fan-out service and the coordinator-side config
// knobs into the handler. Pass nil service to keep the endpoint
// stubbed (useful while the feature is being rolled out).
func (h *StreamHandler) SetService(s StreamService, allowPayload bool, historyLimit int) {
	h.service = s
	h.allowPayload = allowPayload
	if historyLimit < 0 {
		historyLimit = 0
	}
	h.historyLimit = historyLimit
}
