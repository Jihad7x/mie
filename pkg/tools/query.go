// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// Query reads from the memory graph. Supports semantic search, exact lookup, and graph traversal.
func Query(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	query := GetStringArg(args, "query", "")
	if query == "" {
		return NewError("Query must not be empty"), nil
	}

	mode := GetStringArg(args, "mode", "semantic")
	defaultTypes := []string{"fact", "decision", "entity", "event"}
	nodeTypes := GetStringSliceArg(args, "node_types", defaultTypes)
	limit := GetIntArg(args, "limit", 10)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	// Optional filters for search results.
	filters := searchFilters{
		Category:      GetStringArg(args, "category", ""),
		Kind:          GetStringArg(args, "kind", ""),
		ValidOnly:     GetBoolArg(args, "valid_only", true),
		CreatedAfter:  int64(GetFloat64Arg(args, "created_after", 0)),
		CreatedBefore: int64(GetFloat64Arg(args, "created_before", 0)),
	}

	var result *ToolResult
	var err error
	switch mode {
	case "semantic":
		result, err = querySemanticMode(ctx, client, query, nodeTypes, limit, filters)
	case "exact":
		// Include topics in exact search when using defaults
		if !hasUserSpecifiedNodeTypes(args) {
			nodeTypes = append(nodeTypes, "topic")
		}
		result, err = queryExactMode(ctx, client, query, nodeTypes, limit, filters)
	case "graph":
		result, err = queryGraphMode(ctx, client, args)
	default:
		return NewError(fmt.Sprintf("Invalid mode %q. Must be one of: semantic, exact, graph", mode)), nil
	}

	// Increment usage counter on success (never fail the main operation).
	if err == nil && result != nil && !result.IsError {
		_ = client.IncrementCounter(ctx, "total_queries")
	}

	return result, err
}

func hasUserSpecifiedNodeTypes(args map[string]any) bool {
	_, ok := args["node_types"]
	return ok
}

// searchFilters holds optional filters for search results.
type searchFilters struct {
	Category      string // Filter facts by category.
	Kind          string // Filter entities by kind.
	ValidOnly     bool   // Only return valid facts.
	CreatedAfter  int64  // Only return nodes created at or after this unix timestamp.
	CreatedBefore int64  // Only return nodes created at or before this unix timestamp.
}

// applySearchFilters filters search results by category, kind, valid_only, and time range.
func applySearchFilters(results []SearchResult, f searchFilters) []SearchResult {
	needsFiltering := f.Category != "" || f.Kind != "" || f.CreatedAfter > 0 || f.CreatedBefore > 0
	if !needsFiltering && f.ValidOnly {
		hasInvalid := false
		for _, r := range results {
			if fact, ok := r.Metadata.(*Fact); ok && !fact.Valid {
				hasInvalid = true
				break
			}
		}
		if !hasInvalid {
			return results
		}
	}

	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if !matchesSearchFilters(r, f) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// matchesSearchFilters returns true if a single result passes all filters.
func matchesSearchFilters(r SearchResult, f searchFilters) bool {
	var createdAt int64
	switch m := r.Metadata.(type) {
	case *Fact:
		if f.ValidOnly && !m.Valid {
			return false
		}
		if f.Category != "" && m.Category != f.Category {
			return false
		}
		createdAt = m.CreatedAt
	case *Entity:
		if f.Kind != "" && m.Kind != f.Kind {
			return false
		}
		if f.Category != "" {
			return false // category filter implies facts only
		}
		createdAt = m.CreatedAt
	case *Decision:
		if f.Category != "" {
			return false // category filter implies facts only
		}
		if f.Kind != "" {
			return false // kind filter implies entities only
		}
		createdAt = m.CreatedAt
	case *Event:
		if f.Category != "" {
			return false // category filter implies facts only
		}
		if f.Kind != "" {
			return false // kind filter implies entities only
		}
		createdAt = m.CreatedAt
	}
	if f.CreatedAfter > 0 && createdAt < f.CreatedAfter {
		return false
	}
	if f.CreatedBefore > 0 && createdAt > f.CreatedBefore {
		return false
	}
	return true
}

func querySemanticMode(ctx context.Context, client Querier, query string, nodeTypes []string, limit int, filters searchFilters) (*ToolResult, error) {
	if !client.EmbeddingsEnabled() {
		return NewError("Semantic search requires embeddings to be enabled. Enable in config or use mode=exact."), nil
	}

	results, searchErr := client.SemanticSearch(ctx, query, nodeTypes, limit)
	if searchErr != nil && len(results) == 0 {
		return NewError(fmt.Sprintf("Semantic search failed: %v", searchErr)), nil
	}

	results = applySearchFilters(results, filters)

	if len(results) == 0 {
		return NewResult(fmt.Sprintf("## Memory Search Results for: %q\n\n_No results found._\n", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Memory Search Results for: %q\n\n", query))

	if searchErr != nil {
		sb.WriteString(fmt.Sprintf("_Warning: %v. Results may be incomplete._\n\n", searchErr))
	}

	// Warn if all results have low confidence (similarity < 40%).
	allLow := true
	for _, r := range results {
		if SimilarityPercent(r.Distance) >= 40 {
			allLow = false
			break
		}
	}
	if allLow {
		sb.WriteString("_Note: No high-confidence matches found. Results below may not be relevant._\n\n")
	}

	// Group results by type
	grouped := map[string][]SearchResult{}
	for _, r := range results {
		grouped[r.NodeType] = append(grouped[r.NodeType], r)
	}

	for _, nt := range nodeTypes {
		items, ok := grouped[nt]
		if !ok || len(items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%d results)\n", TypeLabels[nt], len(items)))
		for i, item := range items {
			pct := SimilarityPercent(item.Distance)
			indicator := SimilarityIndicator(item.Distance)
			sb.WriteString(fmt.Sprintf("%d. %s %d%% [%s] %q\n", i+1, indicator, pct, item.ID, Truncate(item.Content, 100)))
			if item.Detail != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", item.Detail))
			}
			if item.Metadata != nil {
				if f, ok := item.Metadata.(*Fact); ok && !f.Valid {
					sb.WriteString("   [INVALIDATED]\n")
				}
			}
		}
		sb.WriteString("\n")
	}

	return NewResult(sb.String()), nil
}

func queryExactMode(ctx context.Context, client Querier, query string, nodeTypes []string, limit int, filters searchFilters) (*ToolResult, error) {
	results, err := client.ExactSearch(ctx, query, nodeTypes, limit)
	if err != nil {
		return NewError(fmt.Sprintf("Exact search failed: %v", err)), nil
	}

	results = applySearchFilters(results, filters)

	if len(results) == 0 {
		return NewResult(fmt.Sprintf("## Exact Search Results for: %q\n\n_No results found._\n", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Exact Search Results for: %q\n\n", query))

	grouped := map[string][]SearchResult{}
	for _, r := range results {
		grouped[r.NodeType] = append(grouped[r.NodeType], r)
	}

	for _, nt := range nodeTypes {
		items, ok := grouped[nt]
		if !ok || len(items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%d results)\n", TypeLabels[nt], len(items)))
		for i, item := range items {
			sb.WriteString(fmt.Sprintf("%d. [%s] %q\n", i+1, item.ID, Truncate(item.Content, 100)))
			if item.Detail != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", item.Detail))
			}
			if item.Metadata != nil {
				if f, ok := item.Metadata.(*Fact); ok && !f.Valid {
					sb.WriteString("   [INVALIDATED]\n")
				}
			}
		}
		sb.WriteString("\n")
	}

	return NewResult(sb.String()), nil
}

func queryGraphMode(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeID := GetStringArg(args, "node_id", "")
	if nodeID == "" {
		// If query looks like a node ID, use it as node_id.
		query := GetStringArg(args, "query", "")
		for _, prefix := range []string{"fact:", "ent:", "dec:", "evt:", "top:"} {
			if strings.HasPrefix(query, prefix) {
				nodeID = query
				break
			}
		}
		if nodeID == "" {
			return NewError("node_id is required for graph mode"), nil
		}
	}

	traversal := GetStringArg(args, "traversal", "")
	if traversal == "" {
		return NewError("traversal is required for graph mode"), nil
	}

	// Verify the node exists before traversal.
	if _, err := client.GetNodeByID(ctx, nodeID); err != nil {
		return NewError(fmt.Sprintf("Node %q not found", nodeID)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Graph Traversal: %s from [%s]\n\n", traversal, nodeID)

	var err error
	switch traversal {
	case "related_entities":
		err = traverseRelatedEntities(ctx, client, &sb, nodeID)
	case "related_facts", "facts_about_entity":
		err = traverseRelatedFacts(ctx, client, &sb, nodeID)
	case "invalidation_chain":
		err = traverseInvalidationChain(ctx, client, &sb, nodeID)
	case "decision_entities":
		err = traverseDecisionEntities(ctx, client, &sb, nodeID)
	case "entity_decisions":
		err = traverseEntityDecisions(ctx, client, &sb, nodeID)
	case "facts_about_topic":
		err = traverseFactsAboutTopic(ctx, client, &sb, nodeID)
	case "decisions_about_topic":
		err = traverseDecisionsAboutTopic(ctx, client, &sb, nodeID)
	case "entities_about_topic":
		err = traverseEntitiesAboutTopic(ctx, client, &sb, nodeID)
	default:
		return NewError(fmt.Sprintf("Invalid traversal type %q. Must be one of: related_entities, related_facts, invalidation_chain, decision_entities, facts_about_entity, entity_decisions, facts_about_topic, decisions_about_topic, entities_about_topic", traversal)), nil
	}

	if err != nil {
		return NewError(fmt.Sprintf("Traversal failed: %v", err)), nil
	}

	return NewResult(sb.String()), nil
}

func traverseRelatedEntities(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	entities, err := client.GetRelatedEntities(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		sb.WriteString("_No related entities found._\n")
		return nil
	}
	for i, e := range entities {
		fmt.Fprintf(sb, "%d. [%s] %q (kind: %s)\n", i+1, e.ID, e.Name, e.Kind)
		if e.Description != "" {
			fmt.Fprintf(sb, "   %s\n", Truncate(e.Description, 100))
		}
	}
	return nil
}

func traverseRelatedFacts(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	facts, err := client.GetFactsAboutEntity(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		sb.WriteString("_No related facts found._\n")
		return nil
	}
	for i, f := range facts {
		validStr := "valid"
		if !f.Valid {
			validStr = "invalidated"
		}
		fmt.Fprintf(sb, "%d. [%s] %q (category: %s, confidence: %g, %s)\n",
			i+1, f.ID, Truncate(f.Content, 100), f.Category, f.Confidence, validStr)
	}
	return nil
}

func traverseInvalidationChain(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	chain, err := client.GetInvalidationChain(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(chain) == 0 {
		sb.WriteString("_No invalidation chain found._\n")
		return nil
	}
	for i, inv := range chain {
		fmt.Fprintf(sb, "%d. [%s] -> [%s]\n", i+1, inv.NewFactID, inv.OldFactID)
		fmt.Fprintf(sb, "   Reason: %s\n", inv.Reason)
		if inv.OldContent != "" {
			fmt.Fprintf(sb, "   Old: %q\n", Truncate(inv.OldContent, 80))
		}
		if inv.NewContent != "" {
			fmt.Fprintf(sb, "   New: %q\n", Truncate(inv.NewContent, 80))
		}
	}
	return nil
}

func traverseDecisionEntities(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	entities, err := client.GetDecisionEntities(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		sb.WriteString("_No related entities found for this decision._\n")
		return nil
	}
	for i, e := range entities {
		fmt.Fprintf(sb, "%d. [%s] %q (kind: %s, role: %s)\n",
			i+1, e.ID, e.Name, e.Kind, e.Role)
	}
	return nil
}

func traverseEntityDecisions(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	decisions, err := client.GetEntityDecisions(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(decisions) == 0 {
		sb.WriteString("_No related decisions found for this entity._\n")
		return nil
	}
	for i, d := range decisions {
		fmt.Fprintf(sb, "%d. [%s] %q (status: %s)\n",
			i+1, d.ID, Truncate(d.Title, 100), d.Status)
	}
	return nil
}

func traverseFactsAboutTopic(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	facts, err := client.GetFactsAboutTopic(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		sb.WriteString("_No related facts found._\n")
		return nil
	}
	for i, f := range facts {
		validStr := "valid"
		if !f.Valid {
			validStr = "invalidated"
		}
		fmt.Fprintf(sb, "%d. [%s] %q (category: %s, confidence: %g, %s)\n",
			i+1, f.ID, Truncate(f.Content, 100), f.Category, f.Confidence, validStr)
	}
	return nil
}

func traverseDecisionsAboutTopic(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	decisions, err := client.GetDecisionsAboutTopic(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(decisions) == 0 {
		sb.WriteString("_No related decisions found for this topic._\n")
		return nil
	}
	for i, d := range decisions {
		fmt.Fprintf(sb, "%d. [%s] %q (status: %s)\n",
			i+1, d.ID, Truncate(d.Title, 100), d.Status)
	}
	return nil
}

func traverseEntitiesAboutTopic(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	entities, err := client.GetEntitiesAboutTopic(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		sb.WriteString("_No related entities found for this topic._\n")
		return nil
	}
	for i, e := range entities {
		fmt.Fprintf(sb, "%d. [%s] %q (kind: %s)\n", i+1, e.ID, e.Name, e.Kind)
		if e.Description != "" {
			fmt.Fprintf(sb, "   %s\n", Truncate(e.Description, 100))
		}
	}
	return nil
}
