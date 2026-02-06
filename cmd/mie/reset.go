//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
)

// runReset deletes all local memory data for the current MIE instance.
func runReset(args []string, configPath string, globals GlobalFlags) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	confirm := fs.Bool("yes", false, "Confirm the reset (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie reset [options]

Description:
  WARNING: This is a destructive operation that deletes all memory data.

  Removes the MIE database file. This deletes all stored facts, decisions,
  entities, events, topics, and relationships.

  Configuration (.mie/config.yaml) is NOT deleted.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  mie reset --yes       Delete all memory data

Notes:
  After resetting, the database will be recreated automatically when
  the MCP server starts again.

`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if !*confirm {
		fmt.Fprintf(os.Stderr, "Error: the --yes flag is required to confirm this destructive operation\n")
		fmt.Fprintf(os.Stderr, "Run 'mie reset --yes' to confirm\n")
		os.Exit(1)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		// If no config, try default data directory
		cfg = DefaultConfig()
	}

	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitDatabase)
	}

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if !globals.Quiet {
			fmt.Fprintf(os.Stderr, "No data found at %s\n", dataDir)
		}
		return
	}

	if !globals.Quiet {
		fmt.Printf("Deleting memory data at %s...\n", dataDir)
	}

	if err := os.RemoveAll(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot delete data directory: %v\n", err)
		os.Exit(ExitDatabase)
	}

	if !globals.Quiet {
		fmt.Println("Reset complete. All memory data has been deleted.")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  mie --mcp    Start MCP server (database will be recreated)")
	}
}
