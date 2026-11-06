// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package scripting

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/sipparser"
)

const tagScript = `
function onPacket()
  local seen = GetRawMessage()
  for i = 1, 40 do
    local cur = GetRawMessage()
    if cur ~= seen then
      seen = cur
    end
  end
  SetSIPHeader("X-Lua-Tag", seen)
end
`

func newTagEngine(t *testing.T, dir string) *LuaEngine {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "tag.lua"), []byte(tagScript), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := NewLuaEngine(&ScriptConfig{Enable: true, Engine: "lua", Folder: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine
}

func taggedPacket(payload string) *decoder.HEP {
	return &decoder.HEP{
		ProtoType: 1,
		Payload:   payload,
		SIP:       &sipparser.SipMsg{},
	}
}

func TestLuaEngineRunTagsOnlyItsPacket(t *testing.T) {
	engine := newTagEngine(t, t.TempDir())
	hep := taggedPacket("call-1")
	if err := engine.Run(hep); err != nil {
		t.Fatal(err)
	}
	if got := hep.SIP.CustomHeader["X-Lua-Tag"]; got != "call-1" {
		t.Fatalf("tag = %q, want call-1", got)
	}
}

// TestLuaEngineConcurrentRunKeepsPacketsIsolated is the regression for
// ingest workers sharing one LuaEngine. Each Run must tag the packet it
// was given, including when many goroutines call Run together.
func TestLuaEngineConcurrentRunKeepsPacketsIsolated(t *testing.T) {
	engine := newTagEngine(t, t.TempDir())

	const (
		workers = 4
		perG    = 80
	)
	var wg sync.WaitGroup
	errCh := make(chan error, workers*perG)
	start := make(chan struct{})

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for n := 0; n < perG; n++ {
				payload := fmt.Sprintf("w%d-%d", id, n)
				hep := taggedPacket(payload)
				if err := engine.Run(hep); err != nil {
					errCh <- err
					return
				}
				got := hep.SIP.CustomHeader["X-Lua-Tag"]
				if got != payload {
					errCh <- fmt.Errorf("packet %s tagged %q", payload, got)
					return
				}
			}
		}(w)
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestLuaEngineReloadDuringRun(t *testing.T) {
	dir := t.TempDir()
	engine := newTagEngine(t, dir)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				hep := taggedPacket("live")
				if err := engine.Run(hep); err != nil {
					t.Errorf("run: %v", err)
					return
				}
				if got := hep.SIP.CustomHeader["X-Lua-Tag"]; got != "live" {
					t.Errorf("tag = %q, want live", got)
					return
				}
			}
		}
	}()

	for i := 0; i < 20; i++ {
		if err := engine.Reload(); err != nil {
			close(stop)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()

	hep := taggedPacket("after")
	if err := engine.Run(hep); err != nil {
		t.Fatal(err)
	}
	if got := hep.SIP.CustomHeader["X-Lua-Tag"]; got != "after" {
		t.Fatalf("tag = %q, want after", got)
	}
}
