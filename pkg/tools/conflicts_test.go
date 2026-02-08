// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestConflicts_Found(t *testing.T) {
	mock := &MockQuerier{
		DetectConflictsFunc: func(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
			if opts.Threshold != 0.85 {
				t.Errorf("Expected threshold=0.85, got %f", opts.Threshold)
			}
			return []Conflict{
				{
					FactA:      Fact{ID: "fact:old", Content: "User lives in Buenos Aires", Category: "personal", Confidence: 0.85, CreatedAt: 1000},
					FactB:      Fact{ID: "fact:new", Content: "User moved to New York", Category: "personal", Confidence: 0.9, CreatedAt: 2000},
					Similarity: 0.92,
				},
			}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Conflicts(context.Background(), mock, map[string]any{})
	if err != nil {
		t.Fatalf("Conflicts() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Conflicts() returned error: %s", result.Text)
	}

	checks := []string{
		"Potential Conflicts Found (1)",
		"Conflict 1",
		"92%",
		"fact:old",
		"fact:new",
		"Buenos Aires",
		"New York",
		"mie_update",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("Conflicts() output missing %q", check)
		}
	}
}

func TestConflicts_None(t *testing.T) {
	mock := &MockQuerier{
		DetectConflictsFunc: func(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
			return []Conflict{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Conflicts(context.Background(), mock, map[string]any{})
	if err != nil {
		t.Fatalf("Conflicts() error = %v", err)
	}
	if !strings.Contains(result.Text, "No potential conflicts found") {
		t.Error("Conflicts() should indicate no conflicts")
	}
}

func TestConflicts_WithCategory(t *testing.T) {
	mock := &MockQuerier{
		DetectConflictsFunc: func(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
			if opts.Category != "technical" {
				t.Errorf("Expected category=technical, got %s", opts.Category)
			}
			return []Conflict{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	Conflicts(context.Background(), mock, map[string]any{
		"category": "technical",
	})
}

func TestConflicts_CustomThreshold(t *testing.T) {
	mock := &MockQuerier{
		DetectConflictsFunc: func(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
			if opts.Threshold != 0.9 {
				t.Errorf("Expected threshold=0.9, got %f", opts.Threshold)
			}
			return []Conflict{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	Conflicts(context.Background(), mock, map[string]any{
		"threshold": 0.9,
	})
}

func TestConflicts_EmbeddingsDisabled(t *testing.T) {
	mock := &MockQuerier{
		EmbeddingsEnabledFunc: func() bool { return false },
	}

	result, _ := Conflicts(context.Background(), mock, map[string]any{})
	if !result.IsError {
		t.Error("Conflicts() should return error when embeddings disabled")
	}
}

func TestConflicts_RecommendationHighSimilarity(t *testing.T) {
	mock := &MockQuerier{
		DetectConflictsFunc: func(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
			return []Conflict{
				{
					FactA:      Fact{ID: "fact:old", Content: "Old fact", Category: "technical", CreatedAt: 1000},
					FactB:      Fact{ID: "fact:new", Content: "New fact", Category: "technical", CreatedAt: 2000},
					Similarity: 0.92,
				},
			}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, _ := Conflicts(context.Background(), mock, map[string]any{})
	if !strings.Contains(result.Text, "newer fact") {
		t.Error("High similarity + same category should recommend newer supersedes older")
	}
}

func TestConflicts_RecommendationMediumSimilarity(t *testing.T) {
	mock := &MockQuerier{
		DetectConflictsFunc: func(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
			return []Conflict{
				{
					FactA:      Fact{ID: "fact:a", Content: "Fact A", Category: "technical", CreatedAt: 1000},
					FactB:      Fact{ID: "fact:b", Content: "Fact B", Category: "technical", CreatedAt: 2000},
					Similarity: 0.82,
				},
			}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, _ := Conflicts(context.Background(), mock, map[string]any{})
	if !strings.Contains(result.Text, "may be related or contradictory") {
		t.Error("Medium similarity should suggest review for potential contradiction")
	}
}