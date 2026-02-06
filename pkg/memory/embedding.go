// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EmbeddingProvider generates embeddings for text.
type EmbeddingProvider interface {
	// Embed generates an embedding vector for the given text.
	// Returns a normalized vector (L2 norm = 1.0) or error.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedQuery generates an embedding for a search query.
	// Some providers (e.g., Nomic) use different prefixes for queries vs documents.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// RetryConfig controls retry behavior for embedding calls.
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// EmbeddingGenerator manages embedding generation with retries.
type EmbeddingGenerator struct {
	provider EmbeddingProvider
	logger   *slog.Logger
	retry    RetryConfig
}

// NewEmbeddingGenerator creates a new embedding generator.
func NewEmbeddingGenerator(provider EmbeddingProvider, logger *slog.Logger) *EmbeddingGenerator {
	if logger == nil {
		logger = slog.Default()
	}
	return &EmbeddingGenerator{
		provider: provider,
		logger:   logger,
		retry: RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 200 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
			Multiplier:     2.0,
		},
	}
}

// Generate generates an embedding for document text with retry logic.
func (eg *EmbeddingGenerator) Generate(ctx context.Context, text string) ([]float32, error) {
	return eg.embedWithRetry(ctx, text, false)
}

// GenerateQuery generates an embedding for a search query with retry logic.
func (eg *EmbeddingGenerator) GenerateQuery(ctx context.Context, text string) ([]float32, error) {
	return eg.embedWithRetry(ctx, text, true)
}

func (eg *EmbeddingGenerator) embedWithRetry(ctx context.Context, text string, isQuery bool) ([]float32, error) {
	var embedding []float32
	var err error

	for attempt := 0; attempt < eg.retry.MaxRetries; attempt++ {
		if isQuery {
			embedding, err = eg.provider.EmbedQuery(ctx, text)
		} else {
			embedding, err = eg.provider.Embed(ctx, text)
		}
		if err == nil {
			return embedding, nil
		}
		if !isRetryableEmbeddingError(err) || attempt == eg.retry.MaxRetries-1 {
			break
		}
		sleep := computeBackoffWithJitter(eg.retry.InitialBackoff, attempt, eg.retry.Multiplier, eg.retry.MaxBackoff)
		eg.logger.Warn("embedding.retry", "attempt", attempt+1, "sleep_ms", sleep.Milliseconds(), "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleep):
		}
	}

	return nil, fmt.Errorf("embedding failed after %d attempts: %w", eg.retry.MaxRetries, err)
}

// =============================================================================
// MOCK EMBEDDING PROVIDER
// =============================================================================

// MockEmbeddingProvider generates deterministic mock embeddings for testing.
type MockEmbeddingProvider struct {
	dimension int
	logger    *slog.Logger
}

// NewMockEmbeddingProvider creates a mock embedding provider.
func NewMockEmbeddingProvider(dimension int, logger *slog.Logger) *MockEmbeddingProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &MockEmbeddingProvider{dimension: dimension, logger: logger}
}

// Embed generates a deterministic mock embedding based on text hash.
func (m *MockEmbeddingProvider) Embed(_ context.Context, text string) ([]float32, error) {
	return m.generateDeterministic(text), nil
}

// EmbedQuery generates a deterministic mock query embedding.
func (m *MockEmbeddingProvider) EmbedQuery(_ context.Context, text string) ([]float32, error) {
	return m.generateDeterministic(text), nil
}

func (m *MockEmbeddingProvider) generateDeterministic(text string) []float32 {
	hash := hashString(text)
	embedding := make([]float32, m.dimension)
	for i := 0; i < m.dimension; i++ {
		val := float32((hash+uint64(i)*7919)%10000) / 10000.0 //nolint:gosec
		embedding[i] = val*2.0 - 1.0
	}
	return normalizeEmbedding(embedding)
}

func hashString(s string) uint64 {
	var hash uint64 = 5381
	for _, c := range s {
		hash = ((hash << 5) + hash) + uint64(c)
	}
	return hash
}

// =============================================================================
// OLLAMA EMBEDDING PROVIDER
// =============================================================================

// OllamaEmbeddingProvider generates embeddings using a local Ollama server.
type OllamaEmbeddingProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"`
}

type ollamaErrorResponse struct {
	Error string `json:"error"`
}

// NewOllamaEmbeddingProvider creates a new Ollama embedding provider.
func NewOllamaEmbeddingProvider(baseURL, model string, logger *slog.Logger) *OllamaEmbeddingProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &OllamaEmbeddingProvider{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		logger: logger,
	}
}

// Embed generates an embedding for document text using Ollama.
func (o *OllamaEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	prompt := text
	if isNomicModel(o.model) {
		prompt = "search_document: " + text
	}
	return o.embed(ctx, prompt)
}

// EmbedQuery generates an embedding for a search query using Ollama.
func (o *OllamaEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	prompt := text
	if isNomicModel(o.model) {
		prompt = "search_query: " + text
	}
	return o.embed(ctx, prompt)
}

func (o *OllamaEmbeddingProvider) embed(ctx context.Context, prompt string) ([]float32, error) {
	reqBody := ollamaEmbedRequest{Model: o.model, Prompt: prompt}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/api/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request (is Ollama running at %s?): %w", o.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ollamaErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, errResp.Error)
		}
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	var embedResp ollamaEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(embedResp.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}

	embedding := make([]float32, len(embedResp.Embedding))
	for i, v := range embedResp.Embedding {
		embedding[i] = float32(v)
	}

	return normalizeEmbedding(embedding), nil
}

// =============================================================================
// OPENAI-COMPATIBLE EMBEDDING PROVIDER
// =============================================================================

// OpenAIEmbeddingProvider generates embeddings using OpenAI or compatible APIs.
type OpenAIEmbeddingProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

type openAIEmbedRequest struct {
	Input          string `json:"input"`
	Model          string `json:"model"`
	EncodingFormat string `json:"encoding_format,omitempty"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// NewOpenAIEmbeddingProvider creates a new OpenAI embedding provider.
func NewOpenAIEmbeddingProvider(apiKey, baseURL, model string, logger *slog.Logger) *OpenAIEmbeddingProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &OpenAIEmbeddingProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

// Embed generates an embedding for document text using OpenAI API.
func (o *OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return o.embed(ctx, text)
}

// EmbedQuery generates an embedding for a search query using OpenAI API.
func (o *OpenAIEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return o.embed(ctx, text)
}

func (o *OpenAIEmbeddingProvider) embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := openAIEmbedRequest{
		Input:          text,
		Model:          o.model,
		EncodingFormat: "float",
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(body))
	}

	var embedResp openAIEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding")
	}

	embedding := make([]float32, len(embedResp.Data[0].Embedding))
	for i, v := range embedResp.Data[0].Embedding {
		embedding[i] = float32(v)
	}

	return normalizeEmbedding(embedding), nil
}

// =============================================================================
// NOMIC EMBEDDING PROVIDER
// =============================================================================

// NomicEmbeddingProvider generates embeddings using the Nomic Atlas API.
type NomicEmbeddingProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

type nomicEmbedRequest struct {
	Texts    []string `json:"texts"`
	Model    string   `json:"model"`
	TaskType string   `json:"task_type,omitempty"`
}

type nomicEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

type nomicErrorResponse struct {
	Detail string `json:"detail"`
}

// NewNomicEmbeddingProvider creates a new Nomic embedding provider.
func NewNomicEmbeddingProvider(apiKey, baseURL, model string, logger *slog.Logger) *NomicEmbeddingProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &NomicEmbeddingProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

// Embed generates an embedding for document text using Nomic API.
func (n *NomicEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return n.embed(ctx, text, "search_document")
}

// EmbedQuery generates an embedding for a search query using Nomic API.
func (n *NomicEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return n.embed(ctx, text, "search_query")
}

func (n *NomicEmbeddingProvider) embed(ctx context.Context, text, taskType string) ([]float32, error) {
	reqBody := nomicEmbedRequest{
		Texts:    []string{text},
		Model:    n.model,
		TaskType: taskType,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := n.baseURL + "/embedding/text"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+n.apiKey)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp nomicErrorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Detail != "" {
			return nil, fmt.Errorf("nomic API error (status %d): %s", resp.StatusCode, errResp.Detail)
		}
		return nil, fmt.Errorf("nomic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var embedResp nomicEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(embedResp.Embeddings) == 0 {
		return nil, fmt.Errorf("nomic returned empty embeddings")
	}

	embedding := make([]float32, len(embedResp.Embeddings[0]))
	for i, v := range embedResp.Embeddings[0] {
		embedding[i] = float32(v)
	}

	return normalizeEmbedding(embedding), nil
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// normalizeEmbedding normalizes an embedding vector to unit length (L2 norm = 1).
func normalizeEmbedding(embedding []float32) []float32 {
	if len(embedding) == 0 {
		return embedding
	}

	var norm float64
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return embedding
	}

	normf := float32(norm)
	for i := range embedding {
		embedding[i] /= normf
	}

	return embedding
}

func isNomicModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "nomic")
}

// isRetryableEmbeddingError classifies provider errors.
func isRetryableEmbeddingError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	retrySubstr := []string{"timeout", "temporarily unavailable", "connection refused", "connection reset", "deadline exceeded", "EOF"}
	for _, s := range retrySubstr {
		if containsFold(msg, s) {
			return true
		}
	}
	httpRetry := []string{" 429 ", " 500 ", " 502 ", " 503 ", " 504 "}
	for _, s := range httpRetry {
		if containsFold(msg, s) {
			return true
		}
	}
	return false
}

func computeBackoffWithJitter(base time.Duration, attempt int, mult float64, capDur time.Duration) time.Duration {
	exp := float64(base)
	for i := 0; i < attempt; i++ {
		exp *= mult
	}
	d := time.Duration(exp)
	if d > capDur {
		d = capDur
	}
	if d <= 0 {
		return base
	}
	n := time.Duration(randInt63n(int64(d) + 1))
	return n
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

var randMu sync.Mutex
var randSeed int64

func randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	randMu.Lock()
	defer randMu.Unlock()
	const a = 6364136223846793005
	const c = 1
	const m = 1<<63 - 1
	if randSeed == 0 {
		randSeed = time.Now().UnixNano() & m
	}
	randSeed = (a*randSeed + c) & m
	if randSeed < 0 {
		randSeed = -randSeed
	}
	return randSeed % n
}

// CreateEmbeddingProvider creates an embedding provider based on config.
func CreateEmbeddingProvider(providerType, apiKey, baseURL, model string, logger *slog.Logger) (EmbeddingProvider, error) {
	switch providerType {
	case "mock":
		return NewMockEmbeddingProvider(768, logger), nil

	case "nomic":
		if apiKey == "" {
			return nil, fmt.Errorf("api_key is required for nomic provider")
		}
		if baseURL == "" {
			baseURL = "https://api-atlas.nomic.ai/v1"
		}
		if model == "" {
			model = "nomic-embed-text-v1.5"
		}
		return NewNomicEmbeddingProvider(apiKey, baseURL, model, logger), nil

	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		if model == "" {
			model = "nomic-embed-text"
		}
		return NewOllamaEmbeddingProvider(baseURL, model, logger), nil

	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("api_key is required for openai provider")
		}
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		if model == "" {
			model = "text-embedding-3-small"
		}
		return NewOpenAIEmbeddingProvider(apiKey, baseURL, model, logger), nil

	default:
		return nil, fmt.Errorf("unknown embedding provider: %s (supported: mock, nomic, ollama, openai)", providerType)
	}
}