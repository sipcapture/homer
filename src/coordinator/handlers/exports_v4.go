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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type ExportsHandler struct {
	store  *ExportStore
	shared *services.RuntimeState
}

func (h *ExportsHandler) SetRuntimeState(state *services.RuntimeState) { h.shared = state }

type sharedExport struct {
	Job     exportJob
	Payload []byte
}

func (h *ExportsHandler) sharedExport(c echo.Context, id string) (sharedExport, bool, error) {
	payload, found, err := h.shared.Get(c.Request().Context(), "export", id, false)
	var result sharedExport
	if err == nil && found {
		err = json.Unmarshal(payload, &result)
	}
	return result, found, err
}

func NewExportsHandler(store *ExportStore) *ExportsHandler {
	return &ExportsHandler{store: store}
}

type ExportRequestV4 struct {
	Type  string      `json:"type"`
	Query interface{} `json:"query"`
}

type ExportResponseV4 struct {
	Data struct {
		ExportID string `json:"export_id"`
		Status   string `json:"status"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type ExportStatusResponseV4 struct {
	Data struct {
		ExportID    string `json:"export_id"`
		Status      string `json:"status"`
		DownloadURL string `json:"download_url,omitempty"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *ExportsHandler) V4ExportsCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	var req ExportRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if req.Type == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "type is required")
	}
	if !isExportTypeAllowed(req.Type) {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid export type")
	}

	job := exportJob{ID: newExportID(), Type: strings.ToLower(req.Type), CreatedAt: time.Now().UTC(), Status: "completed"}
	job.DownloadURL = "/api/v4/exports/" + job.ID + "/download"
	if h.shared != nil {
		payload, contentType, err := exportPayload(job.ID, job.Type, req.Query)
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create export")
		}
		job.ContentType = contentType
		data, err := json.Marshal(sharedExport{Job: job, Payload: payload})
		if err != nil {
			return err
		}
		if err := h.shared.Put(c.Request().Context(), "export", job.ID, data, time.Now().Add(24*time.Hour)); err != nil {
			return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "Failed to store export")
		}
	} else {
		filePath, contentType, err := writeExportFile(job.ID, job.Type, req.Query)
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create export file")
		}
		job.FilePath, job.ContentType = filePath, contentType
		h.store.Update(job)
	}
	resp := ExportResponseV4{Meta: buildMeta(c, "")}
	resp.Data.ExportID = job.ID
	resp.Data.Status = job.Status
	return c.JSON(http.StatusCreated, resp)
}

func (h *ExportsHandler) V4ExportsGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	exportID := c.Param("exportId")
	if exportID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Export id is required")
	}

	job, ok := h.store.Get(exportID)
	if h.shared != nil {
		shared, found, err := h.sharedExport(c, exportID)
		if err != nil {
			return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "Failed to retrieve export")
		}
		job, ok = shared.Job, found
	}
	if !ok {
		return writeError(c, http.StatusNotFound, "Not Found", "Export not found")
	}

	resp := ExportStatusResponseV4{Meta: buildMeta(c, "")}
	resp.Data.ExportID = job.ID
	resp.Data.Status = job.Status
	resp.Data.DownloadURL = job.DownloadURL
	return c.JSON(http.StatusOK, resp)
}

func (h *ExportsHandler) V4ExportsDownload(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	exportID := c.Param("exportId")
	if exportID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Export id is required")
	}

	if h.shared != nil {
		shared, found, err := h.sharedExport(c, exportID)
		if err != nil {
			return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "Failed to retrieve export")
		}
		if !found {
			return writeError(c, http.StatusNotFound, "Not Found", "Export not found")
		}
		return c.Blob(http.StatusOK, shared.Job.ContentType, shared.Payload)
	}
	job, ok := h.store.Get(exportID)
	if !ok || job.FilePath == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Export not found")
	}

	return c.File(job.FilePath)
}

func isExportTypeAllowed(value string) bool {
	switch strings.ToLower(value) {
	case "pcap", "text":
		return true
	default:
		return false
	}
}

func writeExportFile(exportID, exportType string, query interface{}) (string, string, error) {
	baseDir := filepath.Join(os.TempDir(), "homer-exports")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", "", err
	}

	var (
		ext         string
		contentType string
	)
	switch strings.ToLower(exportType) {
	case "pcap":
		ext = ".pcap"
		contentType = "application/vnd.tcpdump.pcap"
	case "text":
		ext = ".txt"
		contentType = "text/plain"
	default:
		ext = ".dat"
		contentType = "application/octet-stream"
	}

	filePath := filepath.Join(baseDir, exportID+ext)
	payload, _, err := exportPayload(exportID, exportType, query)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filePath, payload, 0o644); err != nil {
		return "", "", err
	}
	return filePath, contentType, nil
}

func exportPayload(exportID, exportType string, query interface{}) ([]byte, string, error) {
	if strings.EqualFold(exportType, "pcap") {
		return []byte{}, "application/vnd.tcpdump.pcap", nil
	}
	payload, err := json.MarshalIndent(map[string]interface{}{
		"export_id": exportID,
		"type":      exportType,
		"query":     query,
	}, "", "  ")
	return payload, "text/plain", err
}
