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
			sb.WriteString(fmt.Sprintf(":put mie_fact { id: '%s', content: '%s', category: '%s', confidence: %f, source_agent: '%s', source_conversation: '%s', valid: %s, created_at: %d, updated_at: %d }\n",
				escapeForDatalog(f.ID), escapeForDatalog(f.Content), escapeForDatalog(f.Category), f.Confidence, escapeForDatalog(f.SourceAgent), escapeForDatalog(f.SourceConversation), boolToDatalog(f.Valid), f.CreatedAt, f.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export decisions
	if data.Decisions != nil {
		for _, d := range data.Decisions {
			sb.WriteString(fmt.Sprintf(":put mie_decision { id: '%s', title: '%s', rationale: '%s', alternatives: '%s', context: '%s', source_agent: '%s', source_conversation: '%s', status: '%s', created_at: %d, updated_at: %d }\n",
				escapeForDatalog(d.ID), escapeForDatalog(d.Title), escapeForDatalog(d.Rationale), escapeForDatalog(d.Alternatives), escapeForDatalog(d.Context), escapeForDatalog(d.SourceAgent), escapeForDatalog(d.SourceConversation), escapeForDatalog(d.Status), d.CreatedAt, d.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export entities
	if data.Entities != nil {
		for _, e := range data.Entities {
			sb.WriteString(fmt.Sprintf(":put mie_entity { id: '%s', name: '%s', kind: '%s', description: '%s', source_agent: '%s', created_at: %d, updated_at: %d }\n",
				escapeForDatalog(e.ID), escapeForDatalog(e.Name), escapeForDatalog(e.Kind), escapeForDatalog(e.Description), escapeForDatalog(e.SourceAgent), e.CreatedAt, e.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export events
	if data.Events != nil {
		for _, ev := range data.Events {
			sb.WriteString(fmt.Sprintf(":put mie_event { id: '%s', title: '%s', description: '%s', event_date: '%s', source_agent: '%s', source_conversation: '%s', created_at: %d, updated_at: %d }\n",
				escapeForDatalog(ev.ID), escapeForDatalog(ev.Title), escapeForDatalog(ev.Description), escapeForDatalog(ev.EventDate), escapeForDatalog(ev.SourceAgent), escapeForDatalog(ev.SourceConversation), ev.CreatedAt, ev.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	// Export topics
	if data.Topics != nil {
		for _, t := range data.Topics {
			sb.WriteString(fmt.Sprintf(":put mie_topic { id: '%s', name: '%s', description: '%s', created_at: %d, updated_at: %d }\n",
				escapeForDatalog(t.ID), escapeForDatalog(t.Name), escapeForDatalog(t.Description), t.CreatedAt, t.UpdatedAt))
		}
		sb.WriteString("\n")
	}

	output := sb.String()
	if len(output) > 100000 {
		output = output[:100000] + "\n\n// ... (output truncated)"
	}

	return NewResult(output), nil
}

// escapeForDatalog escapes a string for use in CozoDB single-quoted Datalog literals.
func escapeForDatalog(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

func boolToDatalog(b bool) string {
	if b {
		return "true"
	}
	return "false"
}