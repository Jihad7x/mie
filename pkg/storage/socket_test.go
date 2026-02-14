// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package storage

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
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

	go func() { _ = d.Serve(ctx) }()
	waitForSocket(t, sockPath)

	t.Cleanup(func() {
		cancel()
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