package writer

import (
	"testing"
)

func TestRetentionDaysForTable(t *testing.T) {
	svc := &CompactionService{
		config: CompactionConfig{
			RetentionDays: 120,
			RetentionDaysByTable: map[string]int{
				"hep_proto_1_registration": 30,
				"hep_proto_1_call":         0, // explicit disable
			},
		},
	}

	tests := []struct {
		name string
		table string
		want  int
	}{
		{"global default", "homer_lake.main.hep_proto_1_default", 120},
		{"override shorter", "homer_lake.main.hep_proto_1_registration", 30},
		{"override disable", "homer_lake.main.hep_proto_1_call", 0},
		{"bare table name", "hep_proto_1_registration", 30},
		{"unknown bare uses global", "hep_proto_1_option", 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.retentionValueForTable(tt.table); got != tt.want {
				t.Fatalf("retentionValueForTable(%q)=%d, want %d", tt.table, got, tt.want)
			}
		})
	}
}

func TestRetentionEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config CompactionConfig
		want   bool
	}{
		{"all zero", CompactionConfig{}, false},
		{"global only", CompactionConfig{RetentionDays: 30}, true},
		{"override only", CompactionConfig{RetentionDaysByTable: map[string]int{"hep_proto_1_registration": 14}}, true},
		{"global zero with disable overrides", CompactionConfig{
			RetentionDays:        0,
			RetentionDaysByTable: map[string]int{"hep_proto_1_call": 0},
		}, false},
		{"global zero with positive override", CompactionConfig{
			RetentionDays:        0,
			RetentionDaysByTable: map[string]int{"hep_proto_1_registration": 7},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &CompactionService{config: tt.config}
			if got := svc.retentionEnabled(); got != tt.want {
				t.Fatalf("retentionEnabled()=%v, want %v", got, tt.want)
			}
		})
	}
}
