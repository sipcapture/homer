// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import "testing"

func TestWidgetCategoryEnabled(t *testing.T) {
	control := map[string]bool{"games": false}
	if WidgetCategoryEnabled(control, "Games") {
		t.Fatal("Games should be disabled")
	}
	if !WidgetCategoryEnabled(control, "Search") {
		t.Fatal("Search should default enabled")
	}
	if !WidgetCategoryEnabled(nil, "games") {
		t.Fatal("nil control: missing key means enabled")
	}
}

func TestNormalizeWidgetControl(t *testing.T) {
	out := NormalizeWidgetControl(map[string]bool{"games": false})
	for _, c := range WidgetControlCategories {
		if c == "games" {
			if out[c] {
				t.Fatal("games should be false")
			}
			continue
		}
		if !out[c] {
			t.Fatalf("%s should default true", c)
		}
	}
}
