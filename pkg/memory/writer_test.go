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

func TestWriterStoreFact(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	fact, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:     "I live in Buenos Aires",
		Category:    "personal",
		Confidence:  0.9,
		SourceAgent: "claude",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}

	if fact.ID == "" {
		t.Error("expected non-empty fact ID")
	}
	if fact.Content != "I live in Buenos Aires" {
		t.Errorf("expected content 'I live in Buenos Aires', got %q", fact.Content)
	}
	if fact.Category != "personal" {
		t.Errorf("expected category 'personal', got %q", fact.Category)
	}
	if !fact.Valid {
		t.Error("new fact should be valid")
	}
	if fact.CreatedAt == 0 {
		t.Error("expected non-zero created_at")
	}

	// Verify it was written to DB
	result, err := backend.Query(ctx, `?[id, content] := *mie_fact { id, content }`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(result.Rows))
	}
}

func TestWriterStoreFactValidation(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	// Empty content should fail
	_, err := w.StoreFact(ctx, tools.StoreFactRequest{})
	if err == nil {
		t.Error("expected error for empty content")
	}

	// Invalid category should default to "general"
	fact, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "test",
		Category: "invalid",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}
	if fact.Category != "general" {
		t.Errorf("expected category 'general', got %q", fact.Category)
	}

	// Invalid confidence should default to 0.8
	fact2, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:    "test2",
		Confidence: -1.0,
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}
	if fact2.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", fact2.Confidence)
	}
}

func TestWriterStoreDecision(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	decision, err := w.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title:     "Use Go for backend",
		Rationale: "CGO CozoDB bindings",
	})
	if err != nil {
		t.Fatalf("StoreDecision failed: %v", err)
	}

	if decision.Status != "active" {
		t.Errorf("expected status 'active', got %q", decision.Status)
	}

	// Validation: empty title
	_, err = w.StoreDecision(ctx, tools.StoreDecisionRequest{Rationale: "test"})
	if err == nil {
		t.Error("expected error for empty title")
	}
}

func TestWriterStoreEntity(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	entity, err := w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name:        "Kraklabs",
		Kind:        "company",
		Description: "AI software lab",
	})
	if err != nil {
		t.Fatalf("StoreEntity failed: %v", err)
	}

	if entity.Kind != "company" {
		t.Errorf("expected kind 'company', got %q", entity.Kind)
	}

	// Invalid kind defaults to "other"
	entity2, err := w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "test",
		Kind: "robot",
	})
	if err != nil {
		t.Fatalf("StoreEntity failed: %v", err)
	}
	if entity2.Kind != "other" {
		t.Errorf("expected kind 'other', got %q", entity2.Kind)
	}
}

func TestWriterStoreEvent(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	event, err := w.StoreEvent(ctx, tools.StoreEventRequest{
		Title:     "Launch v1.0",
		EventDate: "2026-02-05",
	})
	if err != nil {
		t.Fatalf("StoreEvent failed: %v", err)
	}

	if event.EventDate != "2026-02-05" {
		t.Errorf("expected event_date '2026-02-05', got %q", event.EventDate)
	}
}

func TestWriterStoreTopic(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	topic, err := w.StoreTopic(ctx, tools.StoreTopicRequest{
		Name:        "Architecture",
		Description: "System design decisions",
	})
	if err != nil {
		t.Fatalf("StoreTopic failed: %v", err)
	}

	if topic.Name != "Architecture" {
		t.Errorf("expected name 'Architecture', got %q", topic.Name)
	}
}

func TestWriterInvalidateFact(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	// Store two facts
	oldFact, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I live in Buenos Aires",
		Category: "personal",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}

	newFact, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I live in New York",
		Category: "personal",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}

	// Invalidate old fact
	err = w.InvalidateFact(ctx, oldFact.ID, newFact.ID, "User moved")
	if err != nil {
		t.Fatalf("InvalidateFact failed: %v", err)
	}

	// Verify old fact is now invalid
	result, err := backend.Query(ctx, `?[valid] := *mie_fact { id, valid }, id = "`+escapeDatalog(oldFact.ID)+`"`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatal("expected row")
	}
	if toBool(result.Rows[0][0]) {
		t.Error("old fact should be invalid")
	}

	// Verify invalidation edge exists
	result, err = backend.Query(ctx, `?[reason] := *mie_invalidates { new_fact_id, old_fact_id, reason }`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 invalidation edge, got %d", len(result.Rows))
	}
}

func TestWriterAddRelationship(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	// Store a fact and entity first
	fact, _ := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "Test fact",
		Category: "general",
	})
	entity, _ := w.StoreEntity(ctx, tools.StoreEntityRequest{
		Name: "Test Entity",
		Kind: "other",
	})

	// Add relationship
	err := w.AddRelationship(ctx, "mie_fact_entity", map[string]string{
		"fact_id":   fact.ID,
		"entity_id": entity.ID,
	})
	if err != nil {
		t.Fatalf("AddRelationship failed: %v", err)
	}

	// Unknown edge type
	err = w.AddRelationship(ctx, "mie_unknown", nil)
	if err == nil {
		t.Error("expected error for unknown edge type")
	}
}

func TestWriterUpdateStatus(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	w := NewWriter(backend, nil, nil)
	ctx := context.Background()

	decision, err := w.StoreDecision(ctx, tools.StoreDecisionRequest{
		Title:     "Test decision",
		Rationale: "Test rationale",
	})
	if err != nil {
		t.Fatalf("StoreDecision failed: %v", err)
	}

	err = w.UpdateStatus(ctx, decision.ID, "superseded")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify status changed
	result, err := backend.Query(ctx, `?[status] := *mie_decision { id, status }, id = "`+escapeDatalog(decision.ID)+`"`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if toString(result.Rows[0][0]) != "superseded" {
		t.Errorf("expected status 'superseded', got %v", result.Rows[0][0])
	}

	// Invalid status
	err = w.UpdateStatus(ctx, decision.ID, "invalid")
	if err == nil {
		t.Error("expected error for invalid status")
	}
}