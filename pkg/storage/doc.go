// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

// Package storage provides storage backend abstractions for MIE.
//
// This package defines the Backend interface that allows MIE tools to work
// with different storage implementations. The abstraction enables the same
// MCP tools to operate against either a local embedded database or a remote
// MIE service.
//
// # Available Backends
//
// The package provides these backend implementations:
//
//   - EmbeddedBackend: Local CozoDB instance for standalone/open-source use
//   - Remote backends: Available in MIE Enterprise (not included in this package)
//
// # Quick Start
//
// Create an embedded backend and execute queries:
//
//	backend, err := storage.NewEmbeddedBackend(storage.EmbeddedConfig{
//	    DataDir:   "/path/to/data",
//	    Engine:    "rocksdb",
//	    ProjectID: "myproject",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer backend.Close()
//
//	// Initialize schema
//	if err := backend.EnsureSchema(); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Execute a query
//	result, err := backend.Query(ctx, `
//	    ?[id, content] := *mie_fact{id, content}
//	    :limit 10
//	`)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, row := range result.Rows {
//	    fmt.Printf("%s: %s\n", row[0], row[1])
//	}
//
// # Schema Initialization
//
// Before storing memories, initialize the MIE schema:
//
//	// Create base metadata table (idempotent)
//	err := backend.EnsureSchema()
//
// The full domain schema (facts, decisions, entities, events, embeddings)
// is managed by pkg/memory/schema.go.
//
// # Query vs Execute
//
// Use Query for read operations and Execute for mutations:
//
//	// Read-only query (uses RunReadOnly internally)
//	result, err := backend.Query(ctx, `?[count(f)] := *mie_fact{id: f}`)
//
//	// Mutation (uses Run internally)
//	err := backend.Execute(ctx, `:rm mie_fact { id: "fact123" }`)
//
// # Configuration
//
// EmbeddedConfig controls the backend behavior:
//
//	config := storage.EmbeddedConfig{
//	    DataDir:   "/path/to/data",  // Where to store CozoDB data
//	    Engine:    "rocksdb",        // Storage engine: mem, sqlite, rocksdb
//	    ProjectID: "myproject",      // Namespaces data directory
//	}
//
// Default values if not specified:
//   - DataDir: ~/.mie/data/<project_id>
//   - Engine: "rocksdb" (recommended for production)
//
// # Thread Safety
//
// EmbeddedBackend is safe for concurrent use. Read operations use a read
// lock while write operations use an exclusive lock, allowing concurrent
// reads but exclusive writes.
//
// # Direct Database Access
//
// For advanced operations, access the underlying CozoDB instance:
//
//	db := backend.DB()
//	result, err := db.Run(`::relations`, nil)  // List all relations
//
// Use with caution - prefer the Backend interface methods for normal operations.
package storage