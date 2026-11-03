// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package input

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/homerconfig"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

func TestPostgresLegacyCatalogLogRedaction(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL legacy logging integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_log_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); _ = admin.Close(ctx) })
	pg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(pg.Host, strconv.Itoa(int(pg.Port))), User: url.UserPassword(pg.User, pg.Password), Path: "/" + name, RawQuery: "sslmode=disable"}
	previousConfig, previousLogger := homerconfig.MainConfig, logger.Logger
	defer func() { homerconfig.MainConfig = previousConfig; logger.Logger = previousLogger }()
	var output bytes.Buffer
	logger.Logger = slog.New(slog.NewTextHandler(&output, nil))
	homerconfig.MainConfig = &homerconfig.HomerServerConfig{Setting: &homerconfig.HomerServerSettings{}}
	settings := &homerconfig.MainConfig.Setting.DUCKLAKE_SETTINGS
	settings.Enable = true
	settings.CatalogPath = u.String()
	settings.DataPath = t.TempDir()
	settings.LakeName = "homer_lake"
	settings.ShardCount = 1
	input := NewHEPInput()
	if input.ducklakeManager == nil {
		t.Fatal("legacy storage did not start")
	}
	if err := input.ducklakeManager.Stop(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "DuckLake storage enabled") || !strings.Contains(got, name) {
		t.Fatal("missing catalog startup diagnostic")
	}
	if strings.Contains(got, u.String()) || (pg.Password != "" && strings.Contains(got, pg.Password)) {
		t.Fatal("legacy startup logged catalog credentials")
	}
}
