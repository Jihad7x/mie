//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/kraklabs/mie/pkg/memory"
)

// StatusResult represents the memory graph status for JSON output.
type StatusResult struct {
	StorageEngine     string    `json:"storage_engine"`
	DataDir           string    `json:"data_dir"`
	Connected         bool      `json:"connected"`
	Facts             int       `json:"facts"`
	ValidFacts        int       `json:"valid_facts"`
	InvalidatedFacts  int       `json:"invalidated_facts"`
	Decisions         int       `json:"decisions"`
	ActiveDecisions   int       `json:"active_decisions"`
	Entities          int       `json:"entities"`
	Events            int       `json:"events"`
	Topics            int       `json:"topics"`
	Edges             int       `json:"edges"`
	EmbeddingsEnabled bool      `json:"embeddings_enabled"`
	Timestamp         time.Time `json:"timestamp"`
	Error             string    `json:"error,omitempty"`
}

// runStatus displays memory graph statistics.
func runStatus(args []string, configPath string, globals GlobalFlags) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie status [options]

Description:
  Display the current status of the MIE memory graph including
  node counts, configuration, and health information.

Options (inherited):
  --json    Output as JSON

Examples:
  mie status            Show human-readable status
  mie status --json     Output as JSON

`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		cfg = DefaultConfig()
		cfg.applyEnvOverrides()
	}

	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	result := &StatusResult{
		StorageEngine:     cfg.Storage.Engine,
		DataDir:           dataDir,
		EmbeddingsEnabled: cfg.Embedding.Enabled,
		Timestamp:         time.Now(),
	}

	// Check if data directory exists
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		result.Connected = false
		result.Error = "No data found. Run 'mie --mcp' to start the server and create the database."
		if globals.JSON {
			outputStatusJSON(result)
		} else {
			fmt.Println("MIE Memory Status")
			fmt.Println()
			fmt.Printf("  No data found at %s\n", dataDir)
			fmt.Println("  Run 'mie --mcp' to start the MCP server.")
		}
		return
	}

	// Open memory client
	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:       dataDir,
		StorageEngine: cfg.Storage.Engine,
	})
	if err != nil {
		result.Connected = false
		result.Error = fmt.Sprintf("Cannot open database: %v", err)
		if globals.JSON {
			outputStatusJSON(result)
		} else {
			fmt.Fprintf(os.Stderr, "Error: cannot open database: %v\n", err)
		}
		os.Exit(ExitDatabase)
	}
	defer func() { _ = client.Close() }()

	result.Connected = true
	ctx := context.Background()

	stats, err := client.GetStats(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("Cannot read stats: %v", err)
		if globals.JSON {
			outputStatusJSON(result)
		} else {
			fmt.Fprintf(os.Stderr, "Error: cannot read stats: %v\n", err)
		}
		os.Exit(ExitDatabase)
	}

	result.Facts = stats.TotalFacts
	result.ValidFacts = stats.ValidFacts
	result.InvalidatedFacts = stats.InvalidatedFacts
	result.Decisions = stats.TotalDecisions
	result.ActiveDecisions = stats.ActiveDecisions
	result.Entities = stats.TotalEntities
	result.Events = stats.TotalEvents
	result.Topics = stats.TotalTopics
	result.Edges = stats.TotalEdges

	if globals.JSON {
		outputStatusJSON(result)
	} else {
		printStatus(result, cfg)
	}
}

func outputStatusJSON(result *StatusResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
}

func printStatus(result *StatusResult, cfg *Config) {
	fmt.Println("MIE Memory Status")
	fmt.Println()

	fmt.Println("Graph Statistics:")
	fmt.Printf("  Facts:       %d (%d valid, %d invalidated)\n", result.Facts, result.ValidFacts, result.InvalidatedFacts)
	fmt.Printf("  Decisions:   %d (%d active)\n", result.Decisions, result.ActiveDecisions)
	fmt.Printf("  Entities:    %d\n", result.Entities)
	fmt.Printf("  Events:      %d\n", result.Events)
	fmt.Printf("  Topics:      %d\n", result.Topics)
	fmt.Printf("  Edges:       %d total\n", result.Edges)
	fmt.Println()

	fmt.Println("Configuration:")
	fmt.Printf("  Storage:     %s (%s)\n", cfg.Storage.Engine, result.DataDir)
	if cfg.Embedding.Enabled {
		fmt.Printf("  Embeddings:  enabled (%s, %dd)\n", cfg.Embedding.Model, cfg.Embedding.Dimensions)
	} else {
		fmt.Printf("  Embeddings:  disabled\n")
	}
	fmt.Printf("  Schema:      v%s\n", configVersion)
}
