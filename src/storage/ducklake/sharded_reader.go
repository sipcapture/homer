// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake


// ShardedMultiTableReader provides read operations across multiple shards.
// Each shard has its own DuckDB instance, so queries are executed per-shard
// and results are merged.
type ShardedMultiTableReader struct {
	readers []*MultiTableReader
}

// NewShardedMultiTableReader creates a reader that queries all shards.
func NewShardedMultiTableReader(sw *ShardedWriter) *ShardedMultiTableReader {
	readers := make([]*MultiTableReader, len(sw.Shards()))
	for i, shard := range sw.Shards() {
		readers[i] = NewMultiTableReader(shard)
	}
	return &ShardedMultiTableReader{readers: readers}
}

// GetTimeRange returns min/max timestamps across all shards.
func (sr *ShardedMultiTableReader) GetTimeRange() (minTs, maxTs int64, err error) {
	for _, r := range sr.readers {
		rMin, rMax, rErr := r.GetTimeRange()
		if rErr != nil {
			continue
		}
		if rMin > 0 && (minTs == 0 || rMin < minTs) {
			minTs = rMin
		}
		if rMax > maxTs {
			maxTs = rMax
		}
	}
	return minTs, maxTs, nil
}

// GetTimeRangeForTableKey returns time range for specific TableKey across all shards.
func (sr *ShardedMultiTableReader) GetTimeRangeForTableKey(key TableKey) (minTs, maxTs int64, err error) {
	for _, r := range sr.readers {
		rMin, rMax, rErr := r.GetTimeRangeForTableKey(key)
		if rErr != nil {
			continue
		}
		if rMin > 0 && (minTs == 0 || rMin < minTs) {
			minTs = rMin
		}
		if rMax > maxTs {
			maxTs = rMax
		}
	}
	return minTs, maxTs, nil
}

// HasDataInRange checks if any shard has data in the given time range.
func (sr *ShardedMultiTableReader) HasDataInRange(queryMinTs, queryMaxTs int64) (bool, error) {
	for _, r := range sr.readers {
		has, err := r.HasDataInRange(queryMinTs, queryMaxTs)
		if err != nil {
			continue
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

// GetRowCount returns total row count across all shards.
func (sr *ShardedMultiTableReader) GetRowCount() (int64, error) {
	var total int64
	for _, r := range sr.readers {
		count, err := r.GetRowCount()
		if err != nil {
			continue
		}
		total += count
	}
	return total, nil
}

// GetRowCountForTableKey returns row count for specific TableKey across all shards.
func (sr *ShardedMultiTableReader) GetRowCountForTableKey(key TableKey) (int64, error) {
	var total int64
	for _, r := range sr.readers {
		count, err := r.GetRowCountForTableKey(key)
		if err != nil {
			continue
		}
		total += count
	}
	return total, nil
}

// Query executes a query on specific table by TableKey across all shards.
func (sr *ShardedMultiTableReader) Query(key TableKey, whereClause string, limit int) ([]map[string]interface{}, error) {
	var allResults []map[string]interface{}
	for _, r := range sr.readers {
		results, err := r.Query(key, whereClause, limit)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	sortByTimestampDesc(allResults)
	if limit > 0 && len(allResults) > limit {
		allResults = allResults[:limit]
	}
	return allResults, nil
}

// QueryAll executes a query across all tables in all shards.
func (sr *ShardedMultiTableReader) QueryAll(whereClause string, limit int) ([]map[string]interface{}, error) {
	var allResults []map[string]interface{}
	for _, r := range sr.readers {
		results, err := r.QueryAll(whereClause, limit)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	sortByTimestampDesc(allResults)
	if limit > 0 && len(allResults) > limit {
		allResults = allResults[:limit]
	}
	return allResults, nil
}

// ListTableKeys returns all TableKeys (from the first reader).
func (sr *ShardedMultiTableReader) ListTableKeys() []TableKey {
	if len(sr.readers) > 0 {
		return sr.readers[0].ListTableKeys()
	}
	return nil
}

// ListProtoTypes returns unique proto_types (from the first reader).
func (sr *ShardedMultiTableReader) ListProtoTypes() []uint32 {
	if len(sr.readers) > 0 {
		return sr.readers[0].ListProtoTypes()
	}
	return nil
}

