// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// env.go extends viper's env binding to cover two classes of config
// fields that AutomaticEnv() alone does not handle:
//
//   1. Scalar fields that are not registered via v.SetDefault() in
//      setDefaults(). viper.AutomaticEnv() only reads ENV for keys it
//      already "knows" — so variables like HOMER_COORDINATOR_JWT_SECRET
//      or HOMER_STORAGE_DUCKLAKE_COMPACTION_ENABLE used to be silently
//      ignored. bindAllEnvs() walks Config{} via reflection and calls
//      v.BindEnv() for every scalar leaf.
//
//   2. Indexed slices of structs (volumes, nodes). viper cannot guess
//      how many indices exist, so applySliceEnvOverrides() scans
//      os.Environ() itself, groups HOMER_<PATH>_<idx>_<field> entries
//      and feeds the result into viper via v.Set(...) — Unmarshal with
//      WeaklyTypedInput=true then decodes it into []VolumeConfig /
//      []NodeEndpoint.

package config

import (
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// bindAllEnvs registers BindEnv for every scalar leaf of t so that
// AutomaticEnv() can pick up HOMER_* overrides. A slice of primitives
// is also bound by its root path.
func bindAllEnvs(v *viper.Viper, t reflect.Type, path []string) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := mapstructureName(f)
		if tag == "" {
			continue
		}
		full := append(append([]string(nil), path...), tag)
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Struct:
			bindAllEnvs(v, ft, full)
		case reflect.Slice:
			// Slice of primitives (log.output []string) — bind the path
			// itself; AutomaticEnv is useless for a slice of structs and
			// applySliceEnvOverrides() handles that case.
			elemKind := ft.Elem().Kind()
			if elemKind == reflect.Ptr {
				elemKind = ft.Elem().Elem().Kind()
			}
			if elemKind != reflect.Struct {
				_ = v.BindEnv(strings.Join(full, "."))
			}
		default:
			_ = v.BindEnv(strings.Join(full, "."))
		}
	}
}

// applySliceEnvOverrides finds every slice-of-struct field reachable
// from Config{} and converts HOMER_<PATH>_<idx>_<field> ENV variables
// into a []map[string]any written to viper via v.Set(). Unmarshal (with
// WeaklyTypedInput=true) handles type coercion from strings.
//
// Behaviour:
//   - sparse indices (0, 2, 5) are allowed — gaps are filled with empty
//     entries, final length = max(idx)+1;
//   - unknown FIELD names are silently ignored (protects against typos
//     in unrelated ENV sets);
//   - values are kept as strings, mapstructure performs type coercion.
func applySliceEnvOverrides(v *viper.Viper) {
	walkSliceOfStruct(reflect.TypeOf(Config{}), nil, func(path []string, elem reflect.Type) {
		prefix := "HOMER_" + strings.ToUpper(strings.Join(path, "_")) + "_"
		fields := uppercaseTagMap(elem)
		if len(fields) == 0 {
			return
		}
		entries := map[int]map[string]any{}
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, prefix) {
				continue
			}
			eq := strings.IndexByte(env, '=')
			if eq < 0 {
				continue
			}
			rest := strings.TrimPrefix(env[:eq], prefix)
			us := strings.IndexByte(rest, '_')
			if us < 0 {
				continue
			}
			idx, err := strconv.Atoi(rest[:us])
			if err != nil || idx < 0 {
				continue
			}
			fieldKey, ok := fields[rest[us+1:]]
			if !ok {
				continue
			}
			if entries[idx] == nil {
				entries[idx] = map[string]any{}
			}
			entries[idx][fieldKey] = env[eq+1:]
		}
		if len(entries) == 0 {
			return
		}
		maxIdx := 0
		for i := range entries {
			if i > maxIdx {
				maxIdx = i
			}
		}
		slice := make([]map[string]any, maxIdx+1)
		for i := 0; i <= maxIdx; i++ {
			if entries[i] != nil {
				slice[i] = entries[i]
			} else {
				slice[i] = map[string]any{}
			}
		}
		v.Set(strings.Join(path, "."), slice)
	})
}

// applyPrimitiveSliceEnvOverrides finds every slice-of-primitive field
// reachable from Config{} (e.g. ingest.sip.aleg_ids, ingest.sip.custom_headers,
// log.output) and converts indexed HOMER_<PATH>_<idx> ENV variables into an
// ordered slice written to viper via v.Set().
//
// This complements bindAllEnvs, which only binds the non-indexed HOMER_<PATH>
// form for primitive slices. The docker-compose convention documented for
// these fields uses the indexed form (HOMER_INGEST_SIP_ALEG_IDS_0=X-Session),
// which viper's AutomaticEnv() does not understand on its own.
//
// Behaviour:
//   - sparse indices (0, 2, 5) are allowed — gaps are filled with empty
//     strings, final length = max(idx)+1;
//   - the suffix after the prefix must be a pure integer, which keeps this
//     from colliding with slice-of-struct overrides (HOMER_<PATH>_<idx>_<field>)
//     handled by applySliceEnvOverrides;
//   - values are kept as strings, mapstructure performs type coercion.
func applyPrimitiveSliceEnvOverrides(v *viper.Viper) {
	walkSliceOfPrimitive(reflect.TypeOf(Config{}), nil, func(path []string) {
		prefix := "HOMER_" + strings.ToUpper(strings.Join(path, "_")) + "_"
		entries := map[int]string{}
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, prefix) {
				continue
			}
			eq := strings.IndexByte(env, '=')
			if eq < 0 {
				continue
			}
			idx, err := strconv.Atoi(env[len(prefix):eq])
			if err != nil || idx < 0 {
				continue
			}
			entries[idx] = env[eq+1:]
		}
		if len(entries) == 0 {
			return
		}
		maxIdx := 0
		for i := range entries {
			if i > maxIdx {
				maxIdx = i
			}
		}
		slice := make([]string, maxIdx+1)
		for i, val := range entries {
			slice[i] = val
		}
		v.Set(strings.Join(path, "."), slice)
	})
}

// walkSliceOfPrimitive recursively walks a struct type and calls visit on
// every slice-of-primitive field, passing the accumulated mapstructure path.
func walkSliceOfPrimitive(t reflect.Type, path []string, visit func([]string)) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := mapstructureName(f)
		if tag == "" {
			continue
		}
		full := append(append([]string(nil), path...), tag)
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Struct:
			walkSliceOfPrimitive(ft, full, visit)
		case reflect.Slice:
			elem := ft.Elem()
			if elem.Kind() == reflect.Ptr {
				elem = elem.Elem()
			}
			if elem.Kind() != reflect.Struct {
				visit(full)
			}
		}
	}
}

// walkSliceOfStruct recursively walks a struct type and calls visit on
// every slice-of-struct field, passing the accumulated mapstructure path.
func walkSliceOfStruct(t reflect.Type, path []string, visit func([]string, reflect.Type)) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := mapstructureName(f)
		if tag == "" {
			continue
		}
		full := append(append([]string(nil), path...), tag)
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		switch ft.Kind() {
		case reflect.Struct:
			walkSliceOfStruct(ft, full, visit)
		case reflect.Slice:
			elem := ft.Elem()
			if elem.Kind() == reflect.Ptr {
				elem = elem.Elem()
			}
			if elem.Kind() == reflect.Struct {
				visit(full, elem)
			}
		}
	}
}

// uppercaseTagMap returns map[UPPER_MAPSTRUCTURE_TAG]original_tag for
// every exported field of a slice element.
func uppercaseTagMap(t reflect.Type) map[string]string {
	out := map[string]string{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := mapstructureName(f)
		if tag == "" {
			continue
		}
		out[strings.ToUpper(tag)] = tag
	}
	return out
}

// mapstructureName extracts the name part of a mapstructure tag,
// trimming modifiers (",omitempty", ",squash"). Fields without a tag,
// with tag "-", or with an empty name are skipped.
func mapstructureName(f reflect.StructField) string {
	if !f.IsExported() {
		return ""
	}
	tag := f.Tag.Get("mapstructure")
	if tag == "" || tag == "-" {
		return ""
	}
	if comma := strings.IndexByte(tag, ','); comma >= 0 {
		tag = tag[:comma]
	}
	return tag
}
