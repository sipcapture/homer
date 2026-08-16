package writer

import (
	"strings"
	"testing"
)

func TestEffectiveMaxCompactedFiles(t *testing.T) {
	tests := []struct {
		name   string
		config CompactionConfig
		want   int
	}{
		{"zero uses default", CompactionConfig{}, defaultDuckDBMaxCompactedFiles},
		{"explicit small kept", CompactionConfig{MaxCompactedFiles: 4}, 4},
		{"example 100 clamped on duckdb", CompactionConfig{MaxCompactedFiles: 100}, duckdbMaxCompactedFilesCap},
		{"engine duckdb still clamped", CompactionConfig{Engine: EngineDuckDB, MaxCompactedFiles: 100}, duckdbMaxCompactedFilesCap},
		{"native not clamped", CompactionConfig{Engine: EngineNativeGo, MaxCompactedFiles: 100}, 100},
		{"native zero uses default", CompactionConfig{Engine: EngineNativeGo}, defaultDuckDBMaxCompactedFiles},
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
		config:   CompactionConfig{MaxCompactedFiles: 100},
	}
	sql := svc.buildMergeSQL("hep_proto_1_call")
	if !strings.Contains(sql, "max_compacted_files => 16") {
		t.Fatalf("expected clamped max_compacted_files in %q", sql)
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
			MinFileSizeBytes: 1024,
			MaxFileSizeBytes: 10 << 20,
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
