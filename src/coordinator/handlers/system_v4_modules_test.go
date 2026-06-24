// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
)

func TestV4ModulesStatus_WidgetControl(t *testing.T) {
	e := echo.New()
	h := NewModulesHandler()
	h.SetWidgetControl(config.NormalizeWidgetControl(map[string]bool{"games": false}))

	req := httptest.NewRequest(http.MethodGet, "/api/v4/modules", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.V4ModulesStatus(c); err != nil {
		t.Fatalf("V4ModulesStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}

	var body ModulesStatusResponseV4
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Widgets.Control["games"] {
		t.Fatal("games should be false")
	}
	if !body.Data.Widgets.Control["search"] {
		t.Fatal("search should default true")
	}
}
