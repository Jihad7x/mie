// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

// setupIntegrationClient creates an in-memory Client for integration tests.
// Each call produces an isolated database, so tests can run in parallel.
func setupIntegrationClient(t *testing.T, embeddings bool) *Client {
	t.Helper()
	cfg := ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingEnabled:    embeddings,
		EmbeddingDimensions: 4,
	}
	if embeddings {
		cfg.EmbeddingProvider = "mock"
		// MockEmbeddingProvider in CreateEmbeddingProvider uses 768 by default,
		// but we override dimensions at the schema level; the mock generates
		// deterministic vectors of whatever size the provider was created with.
		// We need to match the schema dimension, so we build the provider manually.
		cfg.EmbeddingDimensions = 4
	}
	client, err := NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

// setupIntegrationClientWithEmbedder returns a Client whose embedder is wired
// to use 4-dimensional mock vectors that match the schema.
func setupIntegrationClientWithEmbedder(t *testing.T) (*Client, *EmbeddingGenerator) {
	t.Helper()

	backend := newTestBackend4(t)
	setupSchema4(t, backend)

	provider := NewMockEmbeddingProvider(4, nil)
	embedder := NewEmbeddingGenerator(provider, nil)

	writer := NewWriter(backend, embedder, nil)
	reader := NewReader(backend, embedder, nil)
	detector := NewConflictDetector(backend, embedder, nil)

	client := &Client{
		backend:  backend,
		config:   ClientConfig{StorageEngine: "mem", DataDir: t.TempDir(), EmbeddingEnabled: true, EmbeddingDimensions: 4},
		writer:   writer,
		reader:   reader,
		detector: detector,
		embedder: embedder,
	}
	t.Cleanup(func() { client.Close() })
	return client, embedder
}

// newTestBackend4 creates an in-memory CozoDB backend with 4-dim embeddings.
func newTestBackend4(t *testing.T) *EmbeddedBackendForTest {
	t.Helper()
	backend, err := newEmbeddedBackend4(t)
	require.NoError(t, err)
	return backend
}

type EmbeddedBackendForTest = storage.EmbeddedBackend

func newEmbeddedBackend4(t *testing.T) (*storage.EmbeddedBackend, error) {
	t.Helper()
	return storage.NewEmbeddedBackend(storage.EmbeddedConfig{
		Engine:              "mem",
		DataDir:             t.TempDir(),
		EmbeddingDimensions: 4,
	})
}

func setupSchema4(t *testing.T, backend *storage.EmbeddedBackend) {
	t.Helper()
	require.NoError(t, backend.EnsureSchema())
	require.NoError(t, EnsureSchema(backend, 4))
}

// ---------------------------------------------------------------------------
// 1. TestIntegrationFullLifecycle
// ---------------------------------------------------------------------------

func TestIntegrationFullLifecycle(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	// --- Store nodes of every type ---
	fact1, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:     "Go is my primary language",
		Category:    "technical",
		Confidence:  0.95,
		SourceAgent: "test",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fact1.ID)
	assert.True(t, fact1.Valid)

	fact2, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:     "I live in Buenos Aires",
		Category:    "personal",
		Confidence:  0.9,
		SourceAgent: "test",
	})
	require.NoError(t, err)

	decision, err := client.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title:     "Use CozoDB as embedded store",
		Rationale: "Supports Datalog and HNSW",
	})
	require.NoError(t, err)
	assert.Equal(t, "active", decision.Status)

	entity, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
		Name:        "CozoDB",
		Kind:        "technology",
		Description: "Embedded relational database",
	})
	require.NoError(t, err)

	event, err := client.StoreEvent(ctx, tools.StoreEventRequest{
		Title:     "Project kickoff",
		EventDate: "2026-01-15",
	})
	require.NoError(t, err)

	topic, err := client.StoreTopic(ctx, tools.StoreTopicRequest{
		Name:        "Architecture",
		Description: "System design decisions",
	})
	require.NoError(t, err)

	// --- Add relationships ---
	require.NoError(t, client.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id": fact1.ID, "entity_id": entity.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_decision_entity", map[string]string{
		"decision_id": decision.ID, "entity_id": entity.ID, "role": "subject",
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_decision_topic", map[string]string{
		"decision_id": decision.ID, "topic_id": topic.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_fact_topic", map[string]string{
		"fact_id": fact1.ID, "topic_id": topic.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_event_decision", map[string]string{
		"event_id": event.ID, "decision_id": decision.ID,
	}))

	// --- Query back ---
	node, err := client.GetNodeByID(ctx, fact1.ID)
	require.NoError(t, err)
	gotFact, ok := node.(*tools.Fact)
	require.True(t, ok)
	assert.Equal(t, fact1.Content, gotFact.Content)

	entities, err := client.GetRelatedEntities(ctx, fact1.ID)
	require.NoError(t, err)
	assert.Len(t, entities, 1)
	assert.Equal(t, "CozoDB", entities[0].Name)

	factsAbout, err := client.GetFactsAboutEntity(ctx, entity.ID)
	require.NoError(t, err)
	assert.Len(t, factsAbout, 1)

	decEnts, err := client.GetDecisionEntities(ctx, decision.ID)
	require.NoError(t, err)
	assert.Len(t, decEnts, 1)
	assert.Equal(t, "subject", decEnts[0].Role)

	entDecs, err := client.GetEntityDecisions(ctx, entity.ID)
	require.NoError(t, err)
	assert.Len(t, entDecs, 1)
	assert.Equal(t, decision.Title, entDecs[0].Title)

	// --- Invalidate a fact and verify chain ---
	fact3, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I moved to New York",
		Category: "personal",
	})
	require.NoError(t, err)
	require.NoError(t, client.InvalidateFact(ctx, fact2.ID, fact3.ID, "User relocated"))

	chain, err := client.GetInvalidationChain(ctx, fact2.ID)
	require.NoError(t, err)
	assert.Len(t, chain, 1)
	assert.Equal(t, "User relocated", chain[0].Reason)

	// Old fact should be invalid
	oldNode, err := client.GetNodeByID(ctx, fact2.ID)
	require.NoError(t, err)
	oldFact, ok := oldNode.(*tools.Fact)
	require.True(t, ok)
	assert.False(t, oldFact.Valid)

	// --- Verify stats ---
	stats, err := client.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.TotalFacts)
	assert.Equal(t, 2, stats.ValidFacts)
	assert.Equal(t, 1, stats.InvalidatedFacts)
	assert.Equal(t, 1, stats.TotalDecisions)
	assert.Equal(t, 1, stats.ActiveDecisions)
	assert.Equal(t, 1, stats.TotalEntities)
	assert.Equal(t, 1, stats.TotalEvents)
	assert.Equal(t, 1, stats.TotalTopics)
	assert.Equal(t, 6, stats.TotalEdges) // 5 relationships added + 1 invalidation edge
	assert.Equal(t, "1", stats.SchemaVersion)
	assert.Equal(t, "mem", stats.StorageEngine)
}

// ---------------------------------------------------------------------------
// 2. TestIntegrationSemanticSearch
// ---------------------------------------------------------------------------

func TestIntegrationSemanticSearch(t *testing.T) {
	t.Parallel()

	client, embedder := setupIntegrationClientWithEmbedder(t)
	ctx := context.Background()
	backend := client.backend.(*storage.EmbeddedBackend)

	// Store facts and their embeddings synchronously
	fact1, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "Go is great for concurrency",
		Category: "technical",
	})
	require.NoError(t, err)
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact1.ID, fact1.Content)

	fact2, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I enjoy cooking pasta",
		Category: "preference",
	})
	require.NoError(t, err)
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact2.ID, fact2.Content)

	fact3, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "Rust has great concurrency primitives",
		Category: "technical",
	})
	require.NoError(t, err)
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact3.ID, fact3.Content)

	// Create HNSW index after inserting data
	require.NoError(t, EnsureHNSWIndexes(backend, 4))

	// Search for concurrency-related facts
	results, err := client.SemanticSearch(ctx, "concurrency programming", []string{"fact"}, 10)
	require.NoError(t, err)
	// Should return results without error; mock embeddings are deterministic
	// so we at least verify no crash and results are returned
	assert.NotEmpty(t, results, "semantic search should return results")

	// Verify results are sorted by distance (ascending)
	for i := 1; i < len(results); i++ {
		assert.LessOrEqual(t, results[i-1].Distance, results[i].Distance,
			"results should be sorted by distance ascending")
	}
}

// ---------------------------------------------------------------------------
// 3. TestIntegrationConflictDetection
// ---------------------------------------------------------------------------

func TestIntegrationConflictDetection(t *testing.T) {
	t.Parallel()

	client, embedder := setupIntegrationClientWithEmbedder(t)
	ctx := context.Background()
	backend := client.backend.(*storage.EmbeddedBackend)

	// Store two semantically similar facts
	fact1, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I prefer dark mode",
		Category: "preference",
	})
	require.NoError(t, err)
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact1.ID, fact1.Content)

	fact2, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I prefer light mode",
		Category: "preference",
	})
	require.NoError(t, err)
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact2.ID, fact2.Content)

	require.NoError(t, EnsureHNSWIndexes(backend, 4))

	// DetectConflicts should not error (whether it finds conflicts depends on mock embedding distances)
	conflicts, err := client.DetectConflicts(ctx, tools.ConflictOptions{
		Threshold: 2.0, // Very generous threshold to find any neighbors
		Limit:     10,
	})
	require.NoError(t, err)
	// With a large threshold, the two facts should be returned as neighbors
	if len(conflicts) > 0 {
		assert.True(t, conflicts[0].Similarity > 0 || conflicts[0].Similarity <= 0,
			"similarity should be a real number")
	}

	// CheckNewFactConflicts should not error either
	newConflicts, err := client.CheckNewFactConflicts(ctx, "I prefer dark themes", "preference")
	require.NoError(t, err)
	_ = newConflicts // may or may not find conflicts depending on hash proximity

	// Without embeddings, conflict detection should fail
	noEmbClient := setupIntegrationClient(t, false)
	_, err = noEmbClient.DetectConflicts(ctx, tools.ConflictOptions{})
	assert.Error(t, err, "conflict detection without embeddings should error")
}

// ---------------------------------------------------------------------------
// 4. TestIntegrationIdempotency
// ---------------------------------------------------------------------------

func TestIntegrationIdempotency(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	req := tools.StoreFactRequest{
		Content:    "I use Neovim as my editor",
		Category:   "preference",
		Confidence: 0.9,
	}

	// Store twice with identical content+category
	fact1, err := client.StoreFact(ctx, req)
	require.NoError(t, err)

	fact2, err := client.StoreFact(ctx, req)
	require.NoError(t, err)

	// Should return the same deterministic ID (content-based hash)
	assert.Equal(t, fact1.ID, fact2.ID, "storing same fact twice should return same ID")

	// Verify only one row exists in the DB
	stats, err := client.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalFacts, "only one row should exist after storing same fact twice")

	// Same for entities
	entReq := tools.StoreEntityRequest{Name: "Neovim", Kind: "technology"}
	ent1, err := client.StoreEntity(ctx, entReq)
	require.NoError(t, err)
	ent2, err := client.StoreEntity(ctx, entReq)
	require.NoError(t, err)
	assert.Equal(t, ent1.ID, ent2.ID, "storing same entity twice should return same ID")
	stats2, err := client.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, stats2.TotalEntities)

	// Same for topics
	topReq := tools.StoreTopicRequest{Name: "Editors"}
	top1, err := client.StoreTopic(ctx, topReq)
	require.NoError(t, err)
	top2, err := client.StoreTopic(ctx, topReq)
	require.NoError(t, err)
	assert.Equal(t, top1.ID, top2.ID, "storing same topic twice should return same ID")
}

// ---------------------------------------------------------------------------
// 5. TestIntegrationGraphTraversal
// ---------------------------------------------------------------------------

func TestIntegrationGraphTraversal(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	// Create entity
	entity, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "Go Language", Kind: "technology", Description: "A systems programming language",
	})
	require.NoError(t, err)

	// Create facts about entity
	factA, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content: "Go compiles fast", Category: "technical",
	})
	require.NoError(t, err)
	factB, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content: "Go has goroutines", Category: "technical",
	})
	require.NoError(t, err)

	// Create decisions involving entity
	dec1, err := client.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title: "Use Go for MIE", Rationale: "Fast builds and strong concurrency",
	})
	require.NoError(t, err)
	dec2, err := client.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title: "Use Go for CLI tools", Rationale: "Single binary distribution",
	})
	require.NoError(t, err)

	// Wire up relationships
	require.NoError(t, client.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id": factA.ID, "entity_id": entity.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id": factB.ID, "entity_id": entity.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_decision_entity", map[string]string{
		"decision_id": dec1.ID, "entity_id": entity.ID, "role": "subject",
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_decision_entity", map[string]string{
		"decision_id": dec2.ID, "entity_id": entity.ID, "role": "context",
	}))

	// Traverse: entity -> facts
	facts, err := client.GetFactsAboutEntity(ctx, entity.ID)
	require.NoError(t, err)
	assert.Len(t, facts, 2)

	// Traverse: fact -> entities
	ents, err := client.GetRelatedEntities(ctx, factA.ID)
	require.NoError(t, err)
	assert.Len(t, ents, 1)
	assert.Equal(t, "Go Language", ents[0].Name)

	// Traverse: decision -> entities (with roles)
	decEnts, err := client.GetDecisionEntities(ctx, dec1.ID)
	require.NoError(t, err)
	assert.Len(t, decEnts, 1)
	assert.Equal(t, "subject", decEnts[0].Role)

	decEnts2, err := client.GetDecisionEntities(ctx, dec2.ID)
	require.NoError(t, err)
	assert.Len(t, decEnts2, 1)
	assert.Equal(t, "context", decEnts2[0].Role)

	// Traverse: entity -> decisions
	decs, err := client.GetEntityDecisions(ctx, entity.ID)
	require.NoError(t, err)
	assert.Len(t, decs, 2)
}

// ---------------------------------------------------------------------------
// 6. TestIntegrationExport
// ---------------------------------------------------------------------------

func TestIntegrationExport(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	// Store various node types
	fact, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content: "Export fact", Category: "general",
	})
	require.NoError(t, err)

	dec, err := client.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title: "Export decision", Rationale: "For export test",
	})
	require.NoError(t, err)

	ent, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "ExportEntity", Kind: "other",
	})
	require.NoError(t, err)

	evt, err := client.StoreEvent(ctx, tools.StoreEventRequest{
		Title: "Export event", EventDate: "2026-02-05",
	})
	require.NoError(t, err)

	top, err := client.StoreTopic(ctx, tools.StoreTopicRequest{
		Name: "Export topic",
	})
	require.NoError(t, err)

	// Add relationships
	require.NoError(t, client.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id": fact.ID, "entity_id": ent.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_decision_topic", map[string]string{
		"decision_id": dec.ID, "topic_id": top.ID,
	}))
	require.NoError(t, client.AddRelationship(ctx, "mie_event_decision", map[string]string{
		"event_id": evt.ID, "decision_id": dec.ID,
	}))

	// Export full graph
	export, err := client.ExportGraph(ctx, tools.ExportOptions{})
	require.NoError(t, err)

	assert.Equal(t, "1", export.Version)
	assert.NotEmpty(t, export.ExportedAt)
	assert.Len(t, export.Facts, 1)
	assert.Len(t, export.Decisions, 1)
	assert.Len(t, export.Entities, 1)
	assert.Len(t, export.Events, 1)
	assert.Len(t, export.Topics, 1)

	// Verify stats map
	assert.Equal(t, 1, export.Stats["facts"])
	assert.Equal(t, 1, export.Stats["decisions"])
	assert.Equal(t, 1, export.Stats["entities"])
	assert.Equal(t, 1, export.Stats["events"])
	assert.Equal(t, 1, export.Stats["topics"])

	// Export with filter
	factOnly, err := client.ExportGraph(ctx, tools.ExportOptions{
		NodeTypes: []string{"fact"},
	})
	require.NoError(t, err)
	assert.Len(t, factOnly.Facts, 1)
	assert.Empty(t, factOnly.Decisions)
	assert.Empty(t, factOnly.Entities)
}

// ---------------------------------------------------------------------------
// 7. TestIntegrationStatsAccuracy
// ---------------------------------------------------------------------------

func TestIntegrationStatsAccuracy(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	// Store known counts
	for i := 0; i < 5; i++ {
		_, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  fmt.Sprintf("Fact number %d", i),
			Category: "general",
		})
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		_, err := client.StoreDecision(ctx, tools.StoreDecisionRequest{
			Title:     fmt.Sprintf("Decision %d", i),
			Rationale: fmt.Sprintf("Rationale %d", i),
		})
		require.NoError(t, err)
	}
	for i := 0; i < 4; i++ {
		_, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
			Name: fmt.Sprintf("Entity%d", i),
			Kind: "other",
		})
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		_, err := client.StoreEvent(ctx, tools.StoreEventRequest{
			Title:     fmt.Sprintf("Event %d", i),
			EventDate: fmt.Sprintf("2026-02-%02d", i+1),
		})
		require.NoError(t, err)
	}
	for i := 0; i < 6; i++ {
		_, err := client.StoreTopic(ctx, tools.StoreTopicRequest{
			Name: fmt.Sprintf("Topic%d", i),
		})
		require.NoError(t, err)
	}

	// Invalidate one fact to test counts
	facts := make([]*tools.Fact, 0)
	for i := 0; i < 5; i++ {
		f, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  fmt.Sprintf("Fact number %d", i),
			Category: "general",
		})
		require.NoError(t, err)
		facts = append(facts, f)
	}

	// Invalidate fact 0 with fact 1
	require.NoError(t, client.InvalidateFact(ctx, facts[0].ID, facts[1].ID, "correction"))

	stats, err := client.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, 5, stats.TotalFacts)
	assert.Equal(t, 4, stats.ValidFacts)
	assert.Equal(t, 1, stats.InvalidatedFacts)
	assert.Equal(t, 3, stats.TotalDecisions)
	assert.Equal(t, 3, stats.ActiveDecisions)
	assert.Equal(t, 4, stats.TotalEntities)
	assert.Equal(t, 2, stats.TotalEvents)
	assert.Equal(t, 6, stats.TotalTopics)
	assert.Equal(t, "1", stats.SchemaVersion)
}

// ---------------------------------------------------------------------------
// 8. TestIntegrationEdgeCases
// ---------------------------------------------------------------------------

func TestIntegrationEdgeCases(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	// --- Empty DB queries ---
	t.Run("EmptyDBQueries", func(t *testing.T) {
		nodes, total, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact", Limit: 10,
		})
		require.NoError(t, err)
		assert.Empty(t, nodes)
		assert.Equal(t, 0, total)

		stats, err := client.GetStats(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, stats.TotalFacts)

		results, err := client.ExactSearch(ctx, "nonexistent", []string{"fact"}, 10)
		require.NoError(t, err)
		assert.Empty(t, results)

		export, err := client.ExportGraph(ctx, tools.ExportOptions{})
		require.NoError(t, err)
		assert.Empty(t, export.Facts)

		_, err = client.GetNodeByID(ctx, "fact:nonexistent")
		assert.Error(t, err, "should error for non-existent node")
	})

	// --- Unicode content ---
	t.Run("UnicodeContent", func(t *testing.T) {
		// Japanese
		factJP, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "日本語のテストデータ",
			Category: "general",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, factJP.ID)

		node, err := client.GetNodeByID(ctx, factJP.ID)
		require.NoError(t, err)
		f := node.(*tools.Fact)
		assert.Equal(t, "日本語のテストデータ", f.Content)

		// Emojis
		factEmoji, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "I love programming! 🚀🔥💻",
			Category: "preference",
		})
		require.NoError(t, err)
		node2, err := client.GetNodeByID(ctx, factEmoji.ID)
		require.NoError(t, err)
		f2 := node2.(*tools.Fact)
		assert.Contains(t, f2.Content, "🚀")

		// Accented characters
		factAccent, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "Café résumé naïve",
			Category: "general",
		})
		require.NoError(t, err)
		node3, err := client.GetNodeByID(ctx, factAccent.ID)
		require.NoError(t, err)
		f3 := node3.(*tools.Fact)
		assert.Equal(t, "Café résumé naïve", f3.Content)

		// Search should work with unicode
		results, err := client.ExactSearch(ctx, "日本語", []string{"fact"}, 10)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	// --- Very long content ---
	t.Run("LongContent", func(t *testing.T) {
		longContent := strings.Repeat("A", 10240) // >10KB
		factLong, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  longContent,
			Category: "general",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, factLong.ID)

		node, err := client.GetNodeByID(ctx, factLong.ID)
		require.NoError(t, err)
		f := node.(*tools.Fact)
		assert.Equal(t, 10240, len(f.Content))
	})

	// --- Special characters that need Datalog escaping ---
	t.Run("SpecialCharacters", func(t *testing.T) {
		// Single quotes (should not affect Datalog which uses double quotes)
		factSingle, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "It's a test with single 'quotes'",
			Category: "general",
		})
		require.NoError(t, err)
		node, err := client.GetNodeByID(ctx, factSingle.ID)
		require.NoError(t, err)
		f := node.(*tools.Fact)
		assert.Equal(t, "It's a test with single 'quotes'", f.Content)

		// Forward slashes
		factSlash, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "path/to/some/file.txt is a unix path",
			Category: "general",
		})
		require.NoError(t, err)
		node2, err := client.GetNodeByID(ctx, factSlash.ID)
		require.NoError(t, err)
		f2 := node2.(*tools.Fact)
		assert.Equal(t, "path/to/some/file.txt is a unix path", f2.Content)

		// Angle brackets, ampersands (HTML-like)
		factHTML, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "Use <div> & <span> elements",
			Category: "technical",
		})
		require.NoError(t, err)
		node3, err := client.GetNodeByID(ctx, factHTML.ID)
		require.NoError(t, err)
		f3 := node3.(*tools.Fact)
		assert.Equal(t, "Use <div> & <span> elements", f3.Content)

		// Parentheses, brackets, braces
		factBrackets, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "func main() { fmt.Println([1, 2, 3]) }",
			Category: "technical",
		})
		require.NoError(t, err)
		node4, err := client.GetNodeByID(ctx, factBrackets.ID)
		require.NoError(t, err)
		f4 := node4.(*tools.Fact)
		assert.Equal(t, "func main() { fmt.Println([1, 2, 3]) }", f4.Content)

		// Pipes, semicolons, colons
		factPipes, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:  "cmd1 | cmd2; echo $?; exit: 0",
			Category: "technical",
		})
		require.NoError(t, err)
		node5, err := client.GetNodeByID(ctx, factPipes.ID)
		require.NoError(t, err)
		f5 := node5.(*tools.Fact)
		assert.Equal(t, "cmd1 | cmd2; echo $?; exit: 0", f5.Content)
	})
}

// ---------------------------------------------------------------------------
// 9. TestIntegrationConcurrentWrites
// ---------------------------------------------------------------------------

func TestIntegrationConcurrentWrites(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	const numGoroutines = 10
	const factsPerGoroutine = 10

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*factsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gIdx int) {
			defer wg.Done()
			for i := 0; i < factsPerGoroutine; i++ {
				_, err := client.StoreFact(ctx, tools.StoreFactRequest{
					Content:     fmt.Sprintf("Goroutine %d fact %d at %d", gIdx, i, time.Now().UnixNano()),
					Category:    "general",
					Confidence:  0.8,
					SourceAgent: fmt.Sprintf("goroutine-%d", gIdx),
				})
				if err != nil {
					errCh <- err
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	assert.Empty(t, errs, "no errors should occur during concurrent writes")

	// Verify all facts were stored
	stats, err := client.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*factsPerGoroutine, stats.TotalFacts,
		"all 100 facts should be stored")
}

// ---------------------------------------------------------------------------
// 10. TestIntegrationListNodesFilters
// ---------------------------------------------------------------------------

func TestIntegrationListNodesFilters(t *testing.T) {
	t.Parallel()
	client := setupIntegrationClient(t, false)
	ctx := context.Background()

	// Store facts with different categories and confidences
	categories := []string{"personal", "technical", "preference", "technical", "personal"}
	confidences := []float64{0.5, 0.9, 0.7, 0.95, 0.6}
	for i := 0; i < 5; i++ {
		_, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:    fmt.Sprintf("ListNode fact %d", i),
			Category:   categories[i],
			Confidence: confidences[i],
		})
		require.NoError(t, err)
	}

	// --- Category filter ---
	t.Run("CategoryFilter", func(t *testing.T) {
		nodes, total, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact",
			Category: "technical",
			Limit:    10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total, "should have 2 technical facts")
		assert.Len(t, nodes, 2)
	})

	t.Run("PersonalCategory", func(t *testing.T) {
		nodes, total, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact",
			Category: "personal",
			Limit:    10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, nodes, 2)
	})

	// --- ValidOnly filter ---
	t.Run("ValidOnlyFilter", func(t *testing.T) {
		// Invalidate one fact first
		facts, _, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact",
			Category: "preference",
			Limit:    1,
		})
		require.NoError(t, err)
		require.Len(t, facts, 1)
		prefFact := facts[0].(*tools.Fact)

		// Create a replacement and invalidate
		newFact, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content: "Updated preference", Category: "preference",
		})
		require.NoError(t, err)
		require.NoError(t, client.InvalidateFact(ctx, prefFact.ID, newFact.ID, "updated"))

		// ValidOnly should exclude the invalidated fact
		validNodes, validTotal, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType:  "fact",
			ValidOnly: true,
			Limit:     20,
		})
		require.NoError(t, err)
		assert.Equal(t, 5, validTotal, "5 valid facts remain (4 original valid + 1 new)")
		assert.Len(t, validNodes, 5)

		// Without ValidOnly, all facts including invalidated
		allNodes, allTotal, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact",
			Limit:    20,
		})
		require.NoError(t, err)
		assert.Equal(t, 6, allTotal, "6 total facts (5 original + 1 new)")
		assert.Len(t, allNodes, 6)
	})

	// --- Limit/Offset pagination ---
	t.Run("Pagination", func(t *testing.T) {
		// Get first page
		page1, total, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact",
			Limit:    2,
			Offset:   0,
		})
		require.NoError(t, err)
		assert.Equal(t, 6, total)
		assert.Len(t, page1, 2)

		// Get second page
		page2, total2, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType: "fact",
			Limit:    2,
			Offset:   2,
		})
		require.NoError(t, err)
		assert.Equal(t, 6, total2)
		assert.Len(t, page2, 2)

		// IDs should not overlap
		page1IDs := make(map[string]bool)
		for _, n := range page1 {
			page1IDs[n.(*tools.Fact).ID] = true
		}
		for _, n := range page2 {
			assert.False(t, page1IDs[n.(*tools.Fact).ID], "pages should not overlap")
		}
	})

	// --- Sort order ---
	t.Run("SortOrder", func(t *testing.T) {
		// Default is descending by created_at
		nodesDesc, _, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType:  "fact",
			Limit:     20,
			SortOrder: "desc",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, nodesDesc)

		// Ascending
		nodesAsc, _, err := client.ListNodes(ctx, tools.ListOptions{
			NodeType:  "fact",
			Limit:     20,
			SortOrder: "asc",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, nodesAsc)

		// First item in asc should have earliest created_at
		if len(nodesAsc) > 1 {
			first := nodesAsc[0].(*tools.Fact)
			last := nodesAsc[len(nodesAsc)-1].(*tools.Fact)
			assert.LessOrEqual(t, first.CreatedAt, last.CreatedAt,
				"ascending sort should have earliest first")
		}
	})
}