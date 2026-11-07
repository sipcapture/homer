package writer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/node"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// Gate the production per-table catalog lock. The writer already owns the
// PostgreSQL maintenance lease when real merge work reaches this barrier.
type compactionTestGate struct {
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (g *compactionTestGate) CatalogLock() { g.once.Do(func() { close(g.entered) }); <-g.resume }
func (*compactionTestGate) CatalogUnlock() {}

func checkCompactionMaintenanceContention(t *testing.T, c *CompactionService, dsn, dataPath string) {
	t.Helper()
	var port int
	for attempts := 0; attempts < 100; attempts++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		next, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+1))
		_ = l.Close()
		if err == nil {
			_ = next.Close()
			break
		}
		port = 0
	}
	if port == 0 {
		t.Fatal("no node port pair available")
	}
	cfg := config.NodeConfig{
		FlightServer: config.FlightServerConfig{Host: "127.0.0.1", Port: port, AuthToken: "contention-test", MaxMessageSize: 16777216},
		DuckLake: config.DuckLakeConfig{CatalogType: "postgres", CatalogPath: dsn, DataPath: dataPath, LakeName: "lake",
			Volumes: []config.VolumeConfig{{Name: "hot", Type: "local", CatalogType: "postgres", CatalogPath: dsn, Path: dataPath}},
			Tuning:  config.DuckDBTuning{Threads: 1, MemoryLimit: "256MB"}},
	}
	n, err := node.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	// The tiering manager has its own real DuckDB attached to this catalog.
	volume := ducklake.Volume{Name: "hot", Type: ducklake.VolumeTypeLocal, LakeName: "hot", CatalogPath: dsn, Path: dataPath}
	tier, err := ducklake.NewTieredStorageManager(ducklake.TieredStorageConfig{
		CatalogType: ducklake.CatalogPostgres, CatalogPath: dsn, Volumes: []ducklake.Volume{volume},
		TuningThreads: 1, TuningMemoryLimit: "256MB", TTLMoveIntervalSec: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tier.Start(); err != nil {
		t.Fatal(err)
	}
	defer tier.Stop()
	if err := c.discoverTables(); err != nil {
		t.Fatal(err)
	}
	gate := &compactionTestGate{entered: make(chan struct{}), resume: make(chan struct{})}
	c.catalogLocker = gate
	done := make(chan error, 1)
	go func() { done <- c.mergeTablesDuckDB(c.tables) }()
	var release sync.Once
	unblock := func() { release.Do(func() { close(gate.resume) }) }
	completed := false
	defer func() {
		unblock()
		if !completed {
			select {
			case <-done:
			case <-time.After(20 * time.Second):
				c.cancel()
				t.Error("merge did not stop during test cleanup")
				return
			}
		}
		c.catalogLocker = nil
	}()
	select {
	case <-gate.entered:
	case err := <-done:
		completed = true
		t.Fatalf("merge exited before the barrier: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("merge did not reach its catalog lock")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	// Start uses an asynchronous HTTP listener; readiness is independent of the
	// maintenance barrier, so poll health before issuing the contending request.
	base := fmt.Sprintf("http://127.0.0.1:%d", port+1)
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := client.Get(base + "/health")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/vacuum", nil)
	req.Header.Set("Authorization", "Bearer contention-test")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("vacuum bypassed writer: %d %s", resp.StatusCode, body)
	}
	for name, op := range map[string]func() error{
		"tier delete": func() error {
			return tier.DeletePartition("measurement", time.Now().UTC().Format("2006-01-02"), &volume)
		},
		"tier cleanup": func() error { return tier.RunVolumeMaintenance(&volume, 3600) },
	} {
		if err := op(); err == nil || !strings.Contains(err.Error(), "already running") {
			t.Fatalf("%s did not report writer contention: %v", name, err)
		}
	}
	unblock()
	select {
	case err := <-done:
		completed = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("writer merge did not finish after releasing barrier")
	}
	if files, err := c.getTableFileCount("measurement"); err != nil || files != 1 {
		t.Fatalf("writer did not finish real merge: files=%d err=%v", files, err)
	}
}
