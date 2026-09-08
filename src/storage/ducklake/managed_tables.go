// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

// ManagedLakeTablePredicate is the information_schema.tables filter for
// tables that compaction (retention + merge), CLI one-off maintenance, and
// tiered storage all operate on. Keep this in one place so a new prefix
// cannot fall out of one scanner while remaining in another (#1002, #1003).
const ManagedLakeTablePredicate = `(table_name LIKE 'hep_proto_%' OR table_name LIKE 'otlp_%')`
