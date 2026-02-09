// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

// Writer handles all mutations to the memory graph.
type Writer struct {
	backend     storage.Backend
	embedder    *EmbeddingGenerator
	logger      *slog.Logger
	embeddingWg sync.WaitGroup
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
		w.embeddingWg.Add(1)
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
		w.embeddingWg.Add(1)
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

	// Check if entity already exists to preserve created_at on upsert.
	createdAt := now
	checkScript := fmt.Sprintf(`?[created_at] := *mie_entity { id, created_at }, id = '%s'`, escapeDatalog(id))
	if result, err := w.backend.Query(ctx, checkScript); err == nil && len(result.Rows) > 0 {
		if ts, ok := result.Rows[0][0].(float64); ok {
			createdAt = int64(ts)
		} else if ts, ok := result.Rows[0][0].(int64); ok {
			createdAt = ts
		}
	}

	entity := &tools.Entity{
		ID:          id,
		Name:        req.Name,
		Kind:        req.Kind,
		Description: req.Description,
		SourceAgent: req.SourceAgent,
		CreatedAt:   createdAt,
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
		w.embeddingWg.Add(1)
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
	if req.EventDate != "" {
		if _, err := time.Parse("2006-01-02", req.EventDate); err != nil {
			return nil, fmt.Errorf("invalid event_date format %q: expected ISO date (YYYY-MM-DD)", req.EventDate)
		}
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
		w.embeddingWg.Add(1)
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
		return fmt.Errorf("both node_id and replacement_id are required for fact invalidation")
	}

	// Verify the old fact exists before attempting invalidation.
	check := fmt.Sprintf(`?[id] := *mie_fact { id }, id = '%s'`, escapeDatalog(oldFactID))
	result, err := w.backend.Query(ctx, check)
	if err != nil || len(result.Rows) == 0 {
		return fmt.Errorf("fact %q not found", oldFactID)
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
	schema, ok := ValidEdgeTables[edgeType]
	if !ok {
		return fmt.Errorf("unknown edge type: %s", edgeType)
	}

	// Validate ID prefixes match expected node types for this edge.
	if prefixes, ok := validEdgeNodeTypes[edgeType]; ok && len(schema.Keys) >= 2 {
		for i, key := range schema.Keys[:2] {
			if val, exists := fields[key]; exists {
				if !strings.HasPrefix(val, prefixes[i]) {
					return fmt.Errorf("invalid ID %q for field %q of edge %s: expected prefix %q", val, key, edgeType, prefixes[i])
				}
			}
		}
	}

	// Build column values from all columns (keys + values).
	allCols := schema.AllColumns()
	var colNames []string
	var colValues []string
	for _, col := range allCols {
		val, exists := fields[col]
		if !exists {
			return fmt.Errorf("missing required field %q for edge type %s", col, edgeType)
		}
		colNames = append(colNames, col)
		colValues = append(colValues, fmt.Sprintf(`'%s'`, escapeDatalog(val)))
	}

	mutation := fmt.Sprintf(
		`?[%s] <- [[%s]] :put %s { %s }`,
		strings.Join(colNames, ", "),
		strings.Join(colValues, ", "),
		edgeType,
		strings.Join(colNames, ", "),
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

	// Verify the node actually exists in the database.
	table := nodeTypeToTable(nodeType)
	checkScript := fmt.Sprintf(`?[id] := *%s { id }, id = '%s'`, table, escapeDatalog(nodeID))
	checkResult, err := w.backend.Query(ctx, checkScript)
	if err != nil {
		return fmt.Errorf("check node existence: %w", err)
	}
	if len(checkResult.Rows) == 0 {
		return fmt.Errorf("node %q not found", nodeID)
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

	// Verify the decision exists before updating.
	checkScript := fmt.Sprintf(`?[id] := *mie_decision { id }, id = '%s'`, escapeDatalog(nodeID))
	checkResult, err := w.backend.Query(ctx, checkScript)
	if err != nil {
		return fmt.Errorf("check decision existence: %w", err)
	}
	if len(checkResult.Rows) == 0 {
		return fmt.Errorf("decision %q not found", nodeID)
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

// WaitForEmbeddings blocks until all background embedding operations complete.
func (w *Writer) WaitForEmbeddings() {
	w.embeddingWg.Wait()
}

// storeEmbeddingAsync generates and stores an embedding in the background.
func (w *Writer) storeEmbeddingAsync(table, idCol, nodeID, text string) {
	defer w.embeddingWg.Done()
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

// BackfillEmbeddings generates embeddings for nodes that don't have them yet.
// This handles nodes created before embeddings were enabled or when the
// embedding provider was unavailable.
func (w *Writer) BackfillEmbeddings(ctx context.Context) (int, error) {
	if w.embedder == nil {
		return 0, nil
	}

	type backfillItem struct {
		table  string
		idCol  string
		nodeID string
		text   string
	}

	var items []backfillItem

	// Find facts without embeddings.
	qr, err := w.backend.Query(ctx, `?[id, content] := *mie_fact{id, content}, not *mie_fact_embedding{fact_id: id}`)
	if err == nil {
		for _, row := range qr.Rows {
			items = append(items, backfillItem{"mie_fact_embedding", "fact_id", toString(row[0]), toString(row[1])})
		}
	}

	// Find decisions without embeddings.
	qr, err = w.backend.Query(ctx, `?[id, title, rationale] := *mie_decision{id, title, rationale}, not *mie_decision_embedding{decision_id: id}`)
	if err == nil {
		for _, row := range qr.Rows {
			items = append(items, backfillItem{"mie_decision_embedding", "decision_id", toString(row[0]), toString(row[1]) + ". " + toString(row[2])})
		}
	}

	// Find entities without embeddings.
	qr, err = w.backend.Query(ctx, `?[id, name, description] := *mie_entity{id, name, description}, not *mie_entity_embedding{entity_id: id}`)
	if err == nil {
		for _, row := range qr.Rows {
			items = append(items, backfillItem{"mie_entity_embedding", "entity_id", toString(row[0]), toString(row[1]) + ": " + toString(row[2])})
		}
	}

	// Find events without embeddings.
	qr, err = w.backend.Query(ctx, `?[id, title, description] := *mie_event{id, title, description}, not *mie_event_embedding{event_id: id}`)
	if err == nil {
		for _, row := range qr.Rows {
			items = append(items, backfillItem{"mie_event_embedding", "event_id", toString(row[0]), toString(row[1]) + ". " + toString(row[2])})
		}
	}

	if len(items) == 0 {
		return 0, nil
	}

	w.logger.Info("backfilling embeddings", "count", len(items))
	filled := 0
	for _, item := range items {
		embedding, genErr := w.embedder.Generate(ctx, item.text)
		if genErr != nil {
			w.logger.Warn("backfill embedding failed", "node_id", item.nodeID, "error", genErr)
			continue
		}
		vecStr := formatVector(embedding)
		mutation := fmt.Sprintf(
			`?[%s, embedding] <- [['%s', vec(%s)]] :put %s { %s => embedding }`,
			item.idCol, escapeDatalog(item.nodeID), vecStr, item.table, item.idCol,
		)
		if execErr := w.backend.Execute(ctx, mutation); execErr != nil {
			w.logger.Warn("backfill store failed", "node_id", item.nodeID, "error", execErr)
			continue
		}
		filled++
	}

	w.logger.Info("backfill complete", "filled", filled, "total", len(items))
	return filled, nil
}

// DeleteNode removes a node and all its associated edges and embeddings.
func (w *Writer) DeleteNode(ctx context.Context, nodeID string) error {
	nodeType, err := w.detectNodeType(ctx, nodeID)
	if err != nil {
		return err
	}

	table := nodeTypeToTable(nodeType)
	escaped := escapeDatalog(nodeID)

	// Delete the node itself.
	mutation := fmt.Sprintf(`?[id] <- [['%s']] :rm %s { id }`, escaped, table)
	if err := w.backend.Execute(ctx, mutation); err != nil {
		return fmt.Errorf("delete node %s: %w", nodeID, err)
	}

	// Delete embedding if applicable.
	embTable, embCol := embeddingTableForType(nodeType)
	if embTable != "" {
		embMut := fmt.Sprintf(`?[%s] <- [['%s']] :rm %s { %s }`, embCol, escaped, embTable, embCol)
		// Ignore error — embedding may not exist.
		_ = w.backend.Execute(ctx, embMut)
	}

	// Cascade-delete all edges referencing this node.
	if err := w.cascadeDeleteEdges(ctx, nodeType, nodeID); err != nil {
		return fmt.Errorf("cascade delete edges for %s: %w", nodeID, err)
	}

	return nil
}

// RemoveRelationship deletes a specific edge between two nodes.
func (w *Writer) RemoveRelationship(ctx context.Context, edgeType string, fields map[string]string) error {
	schema, ok := ValidEdgeTables[edgeType]
	if !ok {
		return fmt.Errorf("unknown edge type: %s", edgeType)
	}

	// Use only key columns for :rm operations.
	var colNames []string
	var colValues []string
	for _, col := range schema.Keys {
		val, exists := fields[col]
		if !exists {
			return fmt.Errorf("missing required field %q for edge type %s", col, edgeType)
		}
		colNames = append(colNames, col)
		colValues = append(colValues, fmt.Sprintf(`'%s'`, escapeDatalog(val)))
	}

	mutation := fmt.Sprintf(`?[%s] <- [[%s]] :rm %s { %s }`,
		strings.Join(colNames, ", "),
		strings.Join(colValues, ", "),
		edgeType,
		strings.Join(colNames, ", "),
	)

	if err := w.backend.Execute(ctx, mutation); err != nil {
		return fmt.Errorf("remove relationship %s: %w", edgeType, err)
	}
	return nil
}

// embeddingTableForType returns the embedding table and ID column for a node type.
func embeddingTableForType(nodeType string) (table, col string) {
	switch nodeType {
	case "fact":
		return "mie_fact_embedding", "fact_id"
	case "decision":
		return "mie_decision_embedding", "decision_id"
	case "entity":
		return "mie_entity_embedding", "entity_id"
	case "event":
		return "mie_event_embedding", "event_id"
	default:
		return "", ""
	}
}

// cascadeDeleteEdges removes all edges that reference the given node.
func (w *Writer) cascadeDeleteEdges(ctx context.Context, nodeType, nodeID string) error {
	escaped := escapeDatalog(nodeID)

	// Map of edge table → column that might reference this node type.
	var edgesToClean []struct{ table, col string }

	switch nodeType {
	case "fact":
		edgesToClean = []struct{ table, col string }{
			{"mie_fact_entity", "fact_id"},
			{"mie_fact_topic", "fact_id"},
			{"mie_invalidates", "new_fact_id"},
			{"mie_invalidates", "old_fact_id"},
		}
	case "entity":
		edgesToClean = []struct{ table, col string }{
			{"mie_fact_entity", "entity_id"},
			{"mie_decision_entity", "entity_id"},
			{"mie_entity_topic", "entity_id"},
		}
	case "decision":
		edgesToClean = []struct{ table, col string }{
			{"mie_decision_topic", "decision_id"},
			{"mie_decision_entity", "decision_id"},
			{"mie_event_decision", "decision_id"},
		}
	case "event":
		edgesToClean = []struct{ table, col string }{
			{"mie_event_decision", "event_id"},
		}
	case "topic":
		edgesToClean = []struct{ table, col string }{
			{"mie_fact_topic", "topic_id"},
			{"mie_decision_topic", "topic_id"},
			{"mie_entity_topic", "topic_id"},
		}
	}

	var errs []string
	for _, edge := range edgesToClean {
		schema, ok := ValidEdgeTables[edge.table]
		if !ok || len(schema.Keys) < 2 {
			continue
		}
		// Query all columns but only specify key columns in :rm.
		allCols := schema.AllColumns()
		allColList := strings.Join(allCols, ", ")
		keyColList := strings.Join(schema.Keys, ", ")
		script := fmt.Sprintf(`?[%s] := *%s { %s }, %s = '%s' :rm %s { %s }`,
			keyColList, edge.table, allColList, edge.col, escaped, edge.table, keyColList)
		if err := w.backend.Execute(ctx, script); err != nil {
			w.logger.Warn("cascade delete edge failed", "table", edge.table, "node_id", nodeID, "error", err)
			errs = append(errs, fmt.Sprintf("%s.%s: %v", edge.table, edge.col, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cascade delete errors: %s", strings.Join(errs, "; "))
	}
	return nil
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
