// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"testing"
)

func TestGenerateID(t *testing.T) {
	id1 := GenerateID("test", "a", "b")
	id2 := GenerateID("test", "a", "b")
	id3 := GenerateID("test", "a", "c")

	if id1 != id2 {
		t.Errorf("deterministic IDs should match: %s != %s", id1, id2)
	}
	if id1 == id3 {
		t.Error("different inputs should produce different IDs")
	}
	if len(id1) < 5 {
		t.Errorf("ID too short: %s", id1)
	}
}

func TestFactID(t *testing.T) {
	id := FactID("I live in Buenos Aires", "personal")
	if id == "" {
		t.Error("FactID returned empty string")
	}
	if id[:5] != "fact:" {
		t.Errorf("FactID should start with 'fact:': %s", id)
	}

	// Deterministic
	id2 := FactID("I live in Buenos Aires", "personal")
	if id != id2 {
		t.Error("FactID should be deterministic")
	}
}

func TestDecisionID(t *testing.T) {
	id := DecisionID("Use Go for backend", "CGO CozoDB bindings")
	if id == "" || id[:4] != "dec:" {
		t.Errorf("DecisionID bad format: %s", id)
	}
}

func TestEntityID(t *testing.T) {
	id := EntityID("Kraklabs", "company")
	if id == "" || id[:4] != "ent:" {
		t.Errorf("EntityID bad format: %s", id)
	}

	// Case-insensitive
	id2 := EntityID("KRAKLABS", "company")
	if id != id2 {
		t.Error("EntityID should be case-insensitive on name")
	}
}

func TestEventID(t *testing.T) {
	id := EventID("Launch v1.0", "2026-02-05")
	if id == "" || id[:4] != "evt:" {
		t.Errorf("EventID bad format: %s", id)
	}
}

func TestTopicID(t *testing.T) {
	id := TopicID("architecture")
	if id == "" || id[:4] != "top:" {
		t.Errorf("TopicID bad format: %s", id)
	}

	id2 := TopicID("Architecture")
	if id != id2 {
		t.Error("TopicID should be case-insensitive")
	}
}
