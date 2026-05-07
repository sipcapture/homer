# MCP Query Assistant for Homer Core

This guide explains how to configure and use the built-in MCP query assistant in Homer Core.

The MCP assistant lets users type natural language queries (for example: "find INVITE messages from the last hour"), and executes them in:

- `structured` mode (`/api/v4/transactions/search`)
- `sql` mode (`/api/v4/query`)
- `auto` mode (defaults to structured, switches to SQL only when explicitly requested)

## 1. Configuration

Add the `mcp` section to your `homer-core.json`:

```json
{
  "mcp": {
    "enable": true,
    "mode": "hybrid",
    "homer_base_url": "http://127.0.0.1:8080",
    "homer_token": "replace-with-jwt-token",
    "default_limit": 100,
    "sql_default_limit": 100,
    "request_timeout_sec": 30,
    "llm": {
      "enable": false,
      "provider": "openai",
      "base_url": "https://api.openai.com/v1",
      "api_key": "",
      "model": "gpt-4o-mini",
      "temperature": 0.1,
      "max_tokens": 400,
      "timeout_sec": 15
    }
  }
}
```

### Fields

- `enable` - Enables the MCP module at server startup.
- `mode` - Global mode policy: `hybrid`, `structured`, or `sql`.
- `homer_base_url` - Base URL for coordinator API calls.
- `homer_token` - Bearer JWT token used by MCP HTTP calls.
- `default_limit` - Default row limit for structured mode.
- `sql_default_limit` - Default row limit for SQL mode.
- `request_timeout_sec` - HTTP timeout for MCP backend calls.
- `llm.*` - Optional LLM-assisted parser for natural language.

## 1.1 Optional LLM Provider Settings

To enable LLM parsing:

- set `mcp.llm.enable=true`
- set `mcp.llm.base_url` to any OpenAI-compatible chat-completions endpoint
- set `mcp.llm.api_key` only when the provider requires it (Ollama / vLLM / LM Studio do not)
- override `model` to match the provider (`gpt-4o-mini`, `llama3.1`, `qwen2.5`, …)

The LLM client speaks the OpenAI Chat Completions JSON-mode protocol, so any
OpenAI-compatible backend works:

| Provider  | `base_url`                     | `api_key` | Notes |
|-----------|--------------------------------|-----------|-------|
| OpenAI    | `https://api.openai.com/v1`    | required  | sk-… |
| Ollama    | `http://localhost:11434/v1`    | empty     | local, no key |
| vLLM      | `http://host:8000/v1`          | empty     | self-hosted |
| LM Studio | `http://localhost:1234/v1`     | empty     | local, no key |
| OpenRouter| `https://openrouter.ai/api/v1` | required  | sk-or-… |

The Coordinator and stdio MCP server share the **same** LLM client
(`src/mcp.LLMClient`), so any provider that works for one transport also works
for the other.

## 2. Run Modes

You can run MCP in two ways:

### A) As part of normal modular server startup

If `mcp.enable=true`, start Homer Core normally:

```bash
./homer --config-path /etc/homer-core/homer-core.json
```

### B) As dedicated stdio MCP process

Use the built-in subcommand:

```bash
./homer mcp --config-path /etc/homer-core/homer-core.json
```

## 3. UI Integration (already wired)

The UI `SearchPanel` has an `AI` tab that sends requests to:

- `POST /api/v4/mcp/query`
- `GET /api/v4/mcp/llm/status` (optional health/status check)

Request body:

```json
{
  "query_text": "find INVITE messages from the last hour",
  "mode": "auto",
  "parser": "auto",
  "limit": 100,
  "timestamp": {
    "from": 1740652800000,
    "to": 1740656400000
  }
}
```

#### `parser` field (added)

The `parser` field selects the NL→filters strategy:

| Value   | Behavior                                                               |
|---------|------------------------------------------------------------------------|
| `auto`  | Default. Try LLM first; transparently fall back to regex on any error. |
| `llm`   | Force LLM. Returns 502/424 if LLM is disabled or fails (no fallback).  |
| `regex` | Skip the LLM completely; use the deterministic regex parser only.      |

The UI exposes this as a `Parser` selector right next to the existing `Mode`
selector in the AI tab.

### Response behavior

- Rows are rendered in `ResultsPanel`.
- In SQL execution mode, generated SQL is returned in `meta.message` and displayed in UI.
- `meta.parser` reports which parser actually ran. The UI shows it as a
  badge next to the row count:

```json
{
  "meta": {
    "parser": {
      "used": "llm",
      "requested": "auto",
      "model": "gpt-4o-mini",
      "latency_ms": 412
    }
  }
}
```

`used` is one of `llm`, `regex_fallback`, or `regex`. When the LLM was tried
and failed, `error` carries the failure reason — useful for diagnosing why a
fallback happened.

### LLM status endpoint

You can verify runtime LLM configuration and connectivity:

```bash
curl -H "Authorization: Bearer <JWT>" \
  "http://127.0.0.1:8080/api/v4/mcp/llm/status?check=true"
```

Notes:

- `check=true` (default) performs a provider ping (`/models` for OpenAI-compatible API).
- `check=false` returns config/runtime status without network call.

## 4. Mode Semantics

### `auto`

- Uses structured mode by default.
- Switches to SQL only when query text explicitly asks for SQL (for example: `sql: ...`, `show sql`, `mode=sql`).

### `structured`

- Converts NL query to safe field filters and time range.
- Executes through the structured search path.

### `sql`

- Converts NL query to SQL and validates it with server-side SQL validator.
- Executes through `/api/v4/query`.

## 5. Example Queries

- `find all INVITE messages from the last hour`
- `show sql for INVITE messages for today`
- `find calls from src ip 10.10.0.5`
- `find call-id abc-123 in the last hour`

## 6. Security Notes

- SQL queries are validated server-side before execution.
- Dangerous SQL operations are blocked by validator rules.
- Use short-lived token for `mcp.homer_token` when possible.

## 7. Troubleshooting

### "query_text is required"

The UI/API request body did not include `query_text`.

### "mode must be one of: auto, structured, sql"

Invalid `mode` value in request body.

### "SQL validation failed"

Generated SQL violated validator constraints (for example forbidden token, unsupported statement, or disallowed syntax).

### Empty results

Check:

- time range (`timestamp.from` / `timestamp.to`)
- SIP method spelling (`INVITE`, `BYE`, etc.)
- selected mode (`auto`, `structured`, `sql`)
- token validity and coordinator connectivity
