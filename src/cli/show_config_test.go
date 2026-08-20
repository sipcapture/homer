package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func TestIsSecretJSONKey(t *testing.T) {
	if !isSecretJSONKey("secret_access_key") {
		t.Fatal("secret_access_key should be redacted")
	}
	if isSecretJSONKey("max_tokens") {
		t.Fatal("max_tokens is not a secret")
	}
	if isSecretJSONKey("token_url") {
		t.Fatal("token_url is not a secret")
	}
	if isSecretJSONKey("engine") {
		t.Fatal("engine is not a secret")
	}
}

func TestRedactJSONSecrets_nested(t *testing.T) {
	root := map[string]any{
		"coordinator": map[string]any{
			"jwt": map[string]any{"secret": "super-secret", "expire": 3600},
		},
		"storage": map[string]any{
			"ducklake": map[string]any{
				"s3": map[string]any{
					"access_key_id":     "AKIA",
					"secret_access_key": "sk",
					"endpoint":          "http://127.0.0.1:9000",
				},
			},
		},
		"mcp": map[string]any{"max_tokens": 400},
	}
	redactJSONSecrets(root)
	jwt := root["coordinator"].(map[string]any)["jwt"].(map[string]any)
	if jwt["secret"] != redactedSecret {
		t.Fatalf("jwt.secret=%v", jwt["secret"])
	}
	if jwt["expire"] != 3600 {
		t.Fatal("non-secret jwt field changed")
	}
	s3 := root["storage"].(map[string]any)["ducklake"].(map[string]any)["s3"].(map[string]any)
	if s3["secret_access_key"] != redactedSecret || s3["access_key_id"] != redactedSecret {
		t.Fatalf("s3 keys not redacted: %#v", s3)
	}
	if s3["endpoint"] != "http://127.0.0.1:9000" {
		t.Fatal("s3 endpoint changed")
	}
	if root["mcp"].(map[string]any)["max_tokens"] != 400 {
		t.Fatal("max_tokens changed")
	}
}

func TestLookupJSONSection(t *testing.T) {
	root := map[string]any{
		"storage": map[string]any{
			"ducklake": map[string]any{
				"compaction": map[string]any{"engine": "duckdb"},
			},
		},
	}
	got, err := lookupJSONSection(root, "storage.ducklake.compaction")
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(map[string]any)
	if !ok || obj["engine"] != "duckdb" {
		t.Fatalf("got %#v", got)
	}
	if _, err := lookupJSONSection(root, "storage.missing"); err == nil {
		t.Fatal("expected missing section error")
	}
}

func TestFormatRunningConfig_defaultsEngineAndRedactsJWT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "homer.json")
	body := `{
  "storage": { "enable": true, "ducklake": { "compaction": { "enable": true } } },
  "coordinator": { "enable": true, "jwt": { "secret": "live-secret" } }
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := formatRunningConfig(cfg, ShowConfigFlags{Section: "storage.ducklake.compaction"})
	if err != nil {
		t.Fatal(err)
	}
	var compaction map[string]any
	if err := json.Unmarshal(out, &compaction); err != nil {
		t.Fatal(err)
	}
	if compaction["engine"] != "duckdb" {
		t.Fatalf("unset engine should default to duckdb, got %#v", compaction["engine"])
	}
	if compaction["enable"] != true {
		t.Fatalf("enable=%v", compaction["enable"])
	}

	full, err := formatRunningConfig(cfg, ShowConfigFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(full), "live-secret") {
		t.Fatal("jwt secret leaked")
	}
	if !strings.Contains(string(full), redactedSecret) {
		t.Fatal("expected redacted placeholder")
	}

	leaked, err := formatRunningConfig(cfg, ShowConfigFlags{IncludeSecrets: true, Compact: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(leaked), "live-secret") {
		t.Fatal("include-secrets should keep jwt secret")
	}
}
