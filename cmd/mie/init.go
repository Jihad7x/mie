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

// runInit creates a new .mie/config.yaml configuration file.
func runInit(args []string, globals GlobalFlags) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing configuration")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie init [options]

Description:
  Create a new .mie/config.yaml configuration file in the current directory
  with sensible defaults.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  mie init              Create configuration with defaults
  mie init --force      Overwrite existing configuration

`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}

	configPath := ConfigPath(cwd)

	if _, err := os.Stat(configPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "Error: %s already exists\n", configPath)
		fmt.Fprintf(os.Stderr, "Use --force to overwrite\n")
		os.Exit(1)
	}

	cfg := DefaultConfig()
	if err := SaveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	if !globals.Quiet {
		fmt.Printf("Created %s\n", configPath)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Edit .mie/config.yaml to customize settings")
		fmt.Println("  2. Run 'mie --mcp' to start the MCP server")
	}
}
