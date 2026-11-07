# Real stateless scenario: PostgreSQL and S3-compatible storage

`scripts/stateless-scenario/run.py` starts two actual Homer binaries on one Linux
host. Both run ingest, writer, node, and coordinator, using shared PostgreSQL and
an existing object-storage bucket. It generates distinct Call-IDs and fails on
missing committed rows, duplicate rows, or unexpected rows.

Supply your S3 endpoint, bucket name, access key ID, and secret access key.
The harness uses path-style S3 URLs and Signature Version 4. The signing region
defaults to `us-east-1` and can be overridden; no backing storage provider is
assumed.

## Prerequisites

- Linux/POSIX signals and Python 3.10 or newer.
- A current Homer executable with the PR fixes and matching DuckDB extensions.
- The tiering probe built from the same source revision (Go is needed to build it).
- A direct/session-pooled PostgreSQL endpoint. The supplied role must be able
  to CREATE DATABASE and own its new schemas/tables. Transaction pooling cannot
  support the session advisory locks.
- An existing S3 bucket and credentials that can list, create, read, and
  delete objects. The script creates no bucket and modifies no bucket policy.
- Free loopback ports; the script allocates separate ingest, node, and
  coordinator ports for each process.

The script always creates two fresh, uniquely named databases; it does not attach
an existing Homer catalog or settings database. Every object key is beneath a
unique run prefix. Processes, generated configuration, and evidence belong only
to that run.

## Run

From the repository root:

```sh
python3 -m venv local/scenario-venv
local/scenario-venv/bin/pip install -r scripts/stateless-scenario/requirements.txt

# Or use the matching release binary.
mkdir -p local
(cd src && go build -o ../local/homer-core .)
(cd src && go build -o ../local/stateless-maintenance-probe ../scripts/stateless-scenario/maintenance_probe.go)

# Preinstall the checksum-verified extensions for this exact DuckDB version.
./scripts/download_duckdb_extensions.sh
mkdir -p "$HOME/.duckdb/extensions"
cp -R bundled_extensions/v1.5.6 "$HOME/.duckdb/extensions/"

# Configure scripts/stateless-scenario/.env using scripts/stateless-scenario/.env.example as a reference.
# The file is loaded automatically; no shell sourcing is needed.
local/scenario-venv/bin/python scripts/stateless-scenario/run.py \
  --binary ./local/homer-core \
  --bucket YOUR_EXISTING_S3_BUCKET
```

The harness loads `scripts/stateless-scenario/.env` relative to the script,
regardless of the working directory. If that file is absent, an existing
`scripts/.env` is used for compatibility. It uses these standard S3 variables:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_ENDPOINT_URL_S3`
- `AWS_S3_BUCKET`: existing bucket name, without `s3://` or a path.

Keep `HOMER_SCENARIO_POSTGRES_DSN` in the same file or export it separately.
The signing region comes from `--region`, `AWS_REGION`, or
`AWS_DEFAULT_REGION`, falling back to `us-east-1` when none is set.
Exported environment values take precedence over the file;
explicit `--bucket`, `--endpoint`, and `--region` flags take precedence over both.
When `AWS_S3_BUCKET` is set, you can omit `--bucket` from the command. Use
`--env-file /path/to/file` for a different dotenv file. Values are read without
executing shell commands or expanding variables inside secrets.

Credentials are passed to child processes through environment variables, not
command-line arguments or generated JSON configuration. The evidence directory
is private; raw process logs should be reviewed before sharing.

Useful options:

- `--maintenance-probe /path/to/stateless-maintenance-probe`: override the probe
  executable, which defaults to beside the Homer binary. Required for the full run.
- `--rows 256`: rows per ingest phase; a multiple of eight, from 16 through 4096.
- `--timeout 180`: seconds allowed for readiness, fault, and verification waits
  and durable-data verification requests.
- `--memory-limit 512MB`: DuckDB memory limit per instance.
- `--output /path/to/evidence`: parent of the private run directory.
- `--no-faults`: smoke test only; report status is `passed_smoke_only`.
- `--cleanup`: after stopping the processes, delete this run's newly created
  databases and current objects under its unique prefix. Evidence is retained.
  Bucket retention and versioning policies remain in effect.
- `--endpoint http://127.0.0.1:9000 --region us-east-1`: local S3-compatible rehearsal.

Resources are retained by default for inspection, even on failure. The report
lists the exact created database names and bucket prefix for later cleanup.
Existing databases, objects outside the run prefix, and the bucket itself are
never deleted by the script.

## What the run proves

1. PUT/GET/LIST/DELETE work through the configured S3 endpoint and credentials.
2. Ingest through A's HTTP HEP receiver becomes searchable through B's local node
   and coordinator, with unique Call-IDs.
3. Cross-replica login, logout/revocation, export retrieval, OAuth PKCE, and replay
   rejection work in that combined topology. OAuth uses a local validating
   provider stub; it does not certify an external identity provider.
4. A writer can be killed after real Parquet upload while its flush is blocked
   at a PostgreSQL catalog commit. The unchanged B process subsequently completes
   compaction; committed rows and shared exports survive.
5. A writer can be killed during actual merge work, after upload but before its
   catalog commit. B resumes and completes compaction without losing or duplicating
   committed rows. A restarts and accepts the original shared JWT.
6. Renaming the private schedule table's completion column causes real schedule
   reads to fail, while new Parquet files still merge. The column is restored
   even when the check fails.
7. With an actual writer merge blocked at its catalog commit, the node vacuum
   endpoint returns HTTP 409. A separate production tiering manager attaches
   the same real catalog and rejects partition deletion and volume cleanup
   because maintenance is already running.
8. A uses per-table flushing (`flush_queue=false`); B uses the central queue
   (`flush_queue=true`). Each writer receives five batches across five timer
   ticks while its real compaction holds the catalog mutex. The test queries
   both memory buffers to prove every new row was processed exactly once before
   releasing that mutex, then requires every row to become durable exactly once.

The harness holds the shared maintenance advisory lock while seeding files. For
fault injection it takes a SHARE lock on the private catalog's
`ducklake_data_file` table and observes `pg_blocking_pids` plus new Parquet
objects. It fails if that state cannot be observed; it never substitutes an
arbitrary sleep and claims a crash occurred during work. During compaction fault and contention checks, the other process is briefly
SIGSTOP'd to select the intended compactor, then resumed in a finally block. B is never killed or restarted for the failover checks.

The harness delays durable-data verification until the current maintenance
cycle finishes, using the existing advisory lock as a test-only barrier. It
holds that lock during verification so another cycle cannot delete files while
the check is reading. Memory-buffer observation and the controlled crash and
contention barriers still run during actual merge work. This avoids the known
immediate-cleanup/read race in the harness; it does **not** fix or validate
production reads that overlap physical cleanup. No production deletion delay
or retention configuration is introduced.

HTTP acknowledgement is **not** a durable commit. Every row observed through B's
committed-data query must remain present exactly once. Packets accepted only by
the interrupted flush may be absent; the report lists these separately as
`uncommitted_at_flush_crash`. They are not automatically replayed, because that
could duplicate a packet whose commit outcome was initially uncertain.

## Evidence and result

The script prints the run directory and writes `report.json` there, alongside
per-generation process logs and secret-free JSON configuration. The report
includes the binary SHA-256, endpoint, prefix, process PIDs, exact confirmed
Call-IDs, blocked PostgreSQL backend PIDs, uploaded object keys, and survivor
completion markers, queue modes, stress row counts, and contention probe logs.

Exit 0 means the requested mode passed. Any failed integrity check, missing fault
observation, startup failure, or failed requested cleanup returns nonzero. A smoke
run is explicitly labeled and does not establish crash coverage. Both child
processes are stopped on success, failure, or Ctrl-C.

A real deployment result must come from running this command against your S3
endpoint and bucket. A local rehearsal does not certify another endpoint,
prolonged production load, or durability of packets still in memory at process
termination.

The harness checks can be run without cloud credentials:

```sh
local/scenario-venv/bin/python -m unittest discover -s scripts/stateless-scenario -p test_run.py -v
```

Real Azure maintenance has **not been tested**. This harness targets
S3-compatible storage and records that limitation in its report; the Azure
maintenance path needs a separate real Azure run.

An earlier external S3-compatible run (`20261107T050726Z-09503959`) passed
with an experimental production cleanup grace period. That production change
has been withdrawn. Its result does not validate the current production code;
the test-only maintenance barrier above is used for subsequent runs.

Run `20261107T081230Z-905dc926` passed the full scenario against the external
S3-compatible endpoint and shared local PostgreSQL using the original production
cleanup code and the test-only verification barrier. It used 16 rows per batch
and covered both queue modes, both writer crashes, and JWT use after restart.
Its private report and logs are under `local/scenario/runs/`. This result does
not validate production reads racing file cleanup.
