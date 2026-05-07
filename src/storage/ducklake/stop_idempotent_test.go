package ducklake

import "testing"

func TestMultiTableWriterStopIsIdempotent(t *testing.T) {
	w := &MultiTableWriter{
		stopChan: make(chan struct{}),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MultiTableWriter.Stop should not panic on repeated calls: %v", r)
		}
	}()

	if err := w.Stop(); err != nil {
		t.Fatalf("unexpected error on first Stop: %v", err)
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("unexpected error on second Stop: %v", err)
	}
}

func TestTieredStorageManagerStopIsIdempotent(t *testing.T) {
	tsm := &TieredStorageManager{
		stopChan: make(chan struct{}),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("TieredStorageManager.Stop should not panic on repeated calls: %v", r)
		}
	}()

	if err := tsm.Stop(); err != nil {
		t.Fatalf("unexpected error on first Stop: %v", err)
	}
	if err := tsm.Stop(); err != nil {
		t.Fatalf("unexpected error on second Stop: %v", err)
	}
}
