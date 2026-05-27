// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	DefaultQueryLimit = 100
	MaxQueryLimit     = 1000
	maxWhereLen       = 512
)

var (
	ErrInvalidTableKey = errors.New("invalid table key")
	ErrInvalidWhere      = errors.New("invalid where clause")

	canonicalSchemas = GetTableSchemas()

	whereBlocklist = []string{
		";", "--", "/*", "*/", "\x00",
		" union ", " select ", " insert ", " update ", " delete ",
		" drop ", " attach ", " copy ", " call ", " exec ", " pragma ",
	}

	whereSplitRE = regexp.MustCompile(`(?i)\s+(?:AND|OR)\s+`)

	predicateCompareRE = regexp.MustCompile(`(?i)^([a-zA-Z_][a-zA-Z0-9_]*)\s*(=|!=|<>|>=|<=|>|<)\s*('(?:[^'\\]|\\.|'')*'|-?\d+)\s*$`)

	predicateIsNullRE = regexp.MustCompile(`(?i)^([a-zA-Z_][a-zA-Z0-9_]*)\s+IS\s+NOT\s+NULL\s*$`)

	predicateIsNullOnlyRE = regexp.MustCompile(`(?i)^([a-zA-Z_][a-zA-Z0-9_]*)\s+IS\s+NULL\s*$`)
)

// ValidateTableKey checks that key is in the static GetTableSchemas allowlist.
func ValidateTableKey(key TableKey) error {
	if _, ok := canonicalSchemas[key]; ok {
		return nil
	}
	return fmt.Errorf("%w: proto_type=%d sub_type=%q", ErrInvalidTableKey, key.ProtoType, key.SubType)
}

// ParseTableKey builds and validates a TableKey from HTTP/API parameters.
func ParseTableKey(protoType uint32, subType string) (TableKey, error) {
	subType = strings.TrimSpace(subType)
	if protoType == ProtoTypeSIP {
		if subType == "" {
			subType = SIPTypeCall
		}
		switch subType {
		case SIPTypeCall, SIPTypeRegistration, SIPTypeDefault:
		default:
			return TableKey{}, fmt.Errorf("%w: invalid sub_type %q for SIP (allowed: call, registration, default)", ErrInvalidTableKey, subType)
		}
		return TableKey{ProtoType: protoType, SubType: subType}, nil
	}
	if subType != "" {
		return TableKey{}, fmt.Errorf("%w: sub_type must be empty for proto_type %d", ErrInvalidTableKey, protoType)
	}
	key := TableKey{ProtoType: protoType}
	if err := ValidateTableKey(key); err != nil {
		return TableKey{}, err
	}
	return key, nil
}

// ResolveTableFQN returns the DuckLake table FQN for a canonical TableKey without
// using GetDefaultSchema (prevents user-controlled suffix injection).
func ResolveTableFQN(writer *MultiTableWriter, key TableKey) (string, error) {
	if err := ValidateTableKey(key); err != nil {
		return "", err
	}
	if tw := writer.GetTable(key); tw != nil {
		return tw.TableFQN(), nil
	}
	schema, ok := canonicalSchemas[key]
	if !ok {
		return "", fmt.Errorf("%w", ErrInvalidTableKey)
	}
	return fmt.Sprintf("%s.hep_proto_%s", writer.GetLakeName(), schema.TableSuffix), nil
}

// ClampLimit bounds limit to [1, maxLimit], using defaultLimit when limit <= 0.
func ClampLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// ColumnsForTableKey returns queryable column names for a canonical table key.
func ColumnsForTableKey(key TableKey) ([]string, error) {
	schema, ok := canonicalSchemas[key]
	if !ok {
		return nil, fmt.Errorf("%w", ErrInvalidTableKey)
	}
	return append([]string(nil), schema.Columns...), nil
}

// AllQueryColumns returns the union of columns across all canonical schemas.
func AllQueryColumns() []string {
	seen := make(map[string]struct{})
	var cols []string
	for _, schema := range canonicalSchemas {
		for _, c := range schema.Columns {
			if _, ok := seen[c]; ok {
				continue
			}
			seen[c] = struct{}{}
			cols = append(cols, c)
		}
	}
	return cols
}

// ValidateWhereClause ensures where is a safe subset of SQL (column op literal predicates).
func ValidateWhereClause(where string, allowedColumns []string) error {
	where = strings.TrimSpace(where)
	if where == "" {
		return nil
	}
	if len(where) > maxWhereLen {
		return fmt.Errorf("%w: too long (max %d characters)", ErrInvalidWhere, maxWhereLen)
	}
	lower := strings.ToLower(where)
	for _, bad := range whereBlocklist {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("%w: contains disallowed token", ErrInvalidWhere)
		}
	}
	allowed := make(map[string]struct{}, len(allowedColumns))
	for _, c := range allowedColumns {
		allowed[strings.ToLower(c)] = struct{}{}
	}
	predicates := whereSplitRE.Split(where, -1)
	for _, pred := range predicates {
		pred = strings.TrimSpace(pred)
		if pred == "" {
			continue
		}
		col, err := validatePredicate(pred, allowed)
		if err != nil {
			return err
		}
		_ = col
	}
	return nil
}

func validatePredicate(pred string, allowed map[string]struct{}) (string, error) {
	var col string
	switch {
	case predicateIsNullRE.MatchString(pred):
		col = predicateIsNullRE.FindStringSubmatch(pred)[1]
	case predicateIsNullOnlyRE.MatchString(pred):
		col = predicateIsNullOnlyRE.FindStringSubmatch(pred)[1]
	case predicateCompareRE.MatchString(pred):
		col = predicateCompareRE.FindStringSubmatch(pred)[1]
	default:
		return "", fmt.Errorf("%w: invalid predicate %q", ErrInvalidWhere, pred)
	}
	if _, ok := allowed[strings.ToLower(col)]; !ok {
		return "", fmt.Errorf("%w: unknown column %q", ErrInvalidWhere, col)
	}
	return col, nil
}

// IsInvalidTableKey reports whether err is ErrInvalidTableKey (including wrapped).
func IsInvalidTableKey(err error) bool {
	return errors.Is(err, ErrInvalidTableKey)
}

// IsClientInputError reports validation failures that should map to HTTP 400.
func IsClientInputError(err error) bool {
	return errors.Is(err, ErrInvalidTableKey) || errors.Is(err, ErrInvalidWhere)
}
