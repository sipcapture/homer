// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package correlation

import "sync"

// compiledScript holds an active correlation script's source plus metadata.
// The Lua state is NOT stored here because it is not goroutine-safe; a fresh
// state is built per request in runScript().
type compiledScript struct {
	key    string // "<hepid>_<profile>"
	guid   string
	source string
}

// scriptCache is a tiny RWMutex-protected map of compiled scripts keyed by
// "<hepid>_<profile>". It is replaced atomically on reload.
type scriptCache struct {
	mu sync.RWMutex
	m  map[string]*compiledScript
}

func newScriptCache() *scriptCache {
	return &scriptCache{m: make(map[string]*compiledScript)}
}

func (c *scriptCache) has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.m[key]
	return ok
}

func (c *scriptCache) get(key string) (*compiledScript, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.m[key]
	return s, ok
}

func (c *scriptCache) replace(next map[string]*compiledScript) {
	c.mu.Lock()
	c.m = next
	c.mu.Unlock()
}

// len reports the number of cached scripts (used by tests).
func (c *scriptCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}
