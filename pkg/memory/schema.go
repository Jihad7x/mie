// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/kraklabs/mie/pkg/storage"
)

// SchemaStatements returns the :create statements for the MIE memory schema.
// The dimension parameter controls embedding vector size (e.g. 768 for nomic, 1536 for OpenAI).
func SchemaStatements(dim int) []string {
	return []string{
		// Core node tables
		`:create mie_fact {
    id: String =>
    content: String,
    category: String,
    confidence: Float,
    source_agent: String,
    source_conversation: String,
    valid: Bool,
    created_at: Int,
    updated_at: Int
}`,

		fmt.Sprintf(`:create mie_fact_embedding {
    fact_id: String =>
    embedding: <F32; %d>
}`, dim),

		`:create mie_decision {
    id: String =>
    title: String,
    rationale: String,
    alternatives: String,
    context: String,
    source_agent: String,
    source_conversation: String,
    status: String,
    created_at: Int,
    updated_at: Int
}`,

		fmt.Sprintf(`:create mie_decision_embedding {
    decision_id: String =>
    embedding: <F32; %d>
}`, dim),

		`:create mie_entity {
    id: String =>
    name: String,
    kind: String,
    description: String,
    source_agent: String,
    created_at: Int,
    updated_at: Int
}`,

		fmt.Sprintf(`:create mie_entity_embedding {
    entity_id: String =>
    embedding: <F32; %d>
}`, dim),

		`:create mie_event {
    id: String =>
    title: String,
    description: String,
    event_date: String,
    source_agent: String,
    source_conversation: String,
    created_at: Int,
    updated_at: Int
}`,

		fmt.Sprintf(`:create mie_event_embedding {
    event_id: String =>
    embedding: <F32; %d>
}`, dim),

		`:create mie_topic {
    id: String =>
    name: String,
    description: String,
    created_at: Int,
    updated_at: Int
}`,

		// Edge tables
		`:create mie_invalidates {
    new_fact_id: String,
    old_fact_id: String =>
    reason: String
}`,

		`:create mie_decision_topic {
    decision_id: String,
    topic_id: String =>
}`,

		`:create mie_decision_entity {
    decision_id: String,
    entity_id: String =>
    role: String
}`,

		`:create mie_event_decision {
    event_id: String,
    decision_id: String =>
}`,

		`:create mie_fact_entity {
    fact_id: String,
    entity_id: String =>
}`,

		`:create mie_fact_topic {
    fact_id: String,
    topic_id: String =>
}`,

		`:create mie_entity_topic {
    entity_id: String,
    topic_id: String =>
}`,

		// Metadata table
		`:create mie_meta {
    key: String =>
    value: String
}`,
	}
}

// HNSWIndexStatements returns the HNSW index creation statements.
func HNSWIndexStatements(dim int) []string {
	return []string{
		fmt.Sprintf(`::hnsw create mie_fact_embedding:fact_embedding_idx {
    dim: %d,
    m: 16,
    ef_construction: 200,
    distance: Cosine,
    fields: [embedding],
    extend_candidates: true,
    keep_pruned_connections: true
}`, dim),

		fmt.Sprintf(`::hnsw create mie_decision_embedding:decision_embedding_idx {
    dim: %d,
    m: 16,
    ef_construction: 200,
    distance: Cosine,
    fields: [embedding],
    extend_candidates: true,
    keep_pruned_connections: true
}`, dim),

		fmt.Sprintf(`::hnsw create mie_entity_embedding:entity_embedding_idx {
    dim: %d,
    m: 16,
    ef_construction: 200,
    distance: Cosine,
    fields: [embedding],
    extend_candidates: true,
    keep_pruned_connections: true
}`, dim),

		fmt.Sprintf(`::hnsw create mie_event_embedding:event_embedding_idx {
    dim: %d,
    m: 16,
    ef_construction: 200,
    distance: Cosine,
    fields: [embedding],
    extend_candidates: true,
    keep_pruned_connections: true
}`, dim),
	}
}

// EnsureSchema creates all MIE schema tables, ignoring "already exists" errors.
// Each :create statement is executed as a separate Run() call as required by CozoDB.
func EnsureSchema(backend storage.Backend, dim int) error {
	ctx := context.Background()

	for _, stmt := range SchemaStatements(dim) {
		if err := backend.Execute(ctx, stmt); err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "already exists") ||
				strings.Contains(errStr, "conflicts with an existing one") {
				continue
			}
			return fmt.Errorf("create schema: %w", err)
		}
	}

	// Set schema version
	versionStmt := `?[key, value] <- [['schema_version', '1']] :put mie_meta { key => value }`
	if err := backend.Execute(ctx, versionStmt); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}

// EnsureHNSWIndexes creates HNSW indexes for semantic search.
// Ignores "already exists" errors so it can be called idempotently.
func EnsureHNSWIndexes(backend storage.Backend, dim int) error {
	ctx := context.Background()

	for _, stmt := range HNSWIndexStatements(dim) {
		if err := backend.Execute(ctx, stmt); err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "already exists") ||
				strings.Contains(errStr, "conflicts with an existing one") ||
				strings.Contains(errStr, "index already exists") {
				continue
			}
			return fmt.Errorf("create hnsw index: %w", err)
		}
	}

	return nil
}

// hnswIndexInfo maps embedding table names to their HNSW index name and
// the ID column + parent node table used for orphan detection.
var hnswIndexInfo = []struct {
	embTable  string
	indexName string
	idCol     string
	nodeTable string
}{
	{"mie_fact_embedding", "fact_embedding_idx", "fact_id", "mie_fact"},
	{"mie_decision_embedding", "decision_embedding_idx", "decision_id", "mie_decision"},
	{"mie_entity_embedding", "entity_embedding_idx", "entity_id", "mie_entity"},
	{"mie_event_embedding", "event_embedding_idx", "event_id", "mie_event"},
}

// RepairHNSWIndexes drops and recreates HNSW indexes after cleaning up
// orphaned embedding rows (embeddings whose parent node no longer exists).
// This fixes HNSW corruption caused by dangling index entries.
func RepairHNSWIndexes(backend storage.Backend, dim int, logger interface{ Info(string, ...any) }) error {
	ctx := context.Background()

	for _, info := range hnswIndexInfo {
		// 1. Drop the HNSW index.
		dropStmt := fmt.Sprintf(`::hnsw drop %s:%s`, info.embTable, info.indexName)
		if err := backend.Execute(ctx, dropStmt); err != nil {
			errStr := err.Error()
			if !strings.Contains(errStr, "not found") && !strings.Contains(errStr, "does not exist") {
				return fmt.Errorf("drop hnsw index %s: %w", info.indexName, err)
			}
		}

		// 2. Remove orphaned embeddings (parent node deleted but embedding remains).
		cleanupStmt := fmt.Sprintf(
			`?[%s] := *%s { %s }, not *%s { id: %s } :rm %s { %s }`,
			info.idCol, info.embTable, info.idCol,
			info.nodeTable, info.idCol,
			info.embTable, info.idCol,
		)
		if err := backend.Execute(ctx, cleanupStmt); err != nil {
			// Non-fatal: log and continue, the index rebuild will still help.
			if logger != nil {
				logger.Info("orphan cleanup skipped", "table", info.embTable, "error", err)
			}
		}
	}

	// 3. Recreate all HNSW indexes.
	if err := EnsureHNSWIndexes(backend, dim); err != nil {
		return fmt.Errorf("recreate hnsw indexes: %w", err)
	}

	return nil
}
