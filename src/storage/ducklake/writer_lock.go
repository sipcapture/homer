package ducklake

import (
	"fmt"
	"os"
	"syscall"
)

// acquireCatalogWriterLock takes a non-blocking exclusive OS lock (flock) on a
// lock file derived from the SQLite catalog path. It prevents two writer
// processes from attaching the same catalog concurrently — concurrent DuckLake
// writers on a SQLite catalog duplicate snapshot/table ids and corrupt it
// ("Corrupt DuckLake - multiple snapshots returned from database").
//
// The returned *os.File must be kept open for the lifetime of the writer;
// closing it releases the lock. The kernel also releases the lock automatically
// when the process exits or crashes, so there are no stale locks to clean up.
func acquireCatalogWriterLock(catalogPath string) (*os.File, error) {
	lockPath := catalogPath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open writer lock %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("another homer writer already holds the catalog lock %s "+
				"(refusing to start: two writers on one SQLite catalog corrupt it); "+
				"ensure only one homer writer runs per catalog/data_path", lockPath)
		}
		return nil, fmt.Errorf("flock %s: %w", lockPath, err)
	}
	return f, nil
}

// releaseCatalogWriterLock releases and removes the writer lock file.
func releaseCatalogWriterLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
