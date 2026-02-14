// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// shortSockPath returns a short Unix socket path under /tmp to stay within
// macOS's 104-char sun_path limit. The long paths from t.TempDir() can
// exceed this limit for tests with long names.
func shortSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mie-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

// TestSocketBackendImplementsMetaBackend verifies SocketBackend satisfies MetaBackend.
func TestSocketBackendImplementsMetaBackend(t *testing.T) {
	var _ MetaBackend = (*SocketBackend)(nil)
}

// waitForSocket polls until the Unix socket is connectable.
func waitForSocket(t *testing.T, path string) {
	t.Helper()
	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", path)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %s never appeared", path)
}

// startTestDaemon creates an in-memory embedded backend, starts a daemon,
// and returns the backend and a cancel function.
func startTestDaemon(t *testing.T, sockPath string) (*EmbeddedBackend, context.CancelFunc) {
	t.Helper()

	embedded, err := NewEmbeddedBackend(EmbeddedConfig{
		DataDir: t.TempDir(),
		Engine:  "mem",
	})
	if err != nil {
		t.Fatalf("create embedded: %v", err)
	}
	if err := embedded.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	d := NewDaemon(embedded, sockPath)
	ctx, cancel := context.WithCancel(context.Background())

	serveDone := make(chan struct{})
	go func() {
		_ = d.Serve(ctx)
		close(serveDone)
	}()
	waitForSocket(t, sockPath)

	t.Cleanup(func() {
		cancel()
		<-serveDone // Wait for Serve to finish before closing backend
		_ = embedded.Close()
	})

	return embedded, cancel
}

// TestSocketBackendRoundTrip verifies Query and Execute work end-to-end.
func TestSocketBackendRoundTrip(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	// Test Query
	result, err := sb.Query(context.Background(), "?[x] := x = 42")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0] != float64(42) {
		t.Errorf("expected [[42]], got %v", result.Rows)
	}

	// Test Execute + Query round-trip
	err = sb.Execute(context.Background(), `:create test_tbl { id: String => val: String }`)
	if err != nil {
		t.Fatalf("execute create: %v", err)
	}

	err = sb.Execute(context.Background(), `?[id, val] <- [["k1", "v1"]] :put test_tbl { id, val }`)
	if err != nil {
		t.Fatalf("execute put: %v", err)
	}

	result, err = sb.Query(context.Background(), `?[id, val] := *test_tbl { id, val }`)
	if err != nil {
		t.Fatalf("query test_tbl: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != "k1" || result.Rows[0][1] != "v1" {
		t.Errorf("expected [k1, v1], got %v", result.Rows[0])
	}
}

// TestSocketBackendMetaOps tests GetMeta and SetMeta through the daemon.
func TestSocketBackendMetaOps(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	// SetMeta + GetMeta
	if err := sb.SetMeta("test_key", "test_value"); err != nil {
		t.Fatalf("set_meta: %v", err)
	}

	val, err := sb.GetMeta("test_key")
	if err != nil {
		t.Fatalf("get_meta: %v", err)
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %q", val)
	}

	// Non-existent key returns empty
	val, err = sb.GetMeta("nonexistent")
	if err != nil {
		t.Fatalf("get_meta nonexistent: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty, got %q", val)
	}
}

// TestSocketBackendConcurrentClients verifies multiple clients can
// read and write concurrently through the daemon.
func TestSocketBackendConcurrentClients(t *testing.T) {
	sockPath := shortSockPath(t)
	embedded, _ := startTestDaemon(t, sockPath)

	// Create a test table
	_, err := embedded.DB().Run(`:create conc_test { id: String => val: String }`, nil)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	const numClients = 5
	const opsPerClient = 10
	var wg sync.WaitGroup

	for c := 0; c < numClients; c++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			sb, err := NewSocketBackend(sockPath)
			if err != nil {
				t.Errorf("client %d connect: %v", clientID, err)
				return
			}
			defer sb.Close()

			for i := 0; i < opsPerClient; i++ {
				id := fmt.Sprintf("c%d-i%d", clientID, i)
				err := sb.Execute(context.Background(),
					fmt.Sprintf(`?[id, val] <- [["%s", "v"]] :put conc_test { id, val }`, id))
				if err != nil {
					t.Errorf("client %d write %d: %v", clientID, i, err)
					return
				}
			}

			result, err := sb.Query(context.Background(), `?[count(id)] := *conc_test { id }`)
			if err != nil {
				t.Errorf("client %d read: %v", clientID, err)
				return
			}
			if len(result.Rows) == 0 {
				t.Errorf("client %d: expected rows", clientID)
			}
		}(c)
	}

	wg.Wait()

	// Verify total count
	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("final connect: %v", err)
	}
	defer sb.Close()

	result, err := sb.Query(context.Background(), `?[count(id)] := *conc_test { id }`)
	if err != nil {
		t.Fatalf("final count: %v", err)
	}
	count := result.Rows[0][0].(float64)
	expected := float64(numClients * opsPerClient)
	if count != expected {
		t.Errorf("expected %v rows, got %v", expected, count)
	}
}

// TestDaemonTwoClientsShareData verifies one client's writes are visible to another.
func TestDaemonTwoClientsShareData(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	// Client 1: create table and write
	sb1, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("client 1 connect: %v", err)
	}
	defer sb1.Close()

	err = sb1.Execute(context.Background(), `:create share_test { id: String => val: String }`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	err = sb1.Execute(context.Background(), `?[id, val] <- [["key1", "from-client1"]] :put share_test { id, val }`)
	if err != nil {
		t.Fatalf("client 1 put: %v", err)
	}

	// Client 2: read what client 1 wrote
	sb2, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("client 2 connect: %v", err)
	}
	defer sb2.Close()

	result, err := sb2.Query(context.Background(), `?[id, val] := *share_test { id, val }`)
	if err != nil {
		t.Fatalf("client 2 query: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][1] != "from-client1" {
		t.Errorf("expected 'from-client1', got %v", result.Rows[0][1])
	}
}

// TestDaemonErrorHandling verifies the daemon returns errors for bad queries.
func TestDaemonErrorHandling(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	// Bad query should return error, not crash
	_, err = sb.Query(context.Background(), `this is not valid datalog`)
	if err == nil {
		t.Error("expected error for invalid query")
	}

	// Should still work after error
	result, err := sb.Query(context.Background(), "?[x] := x = 1")
	if err != nil {
		t.Fatalf("query after error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(result.Rows))
	}
}

// --- C1: Close during active request must not deadlock ---

// TestSocketBackendCloseUnblocksActiveRequest verifies that calling Close()
// while send() is blocked on ReadBytes does not deadlock.
func TestSocketBackendCloseUnblocksActiveRequest(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Start a query in background
	errCh := make(chan error, 1)
	go func() {
		_, err := sb.Query(context.Background(), "?[x] := x = 1")
		errCh <- err
	}()

	// Give query time to start
	time.Sleep(50 * time.Millisecond)

	// Close should not deadlock — must complete within 1 second
	done := make(chan struct{})
	go func() {
		sb.Close()
		close(done)
	}()

	select {
	case <-done:
		// OK: Close completed
	case <-time.After(2 * time.Second):
		t.Fatal("Close() deadlocked")
	}
}

// TestSocketBackendDoubleClose verifies Close() is idempotent.
func TestSocketBackendDoubleClose(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	if err := sb.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	// Second close should not panic or error
	if err := sb.Close(); err != nil {
		t.Fatalf("second close should be no-op, got: %v", err)
	}
}

// --- C4: Ping and stale socket detection ---

// TestSocketBackendPing verifies Ping returns nil when daemon is alive.
func TestSocketBackendPing(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	if err := sb.Ping(); err != nil {
		t.Fatalf("ping should succeed: %v", err)
	}
}

// TestSocketBackendPingAfterClose verifies Ping fails on closed backend.
func TestSocketBackendPingAfterClose(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	sb.Close()

	if err := sb.Ping(); err == nil {
		t.Fatal("ping should fail on closed backend")
	}
}

// TestDaemonStaleSocketCleanup verifies the daemon removes a stale socket file on start.
func TestDaemonStaleSocketCleanup(t *testing.T) {
	sockPath := shortSockPath(t)

	// Create a stale socket file
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("create stale socket: %v", err)
	}
	ln.Close()
	// Socket file still exists but nobody is listening

	// Start daemon — should clean up stale socket and start successfully
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect after stale cleanup: %v", err)
	}
	defer sb.Close()

	if err := sb.Ping(); err != nil {
		t.Fatalf("ping after stale cleanup: %v", err)
	}
}

// --- I1-I3: Daemon shutdown, scanner error, context ---

// TestDaemonGracefulShutdownClosesConnections verifies that canceling the
// daemon context closes active client connections.
func TestDaemonGracefulShutdownClosesConnections(t *testing.T) {
	sockPath := shortSockPath(t)
	_, cancel := startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	// Verify connected
	if err := sb.Ping(); err != nil {
		t.Fatalf("initial ping: %v", err)
	}

	// Shut down daemon
	cancel()

	// Give daemon time to close connections
	time.Sleep(200 * time.Millisecond)

	// Client should get an error on next request
	_, err = sb.Query(context.Background(), "?[x] := x = 1")
	if err == nil {
		t.Error("expected error after daemon shutdown")
	}
}

// --- I6: Clean disconnect with MethodClose ---

// TestSocketBackendClosesendsMethodClose verifies Close sends MethodClose
// before disconnecting.
func TestSocketBackendCloseSendsMethodClose(t *testing.T) {
	sockPath := shortSockPath(t)

	// Create a custom listener to inspect what the client sends
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	receivedClose := make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			lines := strings.Split(strings.TrimSpace(string(buf[:n])), "\n")
			for _, line := range lines {
				var req DaemonRequest
				if json.Unmarshal([]byte(line), &req) == nil && req.Method == MethodClose {
					// Send OK response so client doesn't hang
					resp := DaemonResponse{OK: true, ID: req.ID}
					data, _ := json.Marshal(resp)
					fmt.Fprintf(conn, "%s\n", data)
					receivedClose <- true
					return
				}
				// Send OK for any other request
				resp := DaemonResponse{OK: true, ID: "0"}
				data, _ := json.Marshal(resp)
				fmt.Fprintf(conn, "%s\n", data)
			}
		}
	}()

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	sb.Close()

	select {
	case <-receivedClose:
		// OK: MethodClose was sent
	case <-time.After(2 * time.Second):
		t.Error("Close() did not send MethodClose to daemon")
	}
}

// --- Coverage gaps: socket permissions, schema via socket, request ID validation ---

// TestDaemonSocketPermissions verifies the socket file has 0600 permissions.
func TestDaemonSocketPermissions(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("socket permissions: got %04o, want 0600", perm)
	}
}

// TestSocketBackendEnsureSchemaAndHNSW verifies schema and HNSW methods work via socket.
func TestSocketBackendEnsureSchemaAndHNSW(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	// EnsureSchema should succeed (already done by daemon, but idempotent)
	if err := sb.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema via socket: %v", err)
	}

	// CreateHNSWIndex should succeed
	if err := sb.CreateHNSWIndex(384); err != nil {
		t.Fatalf("create HNSW index via socket: %v", err)
	}
}

// TestSocketBackendDBReturnsNil verifies DB() returns nil for socket backend.
func TestSocketBackendDBReturnsNil(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sb.Close()

	if sb.DB() != nil {
		t.Error("DB() should return nil for SocketBackend")
	}
}

// TestSocketBackendSendAfterClose verifies operations fail after close.
func TestSocketBackendSendAfterClose(t *testing.T) {
	sockPath := shortSockPath(t)
	startTestDaemon(t, sockPath)

	sb, err := NewSocketBackend(sockPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	sb.Close()

	_, err = sb.Query(context.Background(), "?[x] := x = 1")
	if err == nil {
		t.Error("expected error after close")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' in error, got: %v", err)
	}
}
