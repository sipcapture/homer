// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package cli — Homer 7 → Homer 11 migration tool.
//
// Subcommands:
//   - settings : copy users / alias / auth_token / hepsub_mapping_schema /
//     mapping_schema (optional) / dashboard_settings (meta only) /
//     agent_location_session from a homer-app PostgreSQL `homer_config`
//     database into homer-core's DuckDB settings file.
//   - hep      : replay rows from a homer-app `hep_proto_*` PostgreSQL table
//     as HEP3 packets to a running homer-core node, preserving original
//     timestamps and capture metadata. Checkpointable.
//
// Both subcommands are idempotent — they UPSERT by guid/username/token and
// skip rows that already exist. Run them as many times as you like.

package cli

import (
	"flag"
	"fmt"
	"strings"
)

// MigrateFlags holds flags for the "homer-core migrate" subcommand.
type MigrateFlags struct {
	// Action is the migration sub-action: "settings" or "hep".
	Action string

	// Common.
	PgDSN          string
	ConfigPath     string
	DryRun         bool
	Verbose        bool

	// Settings-specific.
	DuckDBPath        string
	OwnerDefault      string
	IncludeMappings   bool
	IncludeDashboards bool
	SkipUsers         bool

	// HEP-replay-specific.
	HEPTarget      string // host:port
	HEPProto       string // udp|tcp
	HEPCaptureID   uint32 // override 0x000c capture agent id (0 = use stored)
	Tables         string // comma-separated, e.g. hep_proto_1_call,hep_proto_5_default
	Since          string // RFC3339, optional
	Until          string // RFC3339, optional
	BatchSize      int
	RatePPS        int
	CheckpointPath string
	Limit          int
}

type migrateFlagRefs struct {
	PgDSN      *string
	ConfigPath *string
	DryRun     *bool
	Verbose    *bool

	DuckDBPath        *string
	OwnerDefault      *string
	IncludeMappings   *bool
	IncludeDashboards *bool
	SkipUsers         *bool

	HEPTarget      *string
	HEPProto       *string
	HEPCaptureID   *uint
	Tables         *string
	Since          *string
	Until          *string
	BatchSize      *int
	RatePPS        *int
	CheckpointPath *string
	Limit          *int
}

// RegisterMigrateFlags creates a FlagSet for "homer-core migrate".
func RegisterMigrateFlags() (*flag.FlagSet, *migrateFlagRefs) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	refs := &migrateFlagRefs{}

	refs.PgDSN = fs.String("pg-dsn", "",
		"PostgreSQL DSN of source homer-app database (e.g. postgres://user:pass@host:5432/homer_config?sslmode=disable)")
	refs.ConfigPath = fs.String("config-path", "",
		"path to homer-core config (used by 'settings' to locate the DuckDB settings file)")
	refs.DryRun = fs.Bool("dry-run", false, "do not write anything; just count and report")
	refs.Verbose = fs.Bool("verbose", false, "log every row")

	refs.DuckDBPath = fs.String("duckdb", "",
		"explicit path to the homer-core settings DuckDB file (overrides config)")
	refs.OwnerDefault = fs.String("owner-default", "admin",
		"username to use as owner for rows that lack one in homer-app (e.g. legacy global mappings)")
	refs.IncludeMappings = fs.Bool("with-mappings", false,
		"also migrate mapping_schema rows (off by default — homer-core ships its own seeds)")
	refs.IncludeDashboards = fs.Bool("with-dashboards", true,
		"migrate dashboard_settings metadata; widget bodies are wiped (incompatible)")
	refs.SkipUsers = fs.Bool("skip-users", false,
		"do not migrate the users table (useful when SSO/LDAP is the source of truth)")

	refs.HEPTarget = fs.String("hep-target", "127.0.0.1:9060",
		"host:port of homer-core node HEP listener")
	refs.HEPProto = fs.String("hep-proto", "udp", "HEP transport: udp or tcp")
	refs.HEPCaptureID = fs.Uint("hep-capture-id", 0,
		"capture agent id for chunk 0x000c; 0 = preserve protocol_header.captureId from source")
	refs.Tables = fs.String("tables",
		"hep_proto_1_call,hep_proto_1_default,hep_proto_1_registration,hep_proto_5_default,hep_proto_100_default",
		"comma-separated list of source tables to replay")
	refs.Since = fs.String("since", "", "only replay rows with create_date >= TIME (RFC3339)")
	refs.Until = fs.String("until", "", "only replay rows with create_date <  TIME (RFC3339)")
	refs.BatchSize = fs.Int("batch-size", 5000, "rows fetched per SELECT batch")
	refs.RatePPS = fs.Int("rate", 5000, "throttle replay to N packets per second (0 = unlimited)")
	refs.CheckpointPath = fs.String("checkpoint", "",
		"file to persist (table → max id processed) for resume; default: ./homer7-migrate.checkpoint.json next to the binary")
	refs.Limit = fs.Int("limit", 0, "stop after N rows total (0 = no limit; useful for testing)")

	return fs, refs
}

// ParseMigrateFlags extracts MigrateFlags from parsed refs.
func ParseMigrateFlags(refs *migrateFlagRefs) MigrateFlags {
	return MigrateFlags{
		PgDSN:      strings.TrimSpace(*refs.PgDSN),
		ConfigPath: *refs.ConfigPath,
		DryRun:     *refs.DryRun,
		Verbose:    *refs.Verbose,

		DuckDBPath:        *refs.DuckDBPath,
		OwnerDefault:      *refs.OwnerDefault,
		IncludeMappings:   *refs.IncludeMappings,
		IncludeDashboards: *refs.IncludeDashboards,
		SkipUsers:         *refs.SkipUsers,

		HEPTarget:      *refs.HEPTarget,
		HEPProto:       strings.ToLower(*refs.HEPProto),
		HEPCaptureID:   uint32(*refs.HEPCaptureID),
		Tables:         *refs.Tables,
		Since:          *refs.Since,
		Until:          *refs.Until,
		BatchSize:      *refs.BatchSize,
		RatePPS:        *refs.RatePPS,
		CheckpointPath: *refs.CheckpointPath,
		Limit:          *refs.Limit,
	}
}

// RunMigrateCmd dispatches to the requested action.
func RunMigrateCmd(f MigrateFlags) error {
	if f.PgDSN == "" {
		return fmt.Errorf("--pg-dsn is required (PostgreSQL DSN of homer-app source DB)")
	}
	switch f.Action {
	case "", "settings":
		return RunMigrateSettings(f)
	case "hep":
		return RunMigrateHEP(f)
	default:
		return fmt.Errorf("unknown migrate action %q (expected 'settings' or 'hep')", f.Action)
	}
}
