// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestAnalyze_WithRelatedNodes(t *testing.T) {
	mock := &MockQuerier{
		SemanticSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			return []SearchResult{
				{
					NodeType: "fact",
					ID:       "fact:abc123",
					Content:  "User works at Kraklabs",
					Distance: 0.08,
					Metadata: &Fact{ID: "fact:abc123", Content: "User works at Kraklabs", Confidence: 0.95, Category: "professional", Valid: true},
				},
				{
					NodeType: "entity",
					ID:       "ent:def456",
					Content:  "Kraklabs",
					Distance: 0.05,
					Metadata: &Entity{ID: "ent:def456", Name: "Kraklabs", Kind: "company"},
				},
			}, nil
		},
		CheckNewFactConflictsFunc: func(ctx context.Context, content, category string) ([]Conflict, error) {
			return []Conflict{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Analyze(context.Background(), mock, map[string]any{
		"content": "I work at Kraklabs as a software engineer",
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Analyze() returned error: %s", result.Text)
	}

	checks := []string{
		"Existing Memory Context",
		"Related Facts",
		"fact:abc123",
		"User works at Kraklabs",
		"Related Entities",
		"ent:def456",
		"Kraklabs",
		"Evaluation Guide",
		"Store Schema Reference",
		"mie_store",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("Analyze() output missing %q", check)
		}
	}
}

func TestAnalyze_EmptyMemory(t *testing.T) {
	mock := &MockQuerier{
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Analyze(context.Background(), mock, map[string]any{
		"content": "Something completely new",
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Analyze() returned error: %s", result.Text)
	}

	if !strings.Contains(result.Text, "No related memory found") {
		t.Error("Analyze() should indicate no related memory")
	}
	if !strings.Contains(result.Text, "Evaluation Guide") {
		t.Error("Analyze() should always include evaluation guide")
	}
}

func TestAnalyze_WithConflicts(t *testing.T) {
	mock := &MockQuerier{
		SemanticSearchFunc: func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
			return []SearchResult{}, nil
		},
		CheckNewFactConflictsFunc: func(ctx context.Context, content, category string) ([]Conflict, error) {
			return []Conflict{
				{
					FactB:      Fact{ID: "fact:old123", Content: "User lives in Buenos Aires", Category: "personal"},
					Similarity: 0.91,
				},
			}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Analyze(context.Background(), mock, map[string]any{
		"content": "I recently moved to New York City",
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if !strings.Contains(result.Text, "Potential Conflicts") {
		t.Error("Analyze() should show conflicts section")
	}
	if !strings.Contains(result.Text, "fact:old123") {
		t.Error("Analyze() should reference conflicting fact ID")
	}
}

func TestAnalyze_MissingContent(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Analyze(context.Background(), mock, map[string]any{})
	if !result.IsError {
		t.Error("Analyze() should return error when content is missing")
	}
	if !strings.Contains(result.Text, "content") {
		t.Error("Error should mention 'content'")
	}
}

func TestAnalyze_EmbeddingsDisabled(t *testing.T) {
	mock := &MockQuerier{
		EmbeddingsEnabledFunc: func() bool { return false },
	}

	result, err := Analyze(context.Background(), mock, map[string]any{
		"content": "Some content",
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Analyze() returned error: %s", result.Text)
	}

	if !strings.Contains(result.Text, "Embeddings are disabled") {
		t.Error("Analyze() should note that embeddings are disabled")
	}
	// Should still include the evaluation guide
	if !strings.Contains(result.Text, "Evaluation Guide") {
		t.Error("Analyze() should always include evaluation guide")
	}
}
