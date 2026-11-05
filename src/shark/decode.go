// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package shark runs tshark on a one-packet PCAP, the HOMER 7 decoder_shark path.
package shark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

const (
	defaultBin     = "/usr/bin/tshark"
	defaultTimeout = 15 * time.Second
	maxOutput      = 4 << 20
)

// Enabled reports whether this proto/event should be sent to tshark.
// An empty protocols list matches every key. An explicit list matches
// "proto_event" (for example "1_call"), the same keys HOMER 7 used.
func Enabled(cfg config.DecoderSharkConfig, protoType int, eventType string) bool {
	if !cfg.Active {
		return false
	}
	if len(cfg.Protocols) == 0 {
		return true
	}
	key := strconv.Itoa(protoType) + "_" + strings.TrimSpace(eventType)
	for _, protocol := range cfg.Protocols {
		protocol = strings.TrimSpace(protocol)
		if protocol == "*" || strings.EqualFold(protocol, key) {
			return true
		}
	}
	return false
}

// DecodePCAP feeds pcap to `tshark -Q -T json -r -` and returns the JSON document.
// Extra arguments come from cfg.Param, split on whitespace. The process is not a shell.
func DecodePCAP(ctx context.Context, cfg config.DecoderSharkConfig, pcap []byte) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		bin = defaultBin
	}
	if strings.ContainsAny(bin, "\x00\n\r") {
		return nil, fmt.Errorf("decoder_shark.bin is invalid")
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-Q", "-T", "json", "-r", "-"}
	if extra := strings.TrimSpace(cfg.Param); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	applyCredentials(cmd, cfg.UID, cfg.GID)
	// Kill the whole process group. tshark (or a shell stand-in) may leave
	// children holding the pipes, and Wait would otherwise block until they exit.
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Stdin = bytes.NewReader(pcap)
	stdout := &limitWriter{n: maxOutput}
	stderr := &limitWriter{n: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	out := bytes.TrimSpace(stdout.Bytes())
	if stdout.over {
		return nil, fmt.Errorf("tshark output exceeded %d bytes", maxOutput)
	}
	if len(out) > 0 && json.Valid(out) {
		return json.RawMessage(append([]byte(nil), out...)), nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("tshark timed out after %s", timeout)
	}
	msg := strings.TrimSpace(stderr.String())
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("tshark: %s", firstLine(msg))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tshark returned no JSON")
	}
	return nil, fmt.Errorf("tshark returned invalid JSON")
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// limitWriter keeps the first n bytes and discards the rest so a chatty
// tshark cannot fill memory. over is set once the cap is hit.
type limitWriter struct {
	buf  bytes.Buffer
	n    int
	over bool
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.over || w.buf.Len() >= w.n {
		w.over = true
		return len(p), nil
	}
	remain := w.n - w.buf.Len()
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.over = true
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *limitWriter) Bytes() []byte  { return w.buf.Bytes() }
func (w *limitWriter) String() string { return w.buf.String() }
