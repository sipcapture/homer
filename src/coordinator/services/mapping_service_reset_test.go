package services

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMappingService_ResetMappings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := SeedDefaultMappingSchema(ctx, db); err != nil {
		t.Fatal(err)
	}

	svc := NewMappingService(db)
	customGUID := "custom-mapping-guid-0001"
	if _, err := svc.CreateMapping(ctx, MappingSchema{
		GUID:           customGUID,
		Profile:        "custom",
		HepID:          99,
		HepAlias:       "TEST",
		CreateIndex:    []byte(`{}`),
		CorrelationMap: []byte(`[]`),
		FieldsMapping:  []byte(`[]`),
		FieldsSettings: []byte(`{}`),
		SchemaMapping:  []byte(`{}`),
		SchemaSettings: []byte(`{}`),
	}); err != nil {
		t.Fatalf("CreateMapping: %v", err)
	}

	if err := svc.ResetMappings(ctx); err != nil {
		t.Fatalf("ResetMappings: %v", err)
	}

	items, err := svc.ListMappings(ctx, MappingListFilters{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.GUID == customGUID {
			t.Fatalf("custom mapping %q still present after reset", customGUID)
		}
	}
	if len(items) < 9 {
		t.Fatalf("expected at least 9 default mappings after reset, got %d", len(items))
	}
}
