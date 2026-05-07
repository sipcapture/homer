// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package remotelog

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	// MaxCustomLokiLabelsPerMessage caps Lua SetLokiLabel cardinality per HEP packet.
	MaxCustomLokiLabelsPerMessage = 5
	maxCustomLokiLabelValueLen    = 1024
)

var lokiLabelKeyRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Built-in Loki label keys produced by homer-core (custom keys must not collide).
var builtinLokiLabelKeys = map[string]struct{}{
	"job": {}, "hostname": {}, "node": {}, "type": {}, "method": {}, "response": {}, "protocol": {},
	"src_ip": {}, "src_port": {}, "dst_ip": {}, "dst_port": {},
}

// ReservedPrometheusStyle rejects double-underscore names used by Loki/Prometheus internals.
func reservedPrometheusLokiKey(key string) bool {
	if strings.HasPrefix(key, "__") {
		return true
	}
	_, isBuiltin := builtinLokiLabelKeys[key]
	return isBuiltin
}

// LokiCustomLabelAllowlist builds a set from config (trimmed, non-empty keys).
func LokiCustomLabelAllowlist(allow []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, k := range allow {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = struct{}{}
	}
	return out
}

// ValidateLokiCustomLabelKey checks allowlist membership, reserved names, and Loki-style key syntax.
func ValidateLokiCustomLabelKey(key string, allow map[string]struct{}) error {
	if len(allow) == 0 {
		return fmt.Errorf("loki_custom_labels allowlist is empty")
	}
	if key == "" {
		return fmt.Errorf("empty key")
	}
	if reservedPrometheusLokiKey(key) {
		return fmt.Errorf("reserved or built-in label key %q", key)
	}
	if !lokiLabelKeyRe.MatchString(key) {
		return fmt.Errorf("invalid label key syntax")
	}
	if _, ok := allow[key]; !ok {
		return fmt.Errorf("key %q not in loki_custom_labels allowlist", key)
	}
	return nil
}

// ValidateLokiCustomLabelValue checks non-empty printable-ish value length.
func ValidateLokiCustomLabelValue(val string) error {
	if strings.TrimSpace(val) == "" {
		return fmt.Errorf("empty value")
	}
	if len(val) > maxCustomLokiLabelValueLen {
		return fmt.Errorf("value exceeds %d bytes", maxCustomLokiLabelValueLen)
	}
	for _, r := range val {
		if r == '\n' || r == '\r' {
			return fmt.Errorf("value must not contain newlines")
		}
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			return fmt.Errorf("value contains non-printable runes")
		}
	}
	return nil
}

// MergeCustomLokiLabelsIntoStream adds validated custom labels; built-in keys on dst always win.
func MergeCustomLokiLabelsIntoStream(dst map[string]string, custom map[string]string) {
	if len(custom) == 0 {
		return
	}
	for k, v := range custom {
		if reservedPrometheusLokiKey(k) {
			continue
		}
		if _, exists := dst[k]; exists {
			continue
		}
		dst[k] = v
	}
}
