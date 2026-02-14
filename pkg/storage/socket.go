// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	cozo "github.com/kraklabs/mie/pkg/cozodb"
)

// SocketBackend implements MetaBackend by forwarding requests to a MIE daemon
// over a Unix domain socket. This allows multiple MCP processes to share a
// single CozoDB instance.
type SocketBackend struct {
	socketPath string
	conn       net.Conn
	reader     *bufio.Reader
	mu         sync.Mutex
	reqID      atomic.Int64
	closed     bool
}

// NewSocketBackend connects to the MIE daemon at the given socket path.
func NewSocketBackend(socketPath string) (*SocketBackend, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %s: %w", socketPath, err)
	}

	return &SocketBackend{
		socketPath: socketPath,
		conn:       conn,
		reader:     bufio.NewReader(conn),
	}, nil
}

// Query executes a read-only Datalog query via the daemon.
func (s *SocketBackend) Query(ctx context.Context, datalog string) (*QueryResult, error) {
	resp, err := s.send(DaemonRequest{
		Method:   MethodQuery,
		Datalog:  datalog,
		ReadOnly: true,
	})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("query failed: %s", resp.Error)
	}
	return &QueryResult{
		Headers: resp.Headers,
		Rows:    resp.Rows,
	}, nil
}

// Execute runs a Datalog mutation via the daemon.
func (s *SocketBackend) Execute(ctx context.Context, datalog string) error {
	resp, err := s.send(DaemonRequest{
		Method:  MethodExecute,
		Datalog: datalog,
	})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("execute failed: %s", resp.Error)
	}
	return nil
}

// Close sends a MethodClose to the daemon for clean disconnect,
// then closes the underlying connection. Safe to call multiple times.
// Closes the connection first (without mutex) to unblock any pending
// ReadBytes call in send(), preventing deadlock.
func (s *SocketBackend) Close() error {
	// Fast check without lock — if already closed, nothing to do.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Best-effort: send MethodClose so daemon cleans up the connection.
	// Ignore errors — the connection may already be broken.
	_ = s.sendClose()

	// Close connection to unblock any goroutine blocked on ReadBytes.
	return s.conn.Close()
}

// sendClose sends a MethodClose request without going through the
// mutex-protected send() path, since Close() already set s.closed.
func (s *SocketBackend) sendClose() error {
	req := DaemonRequest{Method: MethodClose, ID: "close"}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	// Set a short write deadline so we don't block.
	_ = s.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = fmt.Fprintf(s.conn, "%s\n", data)
	return err
}

// Ping sends a ping request to the daemon and verifies it responds.
// Use this to check that the daemon is alive after connecting.
func (s *SocketBackend) Ping() error {
	resp, err := s.send(DaemonRequest{Method: MethodPing})
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("ping failed: %s", resp.Error)
	}
	return nil
}

// GetMeta retrieves a metadata value from the daemon.
func (s *SocketBackend) GetMeta(key string) (string, error) {
	resp, err := s.send(DaemonRequest{
		Method: MethodGetMeta,
		Key:    key,
	})
	if err != nil {
		return "", err
	}
	if !resp.OK {
		return "", fmt.Errorf("get_meta failed: %s", resp.Error)
	}
	return resp.Value, nil
}

// SetMeta sets a metadata value via the daemon.
func (s *SocketBackend) SetMeta(key, value string) error {
	resp, err := s.send(DaemonRequest{
		Method: MethodSetMeta,
		Key:    key,
		Value:  value,
	})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("set_meta failed: %s", resp.Error)
	}
	return nil
}

// EnsureSchema asks the daemon to ensure the base schema exists.
func (s *SocketBackend) EnsureSchema() error {
	resp, err := s.send(DaemonRequest{Method: MethodEnsureSchema})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("ensure_schema failed: %s", resp.Error)
	}
	return nil
}

// CreateHNSWIndex asks the daemon to create HNSW indexes.
func (s *SocketBackend) CreateHNSWIndex(dimensions int) error {
	resp, err := s.send(DaemonRequest{
		Method: MethodCreateHNSWIndex,
		Dims:   dimensions,
	})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("create_hnsw_index failed: %s", resp.Error)
	}
	return nil
}

// DB returns nil — SocketBackend does not have direct database access.
func (s *SocketBackend) DB() *cozo.CozoDB {
	return nil
}

// send serializes a request, sends it to the daemon, and reads the response.
// Thread-safe: uses a mutex to serialize access to the connection.
// On I/O errors, marks the backend as closed so subsequent calls fail fast.
func (s *SocketBackend) send(req DaemonRequest) (*DaemonResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("socket backend is closed")
	}

	req.ID = fmt.Sprintf("%d", s.reqID.Add(1))

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if _, err := fmt.Fprintf(s.conn, "%s\n", data); err != nil {
		s.closed = true
		return nil, fmt.Errorf("send request: %w", err)
	}

	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		s.closed = true
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp DaemonResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.ID != req.ID {
		return nil, fmt.Errorf("protocol error: expected response ID %s, got %s", req.ID, resp.ID)
	}

	return &resp, nil
}
