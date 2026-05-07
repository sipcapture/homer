// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package main

import "embed"

// WebAssets contains the embedded web UI files (empty by default)
// To build with embedded UI:
// 1. Copy the dist/ folder to src/
// 2. Build normally: go build .
//
//go:embed all:dist
var WebAssets embed.FS

// HasEmbeddedUI returns true if UI is embedded
func HasEmbeddedUI() bool {
	_, err := WebAssets.ReadFile("dist/index.html")
	return err == nil
}
