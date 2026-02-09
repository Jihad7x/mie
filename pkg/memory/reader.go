// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

// Reader handles all queries against the memory graph.
type Reader struct {
	backend  storage.Backend
	embedder *EmbeddingGenerator
	logger   *slog.Logger
}

// NewReader creates a new Reader.
func NewReader(backend storage.Backend, embedder *EmbeddingGenerator, logger *slog.Logger) *Reader {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reader{backend: backend, embedder: embedder, logger: logger}
}

// SemanticSearch performs vector similarity search across the memory graph.
func (r *Reader) SemanticSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]tools.SearchResult, error) {
	if r.embedder == nil {
		return nil, fmt.Errorf("semantic search requires embeddings to be enabled")
	}
	if limit <= 0 {
		limit = 10
	}

	queryEmb, err := r.embedder.GenerateQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	vecStr := formatVector(queryEmb)
	var results []tools.SearchResult
	var searchErrors []string

	if len(nodeTypes) == 0 {
		nodeTypes = []string{"fact", "decision", "entity", "event"}
	}

	for _, nt := range nodeTypes {
		var script string
		switch nt {
		case "fact":
			script = fmt.Sprintf(`?[id, content, category, confidence, valid, created_at, distance] :=
    ~mie_fact_embedding:fact_embedding_idx { fact_id | query: q, k: %d, ef: 200, bind_distance: distance },
    q = vec(%s),
    *mie_fact { id: fact_id, content, category, confidence, valid, created_at },
    id = fact_id
    :order distance
    :limit %d`, limit*5, vecStr, limit)
		case "decision":
			script = fmt.Sprintf(`?[id, title, rationale, status, created_at, distance] :=
    ~mie_decision_embedding:decision_embedding_idx { decision_id | query: q, k: %d, ef: 200, bind_distance: distance },
    q = vec(%s),
    *mie_decision { id: decision_id, title, rationale, status, created_at },
    id = decision_id
    :order distance
    :limit %d`, limit*5, vecStr, limit)
		case "entity":
			script = fmt.Sprintf(`?[id, name, kind, description, created_at, distance] :=
    ~mie_entity_embedding:entity_embedding_idx { entity_id | query: q, k: %d, ef: 200, bind_distance: distance },
    q = vec(%s),
    *mie_entity { id: entity_id, name, kind, description, created_at },
    id = entity_id
    :order distance
    :limit %d`, limit*5, vecStr, limit)
		case "event":
			script = fmt.Sprintf(`?[id, title, description, event_date, created_at, distance] :=
    ~mie_event_embedding:event_embedding_idx { event_id | query: q, k: %d, ef: 200, bind_distance: distance },
    q = vec(%s),
    *mie_event { id: event_id, title, description, event_date, created_at },
    id = event_id
    :order distance
    :limit %d`, limit*5, vecStr, limit)
		default:
			continue
		}

		qr, err := r.backend.Query(ctx, script)
		if err != nil {
			r.logger.Warn("semantic search failed for type", "type", nt, "error", err)
			searchErrors = append(searchErrors, fmt.Sprintf("%s: %v", nt, err))
			continue
		}

		for _, row := range qr.Rows {
			sr := r.parseSearchResult(nt, row, qr.Headers)
			results = append(results, sr)
		}
	}

	// Boost results whose content/name/title contains the query (case-insensitive).
	lowerQuery := strings.ToLower(query)
	for i := range results {
		text := strings.ToLower(results[i].Content)
		if strings.Contains(text, lowerQuery) {
			results[i].Distance *= 0.5
			if results[i].Distance < 0.001 {
				results[i].Distance = 0.001
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	// Filter out very low similarity results (distance > 0.6 means < 40% similarity).
	filtered := results[:0]
	for _, r := range results {
		if r.Distance <= 0.6 {
			filtered = append(filtered, r)
		}
	}
	results = filtered

	if len(results) > limit {
		results = results[:limit]
	}

	// Surface HNSW query errors: return results (if any) alongside a non-nil error
	// so the caller can show a warning about partial failures.
	if len(searchErrors) > 0 {
		return results, fmt.Errorf("partial search failure: %s", strings.Join(searchErrors, "; "))
	}

	return results, nil
}

// ExactSearch performs substring matching across the memory graph.
func (r *Reader) ExactSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]tools.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	escaped := escapeDatalog(strings.ToLower(query))
	var results []tools.SearchResult

	if len(nodeTypes) == 0 {
		nodeTypes = []string{"fact", "decision", "entity", "event", "topic"}
	}

	// Distribute limit across types so no single type dominates results.
	perTypeLimit := limit
	if len(nodeTypes) > 1 {
		perTypeLimit = (limit / len(nodeTypes)) + 1
	}

	for _, nt := range nodeTypes {
		var script string
		switch nt {
		case "fact":
			script = fmt.Sprintf(`?[id, content, category, confidence, valid, created_at] :=
    *mie_fact { id, content, category, confidence, valid, created_at },
    str_includes(lowercase(content), '%s')
    :limit %d`, escaped, perTypeLimit)
		case "decision":
			script = fmt.Sprintf(`?[id, title, rationale, status, created_at] :=
    *mie_decision { id, title, rationale, status, created_at },
    or(str_includes(lowercase(title), '%s'), str_includes(lowercase(rationale), '%s'))
    :limit %d`, escaped, escaped, perTypeLimit)
		case "entity":
			script = fmt.Sprintf(`?[id, name, kind, description, created_at] :=
    *mie_entity { id, name, kind, description, created_at },
    or(str_includes(lowercase(name), '%s'), str_includes(lowercase(description), '%s'))
    :limit %d`, escaped, escaped, perTypeLimit)
		case "event":
			script = fmt.Sprintf(`?[id, title, description, event_date, created_at] :=
    *mie_event { id, title, description, event_date, created_at },
    or(str_includes(lowercase(title), '%s'), str_includes(lowercase(description), '%s'))
    :limit %d`, escaped, escaped, perTypeLimit)
		case "topic":
			script = fmt.Sprintf(`?[id, name, description, created_at] :=
    *mie_topic { id, name, description, created_at },
    or(str_includes(lowercase(name), '%s'), str_includes(lowercase(description), '%s'))
    :limit %d`, escaped, escaped, perTypeLimit)
		default:
			continue
		}

		qr, err := r.backend.Query(ctx, script)
		if err != nil {
			r.logger.Warn("exact search failed for type", "type", nt, "error", err)
			continue
		}

		for _, row := range qr.Rows {
			sr := r.parseSearchResult(nt, row, qr.Headers)
			results = append(results, sr)
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ListNodes returns a paginated list of nodes matching the given options.
func (r *Reader) ListNodes(ctx context.Context, opts tools.ListOptions) ([]any, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	table := nodeTypeToTable(opts.NodeType)
	if table == "" {
		return nil, 0, fmt.Errorf("unknown node type: %s", opts.NodeType)
	}

	conditions := buildListConditions(opts)
	columns := columnsForNodeType(opts.NodeType)

	condStr := ""
	if len(conditions) > 0 {
		condStr = ", " + strings.Join(conditions, ", ")
	}

	sortBy := opts.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	// Valid sort columns per node type (map "name" alias to actual column).
	validSortByType := map[string]map[string]string{
		"fact":     {"created_at": "created_at", "updated_at": "updated_at", "id": "id", "content": "content", "name": "content"},
		"decision": {"created_at": "created_at", "updated_at": "updated_at", "id": "id", "title": "title", "name": "title"},
		"entity":   {"created_at": "created_at", "updated_at": "updated_at", "id": "id", "name": "name"},
		"event":    {"created_at": "created_at", "updated_at": "updated_at", "id": "id", "title": "title", "name": "title"},
		"topic":    {"created_at": "created_at", "updated_at": "updated_at", "id": "id", "name": "name"},
	}
	typeMap := validSortByType[opts.NodeType]
	if typeMap != nil {
		if mapped, ok := typeMap[sortBy]; ok {
			sortBy = mapped
		} else {
			sortBy = "created_at"
		}
	}
	sortOrder := sortBy
	if opts.SortOrder != "asc" {
		sortOrder = "-" + sortBy
	}

	// When TopicName is set, resolve to topic ID and join against the edge table.
	topicJoin := ""
	topicCountJoin := ""
	if opts.TopicName != "" {
		topicID, err := r.resolveTopicName(ctx, opts.TopicName)
		if err != nil {
			return nil, 0, err
		}
		if topicID == "" {
			return nil, 0, nil // topic not found → no results
		}
		edgeTable, fkCol := topicEdgeForNodeType(opts.NodeType)
		if edgeTable == "" {
			return nil, 0, fmt.Errorf("topic filter not supported for node type %q", opts.NodeType)
		}
		topicJoin = fmt.Sprintf(`, *%s { %s, topic_id }, topic_id = '%s', id = %s`,
			edgeTable, fkCol, escapeDatalog(topicID), fkCol)
		topicCountJoin = topicJoin
	}

	script := fmt.Sprintf(`?[%s] := *%s { %s }%s%s :order %s :limit %d :offset %d`,
		columns, table, columns, condStr, topicJoin, sortOrder, opts.Limit, opts.Offset,
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, 0, fmt.Errorf("list nodes: %w", err)
	}

	totalCount, err := r.countNodesWithJoin(ctx, table, conditions, condStr, topicCountJoin)
	if err != nil {
		return nil, 0, err
	}

	var nodes []any
	for _, row := range qr.Rows {
		node := r.parseNode(opts.NodeType, row, qr.Headers)
		if node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, totalCount, nil
}

// resolveTopicName looks up a topic ID by name (case-insensitive).
func (r *Reader) resolveTopicName(ctx context.Context, name string) (string, error) {
	escaped := escapeDatalog(strings.ToLower(name))
	script := fmt.Sprintf(`?[id] := *mie_topic { id, name }, lname = lowercase(name), lname = '%s' :limit 1`, escaped)
	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return "", fmt.Errorf("resolve topic name: %w", err)
	}
	if len(qr.Rows) == 0 {
		return "", nil
	}
	return toString(qr.Rows[0][0]), nil
}

// topicEdgeForNodeType returns the edge table and foreign key column for topic filtering.
func topicEdgeForNodeType(nodeType string) (edgeTable, fkCol string) {
	switch nodeType {
	case "fact":
		return "mie_fact_topic", "fact_id"
	case "decision":
		return "mie_decision_topic", "decision_id"
	case "entity":
		return "mie_entity_topic", "entity_id"
	default:
		return "", ""
	}
}

// buildListConditions builds filter conditions for a ListNodes query.
func buildListConditions(opts tools.ListOptions) []string {
	var conditions []string
	switch opts.NodeType {
	case "fact":
		if opts.Category != "" {
			conditions = append(conditions, fmt.Sprintf(`category = '%s'`, escapeDatalog(opts.Category)))
		}
		if opts.ValidOnly {
			conditions = append(conditions, `valid = true`)
		}
	case "decision":
		if opts.Status != "" {
			conditions = append(conditions, fmt.Sprintf(`status = '%s'`, escapeDatalog(opts.Status)))
		}
	case "entity":
		if opts.Kind != "" {
			conditions = append(conditions, fmt.Sprintf(`kind = '%s'`, escapeDatalog(opts.Kind)))
		}
	}

	// Time-range filtering (applies to all node types).
	if opts.CreatedAfter > 0 {
		conditions = append(conditions, fmt.Sprintf(`created_at >= %d`, opts.CreatedAfter))
	}
	if opts.CreatedBefore > 0 {
		conditions = append(conditions, fmt.Sprintf(`created_at <= %d`, opts.CreatedBefore))
	}

	return conditions
}

// columnsForNodeType returns the column list for a given node type.
func columnsForNodeType(nodeType string) string {
	switch nodeType {
	case "fact":
		return "id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at"
	case "decision":
		return "id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at"
	case "entity":
		return "id, name, kind, description, source_agent, created_at, updated_at"
	case "event":
		return "id, title, description, event_date, source_agent, source_conversation, created_at, updated_at"
	case "topic":
		return "id, name, description, created_at, updated_at"
	default:
		return "id"
	}
}

// countNodesWithJoin executes a count query with an optional extra join clause.
func (r *Reader) countNodesWithJoin(ctx context.Context, table string, conditions []string, condStr, joinStr string) (int, error) {
	colSet := map[string]bool{"id": true}
	for _, cond := range conditions {
		// Extract column name from conditions like "col = 'val'", "col >= N", "col <= N".
		for _, op := range []string{" = ", " >= ", " <= "} {
			if idx := strings.Index(cond, op); idx > 0 {
				col := strings.TrimSpace(cond[:idx])
				colSet[col] = true
				break
			}
		}
	}
	countCols := make([]string, 0, len(colSet))
	for col := range colSet {
		countCols = append(countCols, col)
	}
	sort.Strings(countCols)
	countScript := fmt.Sprintf(`?[count(id)] := *%s { %s }%s%s`,
		table, strings.Join(countCols, ", "), condStr, joinStr)
	countResult, err := r.backend.Query(ctx, countScript)
	if err != nil {
		return 0, fmt.Errorf("count nodes: %w", err)
	}

	totalCount := 0
	if len(countResult.Rows) > 0 {
		if v, ok := countResult.Rows[0][0].(float64); ok {
			totalCount = int(v)
		} else if v, ok := countResult.Rows[0][0].(int); ok {
			totalCount = v
		}
	}
	return totalCount, nil
}

// GetNodeByID retrieves a single node by its ID.
func (r *Reader) GetNodeByID(ctx context.Context, nodeID string) (any, error) {
	// Detect node type from prefix
	nodeType := ""
	if len(nodeID) >= 4 {
		switch {
		case strings.HasPrefix(nodeID, "ent:"):
			nodeType = "entity"
		case strings.HasPrefix(nodeID, "evt:"):
			nodeType = "event"
		case strings.HasPrefix(nodeID, "dec:"):
			nodeType = "decision"
		case strings.HasPrefix(nodeID, "top:"):
			nodeType = "topic"
		case strings.HasPrefix(nodeID, "fact:"):
			nodeType = "fact"
		}
	}

	if nodeType != "" {
		node, err := r.getNodeByType(ctx, nodeID, nodeType)
		if err != nil {
			return nil, err
		}
		if node == nil {
			return nil, fmt.Errorf("node %q not found", nodeID)
		}
		return node, nil
	}

	// Fallback: try all types
	types := []string{"fact", "decision", "entity", "event", "topic"}
	for _, nt := range types {
		node, err := r.getNodeByType(ctx, nodeID, nt)
		if err == nil && node != nil {
			return node, nil
		}
	}

	return nil, fmt.Errorf("node %q not found", nodeID)
}

func (r *Reader) getNodeByType(ctx context.Context, nodeID, nodeType string) (any, error) {
	table := nodeTypeToTable(nodeType)
	if table == "" {
		return nil, fmt.Errorf("unknown node type: %s", nodeType)
	}

	var columns string
	switch nodeType {
	case "fact":
		columns = "id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at"
	case "decision":
		columns = "id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at"
	case "entity":
		columns = "id, name, kind, description, source_agent, created_at, updated_at"
	case "event":
		columns = "id, title, description, event_date, source_agent, source_conversation, created_at, updated_at"
	case "topic":
		columns = "id, name, description, created_at, updated_at"
	}

	script := fmt.Sprintf(`?[%s] := *%s { %s }, id = '%s'`, columns, table, columns, escapeDatalog(nodeID))

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}

	if len(qr.Rows) == 0 {
		return nil, nil
	}

	return r.parseNode(nodeType, qr.Rows[0], qr.Headers), nil
}

// FindEntityByName finds an entity by its name (case-insensitive).
func (r *Reader) FindEntityByName(ctx context.Context, name string) (*tools.Entity, error) {
	escaped := escapeDatalog(strings.ToLower(name))
	script := fmt.Sprintf(
		`?[id, name, kind, description, source_agent, created_at, updated_at] :=
    *mie_entity { id, name, kind, description, source_agent, created_at, updated_at },
    lname = lowercase(name),
    lname = '%s'
    :limit 1`, escaped,
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}

	if len(qr.Rows) == 0 {
		return nil, nil
	}

	node := r.parseNode("entity", qr.Rows[0], qr.Headers)
	if ent, ok := node.(*tools.Entity); ok {
		return ent, nil
	}
	return nil, nil
}

// FindFactByContent finds a fact by matching content.
func (r *Reader) FindFactByContent(ctx context.Context, content string) (*tools.Fact, error) {
	escaped := escapeDatalog(content)
	script := fmt.Sprintf(
		`?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] :=
    *mie_fact { id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at },
    str_includes(content, '%s')
    :limit 1`, escaped,
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}

	if len(qr.Rows) == 0 {
		return nil, nil
	}

	node := r.parseNode("fact", qr.Rows[0], qr.Headers)
	if fact, ok := node.(*tools.Fact); ok {
		return fact, nil
	}
	return nil, nil
}

// FindDecisionByTitle finds a decision by matching title.
func (r *Reader) FindDecisionByTitle(ctx context.Context, title string) (*tools.Decision, error) {
	escaped := escapeDatalog(title)
	script := fmt.Sprintf(
		`?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] :=
    *mie_decision { id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at },
    str_includes(title, '%s')
    :limit 1`, escaped,
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}

	if len(qr.Rows) == 0 {
		return nil, nil
	}

	node := r.parseNode("decision", qr.Rows[0], qr.Headers)
	if dec, ok := node.(*tools.Decision); ok {
		return dec, nil
	}
	return nil, nil
}

// GetRelatedEntities returns entities related to a given fact.
func (r *Reader) GetRelatedEntities(ctx context.Context, factID string) ([]tools.Entity, error) {
	script := fmt.Sprintf(
		`?[id, name, kind, description, source_agent, created_at, updated_at] :=
    *mie_fact_entity { fact_id, entity_id },
    fact_id = '%s',
    *mie_entity { id: entity_id, name, kind, description, source_agent, created_at, updated_at },
    id = entity_id`, escapeDatalog(factID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get related entities: %w", err)
	}

	var entities []tools.Entity
	for _, row := range qr.Rows {
		node := r.parseNode("entity", row, qr.Headers)
		if ent, ok := node.(*tools.Entity); ok {
			entities = append(entities, *ent)
		}
	}

	return entities, nil
}

// GetFactsAboutEntity returns facts associated with a given entity.
func (r *Reader) GetFactsAboutEntity(ctx context.Context, entityID string) ([]tools.Fact, error) {
	script := fmt.Sprintf(
		`?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] :=
    *mie_fact_entity { fact_id, entity_id },
    entity_id = '%s',
    *mie_fact { id: fact_id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at },
    id = fact_id`, escapeDatalog(entityID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get facts about entity: %w", err)
	}

	var facts []tools.Fact
	for _, row := range qr.Rows {
		node := r.parseNode("fact", row, qr.Headers)
		if fact, ok := node.(*tools.Fact); ok {
			facts = append(facts, *fact)
		}
	}

	return facts, nil
}

// GetDecisionEntities returns entities involved in a given decision.
func (r *Reader) GetDecisionEntities(ctx context.Context, decisionID string) ([]tools.EntityWithRole, error) {
	script := fmt.Sprintf(
		`?[id, name, kind, description, source_agent, created_at, updated_at, role] :=
    *mie_decision_entity { decision_id, entity_id, role },
    decision_id = '%s',
    *mie_entity { id: entity_id, name, kind, description, source_agent, created_at, updated_at },
    id = entity_id`, escapeDatalog(decisionID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get decision entities: %w", err)
	}

	var entities []tools.EntityWithRole
	for _, row := range qr.Rows {
		ent := tools.EntityWithRole{}
		ent.ID = toString(row[0])
		ent.Name = toString(row[1])
		ent.Kind = toString(row[2])
		ent.Description = toString(row[3])
		ent.SourceAgent = toString(row[4])
		ent.CreatedAt = toInt64(row[5])
		ent.UpdatedAt = toInt64(row[6])
		ent.Role = toString(row[7])
		entities = append(entities, ent)
	}

	return entities, nil
}

// GetInvalidationChain returns the chain of fact invalidations for a given fact.
func (r *Reader) GetInvalidationChain(ctx context.Context, factID string) ([]tools.Invalidation, error) {
	escaped := escapeDatalog(factID)
	// CozoDB or() doesn't work with = comparisons; use rule union (;) instead
	script := fmt.Sprintf(
		`?[new_fact_id, old_fact_id, reason, old_content, new_content] :=
    *mie_invalidates { new_fact_id, old_fact_id, reason },
    new_fact_id = '%s',
    *mie_fact { id: old_fact_id, content: old_content },
    *mie_fact { id: new_fact_id, content: new_content };
?[new_fact_id, old_fact_id, reason, old_content, new_content] :=
    *mie_invalidates { new_fact_id, old_fact_id, reason },
    old_fact_id = '%s',
    *mie_fact { id: old_fact_id, content: old_content },
    *mie_fact { id: new_fact_id, content: new_content }`,
		escaped, escaped,
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get invalidation chain: %w", err)
	}

	var chain []tools.Invalidation
	for _, row := range qr.Rows {
		inv := tools.Invalidation{
			NewFactID:  toString(row[0]),
			OldFactID:  toString(row[1]),
			Reason:     toString(row[2]),
			OldContent: toString(row[3]),
			NewContent: toString(row[4]),
		}
		chain = append(chain, inv)
	}

	return chain, nil
}

// GetRelatedFacts returns facts related to a given entity (alias for GetFactsAboutEntity).
func (r *Reader) GetRelatedFacts(ctx context.Context, entityID string) ([]tools.Fact, error) {
	return r.GetFactsAboutEntity(ctx, entityID)
}

// GetEntityDecisions returns decisions involving a given entity.
func (r *Reader) GetEntityDecisions(ctx context.Context, entityID string) ([]tools.Decision, error) {
	script := fmt.Sprintf(
		`?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] :=
    *mie_decision_entity { decision_id, entity_id },
    entity_id = '%s',
    *mie_decision { id: decision_id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at },
    id = decision_id`, escapeDatalog(entityID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get entity decisions: %w", err)
	}

	var decisions []tools.Decision
	for _, row := range qr.Rows {
		node := r.parseNode("decision", row, qr.Headers)
		if dec, ok := node.(*tools.Decision); ok {
			decisions = append(decisions, *dec)
		}
	}

	return decisions, nil
}

// GetFactsAboutTopic returns facts associated with a given topic.
func (r *Reader) GetFactsAboutTopic(ctx context.Context, topicID string) ([]tools.Fact, error) {
	script := fmt.Sprintf(
		`?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] :=
    *mie_fact_topic { fact_id, topic_id },
    topic_id = '%s',
    *mie_fact { id: fact_id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at },
    id = fact_id`, escapeDatalog(topicID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get facts about topic: %w", err)
	}

	var facts []tools.Fact
	for _, row := range qr.Rows {
		node := r.parseNode("fact", row, qr.Headers)
		if fact, ok := node.(*tools.Fact); ok {
			facts = append(facts, *fact)
		}
	}

	return facts, nil
}

// GetDecisionsAboutTopic returns decisions associated with a given topic.
func (r *Reader) GetDecisionsAboutTopic(ctx context.Context, topicID string) ([]tools.Decision, error) {
	script := fmt.Sprintf(
		`?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] :=
    *mie_decision_topic { decision_id, topic_id },
    topic_id = '%s',
    *mie_decision { id: decision_id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at },
    id = decision_id`, escapeDatalog(topicID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get decisions about topic: %w", err)
	}

	var decisions []tools.Decision
	for _, row := range qr.Rows {
		node := r.parseNode("decision", row, qr.Headers)
		if dec, ok := node.(*tools.Decision); ok {
			decisions = append(decisions, *dec)
		}
	}

	return decisions, nil
}

// GetEntitiesAboutTopic returns entities associated with a given topic.
func (r *Reader) GetEntitiesAboutTopic(ctx context.Context, topicID string) ([]tools.Entity, error) {
	script := fmt.Sprintf(
		`?[id, name, kind, description, source_agent, created_at, updated_at] :=
    *mie_entity_topic { entity_id, topic_id },
    topic_id = '%s',
    *mie_entity { id: entity_id, name, kind, description, source_agent, created_at, updated_at },
    id = entity_id`, escapeDatalog(topicID),
	)

	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("get entities about topic: %w", err)
	}

	var entities []tools.Entity
	for _, row := range qr.Rows {
		node := r.parseNode("entity", row, qr.Headers)
		if ent, ok := node.(*tools.Entity); ok {
			entities = append(entities, *ent)
		}
	}

	return entities, nil
}

// GetStats returns memory graph statistics.
func (r *Reader) GetStats(ctx context.Context) (*tools.GraphStats, error) {
	stats := &tools.GraphStats{}

	queries := []struct {
		query string
		dest  *int
	}{
		{`?[count(id)] := *mie_fact { id }`, &stats.TotalFacts},
		{`?[count(id)] := *mie_fact { id, valid }, valid = true`, &stats.ValidFacts},
		{`?[count(id)] := *mie_fact { id, valid }, valid = false`, &stats.InvalidatedFacts},
		{`?[count(id)] := *mie_decision { id }`, &stats.TotalDecisions},
		{`?[count(id)] := *mie_decision { id, status }, status = 'active'`, &stats.ActiveDecisions},
		{`?[count(id)] := *mie_entity { id }`, &stats.TotalEntities},
		{`?[count(id)] := *mie_event { id }`, &stats.TotalEvents},
		{`?[count(id)] := *mie_topic { id }`, &stats.TotalTopics},
	}

	for _, q := range queries {
		result, err := r.backend.Query(ctx, q.query)
		if err != nil {
			r.logger.Warn("stats query failed", "query", q.query, "error", err)
			continue
		}
		if len(result.Rows) > 0 {
			*q.dest = toInt(result.Rows[0][0])
		}
	}

	// Count total edges across all edge tables
	edgeTables := []string{
		"mie_invalidates", "mie_decision_topic", "mie_decision_entity",
		"mie_event_decision", "mie_fact_entity", "mie_fact_topic", "mie_entity_topic",
	}
	totalEdges := 0
	for _, et := range edgeTables {
		schema := ValidEdgeTables[et]
		if len(schema.Keys) < 2 {
			continue
		}
		allCols := schema.AllColumns()
		query := fmt.Sprintf(`?[count(%s)] := *%s { %s }`, schema.Keys[0], et, strings.Join(allCols, ", "))
		result, err := r.backend.Query(ctx, query)
		if err != nil {
			continue
		}
		if len(result.Rows) > 0 {
			totalEdges += toInt(result.Rows[0][0])
		}
	}
	stats.TotalEdges = totalEdges

	// Read metadata values (schema version, counters, timestamps).
	metaKeys := []struct {
		key    string
		setter func(string)
	}{
		{"schema_version", func(v string) { stats.SchemaVersion = v }},
		{"total_queries", func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				stats.TotalQueries = n
			}
		}},
		{"total_stores", func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				stats.TotalStores = n
			}
		}},
		{"last_query_at", func(v string) {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				stats.LastQueryAt = n
			}
		}},
		{"last_store_at", func(v string) {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				stats.LastStoreAt = n
			}
		}},
	}

	for _, mk := range metaKeys {
		q := fmt.Sprintf(`?[value] := *mie_meta { key, value }, key = '%s'`, mk.key)
		result, err := r.backend.Query(ctx, q)
		if err == nil && len(result.Rows) > 0 {
			mk.setter(toString(result.Rows[0][0]))
		}
	}

	return stats, nil
}

// ExportGraph exports the complete memory graph.
func (r *Reader) ExportGraph(ctx context.Context, opts tools.ExportOptions) (*tools.ExportData, error) {
	export := &tools.ExportData{
		Version:    "1",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Stats:      make(map[string]int),
	}

	nodeTypes := opts.NodeTypes
	if len(nodeTypes) == 0 {
		nodeTypes = []string{"fact", "decision", "entity", "event", "topic"}
	}

	for _, nt := range nodeTypes {
		switch nt {
		case "fact":
			facts, err := r.exportFacts(ctx)
			if err != nil {
				return nil, err
			}
			export.Facts = facts
			export.Stats["facts"] = len(facts)

		case "decision":
			decisions, err := r.exportDecisions(ctx)
			if err != nil {
				return nil, err
			}
			export.Decisions = decisions
			export.Stats["decisions"] = len(decisions)

		case "entity":
			entities, err := r.exportEntities(ctx)
			if err != nil {
				return nil, err
			}
			export.Entities = entities
			export.Stats["entities"] = len(entities)

		case "event":
			events, err := r.exportEvents(ctx)
			if err != nil {
				return nil, err
			}
			export.Events = events
			export.Stats["events"] = len(events)

		case "topic":
			topics, err := r.exportTopics(ctx)
			if err != nil {
				return nil, err
			}
			export.Topics = topics
			export.Stats["topics"] = len(topics)
		}
	}

	// Export relationships (edges), filtered to only edges relevant to requested node types.
	edges, totalEdges := r.exportEdges(ctx, nodeTypes)
	if totalEdges > 0 {
		export.Edges = edges
		export.Stats["edges"] = totalEdges
	}

	return export, nil
}

// edgeRequiredNodeTypes maps each edge table to the node types both endpoints must have.
var edgeRequiredNodeTypes = map[string][2]string{
	"mie_fact_entity":     {"fact", "entity"},
	"mie_fact_topic":      {"fact", "topic"},
	"mie_decision_topic":  {"decision", "topic"},
	"mie_decision_entity": {"decision", "entity"},
	"mie_event_decision":  {"event", "decision"},
	"mie_entity_topic":    {"entity", "topic"},
	"mie_invalidates":     {"fact", "fact"},
}

// exportEdges exports relationship edges, filtered to only edges where at least one
// endpoint node type is in the requested set.
func (r *Reader) exportEdges(ctx context.Context, nodeTypes []string) (map[string]any, int) {
	edges := make(map[string]any)
	totalEdges := 0

	typeSet := make(map[string]bool, len(nodeTypes))
	for _, nt := range nodeTypes {
		typeSet[nt] = true
	}

	for tableName, schema := range ValidEdgeTables {
		if len(schema.Keys) < 2 {
			continue
		}

		// Filter: export edges where at least one endpoint type is in the requested set.
		if req, ok := edgeRequiredNodeTypes[tableName]; ok {
			if !typeSet[req[0]] && !typeSet[req[1]] {
				continue
			}
		}

		allCols := schema.AllColumns()
		colList := strings.Join(allCols, ", ")
		script := fmt.Sprintf(`?[%s] := *%s { %s }`, colList, tableName, colList)

		qr, err := r.backend.Query(ctx, script)
		if err != nil {
			r.logger.Warn("export edges failed", "table", tableName, "error", err)
			continue
		}
		if len(qr.Rows) == 0 {
			continue
		}

		var rows []map[string]string
		for _, row := range qr.Rows {
			entry := make(map[string]string)
			for i, col := range allCols {
				if i < len(row) {
					entry[col] = toString(row[i])
				}
			}
			rows = append(rows, entry)
		}
		// Use short name (strip "mie_" prefix) as key.
		shortName := strings.TrimPrefix(tableName, "mie_")
		edges[shortName] = rows
		totalEdges += len(rows)
	}

	return edges, totalEdges
}

// --- Export helpers ---

func (r *Reader) exportFacts(ctx context.Context) ([]tools.Fact, error) {
	script := `?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] := *mie_fact { id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at }`
	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}
	var facts []tools.Fact
	for _, row := range qr.Rows {
		node := r.parseNode("fact", row, qr.Headers)
		if f, ok := node.(*tools.Fact); ok {
			facts = append(facts, *f)
		}
	}
	return facts, nil
}

func (r *Reader) exportDecisions(ctx context.Context) ([]tools.Decision, error) {
	script := `?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] := *mie_decision { id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at }`
	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}
	var decisions []tools.Decision
	for _, row := range qr.Rows {
		node := r.parseNode("decision", row, qr.Headers)
		if d, ok := node.(*tools.Decision); ok {
			decisions = append(decisions, *d)
		}
	}
	return decisions, nil
}

func (r *Reader) exportEntities(ctx context.Context) ([]tools.Entity, error) {
	script := `?[id, name, kind, description, source_agent, created_at, updated_at] := *mie_entity { id, name, kind, description, source_agent, created_at, updated_at }`
	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}
	var entities []tools.Entity
	for _, row := range qr.Rows {
		node := r.parseNode("entity", row, qr.Headers)
		if e, ok := node.(*tools.Entity); ok {
			entities = append(entities, *e)
		}
	}
	return entities, nil
}

func (r *Reader) exportEvents(ctx context.Context) ([]tools.Event, error) {
	script := `?[id, title, description, event_date, source_agent, source_conversation, created_at, updated_at] := *mie_event { id, title, description, event_date, source_agent, source_conversation, created_at, updated_at }`
	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}
	var events []tools.Event
	for _, row := range qr.Rows {
		node := r.parseNode("event", row, qr.Headers)
		if e, ok := node.(*tools.Event); ok {
			events = append(events, *e)
		}
	}
	return events, nil
}

func (r *Reader) exportTopics(ctx context.Context) ([]tools.Topic, error) {
	script := `?[id, name, description, created_at, updated_at] := *mie_topic { id, name, description, created_at, updated_at }`
	qr, err := r.backend.Query(ctx, script)
	if err != nil {
		return nil, err
	}
	var topics []tools.Topic
	for _, row := range qr.Rows {
		node := r.parseNode("topic", row, qr.Headers)
		if t, ok := node.(*tools.Topic); ok {
			topics = append(topics, *t)
		}
	}
	return topics, nil
}

// --- Parsing helpers ---

func (r *Reader) parseSearchResult(nodeType string, row []any, headers []string) tools.SearchResult {
	sr := tools.SearchResult{
		NodeType: nodeType,
	}

	// Detect whether this is a semantic result (has "distance" column) or exact.
	hasDist := len(headers) > 0 && headers[len(headers)-1] == "distance"

	switch nodeType {
	case "fact":
		// Semantic: id(0), content(1), category(2), confidence(3), valid(4), created_at(5), distance(6)
		// Exact:    id(0), content(1), category(2), confidence(3), valid(4), created_at(5)
		sr.ID = toString(row[0])
		sr.Content = toString(row[1])
		sr.Detail = toString(row[2])
		if hasDist {
			sr.Distance = toFloat64(row[6])
		}
		sr.Metadata = &tools.Fact{
			ID:         sr.ID,
			Content:    sr.Content,
			Category:   toString(row[2]),
			Confidence: toFloat64(row[3]),
			Valid:      toBool(row[4]),
			CreatedAt:  toInt64(row[5]),
		}
	case "decision":
		// Semantic: id(0), title(1), rationale(2), status(3), created_at(4), distance(5)
		// Exact:    id(0), title(1), rationale(2), status(3), created_at(4)
		sr.ID = toString(row[0])
		sr.Content = toString(row[1])
		sr.Detail = toString(row[2])
		if hasDist {
			sr.Distance = toFloat64(row[5])
		}
		sr.Metadata = &tools.Decision{
			ID:        sr.ID,
			Title:     sr.Content,
			Rationale: toString(row[2]),
			Status:    toString(row[3]),
			CreatedAt: toInt64(row[4]),
		}
	case "entity":
		// Semantic: id(0), name(1), kind(2), description(3), created_at(4), distance(5)
		// Exact:    id(0), name(1), kind(2), description(3), created_at(4)
		sr.ID = toString(row[0])
		sr.Content = toString(row[1])
		sr.Detail = toString(row[3])
		if hasDist {
			sr.Distance = toFloat64(row[5])
		}
		sr.Metadata = &tools.Entity{
			ID:          sr.ID,
			Name:        sr.Content,
			Kind:        toString(row[2]),
			Description: toString(row[3]),
			CreatedAt:   toInt64(row[4]),
		}
	case "event":
		// Semantic: id(0), title(1), description(2), event_date(3), created_at(4), distance(5)
		// Exact:    id(0), title(1), description(2), event_date(3), created_at(4)
		sr.ID = toString(row[0])
		sr.Content = toString(row[1])
		sr.Detail = toString(row[2])
		if hasDist {
			sr.Distance = toFloat64(row[5])
		}
		sr.Metadata = &tools.Event{
			ID:        sr.ID,
			Title:     sr.Content,
			EventDate: toString(row[3]),
			CreatedAt: toInt64(row[4]),
		}
	case "topic":
		// Exact: id(0), name(1), description(2), created_at(3)
		sr.ID = toString(row[0])
		sr.Content = toString(row[1])
		if len(row) > 2 {
			sr.Detail = toString(row[2])
		}
		sr.Metadata = &tools.Topic{
			ID:        sr.ID,
			Name:      sr.Content,
			CreatedAt: toInt64(row[3]),
		}
	}

	return sr
}

func (r *Reader) parseNode(nodeType string, row []any, headers []string) any {
	_ = headers
	switch nodeType {
	case "fact":
		if len(row) < 9 {
			return nil
		}
		return &tools.Fact{
			ID:                 toString(row[0]),
			Content:            toString(row[1]),
			Category:           toString(row[2]),
			Confidence:         toFloat64(row[3]),
			SourceAgent:        toString(row[4]),
			SourceConversation: toString(row[5]),
			Valid:              toBool(row[6]),
			CreatedAt:          toInt64(row[7]),
			UpdatedAt:          toInt64(row[8]),
		}
	case "decision":
		if len(row) < 10 {
			return nil
		}
		return &tools.Decision{
			ID:                 toString(row[0]),
			Title:              toString(row[1]),
			Rationale:          toString(row[2]),
			Alternatives:       toString(row[3]),
			Context:            toString(row[4]),
			SourceAgent:        toString(row[5]),
			SourceConversation: toString(row[6]),
			Status:             toString(row[7]),
			CreatedAt:          toInt64(row[8]),
			UpdatedAt:          toInt64(row[9]),
		}
	case "entity":
		if len(row) < 7 {
			return nil
		}
		return &tools.Entity{
			ID:          toString(row[0]),
			Name:        toString(row[1]),
			Kind:        toString(row[2]),
			Description: toString(row[3]),
			SourceAgent: toString(row[4]),
			CreatedAt:   toInt64(row[5]),
			UpdatedAt:   toInt64(row[6]),
		}
	case "event":
		if len(row) < 8 {
			return nil
		}
		return &tools.Event{
			ID:                 toString(row[0]),
			Title:              toString(row[1]),
			Description:        toString(row[2]),
			EventDate:          toString(row[3]),
			SourceAgent:        toString(row[4]),
			SourceConversation: toString(row[5]),
			CreatedAt:          toInt64(row[6]),
			UpdatedAt:          toInt64(row[7]),
		}
	case "topic":
		if len(row) < 5 {
			return nil
		}
		return &tools.Topic{
			ID:          toString(row[0]),
			Name:        toString(row[1]),
			Description: toString(row[2]),
			CreatedAt:   toInt64(row[3]),
			UpdatedAt:   toInt64(row[4]),
		}
	}
	return nil
}

// --- Type conversion helpers ---

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

func toInt(v any) int {
	return int(toInt64(v))
}

func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func toBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
