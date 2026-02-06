// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	cozo "github.com/kraklabs/mie/pkg/cozodb"
)

// EmbeddedBackend implements Backend using a local CozoDB instance.
// This is the default backend for standalone/open-source MIE.
type EmbeddedBackend struct {
	db                  *cozo.CozoDB
	mu                  sync.RWMutex
	closed              bool
	embeddingDimensions int
}

// EmbeddedConfig configures the embedded backend.
type EmbeddedConfig struct {
	// DataDir is the directory where CozoDB stores its data.
	// Defaults to ~/.mie/data/<project_id>
	DataDir string

	// Engine is the CozoDB storage engine: "rocksdb", "sqlite", or "mem".
	// Defaults to "rocksdb" for persistence.
	Engine string

	// ProjectID is used to namespace the data directory.
	ProjectID string

	// EmbeddingDimensions is the vector size for embeddings.
	// Defaults to 768 (nomic-embed-text). Use 1536 for OpenAI.
	EmbeddingDimensions int
}

// NewEmbeddedBackend creates a new embedded CozoDB backend.
func NewEmbeddedBackend(config EmbeddedConfig) (*EmbeddedBackend, error) {
	// Set defaults
	if config.Engine == "" {
		config.Engine = "rocksdb"
	}
	if config.DataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		config.DataDir = filepath.Join(homeDir, ".mie", "data")
		if config.ProjectID != "" {
			config.DataDir = filepath.Join(config.DataDir, config.ProjectID)
		}
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Open CozoDB
	db, err := cozo.New(config.Engine, config.DataDir, nil)
	if err != nil {
		return nil, fmt.Errorf("open cozodb: %w", err)
	}

	// Default embedding dimensions to 768 (nomic-embed-text)
	embeddingDim := config.EmbeddingDimensions
	if embeddingDim <= 0 {
		embeddingDim = 768
	}

	return &EmbeddedBackend{
		db:                  &db,
		embeddingDimensions: embeddingDim,
	}, nil
}

// Query executes a read-only Datalog query.
func (b *EmbeddedBackend) Query(ctx context.Context, datalog string) (*QueryResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, fmt.Errorf("backend is closed")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	result, err := b.db.RunReadOnly(datalog, nil)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return FromNamedRows(result), nil
}

// Execute runs a Datalog mutation.
func (b *EmbeddedBackend) Execute(ctx context.Context, datalog string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("backend is closed")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	_, err := b.db.Run(datalog, nil)
	if err != nil {
		return fmt.Errorf("execute failed: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (b *EmbeddedBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	b.db.Close()
	return nil
}

// DB returns the underlying CozoDB instance for advanced operations.
// Use with caution - prefer the Backend interface methods.
func (b *EmbeddedBackend) DB() *cozo.CozoDB {
	return b.db
}

// EnsureSchema creates the MIE metadata table if it doesn't exist.
// This is idempotent and safe to call multiple times.
//
// Full MIE schema is in pkg/memory/schema.go — applied by memory.EnsureSchema()
func (b *EmbeddedBackend) EnsureSchema() error {
	tables := []string{
		`:create mie_meta { key: String => value: String }`,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, table := range tables {
		_, err := b.db.Run(table, nil)
		if err != nil {
			// Ignore "already exists" errors
			errStr := err.Error()
			if strings.Contains(errStr, "already exists") ||
				strings.Contains(errStr, "conflicts with an existing one") {
				continue
			}
			return fmt.Errorf("create table failed: %w", err)
		}
	}

	return nil
}

// CreateHNSWIndex creates HNSW indexes for semantic search.
// Should be called after schema creation.
// dimensions: embedding vector size (768 for nomic-embed-text, 1536 for OpenAI)
//
// Full MIE schema is in pkg/memory/schema.go — applied by memory.EnsureSchema()
func (b *EmbeddedBackend) CreateHNSWIndex(dimensions int) error {
	// HNSW indexes for MIE embedding tables will be created by
	// memory.EnsureSchema() once the full domain schema is in place.
	return nil
}

// GetMeta retrieves a metadata value by key.
// Returns empty string if key doesn't exist.
func (b *EmbeddedBackend) GetMeta(key string) (string, error) {
	query := `?[value] := *mie_meta{key, value}, key = $key`
	params := map[string]any{"key": key}

	b.mu.RLock()
	result, err := b.db.Run(query, params)
	b.mu.RUnlock()

	if err != nil {
		return "", err
	}

	if len(result.Rows) == 0 {
		return "", nil
	}

	if val, ok := result.Rows[0][0].(string); ok {
		return val, nil
	}
	return "", nil
}

// SetMeta sets a metadata value by key.
func (b *EmbeddedBackend) SetMeta(key, value string) error {
	query := `?[key, value] <- [[$key, $value]] :put mie_meta { key, value }`
	params := map[string]any{"key": key, "value": value}

	b.mu.Lock()
	_, err := b.db.Run(query, params)
	b.mu.Unlock()

	return err
}