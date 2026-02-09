// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// allSearchableNodeTypes lists the node types that support semantic search.
var allSearchableNodeTypes = []string{"fact", "decision", "entity", "event"}

// Analyze provides context for agent self-evaluation before storing new memory.
// It searches the existing memory graph for related nodes and returns a structured
// evaluation prompt for the agent to decide what to persist.
func Analyze(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	content := GetStringArg(args, "content", "")
	if content == "" {
		return NewError("Missing required parameter: content"), nil
	}

	var sb strings.Builder

	// Search for related nodes across all types
	var results []SearchResult
	if client.EmbeddingsEnabled() {
		var err error
		results, err = client.SemanticSearch(ctx, content, allSearchableNodeTypes, 10)
		if err != nil {
			// Non-fatal: continue without search results
			fmt.Fprintf(&sb, "_Note: Semantic search failed: %v_\n\n", err)
		}
	}

	// Filter out invalidated facts from results.
	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if f, ok := r.Metadata.(*Fact); ok && !f.Valid {
			continue
		}
		filtered = append(filtered, r)
	}
	results = filtered

	// Check for potential conflicts
	conflicts, err := client.CheckNewFactConflicts(ctx, content, "")
	if err != nil {
		fmt.Fprintf(&sb, "_Note: Conflict check failed: %v_\n\n", err)
	}

	// Build response
	sb.WriteString("## Existing Memory Context\n\n")

	if len(results) == 0 && !client.EmbeddingsEnabled() {
		sb.WriteString("_Embeddings are disabled. No semantic search results available._\n")
		sb.WriteString("_Enable embeddings in config for semantic memory search._\n\n")
	} else if len(results) == 0 {
		sb.WriteString("_No related memory found. This appears to be new information._\n\n")
	} else {
		formatAnalyzeResults(&sb, results)
	}

	// Conflicts section
	if len(conflicts) > 0 {
		sb.WriteString("### Potential Conflicts\n")
		for _, c := range conflicts {
			fmt.Fprintf(&sb, "- New content may conflict with [%s] %q (similarity: %.0f%%)\n",
				c.FactA.ID, Truncate(c.FactA.Content, 80), c.Similarity*100)
		}
		sb.WriteString("\n")
	}

	// Evaluation guide
	sb.WriteString("---\n\n")
	sb.WriteString("## Evaluation Guide\n\n")
	sb.WriteString("Given the existing memory context above, evaluate if the analyzed content contains:\n\n")
	sb.WriteString("1. **NEW FACT**: A personal truth not already captured (check Related Facts for duplicates)\n")
	sb.WriteString("2. **UPDATED FACT**: An existing fact that should be corrected -- if so, note which fact_id to invalidate\n")
	sb.WriteString("3. **DECISION**: A choice with clear rationale and alternatives considered\n")
	sb.WriteString("4. **NEW ENTITY**: A person, company, project, or technology not yet in the graph\n")
	sb.WriteString("5. **EVENT**: A timestamped occurrence worth recording\n\n")
	sb.WriteString("If you identify something to persist, call `mie_store` with the appropriate type.\n")
	sb.WriteString("If an existing fact needs correction, call `mie_update` to invalidate the old fact first.\n")
	sb.WriteString("If nothing is worth persisting, do nothing.\n\n")

	// Store schema reference
	sb.WriteString("### Store Schema Reference\n\n")
	sb.WriteString("For facts: `{\"type\": \"fact\", \"content\": \"...\", \"category\": \"personal|professional|preference|technical|relationship|general\", \"confidence\": 0.0-1.0}`\n")
	sb.WriteString("For decisions: `{\"type\": \"decision\", \"title\": \"...\", \"rationale\": \"...\", \"alternatives\": \"[...]\", \"context\": \"...\"}`\n")
	sb.WriteString("For entities: `{\"type\": \"entity\", \"name\": \"...\", \"kind\": \"person|company|project|product|technology|place\", \"description\": \"...\"}`\n")
	sb.WriteString("For events: `{\"type\": \"event\", \"title\": \"...\", \"description\": \"...\", \"event_date\": \"YYYY-MM-DD\"}`\n\n")
	sb.WriteString("Relationships can be added via: `{\"relationships\": [{\"edge\": \"fact_entity\", \"target_id\": \"ent:...\"}]}`\n")

	return NewResult(sb.String()), nil
}

func formatAnalyzeResults(sb *strings.Builder, results []SearchResult) {
	// Group results by node type
	grouped := map[string][]SearchResult{}
	for _, r := range results {
		grouped[r.NodeType] = append(grouped[r.NodeType], r)
	}

	typeOrder := []string{"fact", "decision", "entity", "event"}
	analyzeLabels := map[string]string{
		"fact":     "Related Facts",
		"decision": "Related Decisions",
		"entity":   "Related Entities",
		"event":    "Related Events",
	}

	for _, nt := range typeOrder {
		items, ok := grouped[nt]
		if !ok || len(items) == 0 {
			continue
		}
		fmt.Fprintf(sb, "### %s (%d found)\n", analyzeLabels[nt], len(items))
		for _, item := range items {
			pct := SimilarityPercent(item.Distance)
			indicator := SimilarityIndicator(item.Distance)
			detail := formatResultDetail(nt, item)
			fmt.Fprintf(sb, "- [%s] %q %s %s %d%%\n", item.ID, Truncate(item.Content, 80), detail, indicator, pct)
		}
		sb.WriteString("\n")
	}
}

func formatResultDetail(nodeType string, r SearchResult) string {
	switch nodeType {
	case "fact":
		if f, ok := r.Metadata.(*Fact); ok {
			return fmt.Sprintf("(confidence: %.1f, category: %s)", f.Confidence, f.Category)
		}
		return ""
	case "decision":
		if d, ok := r.Metadata.(*Decision); ok {
			return fmt.Sprintf("(status: %s)", d.Status)
		}
		return ""
	case "entity":
		if e, ok := r.Metadata.(*Entity); ok {
			return fmt.Sprintf("(kind: %s)", e.Kind)
		}
		return ""
	case "event":
		if ev, ok := r.Metadata.(*Event); ok {
			return fmt.Sprintf("(date: %s)", ev.EventDate)
		}
		return ""
	default:
		return ""
	}
}
