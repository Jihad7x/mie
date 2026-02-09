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

func TestConflictDetectorRequiresEmbeddings(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	cd := NewConflictDetector(backend, nil, nil)
	ctx := context.Background()

	_, err := cd.DetectConflicts(ctx, tools.ConflictOptions{})
	if err == nil {
		t.Error("expected error when embeddings are disabled")
	}

	_, err = cd.CheckNewFactConflicts(ctx, "test", "general")
	if err == nil {
		t.Error("expected error when embeddings are disabled")
	}
}

func TestConflictDetectorWithMock(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	provider := NewMockEmbeddingProvider(384, nil)
	embedder := NewEmbeddingGenerator(provider, nil)

	w := NewWriter(backend, embedder, nil)
	cd := NewConflictDetector(backend, embedder, nil)
	ctx := context.Background()

	// Store facts and their embeddings synchronously
	fact1, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I prefer dark mode",
		Category: "preference",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact1.ID, fact1.Content)

	fact2, err := w.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "I prefer light mode",
		Category: "preference",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}
	storeEmbeddingSync(t, backend, embedder, "mie_fact_embedding", "fact_id", fact2.ID, fact2.Content)

	// Create HNSW index after data is present
	if err := EnsureHNSWIndexes(backend, 384); err != nil {
		t.Fatalf("EnsureHNSWIndexes failed: %v", err)
	}

	// Check new fact conflicts
	conflicts, err := cd.CheckNewFactConflicts(ctx, "I prefer dark themes", "preference")
	if err != nil {
		t.Fatalf("CheckNewFactConflicts failed: %v", err)
	}
	// Mock embeddings may or may not produce conflicts depending on hash proximity.
	// The important thing is it doesn't error.
	_ = conflicts
}

// storeEmbeddingSync stores an embedding synchronously (unlike writer's async method).
func storeEmbeddingSync(t *testing.T, backend *storage.EmbeddedBackend, embedder *EmbeddingGenerator, table, idCol, nodeID, text string) {
	t.Helper()
	ctx := context.Background()
	embedding, err := embedder.Generate(ctx, text)
	if err != nil {
		t.Fatalf("generate embedding: %v", err)
	}
	vecStr := formatVector(embedding)
	mutation := fmt.Sprintf(
		`?[%s, embedding] <- [["%s", vec(%s)]] :put %s { %s => embedding }`,
		idCol, escapeDatalog(nodeID), vecStr, table, idCol,
	)
	if err := backend.Execute(ctx, mutation); err != nil {
		t.Fatalf("store embedding: %v", err)
	}
}
