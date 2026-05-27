// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package scripting

import (
	"math"
	"strconv"
	"testing"
)

func TestParseUint32HEPField(t *testing.T) {
	u, ok := parseUint32HEPField("42")
	if !ok || u != 42 {
		t.Fatalf("got %d ok=%v", u, ok)
	}

	_, ok = parseUint32HEPField("not-a-number")
	if ok {
		t.Fatal("expected parse failure")
	}

	overflow := strconv.FormatUint(uint64(math.MaxUint32)+1, 10)
	_, ok = parseUint32HEPField(overflow)
	if ok {
		t.Fatal("expected overflow rejection")
	}

	u, ok = parseUint32HEPField(strconv.FormatUint(math.MaxUint32, 10))
	if !ok || u != math.MaxUint32 {
		t.Fatalf("max uint32: got %d ok=%v", u, ok)
	}
}
