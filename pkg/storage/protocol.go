// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"os"
	"path/filepath"
)

// DaemonRequest is a request sent from a SocketBackend client to the daemon.
type DaemonRequest struct {
	Method   string `json:"method"`
	ID       string `json:"id"`
	Datalog  string `json:"datalog,omitempty"`
	ReadOnly bool   `json:"readonly,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	Dims     int    `json:"dimensions,omitempty"`
}

// DaemonResponse is a response sent from the daemon to a SocketBackend client.
type DaemonResponse struct {
	OK      bool     `json:"ok"`
	ID      string   `json:"id"`
	Headers []string `json:"headers,omitempty"`
	Rows    [][]any  `json:"rows,omitempty"`
	Value   string   `json:"value,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Daemon protocol method constants.
const (
	MethodQuery           = "query"
	MethodExecute         = "execute"
	MethodGetMeta         = "get_meta"
	MethodSetMeta         = "set_meta"
	MethodEnsureSchema    = "ensure_schema"
	MethodCreateHNSWIndex = "create_hnsw_index"
	MethodPing            = "ping"
	MethodClose           = "close"
)

// DefaultSocketPath returns the default Unix socket path for the MIE daemon.
func DefaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/mie.sock"
	}
	return filepath.Join(home, ".mie", "mie.sock")
}

// DefaultPIDPath returns the default PID file path for the MIE daemon.
func DefaultPIDPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/mie.pid"
	}
	return filepath.Join(home, ".mie", "mie.pid")
}