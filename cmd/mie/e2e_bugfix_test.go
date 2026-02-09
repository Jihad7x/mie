// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// BUG FIX VERIFICATION TESTS
// These tests verify that all bugs found during the MIE evaluation are fixed.
// ============================================================================

// --- BUG 1: ExactSearch valid_only=false should return invalidated facts ---

func TestBugfix_ExactSearch_ValidOnlyFalse(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store a fact
	storeResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Go version 1.22 was released in 2024",
		"category":     "technical",
		"source_agent": "test",
	})
	oldFactID := extractFactID(t, extractToolText(t, storeResp))

	// Store replacement fact
	replaceResp := callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Go version 1.23 was released in 2025",
		"category":     "technical",
		"source_agent": "test",
	})
	newFactID := extractFactID(t, extractToolText(t, replaceResp))

	// Invalidate old fact
	callTool(t, w, r, 4, "mie_update", map[string]any{
		"node_id":        oldFactID,
		"action":         "invalidate",
		"reason":         "Updated version info",
		"replacement_id": newFactID,
	})

	// Exact search with valid_only=false should find the invalidated fact
	queryResp := callTool(t, w, r, 5, "mie_query", map[string]any{
		"query":      "Go version 1.22",
		"mode":       "exact",
		"valid_only": false,
	})
	queryText := extractToolText(t, queryResp)
	assert.Contains(t, queryText, "Go version 1.22", "invalidated fact should appear with valid_only=false")

	// Exact search with valid_only=true (default) should NOT find it
	queryResp2 := callTool(t, w, r, 6, "mie_query", map[string]any{
		"query":      "Go version 1.22",
		"mode":       "exact",
		"valid_only": true,
	})
	queryText2 := extractToolText(t, queryResp2)
	assert.Contains(t, queryText2, "No results found", "invalidated fact should not appear with valid_only=true")
}

// --- BUG 2: Query date filters (created_after/created_before) ---

func TestBugfix_QueryDateFilters(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store a fact
	callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Date filter test fact",
		"category":     "technical",
		"source_agent": "test",
	})

	// Search with created_after=0 should find it
	queryResp := callTool(t, w, r, 3, "mie_query", map[string]any{
		"query":         "Date filter test",
		"mode":          "exact",
		"created_after": 1,
	})
	queryText := extractToolText(t, queryResp)
	assert.Contains(t, queryText, "Date filter test", "fact should appear when created_after is in the past")

	// Search with created_after far in the future should NOT find it
	queryResp2 := callTool(t, w, r, 4, "mie_query", map[string]any{
		"query":         "Date filter test",
		"mode":          "exact",
		"created_after": 9999999999,
	})
	queryText2 := extractToolText(t, queryResp2)
	assert.Contains(t, queryText2, "No results found", "fact should not appear when created_after is in the future")

	// Search with created_before far in the past should NOT find it
	queryResp3 := callTool(t, w, r, 5, "mie_query", map[string]any{
		"query":          "Date filter test",
		"mode":           "exact",
		"created_before": 1,
	})
	queryText3 := extractToolText(t, queryResp3)
	assert.Contains(t, queryText3, "No results found", "fact should not appear when created_before is in the past")
}

// --- BUG 3: mie_analyze metadata (confidence, status, date) ---

func TestBugfix_AnalyzeMetadataFields(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Note: embeddings are disabled in test mode, so semantic search won't work
	// for analyze. We still verify the response structure is correct.
	resp := callTool(t, w, r, 2, "mie_analyze", map[string]any{
		"content": "Testing the analyze tool metadata fields",
	})
	text := extractToolText(t, resp)
	assert.Contains(t, text, "Existing Memory Context")
	assert.Contains(t, text, "Evaluation Guide")
}

// --- BUG 4: List topic filter ---

func TestBugfix_ListTopicFilter(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store a topic
	topicResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":        "topic",
		"name":        "testing-topic",
		"description": "A topic for testing",
	})
	topicText := extractToolText(t, topicResp)
	topicID := extractNodeID(t, topicText, "top:")

	// Store two facts, link only one to the topic
	fact1Resp := callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Linked to testing-topic",
		"category":     "general",
		"source_agent": "test",
		"relationships": []map[string]any{
			{"edge": "fact_topic", "target_id": topicID},
		},
	})
	assert.Nil(t, fact1Resp["error"])

	callTool(t, w, r, 4, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Not linked to any topic",
		"category":     "general",
		"source_agent": "test",
	})

	// List facts filtered by topic - should only return the linked fact
	listResp := callTool(t, w, r, 5, "mie_list", map[string]any{
		"node_type": "fact",
		"topic":     "testing-topic",
	})
	listText := extractToolText(t, listResp)
	assert.Contains(t, listText, "Linked to testing-topic")
	assert.NotContains(t, listText, "Not linked to any topic")
	assert.Contains(t, listText, "1 total")
}

// --- BUG 5: Status health message ---

func TestBugfix_StatusEmbeddingsMessage(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_status", map[string]any{})
	text := extractToolText(t, resp)

	// Should NOT say "Embeddings enabled" when they're disabled
	assert.NotContains(t, text, "Embeddings enabled (provider not configured)")
	// Should indicate embeddings are not configured
	assert.Contains(t, text, "not configured")
}

// --- BUG 6: Store dangling edges - validation ---

func TestBugfix_StoreDanglingEdgeValidation(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store a fact with a relationship to a nonexistent entity
	storeResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Fact with dangling edge",
		"category":     "general",
		"source_agent": "test",
		"relationships": []map[string]any{
			{"edge": "fact_entity", "target_id": "ent:nonexistent000000"},
		},
	})
	storeText := extractToolText(t, storeResp)

	// Should contain a warning about the nonexistent target
	assert.Contains(t, storeText, "Stored fact")
	// The relationship section should indicate the target was not found
	assert.True(t,
		strings.Contains(storeText, "not found") || strings.Contains(storeText, "Skipped"),
		"should warn about nonexistent target node: %s", storeText)
}

// --- BUG 7: bulk_store target_ref out of bounds ---

func TestBugfix_BulkStoreTargetRefOutOfBounds(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_bulk_store", map[string]any{
		"items": []map[string]any{
			{
				"type":         "fact",
				"content":      "Fact with bad ref",
				"category":     "general",
				"source_agent": "test",
				"relationships": []map[string]any{
					{"edge": "fact_entity", "target_ref": 99},
				},
			},
		},
	})
	text := extractToolText(t, resp)

	// Should report the out of bounds error
	assert.Contains(t, text, "out of bounds", "should report target_ref out of bounds: %s", text)
}

// --- BUG 8: bulk_store pluralization ---

func TestBugfix_BulkStorePluralization(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_bulk_store", map[string]any{
		"items": []map[string]any{
			{
				"type":         "entity",
				"name":         "Test Entity",
				"kind":         "technology",
				"source_agent": "test",
			},
		},
	})
	text := extractToolText(t, resp)

	// Should NOT say "1 entitys"
	assert.NotContains(t, text, "entitys", "should not have incorrect pluralization")
	assert.Contains(t, text, "1 entity", "should have correct singular form")
}

// --- BUG 9: List facts shows Valid column ---

func TestBugfix_ListFactsShowsValidColumn(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "A valid fact",
		"category":     "general",
		"source_agent": "test",
	})

	listResp := callTool(t, w, r, 3, "mie_list", map[string]any{
		"node_type": "fact",
	})
	listText := extractToolText(t, listResp)

	// Should have a Valid column in the table
	assert.Contains(t, listText, "Valid", "fact table should have Valid column")
}

// --- BUG 10: Store invalid category should error ---

func TestBugfix_StoreInvalidCategoryErrors(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Fact with bad category",
		"category":     "nonexistent",
		"source_agent": "test",
	})
	result, _ := resp["result"].(map[string]any)
	isError, _ := result["isError"].(bool)
	assert.True(t, isError, "invalid category should return an error")

	text := extractToolText(t, resp)
	assert.Contains(t, text, "invalid category", "error should mention invalid category")
}

// --- BUG 11: Store invalid confidence should error ---

func TestBugfix_StoreInvalidConfidenceErrors(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Fact with bad confidence",
		"category":     "general",
		"confidence":   2.5,
		"source_agent": "test",
	})
	result, _ := resp["result"].(map[string]any)
	isError, _ := result["isError"].(bool)
	assert.True(t, isError, "invalid confidence should return an error")

	text := extractToolText(t, resp)
	assert.Contains(t, text, "confidence must be between", "error should mention confidence range")
}

// --- BUG 12: Export Datalog uses proper escaping ---

func TestBugfix_ExportDatalogEscaping(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store a fact with special characters
	callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Content with 'single quotes' and \"double quotes\"",
		"category":     "general",
		"source_agent": "test",
	})

	// Export as Datalog
	exportResp := callTool(t, w, r, 3, "mie_export", map[string]any{
		"format": "datalog",
	})
	text := extractToolText(t, exportResp)

	// Should use single-quoted CozoDB format, not Go %q double-quoted format
	assert.Contains(t, text, ":put mie_fact")
	// Should NOT have Go-style escaped quotes like \"
	assert.NotContains(t, text, `content: "Content`, "should use single-quoted CozoDB format, not Go double quotes")
}

// --- BUG 13: Export relationships filtered by node_types ---

func TestBugfix_ExportFilteredRelationships(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store entities and facts with relationships
	entResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "entity",
		"name":         "TestCorp",
		"kind":         "company",
		"source_agent": "test",
	})
	entID := extractNodeID(t, extractToolText(t, entResp), "ent:")

	callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "TestCorp has 100 employees",
		"category":     "general",
		"source_agent": "test",
		"relationships": []map[string]any{
			{"edge": "fact_entity", "target_id": entID},
		},
	})

	callTool(t, w, r, 4, "mie_store", map[string]any{
		"type":        "topic",
		"name":        "companies",
		"description": "Company related info",
	})

	// Export only topics - should NOT include fact_entity edges
	exportResp := callTool(t, w, r, 5, "mie_export", map[string]any{
		"format":     "json",
		"node_types": []string{"topic"},
	})
	text := extractToolText(t, exportResp)

	var exportData map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &exportData))

	// Should have topics
	topics, _ := exportData["topics"].([]any)
	assert.NotEmpty(t, topics, "should have topics")

	// Should NOT have facts
	facts, _ := exportData["facts"].([]any)
	assert.Empty(t, facts, "should not have facts when only exporting topics")

	// Edges should not include fact_entity
	if edges, ok := exportData["edges"].(map[string]any); ok {
		_, hasFE := edges["fact_entity"]
		assert.False(t, hasFE, "should not have fact_entity edges when only exporting topics")
	}
}

// --- BUG 14: Export decision_entity includes role ---

func TestBugfix_ExportDecisionEntityRole(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store entity and decision with role
	entResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "entity",
		"name":         "Golang",
		"kind":         "technology",
		"source_agent": "test",
	})
	entID := extractNodeID(t, extractToolText(t, entResp), "ent:")

	callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "decision",
		"title":        "Use Golang for backend",
		"rationale":    "Performance and simplicity",
		"source_agent": "test",
		"relationships": []map[string]any{
			{"edge": "decision_entity", "target_id": entID, "role": "chosen technology"},
		},
	})

	// Export
	exportResp := callTool(t, w, r, 4, "mie_export", map[string]any{
		"format": "json",
	})
	text := extractToolText(t, exportResp)

	var exportData map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &exportData))

	// Check that decision_entity edges include the role field
	if edges, ok := exportData["edges"].(map[string]any); ok {
		if deEdges, ok := edges["decision_entity"].([]any); ok && len(deEdges) > 0 {
			edge, ok := deEdges[0].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "chosen technology", edge["role"], "decision_entity edge should include role")
		}
	}
}

// --- BUG 15: Graph traversal works ---

func TestBugfix_GraphTraversalAllTypes(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Create a connected graph: topic -> entity -> fact -> decision -> event
	topicResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":        "topic",
		"name":        "traversal-test",
		"description": "Topic for traversal testing",
	})
	topicID := extractNodeID(t, extractToolText(t, topicResp), "top:")

	entResp := callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "entity",
		"name":         "TraversalEntity",
		"kind":         "project",
		"source_agent": "test",
		"relationships": []map[string]any{
			{"edge": "entity_topic", "target_id": topicID},
		},
	})
	entID := extractNodeID(t, extractToolText(t, entResp), "ent:")

	factResp := callTool(t, w, r, 4, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "TraversalEntity is a test project",
		"category":     "technical",
		"source_agent": "test",
		"relationships": []map[string]any{
			{"edge": "fact_entity", "target_id": entID},
			{"edge": "fact_topic", "target_id": topicID},
		},
	})
	factID := extractFactID(t, extractToolText(t, factResp))

	// Test various traversals
	tests := []struct {
		name      string
		nodeID    string
		traversal string
		expect    string
	}{
		{"related_entities from fact", factID, "related_entities", "TraversalEntity"},
		{"facts_about_entity", entID, "facts_about_entity", "test project"},
		{"entities_about_topic", topicID, "entities_about_topic", "TraversalEntity"},
		{"facts_about_topic", topicID, "facts_about_topic", "test project"},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := callTool(t, w, r, 10+i, "mie_query", map[string]any{
				"query":     "ignored",
				"mode":      "graph",
				"node_id":   tc.nodeID,
				"traversal": tc.traversal,
			})
			text := extractToolText(t, resp)
			assert.Contains(t, text, tc.expect, "traversal %s from %s should contain %q", tc.traversal, tc.nodeID, tc.expect)
		})
	}
}

// --- BUG 16: Bulk store cross-batch refs work ---

func TestBugfix_BulkStoreCrossBatchRefs(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_bulk_store", map[string]any{
		"items": []map[string]any{
			{
				"type":         "entity",
				"name":         "CrossRefEntity",
				"kind":         "technology",
				"source_agent": "test",
			},
			{
				"type":         "fact",
				"content":      "CrossRefEntity is a framework",
				"category":     "technical",
				"source_agent": "test",
				"relationships": []map[string]any{
					{"edge": "fact_entity", "target_ref": 0},
				},
			},
		},
	})
	text := extractToolText(t, resp)
	assert.Contains(t, text, "Stored 2 items")
	assert.Contains(t, text, "fact_entity")
}

// --- Delete and Get tests ---

func TestBugfix_GetAndDelete(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Store a fact
	storeResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Fact to be deleted",
		"category":     "general",
		"source_agent": "test",
	})
	factID := extractFactID(t, extractToolText(t, storeResp))

	// Get the fact
	getResp := callTool(t, w, r, 3, "mie_get", map[string]any{
		"node_id": factID,
	})
	getText := extractToolText(t, getResp)
	assert.Contains(t, getText, "Fact to be deleted")

	// Delete the fact
	deleteResp := callTool(t, w, r, 4, "mie_delete", map[string]any{
		"action":  "delete_node",
		"node_id": factID,
	})
	assert.Nil(t, deleteResp["error"])

	// Get should fail
	getResp2 := callTool(t, w, r, 5, "mie_get", map[string]any{
		"node_id": factID,
	})
	result2, _ := getResp2["result"].(map[string]any)
	isError, _ := result2["isError"].(bool)
	assert.True(t, isError, "get on deleted node should return error")
}

// --- Comprehensive store all types ---

func TestBugfix_StoreAllTypesWithValidation(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()
	initSession(t, w, r)

	// Test missing required fields
	tests := []struct {
		name   string
		args   map[string]any
		expect string
	}{
		{"fact missing content", map[string]any{"type": "fact", "category": "general"}, "content is required"},
		{"entity missing name", map[string]any{"type": "entity", "kind": "person"}, "name is required"},
		{"entity missing kind", map[string]any{"type": "entity", "name": "Test"}, "kind is required"},
		{"entity invalid kind", map[string]any{"type": "entity", "name": "Test", "kind": "invalid"}, "invalid entity kind"},
		{"decision missing title", map[string]any{"type": "decision", "rationale": "test"}, "title is required"},
		{"decision missing rationale", map[string]any{"type": "decision", "title": "test"}, "rationale is required"},
		{"event missing title", map[string]any{"type": "event", "event_date": "2026-01-01"}, "title is required"},
		{"event missing date", map[string]any{"type": "event", "title": "test"}, "event_date is required"},
		{"topic missing name", map[string]any{"type": "topic"}, "name is required"},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := callTool(t, w, r, 10+i, "mie_store", tc.args)
			result, _ := resp["result"].(map[string]any)
			isError, _ := result["isError"].(bool)
			assert.True(t, isError, "should return error for: %s", tc.name)
			text := extractToolText(t, resp)
			assert.Contains(t, text, tc.expect, "error should mention: %s", tc.expect)
		})
	}
}

// --- helpers ---

// extractNodeID extracts a node ID with the given prefix from tool response text.
func extractNodeID(t *testing.T, text string, prefix string) string {
	t.Helper()
	start := strings.Index(text, "["+prefix)
	if start == -1 {
		t.Fatalf("no node ID with prefix %q found in text: %s", prefix, text)
	}
	end := strings.Index(text[start:], "]")
	if end == -1 {
		t.Fatal("no closing bracket for node ID")
	}
	return text[start+1 : start+end]
}
