// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"fmt"
	"time"
)

// FormatTextLine returns the header line for a SIP message in homer style (text export).
func FormatTextLine(srcIP, dstIP string, srcPort, dstPort int, ts time.Time, proto string) string {
	return fmt.Sprintf("proto:%s %s  %s:%d ---> %s:%d\r\n\r\n",
		proto,
		ts.UTC().Format(time.RFC3339Nano),
		srcIP, srcPort,
		dstIP, dstPort,
	)
}
