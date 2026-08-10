// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func TestV4StatisticsQuery_RejectsUnsafeSQL(t *testing.T) {
	e := echo.New()
	h := NewStatisticsHandler(services.NewFlightService(nil, 0, false))

	body, err := json.Marshal(map[string]interface{}{
		"param": map[string]interface{}{
			"query": []map[string]string{
				{"rawquery": "DELETE FROM hep_proto_1_call"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v4/statistics/query", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4StatisticsQuery(c); err != nil {
		t.Fatalf("V4StatisticsQuery: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("SQL validation failed")) {
		t.Fatalf("expected validation error message, got %s", rec.Body.String())
	}
}

func TestV4StatisticsQuery_RejectsMultiStatement(t *testing.T) {
	e := echo.New()
	h := NewStatisticsHandler(services.NewFlightService(nil, 0, false))

	body, err := json.Marshal(map[string]interface{}{
		"param": map[string]interface{}{
			"query": []map[string]string{
				{"rawquery": "SELECT 1; DELETE FROM hep"},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v4/statistics/query", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4StatisticsQuery(c); err != nil {
		t.Fatalf("V4StatisticsQuery: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
	}
}
