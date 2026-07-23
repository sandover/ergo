// Purpose: Parse the Markdown child file used by bulk epic creation.
// Exports: EpicTaskInput and ParseEpicFile.
// Role: Turn ordered Markdown chunks into validated child-task inputs.
// Invariants: Each chunk starts with `# Title`; duplicate titles are rejected.
// Notes: File order intentionally does not infer dependencies.
package ergo

import (
	"fmt"
	"os"
	"strings"
)

// EpicTaskInput describes one child task in an epic file.
type EpicTaskInput struct {
	Title string
	Body  string
	After []string
}

func ParseEpicFile(path string) ([]EpicTaskInput, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	chunks := splitEpicChunks(strings.ReplaceAll(string(content), "\r\n", "\n"))
	if len(chunks) == 0 {
		return nil, fmt.Errorf("%s: epic file contains no task chunks", path)
	}

	seenTitles := map[string]struct{}{}
	tasks := make([]EpicTaskInput, 0, len(chunks))
	for idx, chunk := range chunks {
		task, err := parseEpicChunk(chunk)
		if err != nil {
			return nil, fmt.Errorf("%s: chunk %d: %w", path, idx+1, err)
		}
		title := strings.TrimSpace(task.Title)
		if _, exists := seenTitles[title]; exists {
			return nil, fmt.Errorf("%s: duplicate task title %q", path, title)
		}
		seenTitles[title] = struct{}{}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func splitEpicChunks(content string) []string {
	lines := strings.Split(content, "\n")
	chunks := make([]string, 0)
	current := make([]string, 0)
	flush := func() {
		chunk := strings.TrimSpace(strings.Join(current, "\n"))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		current = current[:0]
	}

	for _, line := range lines {
		if line == "---" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return chunks
}

func parseEpicChunk(chunk string) (EpicTaskInput, error) {
	lines := strings.Split(chunk, "\n")
	if len(lines) == 0 {
		return EpicTaskInput{}, fmt.Errorf("empty chunk")
	}
	if !strings.HasPrefix(lines[0], "# ") {
		return EpicTaskInput{}, fmt.Errorf("chunk must start with '# Title'")
	}
	title := strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
	if title == "" {
		return EpicTaskInput{}, fmt.Errorf("chunk title cannot be empty")
	}
	task := EpicTaskInput{Title: title}
	if len(lines) > 1 {
		body := strings.Join(lines[1:], "\n")
		task.Body = body
	}
	return task, nil
}
