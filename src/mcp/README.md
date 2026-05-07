# Homer Core MCP module (Go)

MCP integration for `homer-core`, implemented in Go and wired into the modular stack (`writer`, `node`, `coordinator`).

Built on top of [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go).

## Features

- `homer_search_transactions`: NL → `POST /api/v4/transactions/search`
- `homer_query_sql`: NL → SQL → `POST /api/v4/query`
- `homer_query`: hybrid mode (defaults to structured; SQL when the query explicitly asks for it)

NL parsing has two backends:

- **regex** (default, always available): deterministic SIP-method / IP / Call-ID / time-range extraction
- **LLM** (optional): any OpenAI-compatible chat-completions endpoint (OpenAI, Ollama, vLLM, LM Studio, OpenRouter, Groq, …)

The LLM backend runs **LLM-first with automatic fallback to regex**. Per-tool `parser` argument lets clients force a specific backend.

## Configuration

Configure in `homer-core.json` under `mcp`:

```json
{
  "mcp": {
    "enable": true,
    "mode": "hybrid",
    "homer_base_url": "http://127.0.0.1:8080",
    "homer_token": "replace-with-jwt",
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

When `llm.enable=false` (default) the module behaves exactly as the regex-only build — zero external dependencies.

### Example: OpenAI

```json
"llm": {
  "enable": true,
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model": "gpt-4o-mini"
}
```

### Example: local Ollama

Ollama exposes an OpenAI-compatible API on `:11434/v1` and does **not** require an API key.

```json
"llm": {
  "enable": true,
  "base_url": "http://localhost:11434/v1",
  "api_key": "",
  "model": "llama3.1"
}
```

The same config shape works for vLLM (`http://host:8000/v1`), LM Studio (`http://localhost:1234/v1`), OpenRouter (`https://openrouter.ai/api/v1`) and any other OpenAI-compatible server.

## Parser strategy

Each MCP tool accepts an optional `parser` argument:

| Value | Behavior |
|-------|----------|
| `auto` (default) | LLM-first if enabled, automatic regex fallback on any LLM error |
| `llm` | Force LLM, return tool error if LLM is disabled or fails |
| `regex` | Force the deterministic regex parser, never call LLM |

The tool response always includes parser diagnostics under `meta`:

```json
{
  "meta": {
    "parser_used": "llm",
    "llm_model": "gpt-4o-mini",
    "llm_latency_ms": 412
  }
}
```

When the LLM call fails and we fall back to regex, `meta.parser_used = "regex_fallback"` and `meta.llm_error` carries a short reason string. Operators also see a `MCP LLM parse failed, falling back to regex` warning in the homer logs.

## Running

- As a standalone command (typical for MCP clients):
  - `./homer mcp --config-path /path/to/homer-core.json`
- As part of the modular server:
  - set `mcp.enable=true` and start `./homer` as usual

## Example Cursor MCP client config

```json
{
  "mcpServers": {
    "homer-core": {
      "command": "/home/shurik/Projects/homer-core/src/homer",
      "args": [
        "mcp",
        "--config-path",
        "/etc/homer-core/homer-core.json"
      ]
    }
  }
}
```
