// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	// VirtualKindDataExtraJSON extracts a value from the DuckLake data_extra JSON column.
	VirtualKindDataExtraJSON = "data_extra_json"
)

// VirtualMatchLike wraps the value with SQL LIKE '%value%' (escaped).
// VirtualMatchEquals uses exact equality after trim.
// VirtualMatchAbsent / VirtualMatchPresent are driven by filter.virtual_absent / virtual_present (checkboxes), not filter.virtual values.
const (
	VirtualMatchLike    = "like"
	VirtualMatchEquals  = "equals"
	VirtualMatchAbsent  = "absent"
	VirtualMatchPresent = "present"
)

var virtualPathSegmentRe = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_-]*$`)

// simpleJSONPathSegmentRe matches keys that DuckDB accepts without quoting.
var simpleJSONPathSegmentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// VirtualFieldRule is a validated rule derived from fields_mapping[].virtual.
type VirtualFieldRule struct {
	Kind  string
	Path  string // logical JSON path without leading $. (e.g. to_tag or nested foo_bar)
	Match string // like | equals | absent | present
}

// fieldMappingVirtualRaw is the optional virtual block on a mapping field row.
type fieldMappingVirtualRaw struct {
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Match string `json:"match,omitempty"`
}

// fieldMappingEntryRaw matches one element of fields_mapping JSON arrays.
type fieldMappingEntryRaw struct {
	ID      string                  `json:"id"`
	Virtual *fieldMappingVirtualRaw `json:"virtual,omitempty"`
}

// VirtualRulesFromFieldsMapping parses fields_mapping JSON and returns virtual
// rules keyed by field id. Non-virtual rows are skipped.
func VirtualRulesFromFieldsMapping(raw json.RawMessage) (map[string]VirtualFieldRule, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var entries []fieldMappingEntryRaw
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("fields_mapping: %w", err)
	}
	out := make(map[string]VirtualFieldRule)
	for _, e := range entries {
		if e.Virtual == nil || strings.TrimSpace(e.ID) == "" {
			continue
		}
		rule, err := validateVirtualRule(e.ID, e.Virtual)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", e.ID, err)
		}
		out[e.ID] = rule
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func validateVirtualRule(fieldID string, v *fieldMappingVirtualRaw) (VirtualFieldRule, error) {
	if v == nil {
		return VirtualFieldRule{}, fmt.Errorf("virtual spec is nil")
	}
	kind := strings.TrimSpace(v.Kind)
	if kind != VirtualKindDataExtraJSON {
		return VirtualFieldRule{}, fmt.Errorf("unsupported virtual.kind %q", v.Kind)
	}
	path := strings.TrimSpace(v.Path)
	if path == "" {
		return VirtualFieldRule{}, fmt.Errorf("virtual.path is required")
	}
	if err := validateVirtualJSONPath(path); err != nil {
		return VirtualFieldRule{}, err
	}
	match := strings.TrimSpace(v.Match)
	if match == "" {
		match = VirtualMatchLike
	}
	if match != VirtualMatchLike && match != VirtualMatchEquals && match != VirtualMatchAbsent && match != VirtualMatchPresent {
		return VirtualFieldRule{}, fmt.Errorf("unsupported virtual.match %q", v.Match)
	}
	_ = fieldID // reserved for future aliasing
	return VirtualFieldRule{Kind: kind, Path: path, Match: match}, nil
}

func validateVirtualJSONPath(path string) error {
	parts := strings.Split(path, ".")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return fmt.Errorf("invalid virtual.path %q", path)
		}
		if !virtualPathSegmentRe.MatchString(p) {
			return fmt.Errorf("invalid path segment %q in %q", p, path)
		}
	}
	return nil
}

// DuckJSONPath returns a DuckDB JSON path literal like '$.to_tag' or
// '$.custom_headers."X-icc_uuid"' when a segment contains hyphens or other
// characters that require JSONPath quoting.
func DuckJSONPath(logicalPath string) string {
	parts := strings.Split(logicalPath, ".")
	escaped := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		escaped = append(escaped, duckJSONPathSegment(p))
	}
	return "$." + strings.Join(escaped, ".")
}

func duckJSONPathSegment(segment string) string {
	if simpleJSONPathSegmentRe.MatchString(segment) {
		return segment
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range segment {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
