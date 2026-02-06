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
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/kraklabs/mie/pkg/memory"
)

// runQuery executes a raw CozoScript query for debugging.
func runQuery(args []string, configPath string, globals GlobalFlags) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie query <cozoscript> [options]

Description:
  Execute a raw CozoScript query against the MIE database.
  This is a debugging tool for inspecting the underlying data.

Options (inherited):
  --json    Output as JSON

Examples:
  mie query "?[name] := *mie_entity { name } :limit 10"
  mie query "?[count(id)] := *mie_fact { id }"
  mie query "?[id, content] := *mie_fact { id, content, valid }, valid = true :limit 5"

`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintf(os.Stderr, "Error: query argument required\n")
		fmt.Fprintf(os.Stderr, "Usage: mie query \"<cozoscript>\"\n")
		os.Exit(ExitQuery)
	}

	script := strings.Join(remaining, " ")

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
		fmt.Fprintf(os.Stderr, "Run 'mie --mcp' to start the server and create the database.\n")
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
	result, err := client.RawQuery(ctx, script)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query error: %v\n", err)
		os.Exit(ExitQuery)
	}

	if globals.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	// Human-readable output
	fmt.Printf("Found %d results\n\n", len(result.Rows))

	if len(result.Rows) == 0 {
		fmt.Println("No results.")
		return
	}

	// Print headers
	if len(result.Headers) > 0 {
		fmt.Println(strings.Join(result.Headers, "\t"))
		fmt.Println(strings.Repeat("-", 60))
	}

	// Print rows
	for _, row := range result.Rows {
		vals := make([]string, len(row))
		for i, v := range row {
			vals[i] = fmt.Sprintf("%v", v)
			if len(vals[i]) > 80 {
				vals[i] = vals[i][:80] + "..."
			}
		}
		fmt.Println(strings.Join(vals, "\t"))
	}
}