//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/kraklabs/mie/pkg/tools"
)

// runExport exports the memory graph to stdout or a file.
func runExport(args []string, configPath string, globals GlobalFlags) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	format := fs.String("format", "json", "Export format: json or datalog")
	output := fs.StringP("output", "o", "", "Output file (default: stdout)")
	includeEmbeddings := fs.Bool("include-embeddings", false, "Include embedding vectors (large)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie export [options]

Description:
  Export the complete memory graph for backup or migration.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  mie export                              JSON to stdout
  mie export --output memory.json         JSON to file
  mie export --format datalog             Datalog format
  mie export --include-embeddings         Include vectors (large)

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

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: no data found at %s\n", dataDir)
		os.Exit(ExitDatabase)
	}

	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:       dataDir,
		StorageEngine: cfg.Storage.Engine,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot open database: %v\n", err)
		os.Exit(ExitDatabase)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	exportArgs := map[string]any{
		"format":             *format,
		"include_embeddings": *includeEmbeddings,
	}

	result, err := tools.Export(ctx, client, exportArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitGeneral)
	}

	if *output != "" {
		if err := os.WriteFile(*output, []byte(result.Text), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot write to %s: %v\n", *output, err)
			os.Exit(ExitGeneral)
		}
		if !globals.Quiet {
			fmt.Fprintf(os.Stderr, "Exported to %s\n", *output)
		}
	} else {
		fmt.Print(result.Text)
	}
}