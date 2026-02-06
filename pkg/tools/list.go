// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// validNodeTypes for listing.
var validNodeTypes = map[string]bool{
	"fact": true, "decision": true, "entity": true, "event": true, "topic": true,
}

// List returns memory nodes with filtering, pagination, and sorting.
func List(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeType := GetStringArg(args, "node_type", "")
	if nodeType == "" {
		return NewError("Missing required parameter: node_type"), nil
	}
	if !validNodeTypes[nodeType] {
		return NewError(fmt.Sprintf("Invalid node_type %q. Must be one of: fact, decision, entity, event, topic", nodeType)), nil
	}

	limit := GetIntArg(args, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	offset := GetIntArg(args, "offset", 0)
	if offset < 0 {
		offset = 0
	}

	opts := ListOptions{
		NodeType:  nodeType,
		Category:  GetStringArg(args, "category", ""),
		Kind:      GetStringArg(args, "kind", ""),
		Status:    GetStringArg(args, "status", ""),
		TopicName: GetStringArg(args, "topic", ""),
		ValidOnly: GetBoolArg(args, "valid_only", true),
		Limit:     limit,
		Offset:    offset,
		SortBy:    GetStringArg(args, "sort_by", "created_at"),
		SortOrder: GetStringArg(args, "sort_order", "desc"),
	}

	nodes, total, err := client.ListNodes(ctx, opts)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to list nodes: %v", err)), nil
	}

	var sb strings.Builder

	typeLabels := map[string]string{
		"fact": "Facts", "decision": "Decisions", "entity": "Entities", "event": "Events", "topic": "Topics",
	}
	label := typeLabels[nodeType]

	sb.WriteString(fmt.Sprintf("## %s (%d total, showing %d-%d)\n\n", label, total, offset+1, offset+len(nodes)))

	if len(nodes) == 0 {
		sb.WriteString("_No results found._\n")
		return NewResult(sb.String()), nil
	}

	formatNodeTable(&sb, nodeType, nodes, offset)

	// Pagination info
	if total > offset+len(nodes) {
		sb.WriteString(fmt.Sprintf("\nShowing %d of %d results. Use offset=%d for next page.\n", len(nodes), total, offset+limit))
	}

	return NewResult(sb.String()), nil
}

func formatNodeTable(sb *strings.Builder, nodeType string, nodes []any, offset int) {
	switch nodeType {
	case "fact":
		sb.WriteString("| # | ID | Content | Category | Confidence | Created |\n")
		sb.WriteString("|---|-----|---------|----------|------------|--------|\n")
		for i, node := range nodes {
			if f, ok := node.(*Fact); ok {
				fmt.Fprintf(sb, "| %d | %s | %s | %s | %.1f | %d |\n",
					offset+i+1, f.ID, Truncate(f.Content, 50), f.Category, f.Confidence, f.CreatedAt)
			}
		}

	case "decision":
		sb.WriteString("| # | ID | Title | Status | Created |\n")
		sb.WriteString("|---|-----|-------|--------|--------|\n")
		for i, node := range nodes {
			if d, ok := node.(*Decision); ok {
				fmt.Fprintf(sb, "| %d | %s | %s | %s | %d |\n",
					offset+i+1, d.ID, Truncate(d.Title, 60), d.Status, d.CreatedAt)
			}
		}

	case "entity":
		sb.WriteString("| # | ID | Name | Kind | Description |\n")
		sb.WriteString("|---|-----|------|------|------------|\n")
		for i, node := range nodes {
			if e, ok := node.(*Entity); ok {
				fmt.Fprintf(sb, "| %d | %s | %s | %s | %s |\n",
					offset+i+1, e.ID, e.Name, e.Kind, Truncate(e.Description, 40))
			}
		}

	case "event":
		sb.WriteString("| # | ID | Title | Date | Created |\n")
		sb.WriteString("|---|-----|-------|------|--------|\n")
		for i, node := range nodes {
			if ev, ok := node.(*Event); ok {
				fmt.Fprintf(sb, "| %d | %s | %s | %s | %d |\n",
					offset+i+1, ev.ID, Truncate(ev.Title, 60), ev.EventDate, ev.CreatedAt)
			}
		}

	case "topic":
		sb.WriteString("| # | ID | Name | Description |\n")
		sb.WriteString("|---|-----|------|------------|\n")
		for i, node := range nodes {
			if t, ok := node.(*Topic); ok {
				fmt.Fprintf(sb, "| %d | %s | %s | %s |\n",
					offset+i+1, t.ID, t.Name, Truncate(t.Description, 60))
			}
		}
	}
}