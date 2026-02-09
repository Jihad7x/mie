// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestFedoraServer_Integration(t *testing.T) {
	serverURL := os.Getenv("LLM_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://100.117.59.45:8000/v1"
	}

	provider, err := NewProvider(ProviderConfig{
		Type:         "openai",
		BaseURL:      serverURL,
		DefaultModel: "qwen2.5-coder-32b-instruct-q5_k_m.gguf",
		Timeout:      2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewProvider error: %v", err)
	}

	t.Logf("Provider: %s", provider.Name())

	ctx := context.Background()
	resp, err := provider.Chat(ctx, ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are a helpful coding assistant. Be concise."},
			{Role: "user", Content: "What is 2+2? Answer with just the number."},
		},
		MaxTokens:   10,
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Chat error: %v", err)
	}

	t.Logf("Response: %s", resp.Message.Content)
	t.Logf("Tokens: %d prompt + %d output = %d total", resp.PromptTokens, resp.OutputTokens, resp.TotalTokens)
	t.Logf("Duration: %v", resp.Duration)
}
