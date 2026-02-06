// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"testing"
)

func TestIsValidCategory(t *testing.T) {
	tests := []struct {
		cat  string
		want bool
	}{
		{"personal", true},
		{"professional", true},
		{"preference", true},
		{"technical", true},
		{"relationship", true},
		{"general", true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isValidCategory(tt.cat); got != tt.want {
			t.Errorf("isValidCategory(%q) = %v, want %v", tt.cat, got, tt.want)
		}
	}
}

func TestIsValidEntityKind(t *testing.T) {
	if !isValidEntityKind("person") {
		t.Error("'person' should be valid")
	}
	if isValidEntityKind("robot") {
		t.Error("'robot' should be invalid")
	}
}

func TestIsValidDecisionStatus(t *testing.T) {
	if !isValidDecisionStatus("active") {
		t.Error("'active' should be valid")
	}
	if isValidDecisionStatus("pending") {
		t.Error("'pending' should be invalid")
	}
}

func TestIsValidEntityRole(t *testing.T) {
	if !isValidEntityRole("subject") {
		t.Error("'subject' should be valid")
	}
	if isValidEntityRole("owner") {
		t.Error("'owner' should be invalid")
	}
}

func TestFormatVector(t *testing.T) {
	vec := []float32{0.1, -0.5, 0.9}
	result := formatVector(vec)
	if result == "" {
		t.Error("formatVector returned empty string")
	}
	if result[0] != '[' || result[len(result)-1] != ']' {
		t.Errorf("formatVector should wrap in brackets: %s", result)
	}

	empty := formatVector(nil)
	if empty != "[]" {
		t.Errorf("formatVector(nil) should return '[]': %s", empty)
	}
}

func TestEscapeDatalog(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{`say "hi"`, `say "hi"`},
		{`it's fine`, `it\'s fine`},
		{`path\to\file`, `path\\to\\file`},
	}
	for _, tt := range tests {
		if got := escapeDatalog(tt.input); got != tt.want {
			t.Errorf("escapeDatalog(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNodeTypeToTable(t *testing.T) {
	tests := []struct {
		nodeType string
		want     string
	}{
		{"fact", "mie_fact"},
		{"decision", "mie_decision"},
		{"entity", "mie_entity"},
		{"event", "mie_event"},
		{"topic", "mie_topic"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		if got := nodeTypeToTable(tt.nodeType); got != tt.want {
			t.Errorf("nodeTypeToTable(%q) = %q, want %q", tt.nodeType, got, tt.want)
		}
	}
}

func TestNodeTypeToEmbeddingTable(t *testing.T) {
	if got := nodeTypeToEmbeddingTable("fact"); got != "mie_fact_embedding" {
		t.Errorf("unexpected: %s", got)
	}
	if got := nodeTypeToEmbeddingTable("topic"); got != "" {
		t.Errorf("topic should not have embedding table: %s", got)
	}
}

func TestNodeTypeToHNSWIndex(t *testing.T) {
	if got := nodeTypeToHNSWIndex("fact"); got != "fact_embedding_idx" {
		t.Errorf("unexpected: %s", got)
	}
	if got := nodeTypeToHNSWIndex("topic"); got != "" {
		t.Errorf("topic should not have HNSW index: %s", got)
	}
}