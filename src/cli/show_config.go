package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sipcapture/homer-core/src/config"
)

const redactedSecret = "********"

// secretJSONKeys are exact JSON object keys that must not appear in
// show-running-config output unless --include-secrets is set.
var secretJSONKeys = map[string]struct{}{
	"admin_password_hash":  {},
	"access_key_id":        {},
	"api_key":              {},
	"auth_token":           {},
	"bind_password":        {},
	"client_secret":        {},
	"homer_token":          {},
	"key":                  {},
	"pass":                 {},
	"password":             {},
	"s3_access_key_id":     {},
	"s3_secret_access_key": {},
	"secret":               {},
	"secret_access_key":    {},
	"token":                {},
}

// ShowConfigFlags holds flags for "homer show-running-config".
type ShowConfigFlags struct {
	ConfigPath     string
	Section        string
	IncludeSecrets bool
	Compact        bool
}

type showConfigFlagRefs struct {
	ConfigPath     *string
	Section        *string
	IncludeSecrets *bool
	Compact        *bool
}

// RegisterShowConfigFlags creates a FlagSet for "homer show-running-config".
func RegisterShowConfigFlags() (*flag.FlagSet, *showConfigFlagRefs) {
	fs := flag.NewFlagSet("show-running-config", flag.ExitOnError)
	refs := &showConfigFlagRefs{}
	refs.ConfigPath = fs.String("config-path", "", "path to config file or directory (same as the server)")
	refs.Section = fs.String("section", "", "print a dotted JSON path only, e.g. storage.ducklake.compaction")
	refs.IncludeSecrets = fs.Bool("include-secrets", false, "do not redact passwords, tokens, and keys")
	refs.Compact = fs.Bool("compact", false, "emit a single-line JSON object")
	return fs, refs
}

// ParseShowConfigFlags extracts ShowConfigFlags from parsed flag refs.
func ParseShowConfigFlags(refs *showConfigFlagRefs) ShowConfigFlags {
	return ShowConfigFlags{
		ConfigPath:     *refs.ConfigPath,
		Section:        *refs.Section,
		IncludeSecrets: *refs.IncludeSecrets,
		Compact:        *refs.Compact,
	}
}

// RunShowConfigCmd prints the effective config after file + env + defaults.
// This is not attached to a live process: it is what Homer would start with.
func RunShowConfigCmd(f ShowConfigFlags) error {
	cfg, err := config.Load(f.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	out, err := formatRunningConfig(cfg, f)
	if err != nil {
		return err
	}
	if f.IncludeSecrets {
		fmt.Fprintln(os.Stderr, "warning: --include-secrets prints credentials")
	}
	_, err = os.Stdout.Write(append(out, '\n'))
	return err
}

func formatRunningConfig(cfg *config.Config, f ShowConfigFlags) ([]byte, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode config json: %w", err)
	}
	if section := strings.TrimSpace(f.Section); section != "" {
		root, err = lookupJSONSection(root, section)
		if err != nil {
			return nil, err
		}
	}
	if !f.IncludeSecrets {
		redactJSONSecrets(root)
	}
	if f.Compact {
		return json.Marshal(root)
	}
	return json.MarshalIndent(root, "", "  ")
}

func lookupJSONSection(root any, path string) (any, error) {
	cur := root
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, fmt.Errorf("invalid --section %q", path)
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("section %q is not an object", path)
		}
		next, ok := obj[part]
		if !ok {
			return nil, fmt.Errorf("section %q not found", path)
		}
		cur = next
	}
	return cur, nil
}

func redactJSONSecrets(v any) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			if isSecretJSONKey(k) {
				if s, ok := child.(string); ok && s != "" {
					n[k] = redactedSecret
					continue
				}
			}
			redactJSONSecrets(child)
		}
	case []any:
		for _, child := range n {
			redactJSONSecrets(child)
		}
	}
}

func isSecretJSONKey(key string) bool {
	_, ok := secretJSONKeys[strings.ToLower(key)]
	return ok
}
