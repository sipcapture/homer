package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// CatalogFlags holds flags for the "homer catalog" subcommand.
type CatalogFlags struct {
	Action     string // backup | restore | list
	ConfigPath string
	Keep       int
	Out        string
	From       string
}

type catalogFlagRefs struct {
	ConfigPath *string
	Keep       *int
	Out        *string
	From       *string
}

// RegisterCatalogFlags creates a FlagSet for "homer catalog".
func RegisterCatalogFlags() (*flag.FlagSet, *catalogFlagRefs) {
	fs := flag.NewFlagSet("catalog", flag.ExitOnError)
	refs := &catalogFlagRefs{}

	refs.ConfigPath = fs.String("config-path", "", "path to config file or directory")
	refs.Keep = fs.Int("keep", ducklake.DefaultCatalogBackupKeep,
		"rotating `.bak-*` copies to retain (backup only; 0 = keep all)")
	refs.Out = fs.String("out", "", "write backup to this path instead of a rotating `.bak-*` copy (backup only)")
	refs.From = fs.String("from", "", "backup file to restore (restore only; default: newest copy next to the catalog)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `homer catalog [action] [flags]

Snapshot and restore the DuckLake SQLite catalog (metadata only — parquet is not copied).

Actions:
  backup    Consistent VACUUM INTO snapshot (safe while Homer is running)
  restore   Replace the live catalog from a backup (stop Homer first)
  list      List rotating backups and pre-restore copies next to the catalog

Flags:
`)
		fs.PrintDefaults()
	}
	return fs, refs
}

// ParseCatalogFlags extracts CatalogFlags from parsed flag refs.
func ParseCatalogFlags(refs *catalogFlagRefs) CatalogFlags {
	return CatalogFlags{
		ConfigPath: *refs.ConfigPath,
		Keep:       *refs.Keep,
		Out:        *refs.Out,
		From:       *refs.From,
	}
}

// RunCatalogCmd dispatches catalog backup / restore / list.
func RunCatalogCmd(f CatalogFlags) error {
	switch strings.ToLower(strings.TrimSpace(f.Action)) {
	case "backup":
		return runCatalogBackup(f)
	case "restore":
		return runCatalogRestore(f)
	case "list":
		return runCatalogList(f)
	case "":
		return fmt.Errorf("no catalog action specified; use backup, restore, or list")
	default:
		return fmt.Errorf("unknown catalog action %q; use backup, restore, or list", f.Action)
	}
}

func catalogPathFromConfig(configPath string) (string, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}
	path := strings.TrimSpace(duckLakeConfigFromModular(cfg).CatalogPath)
	if path == "" {
		return "", fmt.Errorf("no sqlite catalog_path in config")
	}
	return path, nil
}

func runCatalogBackup(f CatalogFlags) error {
	catalogPath, err := catalogPathFromConfig(f.ConfigPath)
	if err != nil {
		return err
	}

	var dest string
	if strings.TrimSpace(f.Out) != "" {
		dest, err = ducklake.BackupCatalogTo(catalogPath, f.Out)
	} else {
		dest, err = ducklake.BackupCatalog(catalogPath, f.Keep)
	}
	if err != nil {
		return err
	}
	fmt.Printf("catalog backup: %s\n", dest)
	return nil
}

func runCatalogRestore(f CatalogFlags) error {
	catalogPath, err := catalogPathFromConfig(f.ConfigPath)
	if err != nil {
		return err
	}
	previous, err := ducklake.RestoreCatalog(catalogPath, f.From)
	if err != nil {
		return err
	}
	fmt.Printf("catalog restore: %s\n", catalogPath)
	if previous != "" {
		fmt.Printf("previous catalog saved at: %s\n", previous)
	}
	return nil
}

func runCatalogList(f CatalogFlags) error {
	catalogPath, err := catalogPathFromConfig(f.ConfigPath)
	if err != nil {
		return err
	}
	list, err := ducklake.ListCatalogBackups(catalogPath)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Printf("no catalog backups next to %s\n", catalogPath)
		return nil
	}
	for _, b := range list {
		fmt.Printf("%s\t%d\t%s\n", b.Path, b.Size, b.ModTime.UTC().Format(time.RFC3339))
	}
	return nil
}
