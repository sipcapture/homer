# Lua examples

Small **reference** Lua snippets that mirror what ships inside the binary or
what operators paste into settings.

## `correlation/`

Default **call-id correlation** templates for the coordinator Lua engine
(`POST /api/v4/transactions/messages`). They are the same text as the files
embedded at build time from `src/coordinator/correlation_seed_lua/` and seeded
(disabled) into the DuckDB table `correlation_scripts` when
`coordinator.correlation.seed_default` is enabled.

| File | Seeded as | Role |
|------|-----------|------|
| `sip_call.lua` | `hepid=1`, `profile=call` | B2B / `x_call_id` style expansion for SIP calls |
| `sip_registration.lua` | `hepid=1`, `profile=registration` | Group REGISTER traffic by AOR in the UI time window |

**Keeping in sync:** after editing the copies here, copy the same content into
`src/coordinator/correlation_seed_lua/` (or the other way around) before
merging — `go:embed` only reads the tree under `src/coordinator/`.

Full API (`correlate`, `executeSQL`, `getDataByField`, limits, sandbox) is
documented in [`src/scripting/correlation/README.md`](../../src/scripting/correlation/README.md).

## Elsewhere under `examples/`

- **`scripts-loki-labels/`** — label-shaping Lua for Loki, not correlation.
