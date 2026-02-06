// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"context"
	"math"
	"testing"
)

func TestMockEmbeddingProvider(t *testing.T) {
	provider := NewMockEmbeddingProvider(384, nil)

	ctx := context.Background()
	emb1, err := provider.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(emb1) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(emb1))
	}

	// Check normalization
	var norm float64
	for _, v := range emb1 {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.01 {
		t.Errorf("expected L2 norm ~1.0, got %f", norm)
	}

	// Deterministic
	emb2, err := provider.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	for i := range emb1 {
		if emb1[i] != emb2[i] {
			t.Fatal("MockEmbeddingProvider should be deterministic")
		}
	}

	// Different input, different output
	emb3, err := provider.Embed(ctx, "different text")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	same := true
	for i := range emb1 {
		if emb1[i] != emb3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("different text should produce different embeddings")
	}
}

func TestMockEmbeddingProviderEmbedQuery(t *testing.T) {
	provider := NewMockEmbeddingProvider(768, nil)

	ctx := context.Background()
	emb, err := provider.EmbedQuery(ctx, "test query")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}

	if len(emb) != 768 {
		t.Errorf("expected 768 dimensions, got %d", len(emb))
	}
}

func TestNormalizeEmbedding(t *testing.T) {
	// Normal case
	v := []float32{3.0, 4.0}
	result := normalizeEmbedding(v)
	expectedNorm := float32(5.0)
	if math.Abs(float64(result[0]-3.0/expectedNorm)) > 0.0001 {
		t.Errorf("unexpected result[0]: %f", result[0])
	}

	// Empty vector
	empty := normalizeEmbedding(nil)
	if empty != nil {
		t.Error("expected nil for nil input")
	}

	// Zero vector
	zeros := normalizeEmbedding([]float32{0, 0, 0})
	for _, v := range zeros {
		if v != 0 {
			t.Errorf("zero vector should remain zero, got %f", v)
		}
	}
}

func TestIsNomicModel(t *testing.T) {
	if !isNomicModel("nomic-embed-text") {
		t.Error("should detect nomic model")
	}
	if !isNomicModel("Nomic-Embed-v1.5") {
		t.Error("should be case-insensitive")
	}
	if isNomicModel("text-embedding-3-small") {
		t.Error("should not match non-nomic model")
	}
}

func TestIsRetryableEmbeddingError(t *testing.T) {
	if isRetryableEmbeddingError(nil) {
		t.Error("nil error should not be retryable")
	}

	tests := []struct {
		errMsg string
		want   bool
	}{
		{"connection refused", true},
		{"timeout", true},
		{"status 429 too many requests", true},
		{"status 503 temporarily unavailable", true},
		{"invalid input", false},
	}
	for _, tt := range tests {
		err := &testError{tt.errMsg}
		if got := isRetryableEmbeddingError(err); got != tt.want {
			t.Errorf("isRetryableEmbeddingError(%q) = %v, want %v", tt.errMsg, got, tt.want)
		}
	}
}

func TestCreateEmbeddingProvider(t *testing.T) {
	// Mock provider
	p, err := CreateEmbeddingProvider("mock", "", "", "", nil)
	if err != nil {
		t.Fatalf("failed to create mock provider: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	// Unknown provider
	_, err = CreateEmbeddingProvider("unknown", "", "", "", nil)
	if err == nil {
		t.Error("expected error for unknown provider")
	}

	// Nomic without API key
	_, err = CreateEmbeddingProvider("nomic", "", "", "", nil)
	if err == nil {
		t.Error("expected error for nomic without API key")
	}

	// OpenAI without API key
	_, err = CreateEmbeddingProvider("openai", "", "", "", nil)
	if err == nil {
		t.Error("expected error for openai without API key")
	}

	// Ollama with defaults
	p, err = CreateEmbeddingProvider("ollama", "", "", "", nil)
	if err != nil {
		t.Fatalf("failed to create ollama provider: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestEmbeddingGenerator(t *testing.T) {
	provider := NewMockEmbeddingProvider(384, nil)
	gen := NewEmbeddingGenerator(provider, nil)

	ctx := context.Background()
	emb, err := gen.Generate(ctx, "test text")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(emb) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(emb))
	}

	queryEmb, err := gen.GenerateQuery(ctx, "test query")
	if err != nil {
		t.Fatalf("GenerateQuery failed: %v", err)
	}
	if len(queryEmb) != 384 {
		t.Errorf("expected 384 dimensions, got %d", len(queryEmb))
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
