// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Export dumps the complete memory graph for backup or migration.
func Export(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	format := GetStringArg(args, "format", "json")
	if format != "json" && format != "datalog" {
		return NewError(fmt.Sprintf("Invalid format %q. Must be json or datalog", format)), nil
	}

	includeEmbeddings := GetBoolArg(args, "include_embeddings", false)
	nodeTypes := GetStringSliceArg(args, "node_types", []string{"fact", "decision", "entity", "event", "topic"})

	data, err := client.ExportGraph(ctx, ExportOptions{
		Format:            format,
		IncludeEmbeddings: includeEmbeddings,
		NodeTypes:         nodeTypes,
	})
	if err != nil {
		return NewError(fmt.Sprintf("Failed to export graph: %v", err)), nil
	}

	switch format {
	case "json":
		return exportJSON(data)
	case "datalog":
		return exportDatalog(data)
	default:
		return NewError("Unsupported format"), nil
	}
}

func exportJSON(data *ExportData) (*ToolResult, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return NewError(fmt.Sprintf("Failed to serialize export data: %v", err)), nil
	}

	output := string(jsonBytes)

	// Warn if output is very large
	if len(output) > 100000 {
		output = output[:100000] + "\n\n... (output truncated, export is " + fmt.Sprintf("%d", len(output)) + " bytes)"
	}

	return NewResult(output), nil
}

func exportDatalog(data *ExportData) (*ToolResult, error) {
	var sb strings.Builder
	sb.WriteString("// MIE Memory Export (Datalog format)\n")
	sb.WriteString(fmt.Sprintf("// Exported: %s\n\n", data.ExportedAt))

	// Export facts
	if data.Facts != nil {
		for _, f := range data.Facts {
			sb.WriteString(fmt.Sprintf(":put mie_fact { id: %q, content: %q, category: %q, confidence: %f, source_agent: %q, source_conversation: %q, valid: %s, created_at: %d, updated_at: %d }\n",
				f.ID, f.Content, f.Category, f.Confidence, f.SourceAgent, f.SourceConversation, boolToDatalog(f.Valid), f.CreatedAt, f.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export decisions
	if data.Decisions != nil {
		for _, d := range data.Decisions {
			sb.WriteString(fmt.Sprintf(":put mie_decision { id: %q, title: %q, rationale: %q, alternatives: %q, context: %q, source_agent: %q, source_conversation: %q, status: %q, created_at: %d, updated_at: %d }\n",
				d.ID, d.Title, d.Rationale, d.Alternatives, d.Context, d.SourceAgent, d.SourceConversation, d.Status, d.CreatedAt, d.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export entities
	if data.Entities != nil {
		for _, e := range data.Entities {
			sb.WriteString(fmt.Sprintf(":put mie_entity { id: %q, name: %q, kind: %q, description: %q, source_agent: %q, created_at: %d, updated_at: %d }\n",
				e.ID, e.Name, e.Kind, e.Description, e.SourceAgent, e.CreatedAt, e.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export events
	if data.Events != nil {
		for _, ev := range data.Events {
			sb.WriteString(fmt.Sprintf(":put mie_event { id: %q, title: %q, description: %q, event_date: %q, source_agent: %q, source_conversation: %q, created_at: %d, updated_at: %d }\n",
				ev.ID, ev.Title, ev.Description, ev.EventDate, ev.SourceAgent, ev.SourceConversation, ev.CreatedAt, ev.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export topics
	if data.Topics != nil {
		for _, t := range data.Topics {
			sb.WriteString(fmt.Sprintf(":put mie_topic { id: %q, name: %q, description: %q, created_at: %d, updated_at: %d }\n",
				t.ID, t.Name, t.Description, t.CreatedAt, t.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	output := sb.String()
	if len(output) > 100000 {
		output = output[:100000] + "\n\n// ... (output truncated)"
	}

	return NewResult(output), nil
}

func boolToDatalog(b bool) string {
	if b {
		return "true"
	}
	return "false"
}