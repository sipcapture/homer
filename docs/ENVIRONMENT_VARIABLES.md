# Environment variables (modular Homer server)

When Homer starts **without** a usable JSON/YAML config file, `config.Load` still applies **defaults** and reads **environment variables** ([`src/config/config.go`](../src/config/config.go)).

## Rules

| Mechanism | Detail |
|-----------|--------|
| Prefix | All variables for this path use the prefix **`HOMER`**. |
| Nesting | JSON / mapstructure keys use dots in code (e.g. `storage.ducklake.catalog_path`). In the environment, **dots become underscores**: `HOMER_STORAGE_DUCKLAKE_CATALOG_PATH`. |
| Arrays / slices | Use a **numeric index** in the name (Viper convention), e.g. `HOMER_COORDINATOR_NODES_0_HOST`, `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ENDPOINT`. |
| Precedence | Defaults → optional config file → **environment overrides** (via Viper `AutomaticEnv`). |

Legacy **Homer Server** config (`homerconfig`) uses a different prefix (`HOMERCORE`) and only applies in legacy mode — not the modular stack described here.

## Discovering names

Field names follow the `mapstructure` tags on [`Config` in `src/config/config.go`](../src/config/config.go) and nested structs (`ingest`, `storage`, `node`, `coordinator`, `log`, `prometheus`, …). Example:

- `storage.ducklake.storage_policy.volumes[1].s3_endpoint`  
  → `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ENDPOINT`

## References

- Loader: `config.Load` — `SetEnvPrefix("HOMER")`, `SetEnvKeyReplacer(".", "_")`, `AutomaticEnv()` ([source](../src/config/config.go)).
- Tiered storage fields: [`docs/STORAGE_POLICIES.md`](STORAGE_POLICIES.md) (conceptual); same paths appear under `storage.ducklake.storage_policy` and `node.ducklake` in JSON — mirror them as `HOMER_*` as above.
