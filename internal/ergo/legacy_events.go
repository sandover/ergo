// Purpose: Normalize explicitly supported released event-history forms.
// Role: Compatibility boundary between legacy wire meaning and the current graph.
package ergo

import "strings"

func applyLegacyTitleMigration(graph *Graph) {
	for _, task := range graph.Tasks {
		if strings.TrimSpace(task.Title) != "" {
			continue
		}
		title, body := deriveTitleAndBodyFromLegacy(task.Body)
		task.Title = title
		task.Body = body
	}
}

func deriveTitleAndBodyFromLegacy(body string) (string, string) {
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || isLegacyHeading(trimmed) {
			continue
		}
		title := trimmed
		if i+1 >= len(lines) {
			return title, ""
		}
		return title, strings.Join(lines[i+1:], "\n")
	}
	if strings.TrimSpace(body) == "" {
		return "(untitled)", ""
	}
	return "(untitled)", body
}

func isLegacyHeading(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "#") {
		return false
	}
	for len(line) > 0 && line[0] == '#' {
		line = strings.TrimPrefix(line, "#")
	}
	return strings.TrimSpace(line) != ""
}
