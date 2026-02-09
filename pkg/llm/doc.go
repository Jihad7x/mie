// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

// Package llm provides a unified interface for Large Language Model providers.
//
// This package abstracts the differences between various LLM APIs, providing
// a consistent interface for text generation and chat completions. It is used
// by MIE's tools to generate natural language responses about stored memories.
//
// # Supported Providers
//
// The following LLM providers are supported:
//   - Ollama: Local models, no API key required (default)
//   - OpenAI: GPT-4, GPT-4o-mini, and OpenAI-compatible APIs
//   - Anthropic: Claude models
//   - Mock: For testing without real API calls
//
// # Quick Start
//
// Create a provider explicitly:
//
//	provider, err := llm.NewProvider(llm.ProviderConfig{
//	    Type:   "openai",
//	    APIKey: os.Getenv("OPENAI_API_KEY"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	resp, err := provider.Generate(ctx, llm.GenerateRequest{
//	    Prompt: "Summarize this memory context: ...",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(resp.Text)
//
// Or use convenience functions that auto-detect the provider:
//
//	response, err := llm.QuickGenerate(ctx, "Summarize this context")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(response)
//
// # Chat Completions
//
// For multi-turn conversations, use the Chat method:
//
//	messages := []llm.Message{
//	    {Role: "system", Content: "You are a helpful memory assistant."},
//	    {Role: "user", Content: "What do I know about this topic?"},
//	}
//
//	resp, err := provider.Chat(ctx, llm.ChatRequest{
//	    Messages: messages,
//	})
//
// Or use the BuildChatMessages helper:
//
//	messages := llm.BuildChatMessages(
//	    llm.SystemPrompts.CodeExplain, // system prompt
//	    "What does this function do?", // user prompt
//	)
//
// # Provider Selection
//
// The [DefaultProvider] function automatically selects a provider based on
// available environment variables, checking in order:
//  1. OLLAMA_HOST or OLLAMA_MODEL set - Uses Ollama (local)
//  2. OPENAI_API_KEY set - Uses OpenAI
//  3. ANTHROPIC_API_KEY set - Uses Anthropic
//  4. No credentials - Falls back to mock provider
//
// # Environment Variables
//
// Ollama (local, free):
//   - OLLAMA_HOST: Server URL (default: http://localhost:11434)
//   - OLLAMA_MODEL: Model name (e.g., "llama2", "codellama")
//
// OpenAI:
//   - OPENAI_API_KEY: API key (required)
//   - OPENAI_BASE_URL: API URL for compatible services (e.g., Azure)
//   - OPENAI_MODEL: Model name (default: gpt-4o-mini)
//
// Anthropic:
//   - ANTHROPIC_API_KEY: API key (required)
//   - ANTHROPIC_MODEL: Model name (default: claude-3-5-sonnet-20241022)
//
// # Code Analysis Helpers
//
// The package provides pre-built system prompts for common code tasks:
//
//	// Use predefined prompts
//	messages := llm.BuildChatMessages(
//	    llm.SystemPrompts.CodeReview,  // or CodeExplain, CodeDebug, etc.
//	    "Review this function for bugs...",
//	)
//
// Or build structured code prompts:
//
//	prompt := llm.CodePrompt{
//	    Task:     "Explain this function",
//	    Language: "go",
//	    Code:     "func main() { ... }",
//	}.Build()
//
// # Error Handling
//
// All provider methods return descriptive errors that include context about
// the failure. Network errors, API errors, and validation errors are all
// wrapped with appropriate context.
//
//	resp, err := provider.Generate(ctx, req)
//	if err != nil {
//	    // Error includes provider name and context
//	    // e.g., "openai chat error (status 401): invalid api key"
//	    return err
//	}
package llm
