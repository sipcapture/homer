// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package scripting

import (
	"strings"
	"testing"

	"github.com/sipcapture/golua/lua"
)

func newSandboxedState(t *testing.T) *lua.State {
	t.Helper()
	L := lua.NewState()
	if L == nil {
		t.Fatal("lua.NewState returned nil")
	}
	t.Cleanup(L.Close)
	OpenSandbox(L)
	return L
}

func TestOpenSandboxBlocksOSExecute(t *testing.T) {
	L := newSandboxedState(t)
	err := L.DoString(`os.execute("true")`)
	if err == nil {
		t.Fatal("expected os.execute to fail in the sandbox")
	}
	if !strings.Contains(err.Error(), "os") && !strings.Contains(err.Error(), "nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenSandboxBlocksIOPopen(t *testing.T) {
	L := newSandboxedState(t)
	err := L.DoString(`io.popen("true")`)
	if err == nil {
		t.Fatal("expected io.popen to fail in the sandbox")
	}
}

func TestOpenSandboxKeepsStringGsub(t *testing.T) {
	L := newSandboxedState(t)
	if err := L.DoString(`_G.__sandbox_out = ("id_b2b-1"):gsub("_b2b%-%d+$", "")`); err != nil {
		t.Fatalf("string.gsub should work: %v", err)
	}
	L.GetGlobal("__sandbox_out")
	defer L.Pop(1)
	if got := L.ToString(-1); got != "id" {
		t.Fatalf("gsub result = %q, want %q", got, "id")
	}
}

func TestOpenSandboxDangerousGlobalsAreNil(t *testing.T) {
	L := newSandboxedState(t)
	for _, name := range []string{"os", "io", "package", "debug", "loadfile", "dofile", "require", "ffi", "jit"} {
		L.GetGlobal(name)
		nilish := L.IsNil(-1)
		L.Pop(1)
		if !nilish {
			t.Errorf("%s should be nil", name)
		}
	}
}
