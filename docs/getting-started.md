# Getting Started

MIE (Memory Intelligence Engine) is a local-first personal memory graph for AI agents. It gives your AI assistant persistent memory across conversations by storing facts, decisions, entities, events, and their relationships in a local graph database.

## Prerequisites

- **macOS** (Apple Silicon or Intel) or **Linux** (x86_64 or ARM64)
- An MCP-compatible AI client ([Claude Code](https://docs.anthropic.com/en/docs/claude-code), [Cursor](https://cursor.com), etc.)
- **Optional:** [Ollama](https://ollama.com) for semantic search (recommended)

## Installation

### Homebrew (recommended)

```bash
brew install kraklabs/tap/mie
```

### Build from source

Requires Go 1.24+ and a C compiler (for CozoDB bindings).

```bash
git clone https://github.com/kraklabs/mie.git
cd mie
make deps    # Downloads CozoDB static library
make build   # Builds bin/mie
make install # Installs to ~/go/bin/mie
```

To install to a custom directory:

```bash
make install INSTALL_DIR=/usr/local/bin
```

## First run

### 1. Initialize configuration

Run `mie init` in your project root (or home directory for a global instance):

```bash
mie init
```

This creates `.mie/config.yaml` with sensible defaults:

- **Storage engine:** RocksDB (persistent, fast)
- **Embeddings:** Ollama with `nomic-embed-text` (768 dimensions)
- **Data directory:** `~/.mie/data/default/`

### 2. Configure your MCP client

Add MIE as an MCP server in your AI client's configuration.

#### Claude Code

Add to your project's `.mcp.json` or `~/.claude/mcp.json`:

```json
{
  "mcpServers": {
    "mie": {
      "command": "mie",
      "args": ["--mcp"]
    }
  }
}
```

#### Cursor

Add to `.cursor/mcp.json` in your project or `~/.cursor/mcp.json` globally:

```json
{
  "mcpServers": {
    "mie": {
      "command": "mie",
      "args": ["--mcp"]
    }
  }
}
```

#### Custom config path

If your `.mie/config.yaml` is not in the current directory tree, specify it explicitly:

```json
{
  "mcpServers": {
    "mie": {
      "command": "mie",
      "args": ["--mcp", "-c", "/path/to/.mie/config.yaml"]
    }
  }
}
```

### 3. Verify the setup

Check that MIE is running correctly:

```bash
mie status
```

Expected output:

```
MIE Memory Status

Graph Statistics:
  Facts:       0 (0 valid, 0 invalidated)
  Decisions:   0 (0 active)
  Entities:    0
  Events:      0
  Topics:      0
  Edges:       0 total

Configuration:
  Storage:     rocksdb (~/.mie/data/default)
  Embeddings:  enabled (nomic-embed-text, 768d)
  Schema:      v1
```

## Optional: Ollama setup for semantic search

Semantic search lets MIE find related memories using natural language similarity. It requires an embedding model.

### Install Ollama

```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh
```

### Pull the embedding model

```bash
ollama pull nomic-embed-text
```

### Start Ollama

```bash
ollama serve
```

MIE connects to Ollama at `http://localhost:11434` by default. If Ollama is running elsewhere, set the `OLLAMA_HOST` environment variable or update your config.

### Disable embeddings

If you don't want semantic search, set `embedding.enabled: false` in your config:

```yaml
embedding:
  enabled: false
```

MIE will still work -- `mie_query` with `mode: "exact"` provides substring search, and `mie_list` provides full listing with filters.

## What's next

- [Configuration Reference](configuration.md) -- All config options and environment variables
- [MCP Tools Reference](mcp-tools.md) -- Complete API for all 8 MCP tools
- [Architecture](architecture.md) -- System design and data model
- [CLI Reference](cli-reference.md) -- All CLI commands and flags
