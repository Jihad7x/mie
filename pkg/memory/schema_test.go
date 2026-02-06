// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"testing"
)

func TestSchemaStatements(t *testing.T) {
	stmts := SchemaStatements(768)
	if len(stmts) != 17 {
		t.Errorf("expected 17 schema statements, got %d", len(stmts))
	}

	// Verify each statement starts with :create
	for i, stmt := range stmts {
		if len(stmt) == 0 {
			t.Errorf("statement %d is empty", i)
		}
		if stmt[0] != ':' {
			t.Errorf("statement %d should start with ':', got %q", i, stmt[:min(20, len(stmt))])
		}
	}
}

func TestSchemaStatementsDimensionSubstitution(t *testing.T) {
	stmts768 := SchemaStatements(768)
	stmts1536 := SchemaStatements(1536)

	// The embedding table statements should differ based on dimension
	if stmts768[1] == stmts1536[1] {
		t.Error("embedding table statements should differ for different dimensions")
	}
}

func TestHNSWIndexStatements(t *testing.T) {
	stmts := HNSWIndexStatements(768)
	if len(stmts) != 4 {
		t.Errorf("expected 4 HNSW index statements, got %d", len(stmts))
	}

	for i, stmt := range stmts {
		if len(stmt) == 0 {
			t.Errorf("HNSW statement %d is empty", i)
		}
	}
}

func TestEnsureSchema(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()

	// First call should succeed
	if err := EnsureSchema(backend, 768); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	// Second call (idempotent) should also succeed
	if err := EnsureSchema(backend, 768); err != nil {
		t.Fatalf("EnsureSchema (idempotent) failed: %v", err)
	}

	// Verify schema version was set
	result, err := backend.Query(t.Context(), `?[value] := *mie_meta { key, value }, key = "schema_version"`)
	if err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatal("schema version not set")
	}
	if toString(result.Rows[0][0]) != "1" {
		t.Errorf("expected schema version '1', got %v", result.Rows[0][0])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}