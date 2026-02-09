// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"context"

	cozo "github.com/kraklabs/mie/pkg/cozodb"
)

// Backend is the interface that all storage backends must implement.
// It provides methods for executing queries and mutations on the memory graph.
type Backend interface {
	// Query executes a read-only Datalog query and returns the results.
	Query(ctx context.Context, datalog string) (*QueryResult, error)

	// Execute runs a Datalog mutation (insert, update, delete).
	Execute(ctx context.Context, datalog string) error

	// Close releases any resources held by the backend.
	Close() error
}

// QueryResult represents the result of a Datalog query.
type QueryResult struct {
	Headers []string
	Rows    [][]any
}

// ToNamedRows converts QueryResult to CozoDB NamedRows for compatibility.
func (r *QueryResult) ToNamedRows() cozo.NamedRows {
	return cozo.NamedRows{
		Headers: r.Headers,
		Rows:    r.Rows,
	}
}

// FromNamedRows converts CozoDB NamedRows to QueryResult.
func FromNamedRows(nr cozo.NamedRows) *QueryResult {
	return &QueryResult{
		Headers: nr.Headers,
		Rows:    nr.Rows,
	}
}
