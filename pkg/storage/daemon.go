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
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// Daemon serves CozoDB over a Unix domain socket, allowing multiple
// MCP processes to share a single database instance.
type Daemon struct {
	backend    *EmbeddedBackend
	socketPath string
	listener   net.Listener
	wg         sync.WaitGroup
	connMu     sync.Mutex
	conns      map[net.Conn]struct{}
}

// NewDaemon creates a new daemon that serves the given backend on a Unix socket.
func NewDaemon(backend *EmbeddedBackend, socketPath string) *Daemon {
	return &Daemon{
		backend:    backend,
		socketPath: socketPath,
	}
}

// Serve starts accepting connections. Blocks until ctx is cancelled.
// Cleans up the socket file on exit. On shutdown, closes all active
// client connections so handlers unblock promptly.
func (d *Daemon) Serve(ctx context.Context) error {
	// Remove stale socket file
	if err := os.Remove(d.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", d.socketPath, err)
	}

	// Restrict socket to owner-only to prevent local privilege escalation.
	if err := os.Chmod(d.socketPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	d.listener = ln
	d.conns = make(map[net.Conn]struct{})

	// Clean up socket file on exit
	defer func() {
		ln.Close()
		os.Remove(d.socketPath)
	}()

	// Close listener and all active connections when context is cancelled.
	go func() {
		<-ctx.Done()
		ln.Close()
		d.connMu.Lock()
		for conn := range d.conns {
			conn.Close()
		}
		d.connMu.Unlock()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				done := make(chan struct{})
				go func() { d.wg.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					log.Printf("[DAEMON] shutdown timeout, forcing exit")
				}
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}

		d.connMu.Lock()
		d.conns[conn] = struct{}{}
		d.connMu.Unlock()

		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleConn(ctx, conn)
			d.connMu.Lock()
			delete(d.conns, conn)
			d.connMu.Unlock()
		}()
	}
}

// handleConn reads requests from a client connection and writes responses.
// The context is used to propagate cancellation to backend operations.
func (d *Daemon) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req DaemonRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := DaemonResponse{OK: false, Error: fmt.Sprintf("invalid request: %v", err)}
			d.writeResponse(conn, resp)
			continue
		}

		resp := d.dispatch(ctx, req)
		d.writeResponse(conn, resp)

		if req.Method == MethodClose {
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[DAEMON] scanner error: %v", err)
	}
}

// dispatch handles a single request and returns a response.
// Uses the daemon's context so backend operations are cancelled on shutdown.
func (d *Daemon) dispatch(ctx context.Context, req DaemonRequest) DaemonResponse {
	switch req.Method {
	case MethodPing:
		return DaemonResponse{OK: true, ID: req.ID}

	case MethodQuery:
		result, err := d.backend.Query(ctx, req.Datalog)
		if err != nil {
			return DaemonResponse{OK: false, ID: req.ID, Error: err.Error()}
		}
		return DaemonResponse{
			OK:      true,
			ID:      req.ID,
			Headers: result.Headers,
			Rows:    result.Rows,
		}

	case MethodExecute:
		err := d.backend.Execute(ctx, req.Datalog)
		if err != nil {
			return DaemonResponse{OK: false, ID: req.ID, Error: err.Error()}
		}
		return DaemonResponse{OK: true, ID: req.ID}

	case MethodGetMeta:
		val, err := d.backend.GetMeta(req.Key)
		if err != nil {
			return DaemonResponse{OK: false, ID: req.ID, Error: err.Error()}
		}
		return DaemonResponse{OK: true, ID: req.ID, Value: val}

	case MethodSetMeta:
		err := d.backend.SetMeta(req.Key, req.Value)
		if err != nil {
			return DaemonResponse{OK: false, ID: req.ID, Error: err.Error()}
		}
		return DaemonResponse{OK: true, ID: req.ID}

	case MethodEnsureSchema:
		err := d.backend.EnsureSchema()
		if err != nil {
			return DaemonResponse{OK: false, ID: req.ID, Error: err.Error()}
		}
		return DaemonResponse{OK: true, ID: req.ID}

	case MethodCreateHNSWIndex:
		err := d.backend.CreateHNSWIndex(req.Dims)
		if err != nil {
			return DaemonResponse{OK: false, ID: req.ID, Error: err.Error()}
		}
		return DaemonResponse{OK: true, ID: req.ID}

	case MethodClose:
		return DaemonResponse{OK: true, ID: req.ID}

	default:
		return DaemonResponse{OK: false, ID: req.ID, Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

// writeResponse marshals and writes a response to the connection.
func (d *Daemon) writeResponse(conn net.Conn, resp DaemonResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[DAEMON] marshal response error: %v", err)
		return
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		log.Printf("[DAEMON] write response error: %v", err)
	}
}
