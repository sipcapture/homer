// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type exportJob struct {
	ID          string
	Type        string
	Status      string
	DownloadURL string
	FilePath    string
	ContentType string
	CreatedAt   time.Time
}

type ExportStore struct {
	mu   sync.RWMutex
	jobs map[string]exportJob
}

func NewExportStore() *ExportStore {
	return &ExportStore{
		jobs: make(map[string]exportJob),
	}
}

func (s *ExportStore) Create(exportType string) exportJob {
	job := exportJob{
		ID:        newExportID(),
		Type:      exportType,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	return job
}

func (s *ExportStore) Get(id string) (exportJob, bool) {
	s.mu.RLock()
	job, ok := s.jobs[id]
	s.mu.RUnlock()
	return job, ok
}

func (s *ExportStore) Update(job exportJob) {
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
}

func newExportID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().String()))
	}
	return hex.EncodeToString(buf)
}
