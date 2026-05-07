// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package scripting provides Lua scripting support for HEP packet processing.
package scripting

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/sipcapture/homer-core/src/decoder"
)

// ScriptEngine interface for running scripts on HEP packets
type ScriptEngine interface {
	Run(hep *decoder.HEP) error
	Close()
}

// ScriptConfig holds the scripting configuration
type ScriptConfig struct {
	Enable    bool
	Engine    string
	Folder    string
	HEPFilter []int
	// LokiCustomLabels mirrors remote_logging.loki (allowlist for SetLokiLabel).
	LokiCustomLabels []string
}

var scriptCache = fastcache.New(32 * 1024 * 1024)

// NewScriptEngine creates a new script engine based on configuration
func NewScriptEngine(cfg *ScriptConfig) (ScriptEngine, error) {
	if !cfg.Enable {
		return nil, nil
	}

	switch strings.ToLower(cfg.Engine) {
	case "lua", "":
		return NewLuaEngine(cfg)
	default:
		return nil, fmt.Errorf("unknown script engine: %s", cfg.Engine)
	}
}

// ScanLuaCode scans the script folder for Lua files
func ScanLuaCode(folder string) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(nil)

	if folder == "" {
		return buf, nil
	}

	dir, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	for _, file := range dir {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if !strings.HasSuffix(name, ".lua") {
			continue
		}
		path := filepath.Join(folder, name)
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(buf, f)
		f.Close()
		if err != nil {
			return nil, err
		}
	}

	return buf, nil
}

// ExtractFunctions extracts function names from Lua code
func ExtractFunctions(r io.Reader) []string {
	var funcs []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := cutSpace(scanner.Text())
		if strings.HasPrefix(line, "--") {
			continue
		}
		if strings.HasPrefix(line, "function") {
			if b, e := strings.Index(line, "("), strings.Index(line, ")"); b > -1 && e > -1 && b < e {
				funcs = append(funcs, line[len("function"):e+1])
			}
		}
	}
	return funcs
}

func cutSpace(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, str)
}

// HashString returns md5, sha1 or sha256 sum of a string
func HashString(algo, s string) string {
	switch algo {
	case "md5":
		return fmt.Sprintf("%x", md5.Sum([]byte(s)))
	case "sha1":
		return fmt.Sprintf("%x", sha1.Sum([]byte(s)))
	case "sha256":
		return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
	}
	return s
}

// HashTable is a simple key-value store for scripts
func HashTable(op, key, val string) string {
	switch op {
	case "get":
		if res := scriptCache.Get(nil, []byte(key)); res != nil {
			return string(res)
		}
	case "set":
		scriptCache.Set([]byte(key), []byte(val))
	case "del":
		scriptCache.Del([]byte(key))
	}
	return ""
}

// ShouldProcessHEP checks if the HEP packet should be processed by the script
func ShouldProcessHEP(protoType uint32, filter []int) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if uint32(f) == protoType {
			return true
		}
	}
	return false
}
