// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package cli

import "strings"

// parseCLIQueryLine strips mysql-style line suffixes before executing SQL.
//   - \G  vertical result layout (like mysql client)
//   - \g  execute (same as semicolon)
func parseCLIQueryLine(line string) (sql string, vertical bool) {
	line = strings.TrimSpace(line)
	for line != "" {
		switch {
		case strings.HasSuffix(line, ";"):
			line = strings.TrimSpace(line[:len(line)-1])
		case strings.HasSuffix(line, `\G`):
			vertical = true
			line = strings.TrimSpace(line[:len(line)-2])
		case strings.HasSuffix(line, `\g`):
			line = strings.TrimSpace(line[:len(line)-2])
		default:
			return line, vertical
		}
	}
	return line, vertical
}
