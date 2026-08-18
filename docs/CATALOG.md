# Catalog backup and restore

The `homer catalog` subcommand snapshots and restores the **DuckLake SQLite catalog** — the metadata file that lists tables, snapshots, and parquet paths. The catalog is small (kilobytes to a few megabytes). **Parquet data is not copied.**

Native compaction already takes a rotating `VACUUM INTO` copy before each merge (last 3 files named `<catalog>.bak-<timestamp>`). This CLI is the same mechanism on demand, plus restore.

## Quick start

```bash
# Online-safe snapshot next to the catalog (keeps the last 3 `.bak-*` copies)
homer catalog backup --config-path /etc/homer/homer.json

# Durable copy on another disk (not rotated)
homer catalog backup --config-path /etc/homer/homer.json --out /backup/homer_catalog.sqlite

# List rotating backups and pre-restore copies
homer catalog list --config-path /etc/homer/homer.json

# Rewind catalog metadata. Stop the Homer writer first.
homer catalog restore --config-path /etc/homer/homer.json
homer catalog restore --config-path /etc/homer/homer.json --from homer_catalog.sqlite.bak-20260818T100000Z
```

## Actions

| Action    | Homer running? | Description |
|-----------|----------------|-------------|
| `backup`  | yes            | Consistent `VACUUM INTO` snapshot |
| `list`    | yes            | List `.bak-*` copies and `.pre-restore-*` aside files |
| `restore` | **no**         | Replace the live catalog from a backup |

## Flags

| Flag            | Default | Description |
|-----------------|---------|-------------|
| `--config-path` |         | Path to `homer.json` (or a directory containing it) |
| `--keep`        | `3`     | Rotating `.bak-*` copies to retain (`backup` only; `0` = keep all) |
| `--out`         |         | Write the snapshot to this path instead of a rotating `.bak-*` copy |
| `--from`        | newest `.bak-*` | Backup to restore (`restore` only). Absolute path, path relative to cwd, or a basename next to the catalog |

## What is backed up

DuckLake splits storage in two:

| Piece | Typical path | In the backup? |
|-------|--------------|----------------|
| SQLite catalog | `/data/homer/homer_catalog.sqlite` | yes |
| Parquet files | `/data/homer/parquet/` | **no** |

Use restore to **undo a bad metadata rewrite** (failed compaction cycle, botched catalog edit). It will not bring back parquet files that were deleted, compacted away, or expired.

If the catalog is unreadable (`Corrupt DuckLake - multiple snapshots returned from database` and auto-repair cannot attach it), rebuild from parquet instead: `homer system --rebuild-catalog`. See [OOM — recovering a corrupted catalog](OOM.md#recovering-a-corrupted-catalog).

## Backup

`VACUUM INTO` runs inside a SQLite read transaction, so it is safe while DuckDB has the catalog attached. Compaction uses the same call and keeps 3 rotating copies; a CLI backup without `--out` shares that `.bak-*` pool (the next native merge will prune down to 3 unless you pass `--keep`).

```bash
homer catalog backup --config-path /etc/homer/homer.json
# catalog backup: /data/homer/homer_catalog.sqlite.bak-20260818T100000Z
```

For a copy that compaction will not rotate away, use `--out`:

```bash
homer catalog backup --config-path /etc/homer/homer.json --out /backup/homer_catalog.sqlite
```

`--keep 0` disables pruning of the rotating `.bak-*` files for that run.

## List

```bash
homer catalog list --config-path /etc/homer/homer.json
```

Output is tab-separated: path, size in bytes, UTC modification time, newest first.

```
/data/homer/homer_catalog.sqlite.bak-20260818T100000Z	184320	2026-08-18T10:00:00Z
/data/homer/homer_catalog.sqlite.pre-restore-20260818T095500Z	183296	2026-08-18T09:55:00Z
```

`.pre-restore-*` files are the live catalog that `restore` moved aside. You can pass one to `--from` to undo a restore.

## Restore

Stop the Homer **writer** first. Restore takes the exclusive catalog lock (`<catalog>.lock`) and refuses if another writer still holds it. A read-only node with the catalog open will also fail (`catalog appears in use`).

Without `--from`, restore picks the newest rotating `.bak-*` copy (not a `.pre-restore-*` file).

```bash
# Stop the writer, then:
homer catalog restore --config-path /etc/homer/homer.json
# catalog restore: /data/homer/homer_catalog.sqlite
# previous catalog saved at: /data/homer/homer_catalog.sqlite.pre-restore-20260818T101500Z
```

What happens:

1. Validates that `--from` looks like a DuckLake catalog (`ducklake_snapshot` / `ducklake_table` / `ducklake_data_file`).
2. Takes the writer lock.
3. Renames the live catalog plus `-wal`/`-shm` to `*.pre-restore-<timestamp>`.
4. Writes the backup to the live path with `VACUUM INTO` (clean SQLite file, no WAL).
5. If step 4 fails, the previous catalog is renamed back.

Then start Homer again.

## When to use what

| Situation | Command |
|-----------|---------|
| About to run risky catalog maintenance | `homer catalog backup` (or `--out` off-box) |
| Native merge left the catalog wrong, a `.bak-*` from before that cycle exists | `homer catalog restore` |
| Catalog will not attach; parquet on disk is intact | `homer system --rebuild-catalog` |
| Need the lake data itself off-box | Copy parquet (and the catalog) — this CLI is metadata only |

## See also

- [Storage layout](STORAGE_LAYOUT.md) — catalog file and parquet tree
- [OOM / catalog recovery](OOM.md#recovering-a-corrupted-catalog) — auto-repair vs `--rebuild-catalog`
- [Data retention](RETENTION.md) — snapshot expiration and compaction flags
