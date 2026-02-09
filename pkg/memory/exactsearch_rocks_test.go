// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"testing"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

func TestExactSearchFactRocksDB(t *testing.T) {
	for _, engine := range []string{"mem", "rocksdb"} {
		t.Run(engine, func(t *testing.T) {
			backend, err := storage.NewEmbeddedBackend(storage.EmbeddedConfig{
				Engine:              engine,
				DataDir:             t.TempDir(),
				EmbeddingDimensions: 384,
			})
			if err != nil {
				t.Fatalf("create backend: %v", err)
			}
			defer backend.Close()
			setupSchema(t, backend)

			w := NewWriter(backend, nil, nil)
			r := NewReader(backend, nil, nil)
			ctx := context.Background()

			// Store a fact
			fact, err := w.StoreFact(ctx, tools.StoreFactRequest{
				Content:  "I love coffee and tea",
				Category: "preference",
			})
			if err != nil {
				t.Fatalf("StoreFact: %v", err)
			}
			t.Logf("Stored fact: %s content=%q valid=true", fact.ID, fact.Content)

			// Verify the fact exists via raw query
			script := `?[id, content, valid] := *mie_fact { id, content, valid }`
			qr, err := backend.Query(ctx, script)
			if err != nil {
				t.Fatalf("raw query: %v", err)
			}
			t.Logf("All facts: %d rows", len(qr.Rows))
			for _, row := range qr.Rows {
				t.Logf("  Row: %v", row)
			}

			// Test the exact ExactSearch query pattern
			escaped := escapeDatalog("coffee")
			exactScript := fmt.Sprintf(`?[id, content, category, confidence] :=
    *mie_fact { id, content, category, confidence, valid },
    valid = true,
    str_includes(content, '%s')
    :limit 10`, escaped)

			t.Logf("Running ExactSearch query:\n%s", exactScript)
			qr2, err := backend.Query(ctx, exactScript)
			if err != nil {
				t.Fatalf("ExactSearch query FAILED: %v", err)
			}
			t.Logf("ExactSearch results: %d rows", len(qr2.Rows))
			for _, row := range qr2.Rows {
				t.Logf("  Row: %v", row)
			}

			if len(qr2.Rows) != 1 {
				t.Errorf("expected 1 row, got %d", len(qr2.Rows))
			}

			// Also test via the Reader.ExactSearch method
			results, err := r.ExactSearch(ctx, "coffee", []string{"fact"}, 10)
			if err != nil {
				t.Fatalf("ExactSearch method: %v", err)
			}
			t.Logf("ExactSearch method results: %d", len(results))
			if len(results) != 1 {
				t.Errorf("expected 1 result from method, got %d", len(results))
			}
		})
	}
}
