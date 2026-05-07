// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package correlation

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/VictoriaMetrics/fastcache"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// scriptCacheKV is a small process-wide key/value store used by Lua scripts
// for cross-request memoisation. It mirrors HashTable from the writer-side
// scripting package (src/scripting/scriptengine.go) but lives here so the
// correlation engine has no import cycle with the writer.
var scriptCacheKV = fastcache.New(32 * 1024 * 1024)

// hashString returns md5/sha1/sha256 of s. Unknown algos return s unchanged.
func hashString(algo, s string) string {
	switch strings.ToLower(algo) {
	case "md5":
		return fmt.Sprintf("%x", md5.Sum([]byte(s)))
	case "sha1":
		return fmt.Sprintf("%x", sha1.Sum([]byte(s)))
	case "sha256":
		return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
	}
	return s
}

// hashTable is a simple get/set/del k/v helper for scripts.
func hashTable(op, key, val string) string {
	switch strings.ToLower(op) {
	case "get":
		if res := scriptCacheKV.Get(nil, []byte(key)); res != nil {
			return string(res)
		}
	case "set":
		scriptCacheKV.Set([]byte(key), []byte(val))
	case "del":
		scriptCacheKV.Del([]byte(key))
	}
	return ""
}

// scriptLog forwards a script-emitted message to the process logger.
// ERROR routes to Error, WARN to Warn; everything else is Debug.
func scriptLog(level, msg string) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "ERROR":
		logger.Error(fmt.Sprintf("[correlation/script] %s", msg))
	case "WARN", "WARNING":
		logger.Warn(fmt.Sprintf("[correlation/script] %s", msg))
	case "INFO":
		logger.Info(fmt.Sprintf("[correlation/script] %s", msg))
	default:
		logger.Debug(fmt.Sprintf("[correlation/script] %s", msg))
	}
}

// appendDebug keeps a trimmed in-memory trace of script-emitted log lines so
// the handler could surface them in debug responses. Capped to avoid unbounded
// growth from a misbehaving script.
func appendDebug(buf []string, level, msg string) []string {
	const maxEntries = 64
	entry := fmt.Sprintf("%s: %s", strings.ToUpper(level), truncate(msg, 512))
	if len(buf) >= maxEntries {
		return buf
	}
	return append(buf, entry)
}

// dedupNonEmpty returns the input without duplicates and without empty
// strings, preserving order of first appearance.
func dedupNonEmpty(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, v := range items {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
