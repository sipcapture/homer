# Stateless deployment and horizontal scaling

Enable top-level `stateless: true` to run Homer without a persistent local data
disk. PostgreSQL stores application settings, DuckLake metadata, and shared
coordinator state; S3 or Azure stores Parquet data. Local DuckDB instances remain
the query engine. Stateless validation and shared coordinator HTTP state are
opt-in. Cross-replica PostgreSQL maintenance locks, the shared compaction
schedule, and accelerated eligibility polling are enabled only by top-level
`stateless: true`. With the flag disabled or omitted, PostgreSQL and SQLite keep
their existing maintenance coordination and schedules. General flush-buffer
correctness fixes apply in both modes.

## Prepare shared storage and secrets

Provision two PostgreSQL databases owned by the Homer role: one for settings and
one for the DuckLake catalog. Homer creates tables, but does not create databases
or automatically migrate existing local data. Install matching DuckDB extensions
in the runtime image using `homer-core --install-extensions`.

Enable the flag and provide the same configuration and secrets on every replica.
For example, inject these values through Kubernetes Secrets or your deployment's secret manager:

```sh
HOMER_COORDINATOR_SETTINGS_DB_TYPE=postgres
HOMER_COORDINATOR_SETTINGS_DB_PATH=postgres://homer:password@postgres/homer_settings?sslmode=require
HOMER_STORAGE_DUCKLAKE_CATALOG_PATH=postgres://homer:password@postgres/homer_catalog?sslmode=require
HOMER_NODE_DUCKLAKE_CATALOG_PATH=postgres://homer:password@postgres/homer_catalog?sslmode=require
HOMER_COORDINATOR_JWT_SECRET=<shared-random-signing-secret>
HOMER_NODE_FLIGHT_SERVER_AUTH_TOKEN=<shared-random-node-token>
```

Replace the example credentials. Use the existing S3/Azure credential settings
or a workload identity available to every pod. Configure both writer and reader
credentials. If FlightSQL is enabled, configure its authentication token as well;
the existing node-token inheritance is supported. Startup rejects missing required
secrets instead of generating local secret files.

See [PostgreSQL settings](POSTGRES_SETTINGS.md) for the DuckDB
extension limitations and direct PostgreSQL `RETURNING` workaround, and
[PostgreSQL catalogs](POSTGRES_DUCKLAKE.md) for catalog and shard configuration.

## Configure one combined pod

This example runs ingest, writer, reader, and coordinator in each replica. Replace
the bucket name and supply the connection strings and secrets above through the
environment:

```json
{
  "stateless": true,
  "ingest": { "enable": true, "queue_size": 200000, "hep_stream": { "enable": false } },
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "postgres",
      "data_path": "s3://homer-data/parquet",
      "lake_name": "homer_lake",
      "shard_count": 1,
      "batch_size": 10000,
      "flush_interval_sec": 30,
      "s3": { "region": "us-east-1" },
      "tuning": { "memory_limit": "512MB", "temp_directory": "/tmp/homer-writer" },
      "compaction": { "enable": false }
    }
  },
  "node": {
    "enable": true,
    "flight_server": { "host": "127.0.0.1", "port": 50051 },
    "ducklake": {
      "catalog_type": "postgres",
      "data_path": "s3://homer-data/parquet",
      "lake_name": "homer_lake",
      "s3": { "region": "us-east-1" },
      "tuning": { "memory_limit": "512MB", "temp_directory": "/tmp/homer-node" }
    }
  },
  "coordinator": {
    "enable": true,
    "hep_stream": { "enable": false },
    "auth": { "type": "internal", "admin_user": "admin" },
    "http_server": { "host": "0.0.0.0", "port": 8080 }
  },
  "log": { "output": ["stdout"] }
}
```

With no explicit coordinator nodes, configuration adds the pod's local reader.
Every reader sees the shared catalog's committed data. Do not configure the
coordinator to fan out to all replicas of that same catalog: it can return the
same records multiple times. Leave `search_buffer` disabled for this topology;
unflushed memory buffers belong only to their writer process. Startup rejects
`search_buffer` and either live HEP stream setting in stateless mode; explicitly
disable both HEP stream settings, which default to enabled.

Mount writable ephemeral scratch space for spill and extension caches. No local
settings, catalog, or Parquet volume is required. Size container memory for the
whole process, including Go memory, DuckDB instances, ingest queues, and any
maintenance instance; a DuckDB memory limit is not a container memory limit.

Automatic maintenance is disabled in this example. With `stateless: true`,
replicas using PostgreSQL catalogs acquire a database-scoped session advisory
lock for the whole automatic maintenance cycle (retention, merge, snapshot
expiration, and cleanup).
A replica skips the cycle if another replica holds the lock or acquisition fails.
The lock uses a dedicated connection to the actual primary catalog database;
closing that session releases it. Use a direct PostgreSQL endpoint or session
pooling, not transaction pooling. Local DuckDB lake aliases do not affect lock
identity. Writer compaction and inline flushing, node vacuum requests, CLI
maintenance, and tiering moves/deletion/cleanup use the same lock. Tiering locks
both source and destination catalogs before reading counts or moving a partition;
multiple catalogs are acquired in a stable order with partial acquisitions released
on contention. Tiering retries lock contention within the same operation for up
to five seconds, then returns a timeout so a later tiering cycle can retry.
Vacuum returns HTTP 409 on contention and 503 on acquisition errors;
CLI maintenance returns an error. Automatic operations retry on later cycles.

Automatic compaction stores its shared completion timestamp as the global
`homer.maintenance.compaction.completed_at` key in the Homer-owned
`public.homer_maintenance_schedule` table, created on first use. DuckLake metadata
is untouched. The check and update both run under the
catalog advisory lock, using PostgreSQL time. Keep `compaction.check_interval_sec`
consistent across replicas: after a fully successful cycle, all replicas skip
until that interval has elapsed. The schedule survives pod restarts and is
independent for each catalog/shard.

Stateless PostgreSQL replicas check eligibility every minute (or the configured
interval when shorter), so interrupted or partially failed cycles can be retried
without waiting a full compaction interval. Failed retention, inline flush, merge, or
cleanup steps prevent the completion timestamp from advancing. Tables already
quarantined from merging do not make subsequent cycles incomplete. Other steps may
still finish, and the next attempt works against the current catalog rather than
resuming a checkpoint.

The catalog role needs CREATE on the schedule schema (public) to provision
public.homer_maintenance_schedule, plus SELECT, INSERT, and UPDATE on that table.
DuckLake's own metadata permissions do not automatically grant these Homer-table
permissions. If provisioning or schedule access fails, automatic compaction still
runs under the advisory lock. If writing the marker fails after otherwise
completed work, that replica waits its full configured interval before trying
again. This local fallback cannot coordinate the schedules of different replicas.
When `stateless` is disabled or omitted,
Homer does not acquire cross-replica advisory locks, provision or query this
schedule table, or require its permissions.

Only a valid recent marker suppresses compaction. Inline flushing when compaction
is disabled and manually requested maintenance keep their existing schedules.

Maintenance checks owning sessions before SQL and monitors them every second.
Detected connection loss cancels the operation context, including in-flight SQL
and Azure cleanup, and prevents subsequent statements. Cancellation is cooperative:
a statement or object-store request already in progress may finish before it is
interrupted. The advisory lock is not storage-level fencing during a network
partition. Normal ingestion still uses DuckLake's catalog transactions. Manually
issued SQL does not participate in this application lock and must be coordinated
by the operator. Existing missing objects still require repair.
See [compaction](COMPACTION_SETUP.md) for maintenance settings.

## Add Kubernetes autoscaling

Use a Deployment and Service to scale the whole combined pod. Route coordinator
HTTP traffic through the Service and configure ingest traffic distribution for
the protocols you enable. New replicas do not automatically receive existing
long-lived TCP connections. OAuth and ordinary authenticated HTTP requests do
not require sticky sessions when all replicas share the same settings database
and JWT secret.

Set CPU and memory requests on the Homer container, and choose limits from load
tests. For example, start testing with a CPU request of `1`, memory request of
`2Gi`, and memory limit of `4Gi`; these are illustrative, not sizing guarantees.
An initial HPA for a Deployment named `homer` can use:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: homer
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: homer
  minReplicas: 2
  maxReplicas: 6
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
```

CPU utilization is measured against resource requests. Resource metrics require
the Metrics API, commonly supplied by Metrics Server. Memory and custom metrics
can also be added; with multiple metrics, HPA uses the largest recommended replica
count. See the [Kubernetes HPA documentation](https://kubernetes.io/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale/).

CPU alone can miss storage congestion: a pod waiting on PostgreSQL or object
storage may have low CPU. Monitor per-pod queue pressure and flush latency; expose
a queue-pressure custom metric to HPA through a metrics adapter if needed. The
example above does not configure that adapter. More replicas will not fix an
already saturated shared database or object store.

## Buffers, congestion, and shutdown

| Buffer | Behavior |
| --- | --- |
| Ingest queue | Fixed capacity from `ingest.queue_size`, default 200,000 packets |
| Go batch | `storage.ducklake.batch_size` triggers appending, default 10,000 records per table; not a total memory cap |
| DuckDB buffers | Two memory tables per table, with no fixed row capacity; constrained by memory/spill configuration |

Each buffer admits one appender at a time. A queued flush does not reserve its
buffer: appends can reuse it while the job waits for the catalog lock. Appends
and INSERT/TRUNCATE flushes hold the same exclusive buffer lock. Connection or
appender failures retain the entire Go batch for retry, and partially appended
rows are rolled back. Failed lake inserts retain the memory-table rows. If an insert succeeds but clearing the buffer fails, retries
clear those committed rows without inserting them again. Final flush failures
are returned during shutdown.

Waiting moves pressure to the ingest queue. The shared `EnqueuePacket` path drops
packets when that queue is full, regardless of transport; TCP alone does not
guarantee backpressure or replay. UDP can also lose packets in socket buffers.
Use an external durable queue or replay-capable sender when delivery guarantees
are required. Stateless mode does not make in-flight ingestion durable.

Useful Prometheus signals include:

- `homer_worker_queue_depth` and `homer_worker_queue_capacity` for occupancy.
- `homer_packet_pipeline_queue_wait_duration_seconds` for queue wait time.
- `homer_ducklake_table_flush_duration_seconds` for storage latency.
- `homer_hep_packets_failed_total` with `reason="queue_full"` for queue drops.

Use a conservative scale-down policy and test draining with your real traffic.
Give Kubernetes enough termination grace for routing changes and flushing, but
note that Homer currently has an internal **30-second shutdown timeout**. A
larger `terminationGracePeriodSeconds` does not extend that internal timeout.
Prevent abrupt termination and ensure buffers can drain within the available
budget. Kubernetes documents its signal and grace-period behavior under
[pod termination](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-termination).

## Limitations and verification

Stateless mode shares revoked sessions, OAuth state and one-time login tokens,
avatars, and export jobs/payloads through PostgreSQL. OAuth entries are consumed
atomically. Revocation lookups have a one-second timeout and can reuse a
successful answer for up to five seconds during an outage; without a usable
answer, authentication returns HTTP 503. The cached answer can be "not revoked":
a revocation written by another replica during that outage may not be observed
until the database recovers or the cached answer expires. Runtime-state reads
and writes reuse a native PostgreSQL pool capped at eight connections per
coordinator. Bootstrap schema and seed operations are serialized between replicas.
Export entries expire after
24 hours. Multiplayer Netris/NetChess hubs, live HEP streams, and search buffers
are disabled. Active connections, ingest buffers, and correlation caches remain
ephemeral.

PostgreSQL sharding and tiering cannot currently be combined. Independent shard
databases must be pre-provisioned and reader volumes explicitly configured.
The combined-pod example uses one shard to avoid those additional routing steps.

Integration checks cover two combined processes, each running network ingest,
writer, node, and coordinator with stateless mode enabled. The test ingests on
replica A and searches through replica B's coordinator and local node, checking
unique Call-IDs. The same run covers cross-replica settings, exports, OAuth PKCE
handoff/replay rejection, logout, and process restarts. A separate coordinator-only
case exercises concurrent first-run bootstrap.

Schedule tests use PostgreSQL metadata created by DuckLake and real Parquet
files. They check successful merging, row preservation, shared completion,
and actual compaction while schedule reads and completion writes fail. Node
HTTP vacuum and tiering deletion/cleanup are tested while writer merge work owns
the same catalog's maintenance lease.

Focused buffer tests use PostgreSQL and S3-compatible RustFS in per-table and
central-queue modes. They check reuse while flushes wait for the catalog lock,
and failed-TRUNCATE recovery without losing new batches or duplicating committed
rows. Multi-process lake tests also exercise 20 ms stress flushing with 4,096
unique records and concurrent SQL compaction.

On 2026-11-07 the full two-process scenario passed against an external
S3-compatible endpoint and shared local PostgreSQL, using 16 rows per batch.
It covered both flush queue modes and killing a writer during flush and during
compaction. Durable-data verification waits for maintenance to finish and holds
the existing advisory lock while reading; this is a test-only workaround for the
known immediate-cleanup/read race. Production cleanup behavior is unchanged.
This run does not establish correctness of reads overlapping physical cleanup.

Real Azure maintenance has not been run; its tests use a fake object store.
The scenario also does not establish recovery of uncommitted in-memory packets
after abrupt writer termination, Kubernetes traffic distribution, or sustained
production throughput.

Run from `src`, against disposable services:

```sh
# PostgreSQL test role needs CREATE DATABASE; each test drops its private databases.
export HOMER_TEST_POSTGRES_DSN='postgres://postgres:password@127.0.0.1:5432/test?sslmode=disable'
go test ./coordinator -run TestPostgresStatelessCoordinatorReplicas -count=1 -v

export HOMER_TEST_S3_ENDPOINT='http://127.0.0.1:9000'
export HOMER_TEST_S3_ACCESS_KEY='test-access-key'
export HOMER_TEST_S3_SECRET_KEY='test-secret-key'
go test ./coordinator -run TestPostgresStatelessCombinedReplicas -count=1 -v
go test ./writer -run TestPostgresCompactionSchedule -count=1 -v
go test ./storage/ducklake -run 'TestPostgres(HorizontalWritersAndReaders|ObjectStorage)' -count=1 -v
go test -race ./storage/ducklake -run 'TestBufferReuse|TestFailedBuffer' -count=5
```

The object-storage tests create and remove unique buckets. Install matching
DuckDB extensions and the project's build dependencies before running them.

For a runnable two-process deployment and controlled writer-interruption harness
using an existing S3-compatible bucket, see
[the real scenario guide](../scripts/stateless-scenario/README.md).

Shared export creation accepts request bodies up to 1 MiB. Each generated payload
is limited to 10 MiB, and its encoded runtime-state record to 16 MiB; larger
requests or results return HTTP 413. Shared exports expire after 24 hours.
Status and download endpoints require the creating username in both deployment
modes. Another user, or an older export without an owner, receives HTTP 404.
