// Purpose: Provide shared input-validation helpers and structured errors.
// Exports: ValidationError and suggestion helpers.
// Role: Supports the forward CLI parsers plus internal validation utilities.
// Invariants: Validation errors remain precise; field suggestions remain stable.
// Notes: The old JSON-on-stdin mutation parser was removed during the 1.0 CLI cutover.
package ergo

import (
	"fmt"
	"strings"
)

// ValidationError describes invalid JSON input before rendering it as text.
type ValidationError struct {
	Error   string            `json:"error"`             // "validation_failed" or "parse_error"
	Message string            `json:"message"`           // human-readable summary
	Missing []string          `json:"missing,omitempty"` // list of missing required fields
	Invalid map[string]string `json:"invalid,omitempty"` // field -> reason map
}

func (e *ValidationError) GoError() error {
	// Build a detailed error message that includes missing/invalid field info
	parts := []string{e.Message}

	if len(e.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing required: %s", strings.Join(e.Missing, ", ")))
	}

	for field, reason := range e.Invalid {
		parts = append(parts, fmt.Sprintf("%s: %s", field, reason))
	}

	return fmt.Errorf("%s", strings.Join(parts, "; "))
}

func extractUnknownField(err error) (string, bool) {
	msg := err.Error()
	const prefix = "json: unknown field "
	if !strings.HasPrefix(msg, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(msg, prefix)
	if len(rest) < 2 || rest[0] != '"' {
		return "", false
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end == -1 {
		return "", false
	}
	return rest[:end], true
}

func suggestFieldNameFrom(unknown string, candidates []string) (string, bool) {
	unknown = strings.ToLower(unknown)
	best := ""
	bestDist := 99
	secondBest := 99
	for _, cand := range candidates {
		dist := levenshteinDistance(unknown, cand)
		if dist < bestDist {
			secondBest = bestDist
			bestDist = dist
			best = cand
			continue
		}
		if dist < secondBest {
			secondBest = dist
		}
	}
	if best == "" {
		return "", false
	}
	if bestDist <= 2 && (secondBest-bestDist >= 2 || secondBest > 3) {
		return best, true
	}
	// Levenshtein ambiguity guard rejected the match; try adjacent-swap
	// detection as a tiebreaker (e.g. "aftre" -> "after").
	return suggestByAdjacentSwap(unknown, candidates)
}

// suggestByAdjacentSwap returns a candidate if `unknown` is exactly one
// adjacent-character transposition away from a single candidate.
func suggestByAdjacentSwap(unknown string, candidates []string) (string, bool) {
	match := ""
	for _, cand := range candidates {
		if !isAdjacentSwap(unknown, cand) {
			continue
		}
		if match != "" {
			return "", false // ambiguous
		}
		match = cand
	}
	if match == "" {
		return "", false
	}
	return match, true
}

func isAdjacentSwap(a, b string) bool {
	if len(a) != len(b) || len(a) < 2 || a == b {
		return false
	}
	first := -1
	second := -1
	for i := 0; i < len(a); i++ {
		if a[i] == b[i] {
			continue
		}
		if first == -1 {
			first = i
			continue
		}
		if second == -1 {
			second = i
			continue
		}
		return false
	}
	if first == -1 || second == -1 || second != first+1 {
		return false
	}
	return a[first] == b[second] && a[second] == b[first]
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = minInt(del, minInt(ins, sub))
		}
		prev = cur
	}
	return prev[len(b)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
