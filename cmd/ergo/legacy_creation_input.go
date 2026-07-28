// Purpose: Recognize removed creation JSON only to produce migration guidance.
// Role: CLI compatibility boundary for the pre-Ergo-4 creation interface.
package main

import (
	"encoding/json"
	"sort"
	"strings"
)

var legacyCreationFields = map[string]struct{}{
	"title": {}, "epic": {}, "state": {}, "claim": {}, "result": {},
}

func legacyCreationKeys(argument string) []string {
	trimmed := strings.TrimSpace(argument)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &object); err != nil || object == nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		if _, reserved := legacyCreationFields[key]; reserved {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
