package sqlvalidator

import (
	"fmt"
	"net/netip"
	"strings"
)

// ipLiterals returns the SQL string literals that an IP filter value matches.
// HEP ingest stores RFC 5952 text ("2001:db8::10"), but 11.0.222 to 11.0.375
// stored IPv6 fully expanded ("2001:0db8:0000:0000:0000:0000:0000:0010"), so
// an IPv6 value matches both forms, whichever form the user typed. Any other
// value, IPv4 included, is matched as given.
func ipLiterals(value string) []string {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || addr.Is4() {
		return []string{"'" + SafeString(value) + "'"}
	}
	addr = addr.WithZone("")
	b := addr.As16()
	var expanded strings.Builder
	for i := 0; i < 16; i += 2 {
		if i > 0 {
			expanded.WriteByte(':')
		}
		fmt.Fprintf(&expanded, "%02x%02x", b[i], b[i+1])
	}
	return []string{"'" + addr.Unmap().String() + "'", "'" + expanded.String() + "'"}
}

// IPEqualsClause returns "column = 'value'" for an IP filter. An IPv6 value
// becomes "column IN ('<RFC 5952>', '<expanded>')" (see ipLiterals). column
// must be a trusted identifier.
func IPEqualsClause(column, value string) string {
	lits := ipLiterals(value)
	if len(lits) == 1 {
		return column + " = " + lits[0]
	}
	return column + " IN (" + strings.Join(lits, ", ") + ")"
}

// IPNotEqualsClause is the negation of IPEqualsClause.
func IPNotEqualsClause(column, value string) string {
	lits := ipLiterals(value)
	if len(lits) == 1 {
		return column + " != " + lits[0]
	}
	return column + " NOT IN (" + strings.Join(lits, ", ") + ")"
}
