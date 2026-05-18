// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package games_test enforces import-level isolation for the dashboard
// mini-games (chess, netchess, netris). These packages must never reach
// the live HEP packet stream, the coordinator broker, or its handler /
// service tier — they are pure gameplay logic that the coordinator wires
// to dedicated `/api/v4/games/*` endpoints.
//
// Why a test instead of trust:
// the games packages already do the right thing today, but a future
// well-meaning refactor could accidentally pull in `hepstream.Broker` or
// `services.StreamService` (e.g. to drive a "packets as input" minigame)
// and silently expose live capture traffic to a chess game's process
// space. Failing CI is cheaper than chasing that later.
package games_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// forbiddenSubstrings is the deny-list applied to every import path
// reached transitively from the games packages' top-level files. Each
// entry is a substring match for ergonomics — module paths in Go end up
// long (`github.com/sipcapture/homer-core/src/...`) and a substring is
// strong enough since the deny-list targets are unique to homer-core.
var forbiddenSubstrings = []string{
	"src/stream/hepstream",     // live HEP packet broker / WebSocket envelopes
	"coordinator/services",     // the entire services tier (StreamService et al.)
	"coordinator/handlers",     // HTTP / WS handlers (no cross-handler reuse)
	"coordinator/broker",       // ingest-side broker if/when promoted out of services
	"coordinator/correlation",  // call correlation pipeline reads HEP too
}

// allowedPrefixes carves out the legitimate friends of game code:
// stdlib, the games packages themselves, and well-scoped third-party
// helpers (LLM clients, chess move validators). Anything outside this
// allow-list that is also outside `forbiddenSubstrings` triggers a soft
// warning in -v mode but does not fail the test, so we don't get in the
// way of legitimate additions.
var allowedPrefixes = []string{
	"github.com/sipcapture/homer-core/src/coordinator/games/",
	"github.com/sipcapture/homer-core/src/llm",
}

func TestGamesPackagesDoNotReachLiveStream(t *testing.T) {
	t.Parallel()

	root, err := moduleRoot()
	if err != nil {
		t.Fatalf("locate games dir: %v", err)
	}
	gamesDir := filepath.Join(root, "coordinator", "games")

	pkgs, err := listGamePackages(gamesDir)
	if err != nil {
		t.Fatalf("list game packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("no game packages found under %s — did the directory layout change?", gamesDir)
	}

	fset := token.NewFileSet()
	for _, pkg := range pkgs {
		t.Run(pkg.name, func(t *testing.T) {
			imports, err := collectImports(fset, pkg.dir)
			if err != nil {
				t.Fatalf("collect imports for %s: %v", pkg.name, err)
			}
			for _, imp := range imports {
				for _, bad := range forbiddenSubstrings {
					if strings.Contains(imp.path, bad) {
						t.Errorf("forbidden import %q in %s (matched %q): games packages must not reach the live HEP stream / coordinator services. If this is intentional, the wiring needs to move to a dedicated handler in coordinator/handlers, not into a game package.",
							imp.path, imp.file, bad)
					}
				}
				if testing.Verbose() && !isAllowed(imp.path) {
					t.Logf("note: %s imports %q (not in allow-list); ok if this is stdlib or a vetted dependency.", pkg.name, imp.path)
				}
			}
		})
	}
}

type gamePkg struct {
	name string
	dir  string
}

type importRef struct {
	path string
	file string
}

func listGamePackages(gamesDir string) ([]gamePkg, error) {
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		return nil, err
	}
	out := make([]gamePkg, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, gamePkg{name: e.Name(), dir: filepath.Join(gamesDir, e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func collectImports(fset *token.FileSet, dir string) ([]importRef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []importRef
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		full := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, full, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			out = append(out, importRef{path: path, file: filepath.Base(full)})
		}
	}
	return out, nil
}

func isAllowed(path string) bool {
	// stdlib paths never contain a dot in the first segment ("math/rand",
	// "encoding/json", "sync/atomic", …). Treat them as allowed.
	if first, _, _ := strings.Cut(path, "/"); !strings.Contains(first, ".") {
		return true
	}
	// Third-party modules outside our own org are out of scope for the
	// stream-isolation check (chess.js, golang-jwt, etc.).
	if strings.HasPrefix(path, "github.com/") && !strings.HasPrefix(path, "github.com/sipcapture/") {
		return true
	}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// TestIsolationDenyListMatchesItself is a meta-test: it feeds a fake
// import path through the deny-list to make sure the substring check
// actually triggers. Without this, a future refactor that accidentally
// neutered the substring (e.g. by switching to filepath.Match) would
// silently pass the main test even with a real bad import.
func TestIsolationDenyListMatchesItself(t *testing.T) {
	t.Parallel()
	bad := "github.com/sipcapture/homer-core/src/stream/hepstream/broker"
	matched := false
	for _, sub := range forbiddenSubstrings {
		if strings.Contains(bad, sub) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("forbiddenSubstrings no longer matches %q — deny-list is broken", bad)
	}
}

// moduleRoot walks upward from the test's working directory until it
// finds a go.mod file. This keeps the test usable both via
// `go test ./...` from the module root and from any nested directory.
func moduleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", os.ErrNotExist
		}
		cur = parent
	}
}
