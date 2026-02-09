//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kraklabs/mie/pkg/memory"
)

// runRepair rebuilds HNSW indexes and cleans orphaned embeddings.
func runRepair(configPath string, globals GlobalFlags) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		cfg = DefaultConfig()
		cfg.applyEnvOverrides()
	}

	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitDatabase)
	}

	if !globals.Quiet {
		fmt.Printf("Repairing HNSW indexes at %s...\n", dataDir)
	}

	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:             dataDir,
		StorageEngine:       cfg.Storage.Engine,
		EmbeddingEnabled:    cfg.Embedding.Enabled,
		EmbeddingProvider:   cfg.Embedding.Provider,
		EmbeddingBaseURL:    cfg.Embedding.BaseURL,
		EmbeddingModel:      cfg.Embedding.Model,
		EmbeddingAPIKey:     cfg.Embedding.APIKey,
		EmbeddingDimensions: cfg.Embedding.Dimensions,
		EmbeddingWorkers:    cfg.Embedding.Workers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot initialize MIE: %v\n", err)
		os.Exit(ExitDatabase)
	}
	defer func() { _ = client.Close() }()

	if err := client.RepairHNSWIndexes(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: repair failed: %v\n", err)
		os.Exit(ExitDatabase)
	}

	n, backfillErr := client.BackfillEmbeddings(context.Background())
	if backfillErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: embedding backfill failed: %v\n", backfillErr)
	}

	if !globals.Quiet {
		if n > 0 {
			fmt.Printf("Backfilled %d missing embeddings.\n", n)
		}
		fmt.Println("Repair complete. HNSW indexes rebuilt successfully.")
	}
}
