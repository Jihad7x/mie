# MCP Tools Reference

MIE exposes 10 tools through the [Model Context Protocol](https://modelcontextprotocol.io/). AI agents call these tools to read, write, and search the memory graph.

All tools are invoked via `tools/call` JSON-RPC requests. Each tool returns a text response in `content[0].text`.

---

## mie_analyze

Analyze a conversation fragment for potential memory storage. Returns related existing memory and an evaluation guide for the agent to decide what to persist.

**When to use:** Call at the end of meaningful conversations or when noticing something worth remembering. This is typically the first step before calling `mie_store`.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `content` | string | Yes | -- | Conversation fragment or information to analyze. |

### Example request

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "mie_analyze",
    "arguments": {
      "content": "The user prefers TypeScript over JavaScript for new projects and uses Bun as their runtime."
    }
  }
}
```

### Example response

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "## Existing Memory Context\n\n_No related memory found. This appears to be new information._\n\n---\n\n## Evaluation Guide\n\nGiven the existing memory context above, evaluate if the analyzed content contains:\n\n1. **NEW FACT**: A personal truth not already captured...\n..."
      }
    ]
  }
}
```

### Behavior

1. If embeddings are enabled, performs a semantic search across all node types (facts, decisions, entities, events) to find related existing memory.
2. Checks for potential conflicts with existing facts.
3. Returns a structured evaluation guide with:
   - Related existing memory grouped by type
   - Potential conflicts
   - Instructions for what to store and how

---

## mie_store

Store a new memory node (fact, decision, entity, event, or topic) in the memory graph. Optionally creates relationships to other nodes and invalidates old facts.

**When to use:** After `mie_analyze` confirms something is worth persisting.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `type` | string | Yes | -- | Node type: `fact`, `decision`, `entity`, `event`, or `topic`. |
| `content` | string | Conditional | -- | Fact text content. **Required for `type=fact`.** |
| `category` | string | No | `"general"` | Fact category: `personal`, `professional`, `preference`, `technical`, `relationship`, `general`. |
| `confidence` | number | No | `0.8` | Confidence level (0.0-1.0). |
| `title` | string | Conditional | -- | Title. **Required for `type=decision` and `type=event`.** |
| `rationale` | string | Conditional | -- | Decision rationale. **Required for `type=decision`.** |
| `alternatives` | string | No | `"[]"` | JSON array of alternatives considered (for decisions). |
| `context` | string | No | `""` | Decision context. |
| `name` | string | Conditional | -- | Name. **Required for `type=entity` and `type=topic`.** |
| `kind` | string | Conditional | -- | Entity kind. **Required for `type=entity`.** One of: `person`, `company`, `project`, `product`, `technology`, `place`, `other`. |
| `description` | string | No | `""` | Description for entity, event, or topic. |
| `event_date` | string | Conditional | -- | ISO date (e.g., `2026-02-05`). **Required for `type=event`.** |
| `source_agent` | string | No | `"unknown"` | Agent identifier (e.g., `claude`, `cursor`). |
| `source_conversation` | string | No | `""` | Conversation reference. |
| `relationships` | array | No | -- | Relationships to create after storing. See below. |
| `invalidates` | string | No | -- | Fact ID to invalidate (must start with `fact:`). |

### Relationship objects

Each item in the `relationships` array:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `edge` | string | Yes | Edge type: `fact_entity`, `fact_topic`, `decision_topic`, `decision_entity`, `event_decision`, `entity_topic`. |
| `target_id` | string | Yes | Target node ID. |
| `role` | string | No | Role description (only for `decision_entity` edges). |

### Example: Store a fact

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "mie_store",
    "arguments": {
      "type": "fact",
      "content": "User prefers TypeScript over JavaScript for new projects",
      "category": "preference",
      "confidence": 0.9,
      "source_agent": "claude"
    }
  }
}
```

### Example: Store an entity with relationships

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "mie_store",
    "arguments": {
      "type": "entity",
      "name": "Bun",
      "kind": "technology",
      "description": "JavaScript runtime and toolkit",
      "source_agent": "claude",
      "relationships": [
        {
          "edge": "entity_topic",
          "target_id": "topic:abc123"
        }
      ]
    }
  }
}
```

### Example: Store a decision

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "mie_store",
    "arguments": {
      "type": "decision",
      "title": "Use RocksDB as default storage engine",
      "rationale": "Better performance for read-heavy workloads and persistent storage",
      "alternatives": "[\"SQLite\", \"In-memory only\"]",
      "context": "Choosing storage backend for MIE",
      "source_agent": "claude"
    }
  }
}
```

### Example response

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Stored fact [fact:a1b2c3d4]\nContent: \"User prefers TypeScript over JavaScript for new projects\"\nCategory: preference | Confidence: 0.9 | Source: claude"
      }
    ]
  }
}
```

---

## mie_query

Search the memory graph. Supports three modes: semantic (natural language similarity), exact (substring match), and graph (traverse relationships from a node).

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | Yes | -- | Search query. Natural language for semantic, substring for exact, node ID for graph. |
| `mode` | string | No | `"semantic"` | Search mode: `semantic`, `exact`, or `graph`. |
| `node_types` | array | No | `["fact", "decision", "entity", "event"]` | Node types to search. |
| `limit` | number | No | `10` | Maximum results (1-50). |
| `category` | string | No | -- | Filter facts by category. |
| `kind` | string | No | -- | Filter entities by kind. |
| `valid_only` | boolean | No | `true` | Only return valid (non-invalidated) facts. |
| `node_id` | string | Conditional | -- | Node ID for graph traversal. **Required for `mode=graph`.** |
| `traversal` | string | Conditional | -- | Traversal type. **Required for `mode=graph`.** |

### Traversal types (graph mode)

| Traversal | Description |
|-----------|-------------|
| `related_entities` | Find entities connected to a fact or decision. |
| `related_facts` | Find facts connected to an entity. |
| `facts_about_entity` | Find facts linked to an entity (alias for `related_facts`). |
| `invalidation_chain` | Follow the chain of fact invalidations. |
| `decision_entities` | Find entities involved in a decision (includes roles). |
| `entity_decisions` | Find decisions involving an entity. |

### Example: Semantic search

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "mie_query",
    "arguments": {
      "query": "programming language preferences",
      "mode": "semantic",
      "limit": 5
    }
  }
}
```

### Example: Exact search

```json
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "mie_query",
    "arguments": {
      "query": "TypeScript",
      "mode": "exact",
      "node_types": ["fact", "entity"]
    }
  }
}
```

### Example: Graph traversal

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "mie_query",
    "arguments": {
      "query": "traverse",
      "mode": "graph",
      "node_id": "ent:abc123",
      "traversal": "facts_about_entity"
    }
  }
}
```

### Example response (semantic)

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "## Memory Search Results for: \"programming language preferences\"\n\n### Facts (2 results)\n1. \ud83d\udfe2 87% [fact:a1b2c3d4] \"User prefers TypeScript over JavaScript for new projects\"\n2. \ud83d\udfe1 62% [fact:e5f6g7h8] \"User has experience with Go and Rust\"\n\n"
      }
    ]
  }
}
```

---

## mie_get

Retrieve a single memory node by its ID. Returns full details including all fields.

**When to use:** After finding a node ID via `mie_query` or `mie_list`, use this to inspect the complete node.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `node_id` | string | Yes | -- | The node ID to retrieve (e.g., `fact:abc123`, `ent:def456`, `dec:ghi789`). |

### Example request

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "mie_get",
    "arguments": {
      "node_id": "fact:a1b2c3d4"
    }
  }
}
```

### Behavior

1. Detects the node type from the ID prefix (`fact:`, `ent:`, `dec:`, `evt:`, `topic:`).
2. Queries the corresponding table for full node details.
3. Returns all fields for the node type in a formatted markdown block.

---

## mie_list

List memory nodes with filtering, pagination, and sorting. Returns a formatted table.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `node_type` | string | Yes | -- | Type to list: `fact`, `decision`, `entity`, `event`, `topic`. |
| `category` | string | No | -- | Filter facts by category. |
| `kind` | string | No | -- | Filter entities by kind. |
| `status` | string | No | -- | Filter decisions by status: `active`, `superseded`, `reversed`. |
| `topic` | string | No | -- | Filter by topic name. |
| `valid_only` | boolean | No | `true` | Only return valid (non-invalidated) facts. |
| `limit` | number | No | `20` | Results per page (1-100). |
| `offset` | number | No | `0` | Skip this many results (for pagination). |
| `sort_by` | string | No | `"created_at"` | Sort field: `created_at`, `updated_at`, `name`. |
| `sort_order` | string | No | `"desc"` | Sort direction: `asc` or `desc`. |

### Example: List all entities

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "tools/call",
  "params": {
    "name": "mie_list",
    "arguments": {
      "node_type": "entity",
      "limit": 10
    }
  }
}
```

### Example: List facts by category

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "tools/call",
  "params": {
    "name": "mie_list",
    "arguments": {
      "node_type": "fact",
      "category": "technical",
      "sort_by": "created_at",
      "sort_order": "desc",
      "limit": 20
    }
  }
}
```

### Example response

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "## Entities (3 total, showing 1-3)\n\n| # | ID | Name | Kind | Description |\n|---|-----|------|------|------------|\n| 1 | ent:abc123 | Bun | technology | JavaScript runtime and toolkit |\n| 2 | ent:def456 | Kraklabs | company | Software company |\n| 3 | ent:ghi789 | Alice | person | Team lead |\n"
      }
    ]
  }
}
```

---

## mie_update

Update or invalidate existing memory nodes.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `node_id` | string | Yes | -- | ID of the node to modify. |
| `action` | string | Yes | -- | Action: `invalidate`, `update_description`, or `update_status`. |
| `reason` | string | Conditional | -- | Why the change is being made. **Required for `invalidate`.** |
| `replacement_id` | string | No | -- | ID of the new fact that replaces the invalidated one (must start with `fact:`). |
| `new_value` | string | Conditional | -- | New description or status value. **Required for `update_description` and `update_status`.** |

### Actions

| Action | Applies to | Description |
|--------|-----------|-------------|
| `invalidate` | Facts only (prefix `fact:`) | Marks a fact as invalid. Creates an invalidation edge if `replacement_id` is provided. |
| `update_description` | Entities, events, topics | Updates the description field. |
| `update_status` | Decisions only (prefix `dec:`) | Changes status to `active`, `superseded`, or `reversed`. |

### Example: Invalidate a fact

```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "method": "tools/call",
  "params": {
    "name": "mie_update",
    "arguments": {
      "node_id": "fact:a1b2c3d4",
      "action": "invalidate",
      "reason": "User changed their preference",
      "replacement_id": "fact:i9j0k1l2"
    }
  }
}
```

### Example: Update entity description

```json
{
  "jsonrpc": "2.0",
  "id": 11,
  "method": "tools/call",
  "params": {
    "name": "mie_update",
    "arguments": {
      "node_id": "ent:abc123",
      "action": "update_description",
      "new_value": "Fast JavaScript/TypeScript runtime, bundler, and package manager"
    }
  }
}
```

### Example: Update decision status

```json
{
  "jsonrpc": "2.0",
  "id": 12,
  "method": "tools/call",
  "params": {
    "name": "mie_update",
    "arguments": {
      "node_id": "dec:xyz789",
      "action": "update_status",
      "new_value": "superseded"
    }
  }
}
```

### Example response

```json
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Invalidated [fact:a1b2c3d4]\nReason: User changed their preference\nReplaced by: [fact:i9j0k1l2]"
      }
    ]
  }
}
```

---

## mie_conflicts

Detect potentially contradicting facts in the memory graph. Returns pairs of facts that are semantically similar but may contain conflicting information.

**Requires:** Embeddings must be enabled.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `category` | string | No | -- | Limit scan to a specific fact category. |
| `threshold` | number | No | `0.85` | Similarity threshold (0.0-1.0). Higher = stricter matching, fewer results. |
| `limit` | number | No | `10` | Maximum conflict pairs to return (1-50). |

### Example request

```json
{
  "jsonrpc": "2.0",
  "id": 13,
  "method": "tools/call",
  "params": {
    "name": "mie_conflicts",
    "arguments": {
      "category": "preference",
      "threshold": 0.8,
      "limit": 5
    }
  }
}
```

### Example response

```json
{
  "jsonrpc": "2.0",
  "id": 13,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "## Potential Conflicts Found (1)\n\n### Conflict 1 (similarity: 92%)\n- [fact:a1b2c3d4] \"User prefers TypeScript\" (preference, confidence: 0.9)\n- [fact:m3n4o5p6] \"User prefers JavaScript for small scripts\" (preference, confidence: 0.7)\n  Recommendation: The newer fact [fact:m3n4o5p6] likely supersedes the older one [fact:a1b2c3d4].\n\nTo resolve: call mie_update with action=\"invalidate\" on the outdated fact.\n"
      }
    ]
  }
}
```

---

## mie_export

Export the complete memory graph for backup or migration.

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `format` | string | No | `"json"` | Export format: `json` or `datalog`. |
| `include_embeddings` | boolean | No | `false` | Include embedding vectors (can be very large). |
| `node_types` | array | No | `["fact", "decision", "entity", "event", "topic"]` | Types to export. |

### Example request

```json
{
  "jsonrpc": "2.0",
  "id": 14,
  "method": "tools/call",
  "params": {
    "name": "mie_export",
    "arguments": {
      "format": "json",
      "node_types": ["fact", "entity"]
    }
  }
}
```

### Example response (abbreviated)

```json
{
  "jsonrpc": "2.0",
  "id": 14,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\n  \"version\": \"1\",\n  \"exported_at\": \"2026-02-05T12:00:00Z\",\n  \"stats\": { \"facts\": 2, \"entities\": 1 },\n  \"facts\": [...],\n  \"entities\": [...]\n}"
      }
    ]
  }
}
```

**Note:** Output is truncated at 100,000 characters. For large graphs, use the CLI `mie export` command to write directly to a file.

---

## mie_status

Display memory graph health and statistics. Shows counts of all node types, configuration details, and health checks.

### Parameters

None.

### Example request

```json
{
  "jsonrpc": "2.0",
  "id": 15,
  "method": "tools/call",
  "params": {
    "name": "mie_status",
    "arguments": {}
  }
}
```

### Example response

```json
{
  "jsonrpc": "2.0",
  "id": 15,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "## MIE Memory Status\n\n### Graph Statistics\n- Facts: 12 (10 valid, 2 invalidated)\n- Decisions: 3 (3 active, 0 other)\n- Entities: 8\n- Events: 2\n- Topics: 5\n- Relationships: 15 edges total\n\n### Configuration\n- Storage: rocksdb (~/.mie/data/default)\n- Embeddings: enabled\n- Schema version: 1\n\n### Health\n- Database accessible (30 total nodes)\n- Embeddings enabled\n"
      }
    ]
  }
}
```

### Common use case

Call `mie_status` as a first step when starting a new session to verify MIE is operational and see how much memory is stored.