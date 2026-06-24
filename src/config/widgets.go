// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import "strings"

// WidgetControlCategories lists dashboard picker categories (lowercase keys).
var WidgetControlCategories = []string{
	"search", "visualize", "external", "utility", "games",
}

// WidgetsConfig configures which widget picker categories are available.
type WidgetsConfig struct {
	// Control maps category key → enabled. Missing key or true = enabled.
	Control map[string]bool `json:"control" mapstructure:"control"`
}

// DefaultWidgetControl returns all categories enabled.
func DefaultWidgetControl() map[string]bool {
	out := make(map[string]bool, len(WidgetControlCategories))
	for _, c := range WidgetControlCategories {
		out[c] = true
	}
	return out
}

// NormalizeWidgetControl merges operator overrides onto defaults so API/UI
// always see a full map.
func NormalizeWidgetControl(control map[string]bool) map[string]bool {
	out := DefaultWidgetControl()
	for k, v := range control {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

// WidgetCategoryEnabled reports whether a picker category is enabled.
// category is the UI label (e.g. "Games") or config key (e.g. "games").
func WidgetCategoryEnabled(control map[string]bool, category string) bool {
	key := strings.ToLower(strings.TrimSpace(category))
	if control == nil {
		return true
	}
	v, ok := control[key]
	if !ok {
		return true
	}
	return v
}
