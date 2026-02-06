// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestUpdate_Invalidate(t *testing.T) {
	called := false
	mock := &MockQuerier{
		InvalidateFactFunc: func(ctx context.Context, oldFactID, newFactID, reason string) error {
			called = true
			if oldFactID != "fact:abc123" {
				t.Errorf("Expected oldFactID=fact:abc123, got %s", oldFactID)
			}
			if reason != "User moved" {
				t.Errorf("Expected reason='User moved', got %s", reason)
			}
			return nil
		},
	}

	result, err := Update(context.Background(), mock, map[string]any{
		"node_id": "fact:abc123",
		"action":  "invalidate",
		"reason":  "User moved",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Update() returned error: %s", result.Text)
	}
	if !called {
		t.Error("InvalidateFact should have been called")
	}
	if !strings.Contains(result.Text, "Invalidated") {
		t.Error("Update() should confirm invalidation")
	}
}

func TestUpdate_InvalidateWithReplacement(t *testing.T) {
	mock := &MockQuerier{}
	result, err := Update(context.Background(), mock, map[string]any{
		"node_id":        "fact:abc123",
		"action":         "invalidate",
		"reason":         "Updated info",
		"replacement_id": "fact:new456",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Update() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "fact:new456") {
		t.Error("Update() should mention replacement ID")
	}
}

func TestUpdate_InvalidateNonFact(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id": "ent:abc123",
		"action":  "invalidate",
		"reason":  "test",
	})
	if !result.IsError {
		t.Error("Update() should reject invalidation of non-fact nodes")
	}
}

func TestUpdate_InvalidateMissingReason(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id": "fact:abc123",
		"action":  "invalidate",
	})
	if !result.IsError {
		t.Error("Update() should require reason for invalidation")
	}
}

func TestUpdate_UpdateDescription(t *testing.T) {
	called := false
	mock := &MockQuerier{
		UpdateDescriptionFunc: func(ctx context.Context, nodeID, newDescription string) error {
			called = true
			if nodeID != "ent:abc123" {
				t.Errorf("Expected nodeID=ent:abc123, got %s", nodeID)
			}
			return nil
		},
	}

	result, err := Update(context.Background(), mock, map[string]any{
		"node_id":   "ent:abc123",
		"action":    "update_description",
		"new_value": "Updated description here",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Update() returned error: %s", result.Text)
	}
	if !called {
		t.Error("UpdateDescription should have been called")
	}
}

func TestUpdate_UpdateDescriptionMissingValue(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id": "ent:abc123",
		"action":  "update_description",
	})
	if !result.IsError {
		t.Error("Update() should require new_value for update_description")
	}
}

func TestUpdate_UpdateStatus(t *testing.T) {
	called := false
	mock := &MockQuerier{
		UpdateStatusFunc: func(ctx context.Context, nodeID, newStatus string) error {
			called = true
			if newStatus != "superseded" {
				t.Errorf("Expected status=superseded, got %s", newStatus)
			}
			return nil
		},
	}

	result, err := Update(context.Background(), mock, map[string]any{
		"node_id":   "dec:abc123",
		"action":    "update_status",
		"new_value": "superseded",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Update() returned error: %s", result.Text)
	}
	if !called {
		t.Error("UpdateStatus should have been called")
	}
}

func TestUpdate_UpdateStatusNonDecision(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id":   "fact:abc123",
		"action":    "update_status",
		"new_value": "active",
	})
	if !result.IsError {
		t.Error("Update() should reject status update on non-decision nodes")
	}
}

func TestUpdate_UpdateStatusInvalidValue(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id":   "dec:abc123",
		"action":    "update_status",
		"new_value": "invalid_status",
	})
	if !result.IsError {
		t.Error("Update() should reject invalid status values")
	}
}

func TestUpdate_MissingNodeID(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"action": "invalidate",
	})
	if !result.IsError {
		t.Error("Update() should require node_id")
	}
}

func TestUpdate_MissingAction(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id": "fact:abc",
	})
	if !result.IsError {
		t.Error("Update() should require action")
	}
}

func TestUpdate_InvalidAction(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id": "fact:abc",
		"action":  "delete",
	})
	if !result.IsError {
		t.Error("Update() should reject invalid actions")
	}
}

func TestUpdate_InvalidateError(t *testing.T) {
	mock := &MockQuerier{
		InvalidateFactFunc: func(ctx context.Context, oldFactID, newFactID, reason string) error {
			return fmt.Errorf("db error")
		},
	}
	result, _ := Update(context.Background(), mock, map[string]any{
		"node_id": "fact:abc",
		"action":  "invalidate",
		"reason":  "test",
	})
	if !result.IsError {
		t.Error("Update() should return error when invalidation fails")
	}
}