// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestExport_JSON(t *testing.T) {
	mock := &MockQuerier{
		ExportGraphFunc: func(ctx context.Context, opts ExportOptions) (*ExportData, error) {
			if opts.Format != "json" {
				t.Errorf("Expected format=json, got %s", opts.Format)
			}
			return &ExportData{
				Version:    "1",
				ExportedAt: "2026-02-05T20:30:00Z",
				Stats:      map[string]int{"facts": 2, "entities": 1},
				Facts: []Fact{
					{ID: "fact:abc", Content: "User works at Kraklabs", Category: "professional", Confidence: 0.95, Valid: true},
					{ID: "fact:def", Content: "Uses Go", Category: "technical", Confidence: 0.9, Valid: true},
				},
				Entities: []Entity{
					{ID: "ent:abc", Name: "Kraklabs", Kind: "company"},
				},
			}, nil
		},
	}

	result, err := Export(context.Background(), mock, map[string]any{
		"format": "json",
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Export() returned error: %s", result.Text)
	}

	checks := []string{
		`"version": "1"`,
		`"exported_at"`,
		`"facts"`,
		"fact:abc",
		"User works at Kraklabs",
		"Kraklabs",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("Export() JSON output missing %q", check)
		}
	}
}

func TestExport_Datalog(t *testing.T) {
	mock := &MockQuerier{
		ExportGraphFunc: func(ctx context.Context, opts ExportOptions) (*ExportData, error) {
			if opts.Format != "datalog" {
				t.Errorf("Expected format=datalog, got %s", opts.Format)
			}
			return &ExportData{
				Version:    "1",
				ExportedAt: "2026-02-05T20:30:00Z",
				Stats:      map[string]int{"facts": 1},
				Facts: []Fact{
					{ID: "fact:abc", Content: "User works at Kraklabs", Category: "professional", Confidence: 0.95, SourceAgent: "claude", Valid: true, CreatedAt: 1000, UpdatedAt: 1000},
				},
				Entities: []Entity{
					{ID: "ent:abc", Name: "Kraklabs", Kind: "company", SourceAgent: "claude", CreatedAt: 1000, UpdatedAt: 1000},
				},
			}, nil
		},
	}

	result, err := Export(context.Background(), mock, map[string]any{
		"format": "datalog",
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Export() returned error: %s", result.Text)
	}

	checks := []string{
		":put mie_fact",
		"fact:abc",
		":put mie_entity",
		"Kraklabs",
		"MIE Memory Export",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("Export() datalog output missing %q", check)
		}
	}
}

func TestExport_DefaultFormat(t *testing.T) {
	var capturedFormat string
	mock := &MockQuerier{
		ExportGraphFunc: func(ctx context.Context, opts ExportOptions) (*ExportData, error) {
			capturedFormat = opts.Format
			return &ExportData{Version: "1", ExportedAt: "2026-02-05T00:00:00Z", Stats: map[string]int{}}, nil
		},
	}

	Export(context.Background(), mock, map[string]any{})
	if capturedFormat != "json" {
		t.Errorf("Expected default format=json, got %s", capturedFormat)
	}
}

func TestExport_InvalidFormat(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Export(context.Background(), mock, map[string]any{
		"format": "xml",
	})
	if !result.IsError {
		t.Error("Export() should reject invalid format")
	}
}

func TestExport_NodeTypeFiltering(t *testing.T) {
	mock := &MockQuerier{
		ExportGraphFunc: func(ctx context.Context, opts ExportOptions) (*ExportData, error) {
			if len(opts.NodeTypes) != 2 || opts.NodeTypes[0] != "fact" || opts.NodeTypes[1] != "entity" {
				t.Errorf("Expected node_types=[fact, entity], got %v", opts.NodeTypes)
			}
			return &ExportData{Version: "1", ExportedAt: "2026-02-05T00:00:00Z", Stats: map[string]int{}}, nil
		},
	}

	Export(context.Background(), mock, map[string]any{
		"node_types": []any{"fact", "entity"},
	})
}

func TestExport_IncludeEmbeddings(t *testing.T) {
	mock := &MockQuerier{
		ExportGraphFunc: func(ctx context.Context, opts ExportOptions) (*ExportData, error) {
			if !opts.IncludeEmbeddings {
				t.Error("Expected include_embeddings=true")
			}
			return &ExportData{Version: "1", ExportedAt: "2026-02-05T00:00:00Z", Stats: map[string]int{}}, nil
		},
	}

	Export(context.Background(), mock, map[string]any{
		"include_embeddings": true,
	})
}