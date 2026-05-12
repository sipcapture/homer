// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// RunResetAdminPassword loads config, opens settings DuckDB, and sets
// coordinator.auth.admin_user password_hash from coordinator.auth.admin_password_hash.
func RunResetAdminPassword(configPath string) error {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return fmt.Errorf("--config-path is required for --reset-admin-password")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if strings.TrimSpace(cfg.Coordinator.SettingsDBPath) == "" {
		return fmt.Errorf("coordinator.settings_db_path is empty")
	}
	hash := strings.TrimSpace(cfg.Coordinator.Auth.AdminPasswordHash)
	if hash == "" {
		return fmt.Errorf("coordinator.auth.admin_password_hash is required for --reset-admin-password")
	}
	user := strings.TrimSpace(cfg.Coordinator.Auth.AdminUser)
	if user == "" {
		user = "admin"
	}

	db, err := services.OpenSettingsDB(cfg.Coordinator.SettingsDBPath)
	if err != nil {
		return fmt.Errorf("open settings db: %w", err)
	}
	defer db.Close()

	if err := services.EnsureSettingsSchema(db); err != nil {
		return fmt.Errorf("settings schema: %w", err)
	}

	ctx := context.Background()
	if err := services.ResetOrInsertAdminPassword(ctx, db, user, hash); err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "admin password updated for user %q in settings DB\n", user)
	return nil
}
