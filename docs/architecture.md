# Architecture

MIE is a local-first memory graph that runs as an [MCP](https://modelcontextprotocol.io/) server, providing persistent memory to AI agents over JSON-RPC via stdio.

## High-level architecture

```
AI Client (Claude Code, Cursor, etc.)
    |
    | JSON-RPC over stdio (MCP protocol)
    |
MIE MCP Server  (mie --mcp)
    |
    +-- Tool handlers (pkg/tools/)
    |       Validate input, format output
    |
    +-- Memory client (pkg/memory/)
    |       Write, read, search, conflict detection
    |
    +-- Embedding pipeline (pkg/memory/embedding.go)
    |       Ollama / OpenAI / Nomic
    |
    +-- Storage backend (pkg/storage/)
            CozoDB (RocksDB | SQLite | in-memory)
```

MIE is a single Go binary. The MCP server reads JSON-RPC requests from stdin and writes responses to stdout. Diagnostic logs go to stderr. No network ports are opened -- all communication happens through stdio, making it safe for local use.

## Data model

The memory graph consists of **5 node types** and **7 relationship (edge) types**.

### Node types

| Node type | ID prefix | Description | Key fields |
|-----------|-----------|-------------|------------|
| **Fact** | `fact:` | A personal truth or piece of knowledge | `content`, `category`, `confidence`, `valid` |
| **Decision** | `dec:` | A choice with rationale and alternatives | `title`, `rationale`, `alternatives`, `status` |
| **Entity** | `ent:` | A person, company, project, or technology | `name`, `kind`, `description` |
| **Event** | `evt:` | A timestamped occurrence | `title`, `description`, `event_date` |
| **Topic** | `topic:` | A recurring theme for organizing nodes | `name`, `description` |

All nodes have `created_at` and `updated_at` timestamps (Unix epoch in seconds). Facts, decisions, entities, and events also track `source_agent` (which AI agent created them).

### Fact categories

Facts are classified into one of six categories:

| Category | Description |
|----------|-------------|
| `personal` | Personal information and preferences |
| `professional` | Work-related knowledge |
| `preference` | User preferences and choices |
| `technical` | Technical knowledge and patterns |
| `relationship` | Information about relationships between people/things |
| `general` | Everything else |

### Entity kinds

Entities represent named things in the world:

| Kind | Description |
|------|-------------|
| `person` | A person |
| `company` | A company or organization |
| `project` | A software project or initiative |
| `product` | A product or service |
| `technology` | A programming language, framework, or tool |
| `place` | A physical location |
| `other` | Anything else |

### Decision statuses

| Status | Description |
|--------|-------------|
| `active` | Currently in effect (default) |
| `superseded` | Replaced by a newer decision |
| `reversed` | Explicitly reversed |

### Relationship types

Edges connect nodes to form a graph:

| Edge type | Source | Target | Extra fields |
|-----------|--------|--------|--------------|
| `fact_entity` | Fact | Entity | -- |
| `fact_topic` | Fact | Topic | -- |
| `decision_entity` | Decision | Entity | `role` (optional) |
| `decision_topic` | Decision | Topic | -- |
| `event_decision` | Event | Decision | -- |
| `entity_topic` | Entity | Topic | -- |
| `invalidates` | Fact (new) | Fact (old) | `reason` |

The `invalidates` edge creates a chain of fact revisions, allowing you to track how knowledge evolved over time.

## Storage engines

MIE uses [CozoDB](https://www.cozodb.org/) as its query engine, which supports multiple storage backends:

| Engine | Persistence | Use case |
|--------|-------------|----------|
| **`rocksdb`** (default) | Persistent, on-disk | Production use. Fast writes, efficient storage. |
| **`sqlite`** | Persistent, single file | Alternative when RocksDB is unavailable. |
| **`mem`** | In-memory only | Testing. Data lost on restart. |

The storage engine is configured in `.mie/config.yaml` under `storage.engine`. Data is stored at `~/.mie/data/default/` by default, configurable via `storage.path`.

## Embedding pipeline

MIE generates vector embeddings for facts, decisions, entities, and events to enable semantic search. When a node is stored, its text content is sent to the configured embedding provider, and the resulting vector is stored in a separate embedding table alongside an HNSW index for fast approximate nearest-neighbor search.

### Supported providers

| Provider | Model | Dimensions | Setup |
|----------|-------|------------|-------|
| **Ollama** (default) | `nomic-embed-text` | 768 | Local, free, private |
| **OpenAI** | `text-embedding-3-small` | 1536 | API key required |
| **Nomic** | `nomic-embed-text` | 768 | API key required |

### Search modes

| Mode | Requires embeddings | Description |
|------|---------------------|-------------|
| `semantic` | Yes | Natural language similarity search using HNSW cosine distance |
| `exact` | No | Substring match against node content |
| `graph` | No | Traverse relationships from a specific node |

## MCP protocol details

MIE implements the [Model Context Protocol](https://modelcontextprotocol.io/) specification version `2024-11-05`.

### Transport

- **Protocol:** JSON-RPC 2.0 over stdio (one JSON object per line)
- **Input:** stdin
- **Output:** stdout
- **Logs:** stderr

### Supported methods

| Method | Description |
|--------|-------------|
| `initialize` | Handshake, returns server info and capabilities |
| `notifications/initialized` | Client acknowledgement (no response) |
| `tools/list` | Returns all 8 MCP tool definitions |
| `tools/call` | Executes a tool by name with arguments |

### Server info

```json
{
  "protocolVersion": "2024-11-05",
  "serverInfo": {
    "name": "mie",
    "version": "0.1.0"
  },
  "capabilities": {
    "tools": { "listChanged": true }
  }
}
```

### Available tools

MIE exposes 8 tools through MCP. See [MCP Tools Reference](mcp-tools.md) for full documentation.

| Tool | Description |
|------|-------------|
| `mie_analyze` | Analyze content for potential memory storage |
| `mie_store` | Store a new memory node |
| `mie_query` | Search the memory graph |
| `mie_list` | List nodes with filtering and pagination |
| `mie_update` | Update or invalidate existing nodes |
| `mie_conflicts` | Detect contradicting facts |
| `mie_export` | Export the full memory graph |
| `mie_status` | Display graph health and statistics |
