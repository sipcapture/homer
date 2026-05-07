// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
)

// ConvertHEPToLakeRow maps a decoded HEP record to a DuckLake row (table key + column values).
func ConvertHEPToLakeRow(hep *decoder.HEP) (TableKey, []interface{}, error) {
	var a MultiTableAdapter
	k, v := a.convertHEPToValuesWithSIPSubtype(hep, "")
	return k, v, nil
}

// ConvertHEPToLakeRowSIPForced maps a SIP HEP record into the given logical table (call / registration / default),
// ignoring automatic routing from the SIP method. Non-SIP packets cannot be forced here.
func ConvertHEPToLakeRowSIPForced(hep *decoder.HEP, sipSubType string) (TableKey, []interface{}, error) {
	if hep.ProtoType != ProtoTypeSIP {
		return TableKey{}, nil, fmt.Errorf("force_sip_table applies only to SIP (proto 1)")
	}
	switch sipSubType {
	case SIPTypeCall, SIPTypeRegistration, SIPTypeDefault:
	default:
		return TableKey{}, nil, fmt.Errorf("force_sip_table: expected %q, %q, or %q", SIPTypeCall, SIPTypeRegistration, SIPTypeDefault)
	}
	var a MultiTableAdapter
	k, v := a.convertHEPToValuesWithSIPSubtype(hep, sipSubType)
	return k, v, nil
}

// LakeTableFQN returns qualified table name for INSERT (DuckLake layout).
func LakeTableFQN(lakeName string, schema *TableSchema) string {
	return fmt.Sprintf("%s.main.hep_proto_%s", lakeName, schema.TableSuffix)
}

func schemaForKey(key TableKey) *TableSchema {
	if s, ok := GetTableSchemas()[key]; ok {
		return s
	}
	return GetDefaultSchema(key)
}

// insertColumnNames parses the column list from InsertSQL, e.g. "(uuid, date, ...) VALUES".
func insertColumnNames(schema *TableSchema) []string {
	s := schema.InsertSQL
	open := strings.Index(s, "(")
	if open < 0 {
		return nil
	}
	closeIdx := strings.Index(s, ")")
	if closeIdx <= open {
		return nil
	}
	inner := strings.TrimSpace(s[open+1 : closeIdx])
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func formatSQLLiteral(v interface{}) (string, error) {
	switch x := v.(type) {
	case nil:
		return "NULL", nil
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'", nil
	case int:
		return strconv.Itoa(x), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case bool:
		if x {
			return "TRUE", nil
		}
		return "FALSE", nil
	case time.Time:
		t := x.UTC()
		return "TIMESTAMP '" + t.Format("2006-01-02 15:04:05.999999") + "'", nil
	default:
		return "", fmt.Errorf("unsupported SQL literal type %T", v)
	}
}

// BuildInsertMultiValues builds a single INSERT with multiple value tuples for one table.
func BuildInsertMultiValues(lakeName string, key TableKey, rows [][]interface{}) (string, error) {
	if len(rows) == 0 {
		return "", fmt.Errorf("no rows")
	}
	schema := schemaForKey(key)
	cols := insertColumnNames(schema)
	if len(cols) == 0 {
		return "", fmt.Errorf("no columns for key %v", key)
	}
	for i, row := range rows {
		if len(row) != len(cols) {
			return "", fmt.Errorf("row %d: column count %d != schema %d", i, len(row), len(cols))
		}
	}
	colList := strings.Join(cols, ", ")
	table := LakeTableFQN(lakeName, schema)

	var tuples []string
	for ri, row := range rows {
		parts := make([]string, len(row))
		for ci, cell := range row {
			// data_extra is JSON in DuckLake
			if cols[ci] == "data_extra" {
				if s, ok := cell.(string); ok {
					esc, err := formatSQLLiteral(s)
					if err != nil {
						return "", err
					}
					parts[ci] = fmt.Sprintf("CAST(%s AS JSON)", esc)
					continue
				}
			}
			s, err := formatSQLLiteral(cell)
			if err != nil {
				return "", fmt.Errorf("row %d col %s: %w", ri, cols[ci], err)
			}
			parts[ci] = s
		}
		tuples = append(tuples, "("+strings.Join(parts, ", ")+")")
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", table, colList, strings.Join(tuples, ", "))
	return sql, nil
}
