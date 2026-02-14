// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/kraklabs/mie/pkg/tools"
)

func TestClientNewAndClose(t *testing.T) {
	client, err := NewClient(ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client.EmbeddingsEnabled() {
		t.Error("embeddings should not be enabled without provider")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestClientWithMockEmbeddings(t *testing.T) {
	client, err := NewClient(ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingEnabled:    true,
		EmbeddingProvider:   "mock",
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	if !client.EmbeddingsEnabled() {
		t.Error("embeddings should be enabled with mock provider")
	}
}

func TestClientImplementsQuerier(t *testing.T) {
	// Compile-time check is in client.go: var _ tools.Querier = (*Client)(nil)
	// This test verifies it works at runtime too.
	client, err := NewClient(ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	var q tools.Querier = client
	_ = q // Use the variable to avoid compiler warning
}

func TestClientStoreFact(t *testing.T) {
	client, err := NewClient(ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	fact, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:     "Test client fact",
		Category:    "general",
		Confidence:  0.9,
		SourceAgent: "test",
	})
	if err != nil {
		t.Fatalf("StoreFact failed: %v", err)
	}
	if fact.ID == "" {
		t.Error("expected non-empty fact ID")
	}

	// Retrieve the fact
	node, err := client.GetNodeByID(ctx, fact.ID)
	if err != nil {
		t.Fatalf("GetNodeByID failed: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
}

func TestNewClientWithBackendDimensionMismatch(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	// Simulate daemon storing dimensions as 768
	if err := backend.SetMeta("embedding_dimensions", "768"); err != nil {
		t.Fatalf("set meta: %v", err)
	}

	// Client expects 384 — should fail
	_, err := NewClientWithBackend(backend, ClientConfig{
		EmbeddingDimensions: 384,
	}, nil)
	if err == nil {
		t.Fatal("expected dimension mismatch error")
	}
	if !strings.Contains(err.Error(), "dimension mismatch") {
		t.Errorf("expected 'dimension mismatch' in error, got: %v", err)
	}
}

func TestNewClientWithBackendDimensionMatch(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	// Simulate daemon storing dimensions as 384
	if err := backend.SetMeta("embedding_dimensions", "384"); err != nil {
		t.Fatalf("set meta: %v", err)
	}

	// Client also expects 384 — should succeed
	client, err := NewClientWithBackend(backend, ClientConfig{
		EmbeddingDimensions: 384,
	}, nil)
	if err != nil {
		t.Fatalf("NewClientWithBackend should succeed: %v", err)
	}
	client.Close()
}

func TestNewClientWithBackend(t *testing.T) {
	backend := newTestBackend(t)
	defer backend.Close()
	setupSchema(t, backend)

	client, err := NewClientWithBackend(backend, ClientConfig{
		EmbeddingDimensions: 384,
	}, nil)
	if err != nil {
		t.Fatalf("NewClientWithBackend: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	fact, err := client.StoreFact(ctx, tools.StoreFactRequest{
		Content:  "test fact from NewClientWithBackend",
		Category: "technical",
	})
	if err != nil {
		t.Fatalf("store fact: %v", err)
	}
	if fact.ID == "" {
		t.Error("expected non-empty fact ID")
	}

	node, err := client.GetNodeByID(ctx, fact.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil node")
	}
}

func TestClientGetStats(t *testing.T) {
	client, err := NewClient(ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	client.StoreFact(ctx, tools.StoreFactRequest{Content: "fact1", Category: "general"})
	client.StoreEntity(ctx, tools.StoreEntityRequest{Name: "entity1", Kind: "other"})

	stats, err := client.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalFacts != 1 {
		t.Errorf("expected 1 fact, got %d", stats.TotalFacts)
	}
	if stats.TotalEntities != 1 {
		t.Errorf("expected 1 entity, got %d", stats.TotalEntities)
	}
	if stats.StorageEngine != "mem" {
		t.Errorf("expected storage engine 'mem', got %q", stats.StorageEngine)
	}
}

func TestClientExportGraph(t *testing.T) {
	client, err := NewClient(ClientConfig{
		DataDir:             t.TempDir(),
		StorageEngine:       "mem",
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	client.StoreFact(ctx, tools.StoreFactRequest{Content: "export fact", Category: "general"})

	export, err := client.ExportGraph(ctx, tools.ExportOptions{})
	if err != nil {
		t.Fatalf("ExportGraph failed: %v", err)
	}
	if len(export.Facts) != 1 {
		t.Errorf("expected 1 exported fact, got %d", len(export.Facts))
	}
}
