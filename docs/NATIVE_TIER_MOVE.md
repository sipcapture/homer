# Native tier move (hot → cold)

How Homer copies a date partition from one storage-policy volume to the next.
This is **not** [native compaction](OOM.md#6-native-go-compaction-engine): compaction
rewrites parquet *inside* one lake; a tier move copies files *between* two lakes
(typically local hot → S3 cold).

Related: [Storage policies](STORAGE_POLICIES.md) · [sipcapture/homer#969](https://github.com/sipcapture/homer/issues/969)

## Default

`storage.ducklake.storage_policy.move_engine` defaults to **`duckdb`**. Unset and
empty mean the same thing. Existing deployments do not change behaviour.

```json
"storage_policy": {
  "enable": true,
  "move_engine": "duckdb"
}
```

That path is one `INSERT INTO cold SELECT * FROM hot WHERE date = ?` plus a
source `DELETE`. It works for small partitions. A full `hep_proto_1_call` day
(millions of wide SIP rows) holds the writer catalog lock for the whole copy and
has crashed DuckDB with SIGSEGV on the next flush ([#969](https://github.com/sipcapture/homer/issues/969)).

## Opt-in: `native`

```json
"storage_policy": {
  "enable": true,
  "move_engine": "native"
}
```

Env: `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_MOVE_ENGINE=native`

Native copies parquet **bytes** (local `io.Copy` or S3 multipart upload) and only
asks DuckLake to register the copies. SIP payloads are not decoded. The writer
catalog lock is **not** held during the copy.

Empty `s3_access_key_id` uses the AWS default credential chain (env, shared
config, instance/task role) via `LoadDefaultConfig`. Static keys from the volume
override that chain. Custom endpoints (RustFS / MinIO / R2) use path-style unless
`s3_url_style` is `vhost`. Uploads go through the AWS SDK uploader: files larger
than 8MB use multipart.

```
1. Catalog lock (milliseconds)
   List the partition's parquet files from the hot DuckLake catalog.
   Unlock.

2. No DuckDB, no lock
   Copy each file to the destination data_path, keeping the hive directory
   (date=YYYY-MM-DD/…). Local dest = filesystem; s3:// dest = AWS SDK PUT
   (path-style on custom endpoints / MinIO / R2 / RustFS).

3. No writer catalog lock
   CALL ducklake_add_data_files on the *cold* lake (cold SQLite catalog).
   DuckLake allocates snapshot and file ids — Homer does not write the catalog
   itself (the lesson from early native compaction).

4. Catalog lock (seconds)
   DELETE FROM hot WHERE date = ?  — whole-file retirement, same idea as a
   native compaction swap. Unlock.
```

DuckLake still issues every snapshot id. Two catalogs, no cross-DB transaction:
copy is idempotent (overwrite), register is skipped when cold already has the
row count, a failed source delete is retried as delete-only on the next cycle.

## When native falls back to `duckdb`

The cycle logs `native file move unavailable, using duckdb INSERT` and runs
`INSERT … SELECT` if any of:

| Reason | Why |
|--------|-----|
| Partition has row-level delete files | Copying parquet as-is would resurrect deleted rows. Retention's mid-day cutoff does this. |
| Source lake still has inlined rows | Those rows are not parquet yet. |
| Source `data_path` is `s3://` | Native only opens local files. |
| `ducklake_add_data_files` missing | This DuckLake build cannot register pre-copied files. |
| Table is not `PARTITIONED BY` a single identity column | No safe retirement predicate. |

Fallback is per partition. Other dates still take the native path.

## What to watch in logs

```
TieredStorageManager: Moving partition  engine=native   table=hep_proto_1_call date=2026-07-18
native mover: copying partition files   files=12 rows=8133794
native mover: partition moved           files=12 rows=8133794 bytes=…
TieredStorageManager: Partition moved   engine=native
```

Startup:

```
TieringService: Starting  move_engine=native
```

If you stay on the default, `move_engine=duckdb` and there is no `native mover:` line.

After a native cycle, `partitions_moved` still means “copy + source delete both succeeded”.
`Partition copied, source delete pending` is the same as before: cold has the data, hot delete
retries next cycle.

## Safety

- **Do not write the SQLite catalog by hand.** Registration goes through
  `ducklake_add_data_files` so the live writer's id cache cannot go stale.
- **Do not raise compaction `max_row_group_bytes` for this.** Native *move* does
  not merge row groups; it copies files as they are.
- **Cold catalog is the failure domain.** A bad register is recovered by restoring
  that volume's catalog (see [CATALOG.md](CATALOG.md) / [OOM.md](OOM.md#recovering-a-corrupted-catalog)).
  Hot is only deleted after cold row count matches.
- **Try native on a non-critical instance first**, then on the node that hit #969
  (multi-million-row `hep_proto_1_call` day, live ingest, `concurrent_moves=1`).

## Not native compaction

| | Compaction `engine=native` | Tier `move_engine=native` |
|---|---|---|
| Config | `storage.ducklake.compaction.engine` | `storage.ducklake.storage_policy.move_engine` |
| Default | `duckdb` | `duckdb` |
| Job | Merge small parquet *in one lake* | Copy a date partition *to the next volume* |
| I/O | Concatenate row groups (Arrow) | Byte copy / S3 multipart upload |
| Remote `data_path` | Refused | Destination may be `s3://` |
| Row-level deletes | Skip that partition | Fall back to `INSERT … SELECT` |

You can run DuckDB compaction and native tier move independently.

## Testing against RustFS / MinIO

The mover package has an integration test that talks to an S3 clone. It skips when
nothing is listening.

```bash
# defaults match examples/docker/docker-compose_s3direct.yaml
go test ./storage/ducklake/mover/ -count=1 -timeout 5m \
  -run 'TestS3CopierMultipartRoundTripRustFS|TestNativeMoveLocalToRustFS'
```

Override with `HOMER_TEST_S3_ENDPOINT`, `HOMER_TEST_S3_ACCESS_KEY`,
`HOMER_TEST_S3_SECRET_KEY`, `HOMER_TEST_S3_BUCKET`.

## Check effective setting

```bash
homer config show --config-path /etc/homer/homer.json --section storage.ducklake.storage_policy
```
