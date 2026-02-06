// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"testing"

	cozo "github.com/kraklabs/mie/pkg/cozodb"
)

// TestBackendInterface verifies that EmbeddedBackend implements the Backend interface.
func TestBackendInterface(t *testing.T) {
	var _ Backend = &EmbeddedBackend{}
}

// TestQueryResult_ToNamedRows tests the conversion from QueryResult to CozoDB NamedRows.
func TestQueryResult_ToNamedRows(t *testing.T) {
	qr := &QueryResult{
		Headers: []string{"id", "name", "value"},
		Rows: [][]any{
			{"1", "test", 42},
			{"2", "example", 100},
		},
	}

	nr := qr.ToNamedRows()

	if len(nr.Headers) != 3 {
		t.Errorf("expected 3 headers, got %d", len(nr.Headers))
	}
	if nr.Headers[0] != "id" || nr.Headers[1] != "name" || nr.Headers[2] != "value" {
		t.Errorf("headers mismatch: got %v", nr.Headers)
	}
	if len(nr.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(nr.Rows))
	}
	if len(nr.Rows[0]) != 3 {
		t.Errorf("expected 3 columns in first row, got %d", len(nr.Rows[0]))
	}
}

// TestFromNamedRows tests the conversion from CozoDB NamedRows to QueryResult.
func TestFromNamedRows(t *testing.T) {
	nr := cozo.NamedRows{
		Headers: []string{"fact_id", "content"},
		Rows: [][]any{
			{"fact1", "Go is a compiled language"},
			{"fact2", "CozoDB uses Datalog"},
		},
	}

	qr := FromNamedRows(nr)

	if qr == nil {
		t.Fatal("FromNamedRows returned nil")
	}
	if len(qr.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(qr.Headers))
	}
	if qr.Headers[0] != "fact_id" || qr.Headers[1] != "content" {
		t.Errorf("headers mismatch: got %v", qr.Headers)
	}
	if len(qr.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(qr.Rows))
	}
	if qr.Rows[0][0] != "fact1" || qr.Rows[0][1] != "Go is a compiled language" {
		t.Errorf("row data mismatch: got %v", qr.Rows[0])
	}
}