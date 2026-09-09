// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
)

// lakeIdent is the only catalog name shape interpolated into INSERT FQNs.
// proto_type / sub_type from HTTP never enter this string.
var lakeIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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

// KnownTableSchema returns the allowlisted schema for key. Unknown keys
// (including GetDefaultSchema fallbacks) are rejected so table/column
// identifiers in INSERT SQL never come from request JSON.
func KnownTableSchema(key TableKey) (*TableSchema, error) {
	s, ok := GetTableSchemas()[key]
	if !ok || s == nil {
		return nil, fmt.Errorf("unknown table proto_type=%d sub_type=%q", key.ProtoType, key.SubType)
	}
	return s, nil
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

// InsertColumnNamesForKey returns the INSERT column order for a table
// key, derived from the canonical TableSchema InsertSQL in tables.go.
func InsertColumnNamesForKey(key TableKey) []string {
	s := schemaForKey(key)
	if s == nil {
		return nil
	}
	return insertColumnNames(s)
}

// SIPCallInsertColumnNames returns the INSERT column order for
// hep_proto_1_call, derived from the canonical TableSchema (must stay
// aligned with InsertSQL in tables.go).
func SIPCallInsertColumnNames() []string {
	return InsertColumnNamesForKey(TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall})
}

func jsonCellText(cell interface{}) (string, bool) {
	switch x := cell.(type) {
	case string:
		return x, true
	case json.RawMessage:
		return string(x), true
	case []byte:
		return string(x), true
	case *[]byte:
		if x == nil {
			return "", false
		}
		return string(*x), true
	case map[string]interface{}:
		b, err := json.Marshal(x)
		if err != nil {
			return "", false
		}
		return string(b), true
	default:
		return "", false
	}
}

// JSONSafeInsertRows copies rows so encoding/json can serialize them:
// pooled *[]byte / json.RawMessage become JSON text strings (not base64).
func JSONSafeInsertRows(rows [][]interface{}) [][]interface{} {
	out := make([][]interface{}, len(rows))
	for i, row := range rows {
		nr := make([]interface{}, len(row))
		for j, cell := range row {
			if text, ok := jsonCellText(cell); ok {
				if _, isString := cell.(string); isString {
					nr[j] = cell
				} else {
					nr[j] = text
				}
			} else {
				nr[j] = cell
			}
		}
		out[i] = nr
	}
	return out
}

func bindInsertArg(col string, cell interface{}) interface{} {
	if col == "data_extra" {
		if text, ok := jsonCellText(cell); ok && text != "" {
			return text
		}
		return "{}"
	}
	switch x := cell.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return string(x)
	case string:
		if col == "date" || col == "timestamp" {
			if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
				return t.UTC()
			}
			if t, err := time.Parse(time.RFC3339, x); err == nil {
				return t.UTC()
			}
			if t, err := time.Parse("2006-01-02", x); err == nil {
				return t.UTC()
			}
		}
		return x
	default:
		return cell
	}
}

// BuildParameterizedInsert builds INSERT ... VALUES (?,?,...), ... with bound
// arguments. Table and column names come only from KnownTableSchema (Go
// constants). lakeName must be a simple SQL identifier (node config).
func BuildParameterizedInsert(lakeName string, key TableKey, rows [][]interface{}) (string, []any, error) {
	if len(rows) == 0 {
		return "", nil, fmt.Errorf("no rows")
	}
	if lakeName == "" {
		lakeName = "homer_lake"
	}
	if !lakeIdent.MatchString(lakeName) {
		return "", nil, fmt.Errorf("invalid lake name")
	}
	schema, err := KnownTableSchema(key)
	if err != nil {
		return "", nil, err
	}
	cols := insertColumnNames(schema)
	if len(cols) == 0 {
		return "", nil, fmt.Errorf("no columns for key %v", key)
	}
	tupleParts := make([]string, len(cols))
	for i, col := range cols {
		if col == "data_extra" {
			tupleParts[i] = "CAST(? AS JSON)"
		} else {
			tupleParts[i] = "?"
		}
	}
	tuple := "(" + strings.Join(tupleParts, ", ") + ")"
	placeholders := make([]string, len(rows))
	args := make([]any, 0, len(rows)*len(cols))
	for i, row := range rows {
		if len(row) != len(cols) {
			return "", nil, fmt.Errorf("row %d: column count %d != schema %d", i, len(row), len(cols))
		}
		placeholders[i] = tuple
		for ci, cell := range row {
			args = append(args, bindInsertArg(cols[ci], cell))
		}
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		LakeTableFQN(lakeName, schema),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "))
	return query, args, nil
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
			// data_extra is JSON in DuckLake. Row builders emit it as a
			// string, json.RawMessage or pooled *[]byte (see
			// buildExtraJSONCell); all three carry the raw JSON text.
			if cols[ci] == "data_extra" {
				text, _ := jsonCellText(cell)
				if text != "" {
					esc, err := formatSQLLiteral(text)
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
