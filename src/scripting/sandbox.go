// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package scripting

import "github.com/sipcapture/golua/lua"

// dangerousLuaGlobals are stripped after opening the safe subset of stdlib.
// os/io/package/debug are RCE primitives (GHSA-m726-p857-j3cc). loadfile/dofile
// read the host FS. load/loadstring accept bytecode. getfenv/setfenv can restore
// a previous full _G. ffi/jit are LuaJIT escape hatches.
var dangerousLuaGlobals = []string{
	"os", "io", "package", "debug",
	"dofile", "loadfile", "load", "loadstring",
	"require", "module",
	"getfenv", "setfenv",
	"ffi", "jit",
}

// OpenSandbox loads base, table, string, and math only — not OpenLibs().
// Helper APIs (HEP getters, executeSQL, …) are registered by the caller.
func OpenSandbox(L *lua.State) {
	if L == nil {
		return
	}
	L.OpenBase()
	L.OpenTable()
	L.OpenString()
	L.OpenMath()
	for _, name := range dangerousLuaGlobals {
		L.PushNil()
		L.SetGlobal(name)
	}
}
