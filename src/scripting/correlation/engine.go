// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package correlation provides a Lua-based call-id correlation engine that
// runs at query time inside the coordinator. It mirrors the goja/JavaScript
// correlation flow used by homer-core: a script, stored in the
// settings DuckDB table `correlation_scripts` and keyed by "<hepid>_<profile>"
// (e.g. "1_call"), receives the base rows of a transaction and returns a
// list of additional session_id values to merge into the final result.
//
// Scripts may perform read-only DuckDB queries against node data via the
// `executeSQL` helper; all SQL is validated by sqlvalidator, bounded by a
// per-call row limit, a per-request call counter and a context timeout.
package correlation

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// ScriptLoader abstracts reading active correlation scripts so the engine can
// be tested without the real settings DB.
type ScriptLoader interface {
	LoadActiveCorrelation(ctx context.Context) ([]LoadedScript, error)
}

// LoadedScript is the minimal shape of a row in `correlation_scripts` as consumed
// by the engine.
type LoadedScript struct {
	GUID    string
	HepID   int
	Profile string
	Script  string
}

// SQLExecutor abstracts the read side so executeSQL from Lua can be wired
// to the real FlightService in production and to a fake in tests.
type SQLExecutor interface {
	Query(ctx context.Context, sql string) ([]map[string]interface{}, error)
}

// CorrelationInput is everything the engine passes into a Lua script.
type CorrelationInput struct {
	HepID      int
	Profile    string
	ProtoType  int
	EventType  string
	BaseRows   []map[string]interface{}
	SessionIDs []string
	Nodes      []string
	TimeFrom   int64
	TimeTo     int64
}

// CorrelationResult is everything a Lua script may return.
type CorrelationResult struct {
	ExtraSessionIDs []string
	Debug           []string
}

// CorrelationEngine is the coordinator-side Lua engine.
type CorrelationEngine struct {
	cfg      config.CorrelationScriptingConfig
	loader   ScriptLoader
	executor SQLExecutor
	lakeName string

	scripts *scriptCache

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewEngine creates a correlation engine. When cfg.Enable is false the
// returned engine is nil and callers should treat it as "no correlation".
// executor is typically *services.FlightService; loader is typically
// DBScriptLoader.
func NewEngine(cfg config.CorrelationScriptingConfig, loader ScriptLoader, executor SQLExecutor, lakeName string) *CorrelationEngine {
	if !cfg.Enable {
		return nil
	}
	applyDefaults(&cfg)
	return &CorrelationEngine{
		cfg:      cfg,
		loader:   loader,
		executor: executor,
		lakeName: lakeName,
		scripts:  newScriptCache(),
		stopCh:   make(chan struct{}),
	}
}

// applyDefaults fills in zero-valued knobs so tests and tiny configs behave.
func applyDefaults(cfg *config.CorrelationScriptingConfig) {
	if cfg.SQLTimeoutMS <= 0 {
		cfg.SQLTimeoutMS = 5000
	}
	if cfg.ScriptTimeoutMS <= 0 {
		cfg.ScriptTimeoutMS = 3000
	}
	if cfg.MaxSQLCalls <= 0 {
		cfg.MaxSQLCalls = 16
	}
	if cfg.MaxSQLRows <= 0 {
		cfg.MaxSQLRows = 10000
	}
	if cfg.SyncIntervalSec < 0 {
		cfg.SyncIntervalSec = 0
	}
}

// Has reports whether an active script exists for the given proto/event.
// Used by handlers to skip allocating a VM when no script is registered.
func (e *CorrelationEngine) Has(hepid int, profile string) bool {
	if e == nil {
		return false
	}
	return e.scripts.has(scriptKey(hepid, profile))
}

// Correlate runs the Lua script for (hepid, profile) if one is active,
// returning additional session_id values to merge into the base result.
//
// Failure is never fatal to the calling request: any error is logged and
// nil is returned so the handler can return the base rows unchanged.
func (e *CorrelationEngine) Correlate(ctx context.Context, in CorrelationInput) *CorrelationResult {
	if e == nil {
		return nil
	}
	key := scriptKey(in.HepID, in.Profile)
	scr, ok := e.scripts.get(key)
	if !ok {
		return nil
	}

	scriptTimeout := time.Duration(e.cfg.ScriptTimeoutMS) * time.Millisecond
	vmCtx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()

	res, err := runScript(vmCtx, e, scr, in)
	if err != nil {
		logger.Warn("correlation: script execution failed", "key", key, "err", err.Error())
		return nil
	}
	return res
}

// ReloadFromDB reads all correlation scripts with status=true and replaces
// the in-memory cache.
func (e *CorrelationEngine) ReloadFromDB(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if e.loader == nil {
		return fmt.Errorf("correlation: no script loader configured")
	}
	items, err := e.loader.LoadActiveCorrelation(ctx)
	if err != nil {
		return err
	}
	fresh := make(map[string]*compiledScript, len(items))
	for _, it := range items {
		key := scriptKey(it.HepID, strings.ToLower(strings.TrimSpace(it.Profile)))
		fresh[key] = &compiledScript{
			key:    key,
			guid:   it.GUID,
			source: it.Script,
		}
	}
	e.scripts.replace(fresh)
	logger.Info("correlation: reloaded scripts", "count", len(fresh))
	return nil
}

// SyncLoop reloads the cache from the DB at SyncIntervalSec.
// It blocks until ctx is cancelled or Stop is called.
func (e *CorrelationEngine) SyncLoop(ctx context.Context) {
	if e == nil {
		return
	}
	if err := e.ReloadFromDB(ctx); err != nil {
		logger.Warn("correlation: initial reload failed", "err", err.Error())
	}
	if e.cfg.SyncIntervalSec <= 0 {
		return
	}
	t := time.NewTicker(time.Duration(e.cfg.SyncIntervalSec) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-t.C:
			if err := e.ReloadFromDB(ctx); err != nil {
				logger.Warn("correlation: periodic reload failed", "err", err.Error())
			}
		}
	}
}

// Stop signals the sync loop to exit.
func (e *CorrelationEngine) Stop() {
	if e == nil {
		return
	}
	e.stopOnce.Do(func() { close(e.stopCh) })
}

// scriptKey builds the canonical "<hepid>_<profile>" cache key. The profile
// is lowercased for case-insensitive lookup (handlers pass "call", migrations
// also pass "call").
func scriptKey(hepid int, profile string) string {
	return fmt.Sprintf("%d_%s", hepid, strings.ToLower(strings.TrimSpace(profile)))
}

// DBScriptLoader loads correlation scripts directly from the settings DB.
type DBScriptLoader struct {
	db *sql.DB
}

// NewDBScriptLoader wires the loader to the shared settings DuckDB handle.
func NewDBScriptLoader(db *sql.DB) *DBScriptLoader {
	return &DBScriptLoader{db: db}
}

// LoadActiveCorrelation reads every enabled correlation script.
func (l *DBScriptLoader) LoadActiveCorrelation(ctx context.Context) ([]LoadedScript, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("correlation: settings db not available")
	}
	rows, err := l.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT guid, hepid, profile, script
		 FROM %s
		 WHERE status = TRUE AND type = 'correlation'`, services.CorrelationScriptsTable))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LoadedScript
	for rows.Next() {
		var (
			guid    sql.NullString
			hepid   sql.NullInt64
			profile sql.NullString
			script  sql.NullString
		)
		if err := rows.Scan(&guid, &hepid, &profile, &script); err != nil {
			return nil, err
		}
		if !script.Valid || strings.TrimSpace(script.String) == "" {
			continue
		}
		out = append(out, LoadedScript{
			GUID:    guid.String,
			HepID:   int(hepid.Int64),
			Profile: profile.String,
			Script:  script.String,
		})
	}
	return out, rows.Err()
}

// FlightExecutor adapts *services.FlightService to the SQLExecutor interface
// (purely to make the engine independent of the services package for tests).
type FlightExecutor struct {
	F *services.FlightService
}

// Query satisfies SQLExecutor.
func (f FlightExecutor) Query(ctx context.Context, sql string) ([]map[string]interface{}, error) {
	if f.F == nil {
		return nil, fmt.Errorf("correlation: no flight service configured")
	}
	return f.F.Query(ctx, sql)
}
