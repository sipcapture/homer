package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "https://homer.example.com", want: "https://homer.example.com"},
		{in: " HTTPS://Homer.Example.com/ ", want: "https://homer.example.com"},
		{in: "https://homer.example.com:443", want: "https://homer.example.com"},
		{in: "http://homer.example.com:80", want: "http://homer.example.com"},
		{in: "http://homer.example.com:8080", want: "http://homer.example.com:8080"},
		{in: "https://[::1]:9080", want: "https://[::1]:9080"},
		{in: "", wantErr: true},
		{in: "null", wantErr: true},
		{in: "homer.example.com", wantErr: true},
		{in: "ftp://homer.example.com", wantErr: true},
		{in: "https://", wantErr: true},
		{in: "https://homer.example.com/ui", wantErr: true},
		{in: "https://homer.example.com/?a=1", wantErr: true},
		{in: "https://homer.example.com/#x", wantErr: true},
		{in: "https://user@homer.example.com", wantErr: true},
		{in: "https://*.example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeOrigin(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeOrigin(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeOrigin(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeOrigin(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLoadCookieTrustedOrigins(t *testing.T) {
	t.Run("env override is normalized", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOMER_COORDINATOR_SETTINGS_DB_PATH", filepath.Join(dir, "settings.duckdb"))
		t.Setenv("HOMER_NODE_DUCKLAKE_CATALOG_PATH", filepath.Join(dir, "homer_catalog.sqlite"))
		t.Setenv("HOMER_COORDINATOR_JWT_COOKIE_TRUSTED_ORIGINS_0", "https://Homer.Example.com/")
		t.Setenv("HOMER_COORDINATOR_JWT_COOKIE_TRUSTED_ORIGINS_1", "http://10.0.0.5:9080")
		cfg, err := Load(writeTmpConfig(t, `{}`))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		want := []string{"https://homer.example.com", "http://10.0.0.5:9080"}
		if got := cfg.Coordinator.JWT.CookieTrustedOrigins; !reflect.DeepEqual(got, want) {
			t.Fatalf("CookieTrustedOrigins = %q, want %q", got, want)
		}
	})

	t.Run("invalid entry is rejected", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("HOMER_COORDINATOR_SETTINGS_DB_PATH", filepath.Join(dir, "settings.duckdb"))
		t.Setenv("HOMER_NODE_DUCKLAKE_CATALOG_PATH", filepath.Join(dir, "homer_catalog.sqlite"))
		_, err := Load(writeTmpConfig(t, `{"coordinator":{"jwt":{"cookie_trusted_origins":["https://homer.example.com/ui"]}}}`))
		if err == nil {
			t.Fatal("Load accepted a trusted origin with a path")
		}
		if !strings.Contains(err.Error(), "coordinator.jwt.cookie_trusted_origins[0]") {
			t.Fatalf("error does not name the offending entry: %v", err)
		}
	})
}
