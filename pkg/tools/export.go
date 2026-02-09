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

// edgeSchema describes the key and value columns for an edge table.
type edgeSchema struct {
	Keys   []string
	Values []string
}

func (e edgeSchema) allColumns() []string {
	all := make([]string, 0, len(e.Keys)+len(e.Values))
	all = append(all, e.Keys...)
	all = append(all, e.Values...)
	return all
}

// exportEdgeTables maps edge table names to their key/value column schema.
// Must be kept in sync with memory.ValidEdgeTables.
var exportEdgeTables = map[string]edgeSchema{
	"mie_invalidates":     {Keys: []string{"new_fact_id", "old_fact_id"}, Values: []string{"reason"}},
	"mie_decision_topic":  {Keys: []string{"decision_id", "topic_id"}},
	"mie_decision_entity": {Keys: []string{"decision_id", "entity_id"}, Values: []string{"role"}},
	"mie_event_decision":  {Keys: []string{"event_id", "decision_id"}},
	"mie_fact_entity":     {Keys: []string{"fact_id", "entity_id"}},
	"mie_fact_topic":      {Keys: []string{"fact_id", "topic_id"}},
	"mie_entity_topic":    {Keys: []string{"entity_id", "topic_id"}},
}

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

	var result *ToolResult
	switch format {
	case "json":
		result, err = exportJSON(data)
	case "datalog":
		result, err = exportDatalog(data)
	default:
		return NewError("Unsupported format"), nil
	}
	if err != nil {
		return nil, err
	}

	if includeEmbeddings && data.Stats != nil {
		embCount := data.Stats["fact_embeddings"] + data.Stats["decision_embeddings"] + data.Stats["entity_embeddings"] + data.Stats["event_embeddings"]
		if embCount > 0 {
			result.Text += "\n\nNote: Embedding vectors included. Use mie_repair to rebuild HNSW indexes after import."
		}
	}

	return result, nil
}

func exportJSON(data *ExportData) (*ToolResult, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return NewError(fmt.Sprintf("Failed to serialize export data: %v", err)), nil
	}

	output := string(jsonBytes)

	// Warn if output is very large
	if len(output) > 100000 {
		fullLen := len(output)
		output = output[:100000] + "\n\n... (output truncated, export is " + fmt.Sprintf("%d", fullLen) + " bytes)"
	}

	return NewResult(output), nil
}

func exportDatalog(data *ExportData) (*ToolResult, error) {
	var sb strings.Builder
	totalNodes := 0
	sb.WriteString("// MIE Memory Export (Datalog format)\n")
	sb.WriteString(fmt.Sprintf("// Exported: %s\n\n", data.ExportedAt))

	// Export facts
	// Schema: mie_fact { id => content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at }
	if data.Facts != nil {
		for _, f := range data.Facts {
			sb.WriteString(fmt.Sprintf("?[id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at] <- [['%s', '%s', '%s', %g, '%s', '%s', %s, %d, %d]] :put mie_fact { id => content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at }\n",
				escapeForDatalog(f.ID), escapeForDatalog(f.Content), escapeForDatalog(f.Category), f.Confidence, escapeForDatalog(f.SourceAgent), escapeForDatalog(f.SourceConversation), boolToDatalog(f.Valid), f.CreatedAt, f.UpdatedAt))
		}
		totalNodes += len(data.Facts)
		sb.WriteString("\n")
	}

	// Export decisions
	// Schema: mie_decision { id => title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at }
	if data.Decisions != nil {
		for _, d := range data.Decisions {
			sb.WriteString(fmt.Sprintf("?[id, title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at] <- [['%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', %d, %d]] :put mie_decision { id => title, rationale, alternatives, context, source_agent, source_conversation, status, created_at, updated_at }\n",
				escapeForDatalog(d.ID), escapeForDatalog(d.Title), escapeForDatalog(d.Rationale), escapeForDatalog(d.Alternatives), escapeForDatalog(d.Context), escapeForDatalog(d.SourceAgent), escapeForDatalog(d.SourceConversation), escapeForDatalog(d.Status), d.CreatedAt, d.UpdatedAt))
		}
		totalNodes += len(data.Decisions)
		sb.WriteString("\n")
	}

	// Export entities
	// Schema: mie_entity { id => name, kind, description, source_agent, created_at, updated_at }
	if data.Entities != nil {
		for _, e := range data.Entities {
			sb.WriteString(fmt.Sprintf("?[id, name, kind, description, source_agent, created_at, updated_at] <- [['%s', '%s', '%s', '%s', '%s', %d, %d]] :put mie_entity { id => name, kind, description, source_agent, created_at, updated_at }\n",
				escapeForDatalog(e.ID), escapeForDatalog(e.Name), escapeForDatalog(e.Kind), escapeForDatalog(e.Description), escapeForDatalog(e.SourceAgent), e.CreatedAt, e.UpdatedAt))
		}
		totalNodes += len(data.Entities)
		sb.WriteString("\n")
	}

	// Export events
	// Schema: mie_event { id => title, description, event_date, source_agent, source_conversation, created_at, updated_at }
	if data.Events != nil {
		for _, ev := range data.Events {
			sb.WriteString(fmt.Sprintf("?[id, title, description, event_date, source_agent, source_conversation, created_at, updated_at] <- [['%s', '%s', '%s', '%s', '%s', '%s', %d, %d]] :put mie_event { id => title, description, event_date, source_agent, source_conversation, created_at, updated_at }\n",
				escapeForDatalog(ev.ID), escapeForDatalog(ev.Title), escapeForDatalog(ev.Description), escapeForDatalog(ev.EventDate), escapeForDatalog(ev.SourceAgent), escapeForDatalog(ev.SourceConversation), ev.CreatedAt, ev.UpdatedAt))
		}
		totalNodes += len(data.Events)
		sb.WriteString("\n")
	}

	// Export topics
	// Schema: mie_topic { id => name, description, created_at, updated_at }
	if data.Topics != nil {
		for _, t := range data.Topics {
			sb.WriteString(fmt.Sprintf("?[id, name, description, created_at, updated_at] <- [['%s', '%s', '%s', %d, %d]] :put mie_topic { id => name, description, created_at, updated_at }\n",
				escapeForDatalog(t.ID), escapeForDatalog(t.Name), escapeForDatalog(t.Description), t.CreatedAt, t.UpdatedAt))
		}
		totalNodes += len(data.Topics)
		sb.WriteString("\n")
	}

	// Export relationships using ValidEdgeTables schema for proper key/value separation.
	if data.Edges != nil {
		for edgeName, rows := range data.Edges {
			tableName := "mie_" + edgeName
			schema, hasSchema := exportEdgeTables[tableName]
			if !hasSchema {
				continue
			}
			allCols := schema.allColumns()
			keyCols := schema.Keys
			var valueCols []string
			if len(schema.Values) > 0 {
				valueCols = schema.Values
			}

			putSuffix := strings.Join(keyCols, ", ")
			if len(valueCols) > 0 {
				putSuffix += " => " + strings.Join(valueCols, ", ")
			}

			writeEdgeRow := func(rowData map[string]string) {
				var colNames []string
				var colValues []string
				for _, col := range allCols {
					colNames = append(colNames, col)
					colValues = append(colValues, fmt.Sprintf("'%s'", escapeForDatalog(rowData[col])))
				}
				sb.WriteString(fmt.Sprintf("?[%s] <- [[%s]] :put %s { %s }\n",
					strings.Join(colNames, ", "),
					strings.Join(colValues, ", "),
					tableName,
					putSuffix))
			}

			switch typedRows := rows.(type) {
			case []map[string]string:
				for _, row := range typedRows {
					writeEdgeRow(row)
				}
			case []any:
				for _, item := range typedRows {
					if row, ok := item.(map[string]any); ok {
						strRow := make(map[string]string, len(row))
						for k, v := range row {
							strRow[k] = fmt.Sprint(v)
						}
						writeEdgeRow(strRow)
					}
				}
			default:
				continue
			}
			sb.WriteString("\n")
		}
	}

	output := sb.String()
	if len(output) > 100000 {
		output = output[:100000] + fmt.Sprintf("\n\n// ... (output truncated at 100KB, showing %d total nodes. Use node_types filter to reduce size.)", totalNodes)
	}

	return NewResult(output), nil
}

// escapeForDatalog escapes a string for use in CozoDB single-quoted Datalog literals.
// NOTE: Identical implementation exists in pkg/memory/helpers.go:escapeDatalog.
// Keep both in sync — they are in different packages and cannot be shared without
// adding a dependency from tools -> memory.
func escapeForDatalog(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\x00", `\0`)
	return s
}

func boolToDatalog(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
