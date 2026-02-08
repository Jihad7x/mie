// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"fmt"
	"strings"
)

// ValidFactCategories lists valid categories for facts.
var ValidFactCategories = []string{
	"personal",
	"professional",
	"preference",
	"technical",
	"relationship",
	"general",
}

// ValidEntityKinds lists valid kinds for entities.
var ValidEntityKinds = []string{
	"person",
	"company",
	"project",
	"product",
	"technology",
	"place",
	"other",
}

// ValidDecisionStatuses lists valid statuses for decisions.
var ValidDecisionStatuses = []string{
	"active",
	"superseded",
	"reversed",
}

// ValidEntityRoles lists valid roles for decision-entity relationships.
var ValidEntityRoles = []string{
	"subject",
	"alternative",
	"stakeholder",
	"context",
}

// ValidEdgeTables maps edge table names to their key columns.
var ValidEdgeTables = map[string][]string{
	"mie_invalidates":     {"new_fact_id", "old_fact_id"},
	"mie_decision_topic":  {"decision_id", "topic_id"},
	"mie_decision_entity": {"decision_id", "entity_id"},
	"mie_event_decision":  {"event_id", "decision_id"},
	"mie_fact_entity":     {"fact_id", "entity_id"},
	"mie_fact_topic":      {"fact_id", "topic_id"},
	"mie_entity_topic":    {"entity_id", "topic_id"},
}

func isValidCategory(cat string) bool {
	for _, c := range ValidFactCategories {
		if c == cat {
			return true
		}
	}
	return false
}

func isValidEntityKind(kind string) bool {
	for _, k := range ValidEntityKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func isValidDecisionStatus(status string) bool {
	for _, s := range ValidDecisionStatuses {
		if s == status {
			return true
		}
	}
	return false
}

func isValidEntityRole(role string) bool {
	for _, r := range ValidEntityRoles {
		if r == role {
			return true
		}
	}
	return false
}

// formatVector converts a float32 slice to CozoDB vec() format.
// Example output: "[0.123000, -0.456000, 0.789000]"
func formatVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteString("[")
	for i, f := range v {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%f", f))
	}
	sb.WriteString("]")
	return sb.String()
}

// escapeDatalog escapes a string for safe embedding in single-quoted Datalog queries.
// CozoDB single-quoted strings support \' for literal single quotes and \\ for backslashes.
// Double quotes do not need escaping inside single-quoted strings.
func escapeDatalog(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\x00", `\0`)
	return s
}

// nodeTypeToTable maps a node type string to its CozoDB table name.
func nodeTypeToTable(nodeType string) string {
	switch nodeType {
	case "fact":
		return "mie_fact"
	case "decision":
		return "mie_decision"
	case "entity":
		return "mie_entity"
	case "event":
		return "mie_event"
	case "topic":
		return "mie_topic"
	default:
		return ""
	}
}

// nodeTypeToEmbeddingTable maps a node type to its embedding table.
func nodeTypeToEmbeddingTable(nodeType string) string {
	switch nodeType {
	case "fact":
		return "mie_fact_embedding"
	case "decision":
		return "mie_decision_embedding"
	case "entity":
		return "mie_entity_embedding"
	case "event":
		return "mie_event_embedding"
	default:
		return ""
	}
}

// nodeTypeToHNSWIndex maps a node type to its HNSW index name.
func nodeTypeToHNSWIndex(nodeType string) string {
	switch nodeType {
	case "fact":
		return "fact_embedding_idx"
	case "decision":
		return "decision_embedding_idx"
	case "entity":
		return "entity_embedding_idx"
	case "event":
		return "event_embedding_idx"
	default:
		return ""
	}
}