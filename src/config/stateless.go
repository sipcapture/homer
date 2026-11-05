// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"fmt"
	"net/url"
	"strings"
)

// validateStateless runs before any component starts. Scratch files, spill,
// extensions and caches may be local; durable data and credentials may not.
func validateStateless(cfg *Config) error {
	if !cfg.Stateless {
		return nil
	}
	require := func(ok bool, field, requirement string) error {
		if !ok {
			return fmt.Errorf("stateless: %s %s", field, requirement)
		}
		return nil
	}
	secret := func(value string) bool { return strings.TrimSpace(value) != "" && !IsShippedPlaceholderSecret(value) }
	if cfg.Coordinator.Enable {
		if err := require(IsPostgresSettingsPath(cfg.Coordinator.SettingsDBPath), "coordinator.settings_db_path", "must be a PostgreSQL connection string"); err != nil {
			return err
		}
		if err := require(secret(cfg.Coordinator.JWT.Secret), "coordinator.jwt.secret", "must be explicitly configured and shared across replicas"); err != nil {
			return err
		}
		if cfg.Coordinator.FlightSQLServer.Enable {
			if err := require(secret(cfg.Coordinator.FlightSQLServer.AuthToken), "coordinator.flightsql_server.auth_token", "must be explicitly configured"); err != nil {
				return err
			}
		}
	}
	if cfg.Node.Enable {
		if !IsLoopbackBindHost(cfg.Node.FlightServer.Host) {
			if err := require(secret(cfg.Node.FlightServer.AuthToken), "node.flight_server.auth_token", "must be explicitly configured"); err != nil {
				return err
			}
		}
		if cfg.Node.FlightSQLServer.Enable {
			if err := require(secret(cfg.Node.FlightSQLServer.AuthToken), "node.flightsql_server.auth_token", "must be explicitly configured"); err != nil {
				return err
			}
		}
	}
	remote := func(path string) bool {
		u, err := url.Parse(strings.TrimSpace(path))
		if err != nil || u.Host == "" {
			return false
		}
		switch u.Scheme {
		case "s3", "s3a", "az", "azure":
			return true
		}
		return false
	}
	check := func(field, catalog, path string) error {
		if err := require(IsPostgresSettingsPath(catalog), field+".catalog_path", "must be a PostgreSQL connection string"); err != nil {
			return err
		}
		return require(remote(path), field+" data path", "must use S3 or Azure object storage")
	}
	if cfg.Storage.Enable {
		dl := cfg.Storage.DuckLake
		if err := check("storage.ducklake", dl.CatalogPath, dl.DataPath); err != nil {
			return err
		}
		if dl.StoragePolicy.Enable {
			for i, v := range dl.StoragePolicy.Volumes {
				if v.CatalogPath != "" {
					if err := check(fmt.Sprintf("storage.ducklake.storage_policy.volumes[%d]", i), v.CatalogPath, v.Path); err != nil {
						return err
					}
				}
				if err := require(remote(v.Path), fmt.Sprintf("storage.ducklake.storage_policy.volumes[%d].path", i), "must use S3 or Azure object storage"); err != nil {
					return err
				}
			}
		}
	}
	if cfg.Node.Enable {
		dl := cfg.Node.DuckLake
		if len(dl.Volumes) == 0 {
			if err := check("node.ducklake", dl.CatalogPath, dl.DataPath); err != nil {
				return err
			}
		}
		for i, v := range dl.Volumes {
			if err := check(fmt.Sprintf("node.ducklake.volumes[%d]", i), v.CatalogPath, v.Path); err != nil {
				return err
			}
		}
	}
	for _, output := range cfg.Log.Output {
		if err := require(!strings.EqualFold(strings.TrimSpace(output), "file"), "log.output", "cannot include file; use stdout or syslog"); err != nil {
			return err
		}
	}
	return nil
}
