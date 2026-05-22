// Purpose: Define and parse plan payloads used for bulk task creation.
// Exports: PlanTaskInput, ParsePlanFile, and hasPlanCycle.
// Role: Shared parsing for markdown-plan files and title-based dependency validation.
// Invariants: Each plan chunk starts with `# Title`; duplicate titles are rejected.
// Notes: Markdown plan files intentionally do not infer dependencies from order.
package ergo

import (
	"fmt"
	"os"
	"strings"
)

// PlanTaskInput describes one child task in a markdown plan file.
type PlanTaskInput struct {
	Title string
	Body  string
	After []string
}

func ParsePlanFile(path string) ([]PlanTaskInput, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	chunks := splitPlanChunks(strings.ReplaceAll(string(content), "\r\n", "\n"))
	if len(chunks) == 0 {
		return nil, fmt.Errorf("%s: plan file contains no task chunks", path)
	}

	seenTitles := map[string]struct{}{}
	tasks := make([]PlanTaskInput, 0, len(chunks))
	for idx, chunk := range chunks {
		task, err := parsePlanChunk(chunk)
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

func splitPlanChunks(content string) []string {
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

func parsePlanChunk(chunk string) (PlanTaskInput, error) {
	lines := strings.Split(chunk, "\n")
	if len(lines) == 0 {
		return PlanTaskInput{}, fmt.Errorf("empty chunk")
	}
	if !strings.HasPrefix(lines[0], "# ") {
		return PlanTaskInput{}, fmt.Errorf("chunk must start with '# Title'")
	}
	title := strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
	if title == "" {
		return PlanTaskInput{}, fmt.Errorf("chunk title cannot be empty")
	}
	task := PlanTaskInput{Title: title}
	if len(lines) > 1 {
		body := strings.Join(lines[1:], "\n")
		task.Body = body
	}
	return task, nil
}


