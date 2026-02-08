// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

// Writer handles all mutations to the memory graph.
type Writer struct {
	backend  storage.Backend
	embedder *EmbeddingGenerator
	logger   *slog.Logger
}

// NewWriter creates a new Writer.
func NewWriter(backend storage.Backend, embedder *EmbeddingGenerator, logger *slog.Logger) *Writer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Writer{backend: backend, embedder: embedder, logger: logger}
}

// StoreFact stores a fact in the memory graph.
func (w *Writer) StoreFact(ctx context.Context, req tools.StoreFactRequest) (*tools.Fact, error) {
	if req.Content == "" {
		return nil, fmt.Errorf("fact content is required")
	}
	if !isValidCategory(req.Category) {
		req.Category = "general"
	}
	if req.Confidence < 0 || req.Confidence > 1.0 {
		req.Confidence = 0.8
	}

	id := FactID(req.Content, req.Category)
	now := time.Now().Unix()

	fact := &tools.Fact{
		ID:                 id,
		Content:            req.Content,
		Category:           req.Category,
		Confidence:         req.Confidence,
		SourceAgent:        req.SourceAgent,
		SourceConversation: req.SourceConversation,
		Valid:              true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	mutation := fmt.Sprintf(
		`?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] <- [['%s', '%s', '%s', %f, '%s', '%s', true, %d, %d]] :put mie_fact { id => content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at }`,
		escapeDatalog(fact.ID), escapeDatalog(fact.Content), escapeDatalog(fact.Category),
		fact.Confidence, escapeDatalog(fact.SourceAgent), escapeDatalog(fact.SourceConversation),
		fact.CreatedAt, fact.UpdatedAt,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return nil, fmt.Errorf("store fact: %w", err)
	}

	if w.embedder != nil {
		go w.storeEmbeddingAsync("mie_fact_embedding", "fact_id", fact.ID, fact.Content)
	}

	return fact, nil
}

// StoreDecision stores a decision in the memory graph.
func (w *Writer) StoreDecision(ctx context.Context, req tools.StoreDecisionRequest) (*tools.Decision, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("decision title is required")
	}
	if req.Rationale == "" {
		return nil, fmt.Errorf("decision rationale is required")
	}

	id := DecisionID(req.Title, req.Rationale)
	now := time.Now().Unix()

	decision := &tools.Decision{
		ID:                 id,
		Title:              req.Title,
		Rationale:          req.Rationale,
		Alternatives:       req.Alternatives,
		Context:            req.Context,
		SourceAgent:        req.SourceAgent,
		SourceConversation: req.SourceConversation,
		Status:             "active",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	mutation := fmt.Sprintf(
		`?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] <- [['%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', %d, %d]] :put mie_decision { id => title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at }`,
		escapeDatalog(decision.ID), escapeDatalog(decision.Title), escapeDatalog(decision.Rationale),
		escapeDatalog(decision.Alternatives), escapeDatalog(decision.Context),
		escapeDatalog(decision.SourceAgent), escapeDatalog(decision.SourceConversation),
		escapeDatalog(decision.Status), decision.CreatedAt, decision.UpdatedAt,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return nil, fmt.Errorf("store decision: %w", err)
	}

	if w.embedder != nil {
		text := decision.Title + ". " + decision.Rationale
		go w.storeEmbeddingAsync("mie_decision_embedding", "decision_id", decision.ID, text)
	}

	return decision, nil
}

// StoreEntity stores an entity in the memory graph.
func (w *Writer) StoreEntity(ctx context.Context, req tools.StoreEntityRequest) (*tools.Entity, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("entity name is required")
	}
	if !isValidEntityKind(req.Kind) {
		req.Kind = "other"
	}

	id := EntityID(req.Name, req.Kind)
	now := time.Now().Unix()

	entity := &tools.Entity{
		ID:          id,
		Name:        req.Name,
		Kind:        req.Kind,
		Description: req.Description,
		SourceAgent: req.SourceAgent,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutation := fmt.Sprintf(
		`?[id, name, kind, description, source_agent, created_at, updated_at] <- [['%s', '%s', '%s', '%s', '%s', %d, %d]] :put mie_entity { id => name, kind, description, source_agent, created_at, updated_at }`,
		escapeDatalog(entity.ID), escapeDatalog(entity.Name), escapeDatalog(entity.Kind),
		escapeDatalog(entity.Description), escapeDatalog(entity.SourceAgent),
		entity.CreatedAt, entity.UpdatedAt,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return nil, fmt.Errorf("store entity: %w", err)
	}

	if w.embedder != nil {
		text := entity.Name + ": " + entity.Description
		go w.storeEmbeddingAsync("mie_entity_embedding", "entity_id", entity.ID, text)
	}

	return entity, nil
}

// StoreEvent stores an event in the memory graph.
func (w *Writer) StoreEvent(ctx context.Context, req tools.StoreEventRequest) (*tools.Event, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("event title is required")
	}

	id := EventID(req.Title, req.EventDate)
	now := time.Now().Unix()

	event := &tools.Event{
		ID:                 id,
		Title:              req.Title,
		Description:        req.Description,
		EventDate:          req.EventDate,
		SourceAgent:        req.SourceAgent,
		SourceConversation: req.SourceConversation,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	mutation := fmt.Sprintf(
		`?[id, title, description, event_date, source_agent, source_conversation, created_at, updated_at] <- [['%s', '%s', '%s', '%s', '%s', '%s', %d, %d]] :put mie_event { id => title, description, event_date, source_agent, source_conversation, created_at, updated_at }`,
		escapeDatalog(event.ID), escapeDatalog(event.Title), escapeDatalog(event.Description),
		escapeDatalog(event.EventDate), escapeDatalog(event.SourceAgent),
		escapeDatalog(event.SourceConversation), event.CreatedAt, event.UpdatedAt,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return nil, fmt.Errorf("store event: %w", err)
	}

	if w.embedder != nil {
		text := event.Title + ". " + event.Description
		go w.storeEmbeddingAsync("mie_event_embedding", "event_id", event.ID, text)
	}

	return event, nil
}

// StoreTopic stores a topic in the memory graph.
func (w *Writer) StoreTopic(ctx context.Context, req tools.StoreTopicRequest) (*tools.Topic, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("topic name is required")
	}

	id := TopicID(req.Name)
	now := time.Now().Unix()

	topic := &tools.Topic{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	mutation := fmt.Sprintf(
		`?[id, name, description, created_at, updated_at] <- [['%s', '%s', '%s', %d, %d]] :put mie_topic { id => name, description, created_at, updated_at }`,
		escapeDatalog(topic.ID), escapeDatalog(topic.Name), escapeDatalog(topic.Description),
		topic.CreatedAt, topic.UpdatedAt,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return nil, fmt.Errorf("store topic: %w", err)
	}

	return topic, nil
}

// InvalidateFact marks a fact as invalid and records the invalidation edge.
func (w *Writer) InvalidateFact(ctx context.Context, oldFactID, newFactID, reason string) error {
	if oldFactID == "" || newFactID == "" {
		return fmt.Errorf("both old and new fact IDs are required")
	}

	now := time.Now().Unix()

	// Mark the old fact as invalid by reading its current data and updating
	mutation := fmt.Sprintf(
		`?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] :=
    *mie_fact { id, content, category, confidence, source_agent, source_conversation, created_at },
    id = '%s',
    valid = false,
    updated_at = %d
:put mie_fact { id => content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at }`,
		escapeDatalog(oldFactID), now,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return fmt.Errorf("invalidate fact %s: %w", oldFactID, err)
	}

	// Record the invalidation edge
	edgeMutation := fmt.Sprintf(
		`?[new_fact_id, old_fact_id, reason] <- [['%s', '%s', '%s']] :put mie_invalidates { new_fact_id, old_fact_id => reason }`,
		escapeDatalog(newFactID), escapeDatalog(oldFactID), escapeDatalog(reason),
	)
	if err := w.backend.Execute(ctx, edgeMutation); err != nil {
		return fmt.Errorf("record invalidation edge: %w", err)
	}

	return nil
}

// AddRelationship creates an edge between two nodes in the memory graph.
func (w *Writer) AddRelationship(ctx context.Context, edgeType string, fields map[string]string) error {
	cols, ok := ValidEdgeTables[edgeType]
	if !ok {
		return fmt.Errorf("unknown edge type: %s", edgeType)
	}

	// Build column values
	var colNames []string
	var colValues []string
	for _, col := range cols {
		val, exists := fields[col]
		if !exists {
			return fmt.Errorf("missing required field %q for edge type %s", col, edgeType)
		}
		colNames = append(colNames, col)
		colValues = append(colValues, fmt.Sprintf(`'%s'`, escapeDatalog(val)))
	}

	// Handle optional value columns (like role for mie_decision_entity, reason for mie_invalidates)
	for k, v := range fields {
		found := false
		for _, col := range cols {
			if col == k {
				found = true
				break
			}
		}
		if !found {
			colNames = append(colNames, k)
			colValues = append(colValues, fmt.Sprintf(`'%s'`, escapeDatalog(v)))
		}
	}

	mutation := fmt.Sprintf(
		`?[%s] <- [[%s]] :put %s { %s }`,
		joinStrings(colNames, ", "),
		joinStrings(colValues, ", "),
		edgeType,
		joinStrings(colNames, ", "),
	)

	if err := w.backend.Execute(ctx, mutation); err != nil {
		return fmt.Errorf("add relationship %s: %w", edgeType, err)
	}

	return nil
}

// UpdateDescription updates the description of a node.
func (w *Writer) UpdateDescription(ctx context.Context, nodeID, newDescription string) error {
	nodeType, err := w.detectNodeType(ctx, nodeID)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	var mutation string

	switch nodeType {
	case "entity":
		mutation = fmt.Sprintf(
			`?[id, name, kind, description, source_agent, created_at, updated_at] :=
    *mie_entity { id, name, kind, source_agent, created_at },
    id = '%s',
    description = '%s',
    updated_at = %d
:put mie_entity { id => name, kind, description, source_agent, created_at, updated_at }`,
			escapeDatalog(nodeID), escapeDatalog(newDescription), now,
		)
	case "event":
		mutation = fmt.Sprintf(
			`?[id, title, description, event_date, source_agent, source_conversation, created_at, updated_at] :=
    *mie_event { id, title, event_date, source_agent, source_conversation, created_at },
    id = '%s',
    description = '%s',
    updated_at = %d
:put mie_event { id => title, description, event_date, source_agent, source_conversation, created_at, updated_at }`,
			escapeDatalog(nodeID), escapeDatalog(newDescription), now,
		)
	case "topic":
		mutation = fmt.Sprintf(
			`?[id, name, description, created_at, updated_at] :=
    *mie_topic { id, name, created_at },
    id = '%s',
    description = '%s',
    updated_at = %d
:put mie_topic { id => name, description, created_at, updated_at }`,
			escapeDatalog(nodeID), escapeDatalog(newDescription), now,
		)
	default:
		return fmt.Errorf("node type %q does not support description update", nodeType)
	}

	if err := w.backend.Execute(ctx, mutation); err != nil {
		return fmt.Errorf("update description: %w", err)
	}

	return nil
}

// UpdateStatus updates the status of a decision node.
func (w *Writer) UpdateStatus(ctx context.Context, nodeID, newStatus string) error {
	if !isValidDecisionStatus(newStatus) {
		return fmt.Errorf("invalid status %q; must be one of: active, superseded, reversed", newStatus)
	}

	now := time.Now().Unix()

	mutation := fmt.Sprintf(
		`?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] :=
    *mie_decision { id, title, rationale, alternatives, context, source_agent, source_conversation, created_at },
    id = '%s',
    status = '%s',
    updated_at = %d
:put mie_decision { id => title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at }`,
		escapeDatalog(nodeID), escapeDatalog(newStatus), now,
	)

	if err := w.backend.Execute(ctx, mutation); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	return nil
}

// storeEmbeddingAsync generates and stores an embedding in the background.
func (w *Writer) storeEmbeddingAsync(table, idCol, nodeID, text string) {
	ctx := context.Background()
	embedding, err := w.embedder.Generate(ctx, text)
	if err != nil {
		w.logger.Warn("failed to generate embedding", "node_id", nodeID, "table", table, "error", err)
		return
	}

	vecStr := formatVector(embedding)
	mutation := fmt.Sprintf(
		`?[%s, embedding] <- [['%s', vec(%s)]] :put %s { %s => embedding }`,
		idCol, escapeDatalog(nodeID), vecStr, table, idCol,
	)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		w.logger.Warn("failed to store embedding", "node_id", nodeID, "table", table, "error", err)
	}
}

// detectNodeType determines the type of a node by its ID prefix or by querying tables.
func (w *Writer) detectNodeType(ctx context.Context, nodeID string) (string, error) {
	// Try to detect from ID prefix first
	if len(nodeID) >= 5 && nodeID[:5] == "fact:" {
		return "fact", nil
	}
	if len(nodeID) >= 4 {
		switch nodeID[:4] {
		case "ent:":
			return "entity", nil
		case "evt:":
			return "event", nil
		case "dec:":
			return "decision", nil
		case "top:":
			return "topic", nil
		}
	}

	// Fallback: query each table
	tables := []struct {
		name     string
		nodeType string
	}{
		{"mie_fact", "fact"},
		{"mie_decision", "decision"},
		{"mie_entity", "entity"},
		{"mie_event", "event"},
		{"mie_topic", "topic"},
	}

	for _, t := range tables {
		query := fmt.Sprintf(`?[id] := *%s { id }, id = '%s'`, t.name, escapeDatalog(nodeID))
		result, err := w.backend.Query(ctx, query)
		if err != nil {
			continue
		}
		if len(result.Rows) > 0 {
			return t.nodeType, nil
		}
	}

	return "", fmt.Errorf("node %q not found", nodeID)
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
