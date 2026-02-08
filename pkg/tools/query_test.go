// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestQuery_SemanticMode(t *testing.T) {
	mock := &MockQuerier{
		SemanticSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			return []SearchResult{
				{NodeType: "fact", ID: "fact:abc", Content: "Go is my primary language", Distance: 0.1},
				{NodeType: "fact", ID: "fact:def", Content: "I use Docker for development", Distance: 0.3},
			}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Query(context.Background(), mock, map[string]any{
		"query": "what tech stack do I use",
		"mode":  "semantic",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Query() returned error: %s", result.Text)
	}

	checks := []string{
		"Memory Search Results",
		"what tech stack do I use",
		"fact:abc",
		"Go is my primary language",
		"Facts",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("Query() output missing %q", check)
		}
	}
}

func TestQuery_SemanticMode_NoEmbeddings(t *testing.T) {
	mock := &MockQuerier{
		EmbeddingsEnabledFunc: func() bool { return false },
	}

	result, _ := Query(context.Background(), mock, map[string]any{
		"query": "test",
	})
	if !result.IsError {
		t.Error("Query() should return error when embeddings disabled for semantic mode")
	}
}

func TestQuery_ExactMode(t *testing.T) {
	mock := &MockQuerier{
		ExactSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			return []SearchResult{
				{NodeType: "entity", ID: "ent:abc", Content: "Kraklabs"},
			}, nil
		},
	}

	result, err := Query(context.Background(), mock, map[string]any{
		"query": "Kraklabs",
		"mode":  "exact",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Query() returned error: %s", result.Text)
	}

	if !strings.Contains(result.Text, "Exact Search Results") {
		t.Error("Query() should show exact search header")
	}
	if !strings.Contains(result.Text, "Kraklabs") {
		t.Error("Query() should include found entity")
	}
}

func TestQuery_ExactMode_FactsWithValidOnly(t *testing.T) {
	// Regression test: facts must survive valid_only filtering.
	// parseSearchResult sets Valid=true on Fact metadata, so the default
	// valid_only=true filter must not drop valid facts.
	mock := &MockQuerier{
		ExactSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			return []SearchResult{
				{
					NodeType: "fact", ID: "fact:abc", Content: "I love coffee",
					Metadata: &Fact{ID: "fact:abc", Content: "I love coffee", Category: "preference", Valid: true},
				},
			}, nil
		},
	}

	result, err := Query(context.Background(), mock, map[string]any{
		"query": "coffee",
		"mode":  "exact",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Query() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "fact:abc") {
		t.Error("Query() should include fact result; valid_only must not filter valid facts")
	}
	if strings.Contains(result.Text, "No results found") {
		t.Error("Query() should find results, not show empty state")
	}
}

func TestQuery_GraphMode(t *testing.T) {
	mock := &MockQuerier{
		GetRelatedEntitiesFunc: func(ctx context.Context, factID string) ([]Entity, error) {
			return []Entity{
				{ID: "ent:abc", Name: "Kraklabs", Kind: "company", Description: "AI lab"},
			}, nil
		},
	}

	result, err := Query(context.Background(), mock, map[string]any{
		"query":     "related entities",
		"mode":      "graph",
		"node_id":   "fact:abc123",
		"traversal": "related_entities",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Query() returned error: %s", result.Text)
	}

	if !strings.Contains(result.Text, "Kraklabs") {
		t.Error("Query() should show related entity")
	}
	if !strings.Contains(result.Text, "company") {
		t.Error("Query() should show entity kind")
	}
}

func TestQuery_GraphMode_InvalidationChain(t *testing.T) {
	mock := &MockQuerier{
		GetInvalidationChainFunc: func(ctx context.Context, factID string) ([]Invalidation, error) {
			return []Invalidation{
				{
					NewFactID:  "fact:new123",
					OldFactID:  "fact:old123",
					Reason:     "User moved",
					OldContent: "Lives in Buenos Aires",
					NewContent: "Lives in New York",
				},
			}, nil
		},
	}

	result, err := Query(context.Background(), mock, map[string]any{
		"query":     "chain",
		"mode":      "graph",
		"node_id":   "fact:old123",
		"traversal": "invalidation_chain",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if !strings.Contains(result.Text, "User moved") {
		t.Error("Query() should show invalidation reason")
	}
}

func TestQuery_GraphMode_MissingNodeID(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Query(context.Background(), mock, map[string]any{
		"query":     "test",
		"mode":      "graph",
		"traversal": "related_entities",
	})
	if !result.IsError {
		t.Error("Query() should return error when node_id missing for graph mode")
	}
}

func TestQuery_GraphMode_MissingTraversal(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Query(context.Background(), mock, map[string]any{
		"query":   "test",
		"mode":    "graph",
		"node_id": "fact:abc",
	})
	if !result.IsError {
		t.Error("Query() should return error when traversal missing for graph mode")
	}
}

func TestQuery_MissingQuery(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Query(context.Background(), mock, map[string]any{})
	if !result.IsError {
		t.Error("Query() should return error when query is missing")
	}
}

func TestQuery_InvalidMode(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Query(context.Background(), mock, map[string]any{
		"query": "test",
		"mode":  "invalid",
	})
	if !result.IsError {
		t.Error("Query() should return error for invalid mode")
	}
}

func TestQuery_EmptyResults(t *testing.T) {
	mock := &MockQuerier{
		SemanticSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			return []SearchResult{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Query(context.Background(), mock, map[string]any{
		"query": "something that does not exist",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if !strings.Contains(result.Text, "No results found") {
		t.Error("Query() should indicate no results found")
	}
}

func TestQuery_LimitClamping(t *testing.T) {
	var capturedLimit int
	mock := &MockQuerier{
		SemanticSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			capturedLimit = limit
			return []SearchResult{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	Query(context.Background(), mock, map[string]any{
		"query": "test",
		"limit": float64(100),
	})
	if capturedLimit != 50 {
		t.Errorf("Expected limit clamped to 50, got %d", capturedLimit)
	}
}