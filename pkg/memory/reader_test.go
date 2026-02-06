// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"testing"

	"github.com/kraklabs/mie/pkg/tools"
)

func TestReaderListNodes(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	// Store some facts
	w.StoreFact(ctx, tools.StoreFactRequest{Content: "Fact 1", Category: "personal"})
	w.StoreFact(ctx, tools.StoreFactRequest{Content: "Fact 2", Category: "technical"})
	w.StoreFact(ctx, tools.StoreFactRequest{Content: "Fact 3", Category: "personal"})

	// List all facts
	nodes, total, err := r.ListNodes(ctx, tools.ListOptions{
		NodeType: "fact",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}

	// List with category filter
	nodes, total, err = r.ListNodes(ctx, tools.ListOptions{
		NodeType: "fact",
		Category: "personal",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListNodes (filtered) failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 filtered nodes, got %d", len(nodes))
	}
}

func TestReaderGetNodeByID(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	fact, _ := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "Test fact",
		Category: "general",
	})

	node, err := r.GetNodeByID(ctx, fact.ID)
	if err != nil {
		t.Fatalf("GetNodeByID failed: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	f, ok := node.(*tools.Fact)
	if !ok {
		t.Fatalf("expected *tools.Fact, got %T", node)
	}
	if f.Content != "Test fact" {
		t.Errorf("expected content 'Test fact', got %q", f.Content)
	}

	// Non-existent node
	_, err = r.GetNodeByID(ctx, "fact:nonexistent")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestReaderGetRelatedEntities(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	fact, _ := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I work at Kraklabs",
		Category: "professional",
	})
	entity, _ := w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "Kraklabs",
		Kind: "company",
	})

	w.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id":   fact.ID,
		"entity_id": entity.ID,
	})

	entities, err := r.GetRelatedEntities(ctx, fact.ID)
	if err != nil {
		t.Fatalf("GetRelatedEntities failed: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if entities[0].Name != "Kraklabs" {
		t.Errorf("expected entity name 'Kraklabs', got %q", entities[0].Name)
	}
}

func TestReaderGetFactsAboutEntity(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	entity, _ := w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "Go",
		Kind: "technology",
	})
	fact, _ := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "Go is great for concurrency",
		Category: "technical",
	})

	w.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id":   fact.ID,
		"entity_id": entity.ID,
	})

	facts, err := r.GetFactsAboutEntity(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetFactsAboutEntity failed: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
}

func TestReaderGetDecisionEntities(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	decision, _ := w.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title:     "Use Go",
		Rationale: "Performance",
	})
	entity, _ := w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "Go",
		Kind: "technology",
	})

	w.AddRelationship(ctx, "mie_decision_entity", map[string]string{
		"decision_id": decision.ID,
		"entity_id":   entity.ID,
		"role":        "subject",
	})

	entities, err := r.GetDecisionEntities(ctx, decision.ID)
	if err != nil {
		t.Fatalf("GetDecisionEntities failed: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if entities[0].Role != "subject" {
		t.Errorf("expected role 'subject', got %q", entities[0].Role)
	}
}

func TestReaderGetInvalidationChain(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	old, _ := w.StoreFact(ctx, tools.StoreFactRequest{Content: "old fact", Category: "general"})
	new_, _ := w.StoreFact(ctx, tools.StoreFactRequest{Content: "new fact", Category: "general"})

	w.InvalidateFact(ctx, old.ID, new_.ID, "correction")

	chain, err := r.GetInvalidationChain(ctx, old.ID)
	if err != nil {
		t.Fatalf("GetInvalidationChain failed: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("expected 1 invalidation, got %d", len(chain))
	}
	if chain[0].Reason != "correction" {
		t.Errorf("expected reason 'correction', got %q", chain[0].Reason)
	}
}

func TestReaderGetStats(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	w.StoreFact(ctx, tools.StoreFactRequest{Content: "Fact 1", Category: "general"})
	w.StoreFact(ctx, tools.StoreFactRequest{Content: "Fact 2", Category: "personal"})
	w.StoreEntity(ctx, tools.StoreEntityRequest{Name: "Entity", Kind: "other"})
	w.StoreTopic(ctx, tools.StoreTopicRequest{Name: "Topic"})

	stats, err := r.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalFacts != 2 {
		t.Errorf("expected 2 facts, got %d", stats.TotalFacts)
	}
	if stats.ValidFacts != 2 {
		t.Errorf("expected 2 valid facts, got %d", stats.ValidFacts)
	}
	if stats.TotalEntities != 1 {
		t.Errorf("expected 1 entity, got %d", stats.TotalEntities)
	}
	if stats.TotalTopics != 1 {
		t.Errorf("expected 1 topic, got %d", stats.TotalTopics)
	}
	if stats.SchemaVersion != "1" {
		t.Errorf("expected schema version '1', got %q", stats.SchemaVersion)
	}
}

func TestReaderExportGraph(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	w.StoreFact(ctx, tools.StoreFactRequest{Content: "Fact", Category: "general"})
	w.StoreEntity(ctx, tools.StoreEntityRequest{Name: "Entity", Kind: "other"})

	export, err := r.ExportGraph(ctx, tools.ExportOptions{})
	if err != nil {
		t.Fatalf("ExportGraph failed: %v", err)
	}

	if export.Version != "1" {
		t.Errorf("expected version '1', got %q", export.Version)
	}
	if len(export.Facts) != 1 {
		t.Errorf("expected 1 fact in export, got %d", len(export.Facts))
	}
	if len(export.Entities) != 1 {
		t.Errorf("expected 1 entity in export, got %d", len(export.Entities))
	}
}

func TestReaderExactSearch(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	w.StoreFact(ctx, tools.StoreFactRequest{Content: "I love coffee", Category: "preference"})
	w.StoreFact(ctx, tools.StoreFactRequest{Content: "I prefer tea", Category: "preference"})
	w.StoreEntity(ctx, tools.StoreEntityRequest{Name: "Coffee Shop", Kind: "place"})

	// Search facts
	results, err := r.ExactSearch(ctx, "coffee", []string{"fact"}, 10)
	if err != nil {
		t.Fatalf("ExactSearch failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Search entities
	results, err = r.ExactSearch(ctx, "Coffee", []string{"entity"}, 10)
	if err != nil {
		t.Fatalf("ExactSearch failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 entity result, got %d", len(results))
	}
}

func TestReaderFindEntityByName(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name:        "Kraklabs",
		Kind:        "company",
		Description: "AI lab",
	})

	entity, err := r.FindEntityByName(ctx, "kraklabs")
	if err != nil {
		t.Fatalf("FindEntityByName failed: %v", err)
	}
	if entity == nil {
		t.Fatal("expected non-nil entity")
	}
	if entity.Name != "Kraklabs" {
		t.Errorf("expected name 'Kraklabs', got %q", entity.Name)
	}
}

func TestReaderGetEntityDecisions(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	r := NewReader(backend, nil, nil)
	ctx := context.Background()

	entity, _ := w.StoreEntity(ctx, tools.StoreEntityRequest{Name: "Go", Kind: "technology"})
	decision, _ := w.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title:     "Use Go",
		Rationale: "Fast compilation",
	})

	w.AddRelationship(ctx, "mie_decision_entity", map[string]string{
		"decision_id": decision.ID,
		"entity_id":   entity.ID,
		"role":        "subject",
	})

	decisions, err := r.GetEntityDecisions(ctx, entity.ID)
	if err != nil {
		t.Fatalf("GetEntityDecisions failed: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Title != "Use Go" {
		t.Errorf("expected title 'Use Go', got %q", decisions[0].Title)
	}
}
