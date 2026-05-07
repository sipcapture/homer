// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package correlation

import (
	"context"
	"fmt"

	"github.com/sipcapture/golua/lua"
	"github.com/sipcapture/homer-core/src/scripting/luar"
)

// runScript builds a fresh Lua state for this correlation call, registers
// the helper API, loads the user script and invokes
// `correlate(data, nodes, ctx)`.
//
// The third argument `ctx` is new in homer-core 11.0.64 and carries the
// request-level context that used to be invisible to scripts:
//
//	ctx.time_from     int64   start of the user's timerange (ms epoch)
//	ctx.time_to       int64   end of the user's timerange (ms epoch)
//	ctx.session_ids   {..}    the full callid list from the request
//	ctx.hepid         int     e.g. 1 for SIP
//	ctx.profile       string  e.g. "call", "registration"
//	ctx.proto_type    int     mirrors hepid for convenience
//	ctx.event_type    string  mirrors profile for convenience
//
// Older scripts declared as `function correlate(data, nodes)` keep working
// unchanged because Lua silently drops extra arguments.
//
// Per-request VM keeps isolation simple and avoids the non-thread-safe
// `lua.State` becoming a concurrency hazard across HTTP handlers. Pool-based
// reuse can be added later without changing this signature.
func runScript(ctx context.Context, e *CorrelationEngine, scr *compiledScript, in CorrelationInput) (*CorrelationResult, error) {
	L := lua.NewState()
	if L == nil {
		return nil, fmt.Errorf("correlation: failed to create Lua state")
	}
	defer L.Close()
	L.OpenLibs()

	bridge := newSQLBridge(ctx, e, in)
	result := &CorrelationResult{}

	luar.Register(L, "", luar.Map{
		"executeSQL":     bridge.executeSQL,
		"getDataByField": bridge.getDataByField,
		"scriptLog": func(level, msg string) {
			result.Debug = appendDebug(result.Debug, level, msg)
			scriptLog(level, msg)
		},
		"HashString": hashString,
		"HashTable":  hashTable,
	})

	if err := L.DoString(scr.source); err != nil {
		return nil, fmt.Errorf("correlation: DoString failed: %w", err)
	}

	L.GetGlobal("correlate")
	if !L.IsFunction(-1) {
		L.Pop(1)
		return nil, fmt.Errorf("correlation: script does not define function `correlate(data, nodes)`")
	}

	// Push args. A nil slice must become an empty Lua table so scripts can
	// iterate with `for i = 1, #data do`; luar.GoToLua produces nil for nil
	// slices, which would raise an attempt-to-index error.
	pushRowsAsTable(L, in.BaseRows)
	pushStringsAsTable(L, in.Nodes)
	pushCorrelationContext(L, in)

	if err := ctx.Err(); err != nil {
		L.Pop(4) // function + 3 args pushed above
		return nil, fmt.Errorf("correlation: context cancelled before call: %w", err)
	}

	if err := L.Call(3, 1); err != nil {
		return nil, fmt.Errorf("correlation: correlate() call failed: %w", err)
	}
	defer L.Pop(1) // pop the return value once we're done reading it

	result.ExtraSessionIDs = readStringArray(L, -1)

	if err := ctx.Err(); err != nil {
		// Script ran but context expired while we were unwinding; treat as
		// timeout rather than returning partial results.
		return nil, fmt.Errorf("correlation: context expired: %w", err)
	}

	return result, nil
}

// pushRowsAsTable turns a Go []map[string]interface{} into an array-style
// Lua table so scripts can index it 1..#data. Values inside each row that
// are primitives are converted directly; unknown types are skipped.
func pushRowsAsTable(L *lua.State, rows []map[string]interface{}) {
	L.CreateTable(len(rows), 0)
	for i, row := range rows {
		L.PushInteger(int64(i + 1))
		pushRow(L, row)
		L.SetTable(-3)
	}
}

// pushRow pushes a single row (map[string]interface{}) as a Lua string-keyed
// table onto the stack. Primitive values become their Lua equivalents; all
// other kinds (slice, map, pointer) are stringified via %v to stay
// conservative — scripts shouldn't need nested structures for call-id
// correlation.
func pushRow(L *lua.State, row map[string]interface{}) {
	L.CreateTable(0, len(row))
	for k, v := range row {
		L.PushString(k)
		pushValue(L, v)
		L.SetTable(-3)
	}
}

// pushValue handles the small set of value kinds we care about (string/
// number/bool/nil). Scripts don't need rich value types for correlation.
func pushValue(L *lua.State, v interface{}) {
	switch x := v.(type) {
	case nil:
		L.PushNil()
	case string:
		L.PushString(x)
	case bool:
		L.PushBoolean(x)
	case int:
		L.PushInteger(int64(x))
	case int32:
		L.PushInteger(int64(x))
	case int64:
		L.PushInteger(x)
	case uint:
		L.PushInteger(int64(x))
	case uint32:
		L.PushInteger(int64(x))
	case uint64:
		L.PushInteger(int64(x))
	case float32:
		L.PushNumber(float64(x))
	case float64:
		L.PushNumber(x)
	default:
		L.PushString(fmt.Sprintf("%v", v))
	}
}

// pushStringsAsTable pushes a simple 1-based Lua array of strings.
func pushStringsAsTable(L *lua.State, items []string) {
	L.CreateTable(len(items), 0)
	for i, s := range items {
		L.PushInteger(int64(i + 1))
		L.PushString(s)
		L.SetTable(-3)
	}
}

// pushCorrelationContext pushes the request-level context as a Lua table so
// scripts can see timerange, requested session_ids and routing info without
// having to recompute them from `data`. Keys are chosen to mirror the fields
// a Homer operator already sees in /api/v4/transactions/messages JSON.
func pushCorrelationContext(L *lua.State, in CorrelationInput) {
	L.CreateTable(0, 8)

	L.PushString("time_from")
	L.PushInteger(in.TimeFrom)
	L.SetTable(-3)

	L.PushString("time_to")
	L.PushInteger(in.TimeTo)
	L.SetTable(-3)

	L.PushString("hepid")
	L.PushInteger(int64(in.HepID))
	L.SetTable(-3)

	L.PushString("profile")
	L.PushString(in.Profile)
	L.SetTable(-3)

	// proto_type / event_type duplicate hepid/profile under the names the
	// rest of the HTTP API uses; this keeps Lua scripts aligned with the
	// JSON request shape they probably already know.
	L.PushString("proto_type")
	L.PushInteger(int64(in.ProtoType))
	L.SetTable(-3)

	L.PushString("event_type")
	L.PushString(in.EventType)
	L.SetTable(-3)

	L.PushString("session_ids")
	pushStringsAsTable(L, in.SessionIDs)
	L.SetTable(-3)

	L.PushString("nodes")
	pushStringsAsTable(L, in.Nodes)
	L.SetTable(-3)
}

// readStringArray reads a Lua table at the given stack index into a Go
// []string. Non-string elements are skipped. A non-table on the stack
// returns nil (the script either returned nothing or returned the wrong
// type — both are handled by the caller as "no extras").
func readStringArray(L *lua.State, idx int) []string {
	if !L.IsTable(idx) {
		return nil
	}
	n := int(L.ObjLen(idx))
	if n <= 0 {
		return nil
	}
	// Normalise idx to absolute — Lua stack grows as we iterate.
	absIdx := idx
	if absIdx < 0 {
		absIdx = L.GetTop() + absIdx + 1
	}
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		L.PushInteger(int64(i))
		L.GetTable(absIdx)
		if L.IsString(-1) {
			out = append(out, L.ToString(-1))
		}
		L.Pop(1)
	}
	return dedupNonEmpty(out)
}
