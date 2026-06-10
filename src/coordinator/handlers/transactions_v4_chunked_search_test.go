package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestSplitSearchTimeRange_SingleChunk(t *testing.T) {
	day := int64(24 * time.Hour / time.Millisecond)

	cases := []struct {
		name     string
		from, to int64
	}{
		{"range equal to chunk", 1000, 1000 + day},
		{"range smaller than chunk", 1000, 2000},
		{"unbounded from", 0, 2000},
		{"unbounded to", 1000, 0},
		{"inverted range", 2000, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := splitSearchTimeRange(tc.from, tc.to, day)
			if len(chunks) != 1 {
				t.Fatalf("expected 1 chunk, got %d: %+v", len(chunks), chunks)
			}
			c := chunks[0]
			if c.FromMs != tc.from || c.ToMs != tc.to || !c.ToInclusive {
				t.Fatalf("unexpected chunk: %+v", c)
			}
		})
	}
}

func TestSplitSearchTimeRange_MultiChunkNewestFirst(t *testing.T) {
	day := int64(24 * time.Hour / time.Millisecond)
	from := int64(1_000_000)
	to := from + 7*day + 5000 // 7 days + 5s → 8 chunks, oldest one partial

	chunks := splitSearchTimeRange(from, to, day)
	if len(chunks) != 8 {
		t.Fatalf("expected 8 chunks, got %d", len(chunks))
	}

	// Newest chunk: inclusive upper bound at the original `to`.
	if chunks[0].ToMs != to || !chunks[0].ToInclusive {
		t.Fatalf("newest chunk must end inclusively at to: %+v", chunks[0])
	}
	// Oldest chunk: starts at the original `from`.
	last := chunks[len(chunks)-1]
	if last.FromMs != from {
		t.Fatalf("oldest chunk must start at from: %+v", last)
	}

	for i, c := range chunks {
		if c.ToMs <= c.FromMs {
			t.Fatalf("chunk %d is empty or inverted: %+v", i, c)
		}
		if c.ToMs-c.FromMs > day {
			t.Fatalf("chunk %d larger than chunk size: %+v", i, c)
		}
		if i > 0 {
			if c.ToInclusive {
				t.Fatalf("interior chunk %d must have exclusive upper bound: %+v", i, c)
			}
			// Seamless coverage: interior chunk's exclusive upper bound is
			// exactly the next-newer chunk's inclusive lower bound.
			if c.ToMs != chunks[i-1].FromMs {
				t.Fatalf("gap/overlap between chunk %d and %d: %+v / %+v", i-1, i, chunks[i-1], c)
			}
		}
	}
}

func TestTransactionSearchChunkable(t *testing.T) {
	day := int64(24 * time.Hour / time.Millisecond)

	base := func() *SearchObjectV4 {
		req := &SearchObjectV4{}
		req.Timestamp.From = 1_000_000
		req.Timestamp.To = req.Timestamp.From + 7*day
		return req
	}

	if !transactionSearchChunkable(base()) {
		t.Fatal("7-day range with default params must be chunkable")
	}

	req := base()
	req.Param.OrderBy = "timestamp DESC"
	if !transactionSearchChunkable(req) {
		t.Fatal("explicit default ordering must stay chunkable")
	}

	req = base()
	req.Timestamp.To = req.Timestamp.From + day
	if transactionSearchChunkable(req) {
		t.Fatal("range within one chunk must not be chunked")
	}

	req = base()
	req.Timestamp.From = 0
	if transactionSearchChunkable(req) {
		t.Fatal("unbounded range must not be chunked")
	}

	req = base()
	req.Param.GroupBy = "method"
	if transactionSearchChunkable(req) {
		t.Fatal("aggregation must not be chunked")
	}

	req = base()
	req.Param.Select = "method, count(*) as cnt"
	if transactionSearchChunkable(req) {
		t.Fatal("custom projection must not be chunked")
	}

	req = base()
	req.Param.OrderBy = "response_code ASC"
	if transactionSearchChunkable(req) {
		t.Fatal("custom ordering must not be chunked")
	}
}

func TestBuildSearchSQLV4WithOpts_ToExclusive(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Timestamp.From = 1_000_000
	req.Timestamp.To = 2_000_000

	inclusive, err := buildSearchSQLV4WithOpts("homer_lake", &req, nil, searchSQLOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inclusive, "timestamp <= (to_timestamp(2000000 / 1000.0)") {
		t.Fatalf("expected inclusive upper bound, got:\n%s", inclusive)
	}

	exclusive, err := buildSearchSQLV4WithOpts("homer_lake", &req, nil, searchSQLOpts{toExclusive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exclusive, "timestamp < (to_timestamp(2000000 / 1000.0)") {
		t.Fatalf("expected exclusive upper bound, got:\n%s", exclusive)
	}
	if strings.Contains(exclusive, "timestamp <= (to_timestamp(2000000") {
		t.Fatalf("exclusive SQL must not keep <= upper bound:\n%s", exclusive)
	}
	if !strings.Contains(exclusive, "timestamp >= (to_timestamp(1000000 / 1000.0)") {
		t.Fatalf("lower bound must stay inclusive:\n%s", exclusive)
	}
}

func TestEffectiveSearchLimit(t *testing.T) {
	if effectiveSearchLimit(0) != 50 || effectiveSearchLimit(-1) != 50 || effectiveSearchLimit(50001) != 50 {
		t.Fatal("out-of-range limits must clamp to 50")
	}
	if effectiveSearchLimit(200) != 200 {
		t.Fatal("valid limit must pass through")
	}
}
