// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package shark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

func TestEnabled(t *testing.T) {
	cfg := config.DecoderSharkConfig{Active: true}
	if !Enabled(cfg, 1, "call") {
		t.Fatal("empty protocols should match every key")
	}
	cfg.Protocols = []string{"1_call", "1_registration"}
	if !Enabled(cfg, 1, "call") {
		t.Fatal("1_call should match")
	}
	if Enabled(cfg, 1, "default") {
		t.Fatal("1_default is not in the list")
	}
	cfg.Active = false
	if Enabled(cfg, 1, "call") {
		t.Fatal("inactive decoder must not run")
	}
}

func TestDecodePCAP(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "tshark")
	script := "#!/bin/sh\n" +
		"magic=$(dd bs=4 count=1 2>/dev/null | od -An -tx1 | tr -d ' \\n')\n" +
		"printf '%s' \"$*\" > " + shellQuote(argsFile) + "\n" +
		"cat >/dev/null\n" +
		"if [ \"$magic\" != \"d4c3b2a1\" ]; then echo \"bad pcap $magic\" >&2; exit 1; fi\n" +
		"printf '%s\\n' '[{\"_source\":{\"layers\":{\"isup.called\":\"632107608\"}}}]'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	raw, err := DecodePCAP(context.Background(), config.DecoderSharkConfig{
		Bin:        bin,
		Param:      "-d udp.port==5060,sip",
		TimeoutSec: 5,
	}, []byte{0xd4, 0xc3, 0xb2, 0xa1, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	var packets []map[string]interface{}
	if err := json.Unmarshal(raw, &packets); err != nil {
		t.Fatal(err)
	}
	if len(packets) != 1 {
		t.Fatalf("packets=%d", len(packets))
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	if !strings.Contains(got, "-Q -T json --no-duplicate-keys -r -") || !strings.Contains(got, "-d udp.port==5060,sip") {
		t.Fatalf("args %q", got)
	}
}

func TestDecodePCAPRetriesWithoutDuplicateFlag(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tshark")
	script := "#!/bin/sh\n" +
		"cat >/dev/null\n" +
		"case \" $* \" in\n" +
		"*--no-duplicate-keys*) echo \"tshark: unrecognized option '--no-duplicate-keys'\" >&2; exit 1;;\n" +
		"esac\n" +
		"printf '%s\\n' '[{\"_source\":{\"layers\":{\"isup.message_type\":\"12\"}}}]'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := DecodePCAP(context.Background(), config.DecoderSharkConfig{Bin: bin, TimeoutSec: 5}, []byte{0xd4, 0xc3, 0xb2, 0xa1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "isup.message_type") {
		t.Fatalf("raw %s", raw)
	}
}

func TestDecodePCAPTimeout(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tshark")
	script := "#!/bin/sh\ncat >/dev/null\nsleep 30\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := DecodePCAP(context.Background(), config.DecoderSharkConfig{Bin: bin, TimeoutSec: 1}, []byte{0xd4, 0xc3, 0xb2, 0xa1})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("timeout took %s", time.Since(start))
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
