// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"testing"

	"github.com/kraklabs/mie/pkg/storage"
)

// newTestBackend creates an in-memory CozoDB backend for testing.
func newTestBackend(t *testing.T) *storage.EmbeddedBackend {
	t.Helper()
	backend, err := storage.NewEmbeddedBackend(storage.EmbeddedConfig{
		Engine:              "mem",
		DataDir:             t.TempDir(),
		EmbeddingDimensions: 384,
	})
	if err != nil {
		t.Fatalf("create test backend: %v", err)
	}
	return backend
}

// setupSchema creates all MIE tables in the test backend.
func setupSchema(t *testing.T, backend *storage.EmbeddedBackend) {
	t.Helper()
	if err := backend.EnsureSchema(); err != nil {
		t.Fatalf("ensure storage schema: %v", err)
	}
	if err := EnsureSchema(backend, 384); err != nil {
		t.Fatalf("ensure mie schema: %v", err)
	}
}
