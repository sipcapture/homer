package compactor

import (
	"math/big"
	"strings"
)

// aggregatedColStat is the merged per-column statistic for a batch of source
// files. Because compaction is a pure concatenation (no rows added/removed),
// the merged file's stats are exact: min = min of mins, max = max of maxes,
// counts are summed.
type aggregatedColStat struct {
	columnID        int64
	columnSizeBytes int64
	valueCount      int64
	nullCount       int64
	minValue        *string
	maxValue        *string
	containsNaN     bool
}

// isNumericType reports whether a DuckLake column_type string is numeric, in
// which case its min/max strings must be compared by value (e.g. "80" < "5060")
// rather than lexicographically.
func isNumericType(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	if i := strings.IndexByte(t, '('); i >= 0 { // decimal(18,3) -> decimal
		t = t[:i]
	}
	switch t {
	case "tinyint", "smallint", "integer", "int", "int4", "int2", "int1",
		"bigint", "int8", "hugeint", "int128",
		"utinyint", "usmallint", "uinteger", "ubigint", "uhugeint",
		"uint8", "uint16", "uint32", "uint64", "uint128",
		"float", "float4", "real", "double", "float8", "decimal", "numeric":
		return true
	}
	return false
}

// statLess reports whether stat string a sorts before b for a column type.
// Numeric types compare by value; all others (varchar, date, timestamp, json,
// uuid, ...) compare lexicographically — DuckLake stores dates/timestamps in a
// fixed, byte-sortable ISO layout, so lexicographic order is correct for them.
func statLess(a, b, colType string) bool {
	if isNumericType(colType) {
		af, aok := new(big.Float).SetString(strings.TrimSpace(a))
		bf, bok := new(big.Float).SetString(strings.TrimSpace(b))
		if aok && bok {
			return af.Cmp(bf) < 0
		}
	}
	return a < b
}

// aggregateColumnStats folds per-file column stats into one merged stat per
// column. perFile holds the stat rows of each source file in the batch;
// colTypes maps column_id to its DuckLake type for value-aware min/max.
func aggregateColumnStats(perFile [][]colStat, colTypes map[int64]string) []aggregatedColStat {
	order := []int64{}
	acc := map[int64]*aggregatedColStat{}

	for _, fileStats := range perFile {
		for _, s := range fileStats {
			a, ok := acc[s.columnID]
			if !ok {
				a = &aggregatedColStat{columnID: s.columnID}
				acc[s.columnID] = a
				order = append(order, s.columnID)
			}
			a.columnSizeBytes += s.columnSizeBytes
			a.valueCount += s.valueCount
			a.nullCount += s.nullCount
			if s.containsNaN.Valid && s.containsNaN.Int64 != 0 {
				a.containsNaN = true
			}
			colType := colTypes[s.columnID]
			if s.minValue.Valid {
				if a.minValue == nil || statLess(s.minValue.String, *a.minValue, colType) {
					v := s.minValue.String
					a.minValue = &v
				}
			}
			if s.maxValue.Valid {
				if a.maxValue == nil || statLess(*a.maxValue, s.maxValue.String, colType) {
					v := s.maxValue.String
					a.maxValue = &v
				}
			}
		}
	}

	out := make([]aggregatedColStat, 0, len(order))
	for _, id := range order {
		out = append(out, *acc[id])
	}
	return out
}
