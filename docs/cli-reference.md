# CLI Reference

MIE provides a command-line interface for managing the memory graph. The primary mode of operation is as an MCP server (`mie --mcp`), but CLI commands are available for administration, debugging, and scripting.

## Usage

```
mie <command> [options]
mie --mcp [options]
```

## Global flags

These flags apply to all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | | Output in JSON format. Implies `--quiet`. |
| `--verbose` | `-v` | Increase verbosity. Use `-v` for info, `-vv` for debug. |
| `--quiet` | `-q` | Suppress non-essential output. Cannot be used with `--verbose`. |
| `--mcp` | | Start as MCP server (JSON-RPC over stdio). |
| `--config` | `-c` | Path to `.mie/config.yaml`. |
| `--version` | `-V` | Show version and exit. |

## Commands

### mie init

Create a new `.mie/config.yaml` configuration file in the current directory.

```
mie init [--force]
```

| Flag | Description |
|------|-------------|
| `--force` | Overwrite existing configuration. |

**Examples:**

```bash
# Create config with defaults
mie init

# Overwrite existing config
mie init --force
```

**Output:**

```
Created .mie/config.yaml

Next steps:
  1. Edit .mie/config.yaml to customize settings
  2. Run 'mie --mcp' to start the MCP server
```

---

### mie status

Display the current status of the MIE memory graph including node counts, configuration, and health information.

```
mie status [--json]
```

**Examples:**

```bash
# Human-readable status
mie status

# JSON output
mie status --json
```

**Human-readable output:**

```
MIE Memory Status

Graph Statistics:
  Facts:       12 (10 valid, 2 invalidated)
  Decisions:   3 (3 active)
  Entities:    8
  Events:      2
  Topics:      5
  Edges:       15 total

Configuration:
  Storage:     rocksdb (~/.mie/data/default)
  Embeddings:  enabled (nomic-embed-text, 768d)
  Schema:      v1
```

**JSON output:**

```json
{
  "storage_engine": "rocksdb",
  "data_dir": "/Users/you/.mie/data/default",
  "connected": true,
  "facts": 12,
  "valid_facts": 10,
  "invalidated_facts": 2,
  "decisions": 3,
  "active_decisions": 3,
  "entities": 8,
  "events": 2,
  "topics": 5,
  "edges": 15,
  "embeddings_enabled": true,
  "timestamp": "2026-02-05T12:00:00Z"
}
```

---

### mie reset

Delete all memory data. This is a destructive operation.

```
mie reset --yes
```

| Flag | Description |
|------|-------------|
| `--yes` | **Required.** Confirm the reset. Without this flag, the command refuses to run. |

**What gets deleted:**
- All stored facts, decisions, entities, events, topics
- All relationships
- The entire database directory

**What is preserved:**
- `.mie/config.yaml` configuration file

**Example:**

```bash
mie reset --yes
```

**Output:**

```
Deleting memory data at /Users/you/.mie/data/default...
Reset complete. All memory data has been deleted.

Next steps:
  mie --mcp    Start MCP server (database will be recreated)
```

---

### mie export

Export the complete memory graph for backup or migration.

```
mie export [--format json|datalog] [--output FILE] [--include-embeddings]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--format` | | `json` | Export format: `json` or `datalog`. |
| `--output` | `-o` | stdout | Write to file instead of stdout. |
| `--include-embeddings` | | `false` | Include embedding vectors (can be very large). |

**Examples:**

```bash
# Export JSON to stdout
mie export

# Export JSON to file
mie export --output backup.json

# Export as Datalog
mie export --format datalog

# Export with embeddings
mie export --include-embeddings --output full-backup.json
```

---

### mie query

Execute a raw CozoScript query against the MIE database. This is a debugging tool for inspecting the underlying data.

```
mie query "<cozoscript>" [--json]
```

The query argument is a [CozoScript](https://docs.cozodb.org/) expression.

**Examples:**

```bash
# List entity names
mie query "?[name] := *mie_entity { name } :limit 10"

# Count facts
mie query "?[count(id)] := *mie_fact { id }"

# List valid facts
mie query "?[id, content] := *mie_fact { id, content, valid }, valid = true :limit 5"

# JSON output
mie query "?[name, kind] := *mie_entity { name, kind }" --json
```

**Human-readable output:**

```
Found 3 results

name    kind
------------------------------------------------------------
Bun     technology
Kraklabs        company
Alice   person
```

---

### mie --mcp

Start MIE as an MCP server. This is the primary mode of operation.

```
mie --mcp [-c CONFIG_PATH]
```

The server reads JSON-RPC requests from stdin and writes responses to stdout. Diagnostic messages go to stderr.

**Example:**

```bash
mie --mcp
```

**Startup output (stderr):**

```
MIE MCP Server v0.1.0 starting...
  Storage: rocksdb (~/.mie/data/default)
  Embeddings: ollama (nomic-embed-text, 768d)
```

Typically, you don't run this command directly. Instead, configure your MCP client to launch it. See [Getting Started](getting-started.md).

## Exit codes

| Code | Constant | Description |
|------|----------|-------------|
| `0` | `ExitSuccess` | Command completed successfully. |
| `1` | `ExitGeneral` | General error (I/O, unexpected failure). |
| `2` | `ExitConfig` | Configuration error (missing, invalid, or unsupported config). |
| `3` | `ExitDatabase` | Database error (cannot open, create, or access the database). |
| `4` | `ExitQuery` | Query error (invalid CozoScript or missing query argument). |

## Version info

```bash
mie -V
```

Output:

```
mie version v0.2.0
commit: a1b2c3d
built: 2026-02-05T12:00:00Z
```