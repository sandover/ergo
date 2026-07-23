// Purpose: Parse stdin and recognize creation syntax reserved for migration errors.
// Exports: none; creation commands use these package-local helpers.
// Role: Keeps positional creation simple while recognizing reserved JSON forms.
// Invariants: stdin is read only when piped; unrelated JSON remains a valid title.
// Notes: Storage JSON is unrelated to this command-input boundary.
package ergo

import (
	"encoding/json"
	"io"
	"os"
	"strings"
)

var legacyCreationFields = map[string]struct{}{
	"title": {}, "epic": {}, "state": {}, "claim": {}, "result": {},
}

const (
	NewTaskUsage = `usage: ergo new task "<title>" [--epic <id>]; optional piped stdin becomes the body`
	NewEpicUsage = `usage: ergo new epic "<title>" --file <path>; optional piped stdin becomes the epic body`
)

func readOptionalBodyFromStdin() (string, bool, error) {
	if !stdinIsPiped() {
		return "", false, nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// LegacyCreationKeys returns reserved creation keys found in a JSON object.
func LegacyCreationKeys(arg string) []string {
	trimmed := strings.TrimSpace(arg)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &object); err != nil || object == nil {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		if _, ok := legacyCreationFields[key]; ok {
			keys = append(keys, key)
		}
	}
	return sortedUniqueStrings(keys)
}
