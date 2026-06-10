package coordinator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
)

// newStaticTestCoordinator builds the minimal Coordinator needed to
// exercise setupStaticRoutes (no DB, no services).
func newStaticTestCoordinator(httpCfg config.CoordinatorHTTPServerConfig) *Coordinator {
	return &Coordinator{
		config: &config.CoordinatorConfig{HTTPServer: httpCfg},
		echo:   echo.New(),
	}
}

func TestGamedataRouteServesWadFromDisk(t *testing.T) {
	dir := t.TempDir()
	wad := []byte("IWAD\x00\x00\x00\x00fake")
	if err := os.WriteFile(filepath.Join(dir, "doom1.wad"), wad, 0o644); err != nil {
		t.Fatal(err)
	}

	c := newStaticTestCoordinator(config.CoordinatorHTTPServerConfig{GamedataDir: dir})
	c.setupStaticRoutes()

	rec := httptest.NewRecorder()
	c.echo.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gamedata/doom1.wad", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for existing wad, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != string(wad) {
		t.Fatalf("wad bytes mismatch: %q", got)
	}

	rec = httptest.NewRecorder()
	c.echo.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gamedata/missing.wad", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing file, got %d", rec.Code)
	}
}

func TestGamedataRouteDisabledWhenDirUnset(t *testing.T) {
	c := newStaticTestCoordinator(config.CoordinatorHTTPServerConfig{})
	c.setupStaticRoutes()

	rec := httptest.NewRecorder()
	c.echo.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gamedata/doom1.wad", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 when gamedata_dir is unset, got %d", rec.Code)
	}
}

func TestGamedataRouteRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "..", "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secret) })

	c := newStaticTestCoordinator(config.CoordinatorHTTPServerConfig{GamedataDir: dir})
	c.setupStaticRoutes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gamedata/../secret.txt", nil)
	c.echo.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && rec.Body.String() == "nope" {
		t.Fatal("path traversal escaped gamedata_dir")
	}
}
