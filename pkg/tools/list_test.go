// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestList_Facts(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			if opts.NodeType != "fact" {
				t.Errorf("Expected node_type=fact, got %s", opts.NodeType)
			}
			if !opts.ValidOnly {
				t.Error("Expected valid_only=true by default")
			}
			return []any{
				&Fact{ID: "fact:abc", Content: "User works at Kraklabs", Category: "professional", Confidence: 0.95, CreatedAt: 1000},
				&Fact{ID: "fact:def", Content: "Uses Go primarily", Category: "technical", Confidence: 0.9, CreatedAt: 1001},
			}, 47, nil
		},
	}

	result, err := List(context.Background(), mock, map[string]any{
		"node_type": "fact",
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}

	checks := []string{
		"Facts (47 total",
		"fact:abc",
		"User works at Kraklabs",
		"professional",
		"fact:def",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("List() output missing %q", check)
		}
	}
}

func TestList_Decisions(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			return []any{
				&Decision{ID: "dec:abc", Title: "Chose Go", Status: "active", CreatedAt: 1000},
			}, 1, nil
		},
	}

	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "decision",
	})
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Decisions") {
		t.Error("List() should show 'Decisions' header")
	}
	if !strings.Contains(result.Text, "Chose Go") {
		t.Error("List() should show decision title")
	}
}

func TestList_Entities(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			return []any{
				&Entity{ID: "ent:abc", Name: "Kraklabs", Kind: "company", Description: "AI lab"},
			}, 1, nil
		},
	}

	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "entity",
	})
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Kraklabs") {
		t.Error("List() should show entity name")
	}
}

func TestList_Events(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			return []any{
				&Event{ID: "evt:abc", Title: "Launch", EventDate: "2026-01-15", CreatedAt: 1000},
			}, 1, nil
		},
	}

	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "event",
	})
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Launch") {
		t.Error("List() should show event title")
	}
}

func TestList_Topics(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			return []any{
				&Topic{ID: "top:abc", Name: "architecture", Description: "System design"},
			}, 1, nil
		},
	}

	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "topic",
	})
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "architecture") {
		t.Error("List() should show topic name")
	}
}

func TestList_MissingNodeType(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := List(context.Background(), mock, map[string]any{})
	if !result.IsError {
		t.Error("List() should require node_type")
	}
}

func TestList_InvalidNodeType(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "invalid",
	})
	if !result.IsError {
		t.Error("List() should reject invalid node_type")
	}
}

func TestList_Pagination(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			if opts.Offset != 20 {
				t.Errorf("Expected offset=20, got %d", opts.Offset)
			}
			if opts.Limit != 10 {
				t.Errorf("Expected limit=10, got %d", opts.Limit)
			}
			return []any{
				&Fact{ID: "fact:page2", Content: "Page 2 fact", Category: "general", Confidence: 0.8, CreatedAt: 1000},
			}, 47, nil
		},
	}

	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "fact",
		"limit":     float64(10),
		"offset":    float64(20),
	})
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "showing 21-21") {
		t.Errorf("List() should show correct page range, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "offset=30") {
		t.Error("List() should suggest next page offset")
	}
}

func TestList_WithFilters(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			if opts.Category != "technical" {
				t.Errorf("Expected category=technical, got %s", opts.Category)
			}
			if opts.SortBy != "confidence" {
				t.Errorf("Expected sort_by=confidence, got %s", opts.SortBy)
			}
			if opts.SortOrder != "asc" {
				t.Errorf("Expected sort_order=asc, got %s", opts.SortOrder)
			}
			return []any{}, 0, nil
		},
	}

	List(context.Background(), mock, map[string]any{
		"node_type":  "fact",
		"category":   "technical",
		"sort_by":    "confidence",
		"sort_order": "asc",
	})
}

func TestList_EmptyResults(t *testing.T) {
	mock := &MockQuerier{
		ListNodesFunc: func(ctx context.Context, opts ListOptions) ([]any, int, error) {
			return []any{}, 0, nil
		},
	}

	result, _ := List(context.Background(), mock, map[string]any{
		"node_type": "fact",
	})
	if result.IsError {
		t.Fatalf("List() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "No results found") {
		t.Error("List() should indicate no results")
	}
}