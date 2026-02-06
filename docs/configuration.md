# Configuration Reference

MIE is configured through a YAML file at `.mie/config.yaml`. The file is created by `mie init` and can be customized afterward.

## Config file format

```yaml
version: "1"

storage:
  engine: rocksdb
  path: ""

embedding:
  enabled: true
  provider: ollama
  base_url: http://localhost:11434
  model: nomic-embed-text
  dimensions: 768
  workers: 4

llm:
  enabled: false
  base_url: http://localhost:11434
  model: llama3
  max_tokens: 2000
```

## All fields

### Top-level

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | string | `"1"` | Config schema version. Must be `"1"`. |

### `storage`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `engine` | string | `"rocksdb"` | Storage backend. One of: `rocksdb`, `sqlite`, `mem`. |
| `path` | string | `""` | Database path. Empty string resolves to `~/.mie/data/default/`. |

**Storage path resolution:**
- If `path` is set, that exact path is used.
- If `path` is empty:
  - For `rocksdb` and `mem`: uses `~/.mie/data/default/` (directory).
  - For `sqlite`: uses `~/.mie/data/default/index.db` (file).

### `embedding`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable vector embeddings for semantic search. |
| `provider` | string | `"ollama"` | Embedding provider. One of: `ollama`, `openai`, `nomic`. |
| `base_url` | string | `"http://localhost:11434"` | Provider API endpoint. |
| `model` | string | `"nomic-embed-text"` | Embedding model name. |
| `dimensions` | int | `768` | Embedding vector dimensions. Must match the model (768 for nomic, 1536 for OpenAI). |
| `api_key` | string | `""` | API key for OpenAI or Nomic providers. |
| `workers` | int | `4` | Number of concurrent embedding workers. |

### `llm`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable LLM for narrative generation. |
| `base_url` | string | `"http://localhost:11434"` | LLM API endpoint. |
| `model` | string | `"llama3"` | LLM model name. |
| `max_tokens` | int | `2000` | Maximum tokens for LLM responses. |
| `api_key` | string | `""` | API key for the LLM provider. |

## Environment variables

Environment variables override values in `config.yaml`. This is useful for CI, Docker, or per-session overrides.

| Variable | Overrides | Description |
|----------|-----------|-------------|
| `MIE_CONFIG_PATH` | Config discovery | Absolute path to `config.yaml`. Skips directory search. |
| `MIE_STORAGE_ENGINE` | `storage.engine` | Storage engine: `rocksdb`, `sqlite`, or `mem`. |
| `MIE_STORAGE_PATH` | `storage.path` | Database file/directory path. |
| `MIE_EMBEDDING_ENABLED` | `embedding.enabled` | `true` or `false`. |
| `MIE_EMBEDDING_PROVIDER` | `embedding.provider` | `ollama`, `openai`, or `nomic`. |
| `OLLAMA_HOST` | `embedding.base_url` | Ollama server URL. |
| `OLLAMA_EMBED_MODEL` | `embedding.model` | Ollama embedding model name. |
| `OPENAI_API_KEY` | `embedding.api_key` | Sets API key and switches provider to `openai`. |
| `NOMIC_API_KEY` | `embedding.api_key` | Sets API key and switches provider to `nomic`. |
| `MIE_LLM_URL` | `llm.base_url` | LLM endpoint. Also enables LLM if set. |
| `MIE_LLM_MODEL` | `llm.model` | LLM model name. |
| `MIE_LLM_API_KEY` | `llm.api_key` | LLM API key. |

**Note:** Setting `OPENAI_API_KEY` or `NOMIC_API_KEY` automatically switches the embedding provider from `ollama` to the respective provider.

## Config file resolution

When MIE starts (either as MCP server or CLI), it searches for `.mie/config.yaml`:

1. If `-c` / `--config` flag is provided, use that path.
2. If `MIE_CONFIG_PATH` environment variable is set, use that path.
3. Otherwise, search the current directory for `.mie/config.yaml`.
4. If not found, walk up parent directories until one is found or the filesystem root is reached.
5. If no config file is found, `mie init` must be run first.

When running as an MCP server (`mie --mcp`), if no config file is found, MIE falls back to default configuration with environment variable overrides applied. This allows zero-config startup for basic use cases.

## Example configurations

### Minimal (defaults)

```yaml
version: "1"
storage:
  engine: rocksdb
  path: ""
embedding:
  enabled: true
  provider: ollama
  base_url: http://localhost:11434
  model: nomic-embed-text
  dimensions: 768
  workers: 4
```

### OpenAI embeddings

```yaml
version: "1"
storage:
  engine: rocksdb
  path: ""
embedding:
  enabled: true
  provider: openai
  base_url: https://api.openai.com
  model: text-embedding-3-small
  dimensions: 1536
  api_key: sk-...
  workers: 4
```

### No embeddings (exact search only)

```yaml
version: "1"
storage:
  engine: rocksdb
  path: ""
embedding:
  enabled: false
```

### In-memory (testing)

```yaml
version: "1"
storage:
  engine: mem
  path: ""
embedding:
  enabled: false
```

### Custom storage path

```yaml
version: "1"
storage:
  engine: rocksdb
  path: /data/mie/production
embedding:
  enabled: true
  provider: ollama
  base_url: http://localhost:11434
  model: nomic-embed-text
  dimensions: 768
  workers: 4
```
