// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// defaultCallCorrelationLua / defaultRegistrationCorrelationLua are bundled
// coordinator templates seeded (disabled) into correlation_scripts.
// Editable copies for operators and docs: examples/lua/correlation/*.lua
// Build-time copies (must stay in sync): correlation_seed_lua/*.lua
//
//go:embed correlation_seed_lua/sip_call.lua
var defaultCallCorrelationLua string

//go:embed correlation_seed_lua/sip_registration.lua
var defaultRegistrationCorrelationLua string

// seedSpec describes a single default correlation script the coordinator
// will seed on first start-up.
type seedSpec struct {
	profile  string
	hepAlias string
	hepID    int
	script   string
}

var defaultCorrelationSeeds = []seedSpec{
	{profile: "call", hepAlias: "SIP", hepID: 1, script: defaultCallCorrelationLua},
	{profile: "registration", hepAlias: "SIP", hepID: 1, script: defaultRegistrationCorrelationLua},
}

// seedDefaultCorrelationScript inserts the bundled correlation templates
// into correlation_scripts if no correlation script exists yet for each
// (hepid, profile) pair. status is set to FALSE so the seeds are
// visible in the admin UI but inactive until an operator explicitly
// enables them.
func seedDefaultCorrelationScript(db *sql.DB) error {
	if db == nil {
		return nil
	}
	ctx := context.Background()

	for _, s := range defaultCorrelationSeeds {
		if err := seedOne(ctx, db, s); err != nil {
			return err
		}
	}
	return nil
}

// seedOne inserts a single default correlation script if no row already
// exists for (hepid, profile, type='correlation'). If a previous homer-core
// release wrote a double-escaped copy of this very template, repair it in
// place so operators don't have to delete it manually after upgrading.
func seedOne(ctx context.Context, db *sql.DB, s seedSpec) error {
	// Legacy homer-core releases ran strings.ReplaceAll(script, "'", "''")
	// _before_ handing the script to the DuckDB driver, which itself quotes
	// parameters — so each ' became '' in storage and the UI showed a
	// broken Lua script with doubled quotes. We fingerprint those rows by
	// reproducing the exact mangled string and UPDATE only rows that still
	// match it byte-for-byte. Operator-edited rows are left untouched.
	mangled := strings.ReplaceAll(s.script, "'", "''")

	var count int
	if err := db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s
		 WHERE type = 'correlation' AND hepid = ? AND profile = ?`,
			services.CorrelationScriptsTable),
		s.hepID, s.profile).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s
			 SET script = ?
			 WHERE type = 'correlation'
			   AND hepid = ?
			   AND profile = ?
			   AND script = ?`,
				services.CorrelationScriptsTable),
			s.script, s.hepID, s.profile, mangled); err != nil {
			return err
		}
		return nil
	}

	guid, err := randomGUID()
	if err != nil {
		return err
	}

	// Use positional parameters — the DuckDB driver handles quoting. Do NOT
	// pre-escape single quotes; that would double-escape the stored Lua
	// (every ' would become '' in the database, breaking the editor).
	_, err = db.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO %s
		 (guid, profile, hep_alias, type, hepid, status, script, create_date)
		 VALUES (?, ?, ?, 'correlation', ?, FALSE, ?, current_timestamp)`,
			services.CorrelationScriptsTable),
		guid, s.profile, s.hepAlias, s.hepID, s.script)
	return err
}

// randomGUID returns a 32-hex-char random id without pulling in uuid.
func randomGUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
