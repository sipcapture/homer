package writer

import "testing"

func TestTCPServerStopIsIdempotent(t *testing.T) {
	srv := &TCPServer{
		quit: make(chan struct{}),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("TCPServer.Stop should not panic on repeated calls: %v", r)
		}
	}()

	srv.Stop()
	srv.Stop()
}

func TestTLSServerStopIsIdempotent(t *testing.T) {
	srv := &TLSServer{
		quit: make(chan struct{}),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("TLSServer.Stop should not panic on repeated calls: %v", r)
		}
	}()

	srv.Stop()
	srv.Stop()
}

func TestTieringServiceStopIsIdempotent(t *testing.T) {
	ts := &TieringService{
		stopChan: make(chan struct{}),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("TieringService.Stop should not panic on repeated calls: %v", r)
		}
	}()

	if err := ts.Stop(); err != nil {
		t.Fatalf("unexpected error on first Stop: %v", err)
	}
	if err := ts.Stop(); err != nil {
		t.Fatalf("unexpected error on second Stop: %v", err)
	}
}
