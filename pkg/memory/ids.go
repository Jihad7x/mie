// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// GenerateID creates a deterministic ID from input fields.
// Pattern: prefix + ":" + sha256(fields joined by "|")[:16]
// This matches CIE's ID generation pattern.
func GenerateID(prefix string, fields ...string) string {
	input := strings.Join(fields, "|")
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%s:%x", prefix, hash[:8]) // 16 hex chars
}

// FactID generates a deterministic ID for a fact.
func FactID(content, category string) string {
	return GenerateID("fact", content, category)
}

// DecisionID generates a deterministic ID for a decision.
func DecisionID(title, rationale string) string {
	return GenerateID("dec", title, rationale)
}

// EntityID generates a deterministic ID for an entity.
// Name is lowercased for case-insensitive deduplication.
func EntityID(name, kind string) string {
	return GenerateID("ent", strings.ToLower(name), kind)
}

// EventID generates a deterministic ID for an event.
func EventID(title, eventDate string) string {
	return GenerateID("evt", title, eventDate)
}

// TopicID generates a deterministic ID for a topic.
// Name is lowercased for case-insensitive deduplication.
func TopicID(name string) string {
	return GenerateID("top", strings.ToLower(name))
}
