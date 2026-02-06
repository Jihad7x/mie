// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

// Package cozodb provides a Go binding for CozoDB v0.7.6+.
//
// CozoDB is a Datalog-based embedded database designed for graph queries
// and complex data relationships. MIE uses it to store memory intelligence
// data including facts, decisions, entities, events, and vector embeddings
// for semantic search.
//
// # Requirements
//
// This package requires CGO and the CozoDB C library (libcozo_c). Build with:
//
//	CGO_ENABLED=1 go build -tags cozodb
//
// The CozoDB library must be installed on your system:
//
//	# macOS (Homebrew)
//	brew install cozodb
//
//	# Linux (from source or package manager)
//	# See https://github.com/cozodb/cozo for installation
//
// You may need to set library paths:
//
//	export CGO_LDFLAGS="-L/path/to/libcozo_c"
//	export CGO_CFLAGS="-I/path/to/cozo_c.h"
//
// # Storage Engines
//
// CozoDB supports multiple storage backends:
//   - "mem": In-memory, fast but not persisted (good for testing)
//   - "sqlite": SQLite-backed, single-file persistence
//   - "rocksdb": RocksDB-backed, best performance for production
//
// # Quick Start
//
// Open a database and run queries:
//
//	// Open with RocksDB storage
//	db, err := cozodb.New("rocksdb", "/path/to/data", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	// Run a simple query
//	result, err := db.Run(`?[x] := x = 1 + 1`, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("1 + 1 = %v\n", result.Rows[0][0])
//
// # Read-Only Queries
//
// Use RunReadOnly for queries that should not modify data:
//
//	// This enforces read-only semantics at the database level
//	result, err := db.RunReadOnly(`?[content] := *mie_fact{content}`, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Parameterized Queries
//
// Pass parameters to prevent injection and improve readability:
//
//	params := map[string]any{
//	    "topic": "architecture",
//	}
//	result, err := db.Run(`
//	    ?[id, content] :=
//	        *mie_fact{id, content, topic},
//	        topic == $topic
//	`, params)
//
// # Backup and Restore
//
// Create and restore database backups:
//
//	// Create backup
//	err := db.Backup("/path/to/backup.db")
//
//	// Restore from backup
//	err := db.Restore("/path/to/backup.db")
//
// # MIE Data Model
//
// MIE uses these main relations (tables) for memory intelligence:
//
//	mie_fact              - Facts extracted from conversations
//	mie_decision          - Decisions and their rationale
//	mie_entity            - Named entities (people, projects, tools)
//	mie_event             - Time-anchored events
//	mie_topic             - Topic categorization
//	mie_fact_embedding    - Vector embeddings for semantic search
//	mie_decision_embedding - Decision embeddings
//	mie_entity_embedding  - Entity embeddings
//	mie_event_embedding   - Event embeddings
//	mie_meta              - Project metadata
//
// # Version Compatibility
//
// This binding targets CozoDB v0.7.6+ which includes the immutable_query
// parameter in the C API. Earlier versions may not work correctly with
// the RunReadOnly method.
package cozodb
