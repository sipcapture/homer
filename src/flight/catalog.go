// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package flight

import (
	"context"
	"sync"

	"github.com/hugr-lab/airport-go/catalog"
	"github.com/sipcapture/homer-core/src/decoder"
)

// HEPCatalog implements the airport catalog interface for HEP data
type HEPCatalog struct {
	schemas    map[string]*HEPSchema
	bufferSize int
	mu         sync.RWMutex
}

// NewHEPCatalog creates a new HEP catalog
func NewHEPCatalog(bufferSize int) *HEPCatalog {
	c := &HEPCatalog{
		schemas:    make(map[string]*HEPSchema),
		bufferSize: bufferSize,
	}

	// Create default "hep" schema with tables
	hepSchema := NewHEPSchema("hep", bufferSize)
	c.schemas["hep"] = hepSchema

	return c
}

// Name returns the catalog name
func (c *HEPCatalog) Name() string {
	return "homer"
}

// Schemas returns all schemas in the catalog
func (c *HEPCatalog) Schemas(ctx context.Context) ([]catalog.Schema, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	schemas := make([]catalog.Schema, 0, len(c.schemas))
	for _, s := range c.schemas {
		schemas = append(schemas, s)
	}
	return schemas, nil
}

// Schema returns a schema by name
func (c *HEPCatalog) Schema(ctx context.Context, name string) (catalog.Schema, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if s, ok := c.schemas[name]; ok {
		return s, nil
	}
	return nil, catalog.ErrNotFound
}

// AddHEP adds a HEP packet to the catalog (to all relevant tables)
func (c *HEPCatalog) AddHEP(hep *decoder.HEP) {
	c.mu.RLock()
	schema, ok := c.schemas["hep"]
	c.mu.RUnlock()

	if ok {
		schema.AddHEP(hep)
	}
}

// GetStats returns statistics about the catalog
func (c *HEPCatalog) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, schema := range c.schemas {
		stats[name] = schema.GetStats()
	}
	return stats
}

// HEPSchema implements the catalog.Schema interface
type HEPSchema struct {
	name       string
	tables     map[string]*HEPTable
	bufferSize int
	mu         sync.RWMutex
}

// NewHEPSchema creates a new HEP schema with default tables
func NewHEPSchema(name string, bufferSize int) *HEPSchema {
	s := &HEPSchema{
		name:       name,
		tables:     make(map[string]*HEPTable),
		bufferSize: bufferSize,
	}

	// Create tables for different HEP types
	s.tables["packets"] = NewHEPTable("packets", "All HEP packets", bufferSize)
	s.tables["sip"] = NewHEPTable("sip", "SIP packets (proto_type=1)", bufferSize)
	s.tables["rtcp"] = NewHEPTable("rtcp", "RTCP packets (proto_type=5)", bufferSize)
	s.tables["logs"] = NewHEPTable("logs", "Log packets (proto_type=100)", bufferSize)

	return s
}

// Name returns the schema name
func (s *HEPSchema) Name() string {
	return s.name
}

// Comment returns the schema description
func (s *HEPSchema) Comment() string {
	return "HEP packet data from Homer Server"
}

// Tables returns all tables in the schema
func (s *HEPSchema) Tables(ctx context.Context) ([]catalog.Table, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tables := make([]catalog.Table, 0, len(s.tables))
	for _, t := range s.tables {
		tables = append(tables, t)
	}
	return tables, nil
}

// Table returns a table by name
func (s *HEPSchema) Table(ctx context.Context, name string) (catalog.Table, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if t, ok := s.tables[name]; ok {
		return t, nil
	}
	return nil, catalog.ErrNotFound
}

// ScalarFunctions returns all scalar functions (none for now)
func (s *HEPSchema) ScalarFunctions(ctx context.Context) ([]catalog.ScalarFunction, error) {
	return nil, nil
}

// TableFunctions returns all table functions (none for now)
func (s *HEPSchema) TableFunctions(ctx context.Context) ([]catalog.TableFunction, error) {
	return nil, nil
}

// TableFunctionsInOut returns all table functions with row input (none for now)
func (s *HEPSchema) TableFunctionsInOut(ctx context.Context) ([]catalog.TableFunctionInOut, error) {
	return nil, nil
}

// AddHEP adds a HEP packet to the appropriate tables
func (s *HEPSchema) AddHEP(hep *decoder.HEP) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Add to "packets" table (all packets)
	if t, ok := s.tables["packets"]; ok {
		t.AddHEP(hep)
	}

	// Add to type-specific tables
	switch hep.ProtoType {
	case 1: // SIP
		if t, ok := s.tables["sip"]; ok {
			t.AddHEP(hep)
		}
	case 5: // RTCP
		if t, ok := s.tables["rtcp"]; ok {
			t.AddHEP(hep)
		}
	case 100: // Log
		if t, ok := s.tables["logs"]; ok {
			t.AddHEP(hep)
		}
	}
}

// GetStats returns statistics about the schema
func (s *HEPSchema) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, table := range s.tables {
		stats[name] = table.GetStats()
	}
	return stats
}
