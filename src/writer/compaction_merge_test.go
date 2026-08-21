package writer

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBeginCycleSingleFlight(t *testing.T) {
	svc := &CompactionService{}
	if !svc.beginCycle() {
		t.Fatal("first beginCycle() must start")
	}
	if svc.beginCycle() {
		t.Fatal("second beginCycle() must skip while the first is running")
	}
	svc.endCycle()
	if !svc.beginCycle() {
		t.Fatal("beginCycle() after endCycle() must start")
	}
}

func TestBeginCycleConcurrentSkip(t *testing.T) {
	svc := &CompactionService{}
	var started atomic.Int32
	var wg sync.WaitGroup
	holding := make(chan struct{})
	release := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !svc.beginCycle() {
				return
			}
			if started.Add(1) == 1 {
				close(holding)
			}
			<-release
			svc.endCycle()
		}()
	}
	select {
	case <-holding:
	case <-time.After(2 * time.Second):
		close(release)
		wg.Wait()
		t.Fatal("timed out waiting for a cycle to start")
	}
	if got := started.Load(); got != 1 {
		close(release)
		wg.Wait()
		t.Fatalf("concurrent beginCycle started %d cycles, want 1", got)
	}
	close(release)
	wg.Wait()
}

func TestResolvedEngine(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		want   string
	}{
		{"empty is duckdb", "", EngineDuckDB},
		{"explicit duckdb", EngineDuckDB, EngineDuckDB},
		{"native", EngineNativeGo, EngineNativeGo},
		{"unknown treated as duckdb", "other", EngineDuckDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &CompactionService{config: CompactionConfig{Engine: tt.engine}}
			if got := svc.resolvedEngine(); got != tt.want {
				t.Fatalf("resolvedEngine()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestEffectiveMaxCompactedFiles(t *testing.T) {
	tests := []struct {
		name   string
		config CompactionConfig
		want   int
	}{
		{"zero uses default", CompactionConfig{}, defaultDuckDBMaxCompactedFiles},
		{"explicit small kept", CompactionConfig{MaxCompactedFiles: 4}, 4},
		{"explicit 100 kept", CompactionConfig{MaxCompactedFiles: 100}, 100},
		{"engine duckdb explicit kept", CompactionConfig{Engine: EngineDuckDB, MaxCompactedFiles: 100}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &CompactionService{config: tt.config}
			if got := svc.effectiveMaxCompactedFiles(); got != tt.want {
				t.Fatalf("effectiveMaxCompactedFiles()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestEffectiveMaxFileSizeBytes(t *testing.T) {
	svc := &CompactionService{}
	if got := svc.effectiveMaxFileSizeBytes(); got != defaultDuckDBMaxFileSizeBytes {
		t.Fatalf("zero config: got %d, want %d", got, defaultDuckDBMaxFileSizeBytes)
	}
	svc.config.MaxFileSizeBytes = 134217728
	if got := svc.effectiveMaxFileSizeBytes(); got != 134217728 {
		t.Fatalf("explicit: got %d, want 134217728", got)
	}
}

func TestBuildMergeSQLAlwaysBoundsBatch(t *testing.T) {
	svc := &CompactionService{
		lakeName: "homer_lake",
		config:   CompactionConfig{},
	}
	sql := svc.buildMergeSQL("hep_proto_1_call")
	if !strings.Contains(sql, "max_compacted_files => 32") {
		t.Fatalf("expected default max_compacted_files in %q", sql)
	}
	if !strings.Contains(sql, "max_file_size => 67108864") {
		t.Fatalf("expected default max_file_size in %q", sql)
	}
	if !strings.Contains(sql, "schema => 'main'") {
		t.Fatalf("expected schema in %q", sql)
	}
	if !strings.Contains(sql, "'hep_proto_1_call'") {
		t.Fatalf("expected table name in %q", sql)
	}
}

func TestBuildMergeSQLRespectsMinFileSize(t *testing.T) {
	svc := &CompactionService{
		lakeName: "homer_lake",
		config: CompactionConfig{
			MinFileSizeBytes:  1024,
			MaxFileSizeBytes:  10 << 20,
			MaxCompactedFiles: 2,
		},
	}
	sql := svc.buildMergeSQL("hep_proto_1_call")
	if !strings.Contains(sql, "min_file_size => 1024") {
		t.Fatalf("expected min_file_size in %q", sql)
	}
	if !strings.Contains(sql, "max_file_size => 10485760") {
		t.Fatalf("expected explicit max_file_size in %q", sql)
	}
	if !strings.Contains(sql, "max_compacted_files => 2") {
		t.Fatalf("expected max_compacted_files in %q", sql)
	}
}
