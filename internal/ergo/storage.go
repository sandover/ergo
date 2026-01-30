// Data directory discovery and event storage helpers.
//
// Key responsibilities:
// - `.ergo/` discovery (`resolveErgoDir`, `ergoDir`)
// - Append-only JSONL event log I/O (`readEvents`, `appendEvents`, `writeEventsFile`)
//
// Resilience:
// - `readEvents` tolerates a truncated final line (when the file doesn't end in '\n').
// - Parse errors include file+line and hints for common corruption (e.g. git conflicts).
package ergo

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	dataDirName = ".ergo"
)

func resolveErgoDir(start string) (string, error) {
	current := start
	for {
		candidate := filepath.Join(current, dataDirName)
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate, nil
			}
			return "", fmt.Errorf("%s exists but is not a directory", candidate)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if current == filepath.Dir(current) {
			break
		}
		current = filepath.Dir(current)
	}

	if filepath.Base(start) == dataDirName {
		info, err := os.Stat(start)
		if err == nil {
			if info.IsDir() {
				return start, nil
			}
			return "", fmt.Errorf("%s exists but is not a directory", start)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}

	return "", fmt.Errorf("%w (run ergo init)", ErrNoErgoDir)
}

func ergoDir(opts GlobalOptions) (string, error) {
	start := opts.StartDir
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	debugf(opts, "discover start=%s", start)
	return resolveErgoDir(start)
}

func loadGraph(dir string) (*Graph, error) {
	eventsPath := filepath.Join(dir, "events.jsonl")
	events, err := readEvents(eventsPath)
	if err != nil {
		return nil, err
	}
	return replayEvents(events)
}

func readEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	const maxEventLineBytes = 10 * 1024 * 1024

	endsWithNewline := false
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, info.Size()-1); err == nil {
			endsWithNewline = last[0] == '\n'
		}
	}

	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventLineBytes)
	var pending []byte
	pendingNo := 0
	currentNo := 0

	processLine := func(lineNo int, line []byte) error {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		var event Event
		if err := json.Unmarshal(trimmed, &event); err != nil {
			return formatEventsParseError(path, lineNo, trimmed, err)
		}
		events = append(events, event)
		return nil
	}

	for scanner.Scan() {
		currentNo++
		line := append([]byte(nil), scanner.Bytes()...) // copy (scanner buffer is reused)
		if pending != nil {
			if err := processLine(pendingNo, pending); err != nil {
				return nil, err
			}
		}
		pending = line
		pendingNo = currentNo
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("%s: event line too long (> %d bytes); events.jsonl may be corrupted (e.g. missing newlines)", path, maxEventLineBytes)
		}
		return nil, err
	}

	if pending != nil {
		// Tolerate a truncated final line (common after crashes or partial writes).
		// Only ignore when the file does not end in '\n'.
		if err := processLine(pendingNo, pending); err != nil {
			if !endsWithNewline {
				return events, nil
			}
			return nil, err
		}
	}
	return events, nil
}

func formatEventsParseError(path string, lineNo int, line []byte, cause error) error {
	snippet := string(line)
	if len(snippet) > 160 {
		snippet = snippet[:160] + "…"
	}
	trimmed := bytes.TrimSpace(line)
	if bytes.HasPrefix(trimmed, []byte("<<<<<<<")) || bytes.HasPrefix(trimmed, []byte("=======")) || bytes.HasPrefix(trimmed, []byte(">>>>>>>")) {
		return fmt.Errorf("%s:%d: git conflict markers in events log (resolve then run `ergo compact`): %s", path, lineNo, snippet)
	}
	return fmt.Errorf("%s:%d: invalid JSON in events log (run `ergo compact` after fixing): %s (%v)", path, lineNo, snippet, cause)
}

func appendEvents(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		line := append(data, '\n')
		if err := writeAll(file, line); err != nil {
			return err
		}
	}
	return nil
}

func writeEventsFile(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeAll(w *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func writeLinkEvent(dir string, opts GlobalOptions, eventType, from, to string) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		fromItem, ok := graph.Tasks[from]
		if !ok {
			return fmt.Errorf("unknown id %s", from)
		}
		toItem, ok := graph.Tasks[to]
		if !ok {
			return fmt.Errorf("unknown id %s", to)
		}
		// Validate dependency rules
		if err := validateDepSelf(from, to); err != nil {
			return err
		}
		if err := validateDepKinds(isEpic(fromItem), isEpic(toItem)); err != nil {
			return err
		}
		// Cycle detection for new links
		if eventType == "link" {
			if hasCycle(graph, from, to) {
				return errors.New("dependency would create a cycle")
			}
		}
		now := time.Now().UTC()
		event, err := newEvent(eventType, now, LinkEvent{
			FromID: from,
			ToID:   to,
			Type:   dependsLinkType,
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func createTask(dir string, opts GlobalOptions, epicID string, isEpic bool, title, body string) (createOutput, error) {
	eventsPath := filepath.Join(dir, "events.jsonl")
	lockPath := filepath.Join(dir, "lock")
	return createTaskWithDir(dir, opts, lockPath, eventsPath, epicID, isEpic, title, body)
}

func createTaskWithDir(dir string, opts GlobalOptions, lockPath, eventsPath, epicID string, isEpic bool, title, body string) (createOutput, error) {
	var output createOutput
	err := withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if !isEpic && epicID != "" {
			epic, ok := graph.Tasks[epicID]
			if !ok {
				return fmt.Errorf("unknown epic id %s", epicID)
			}
			if epic.EpicID != "" {
				return fmt.Errorf("task %s is not an epic", epicID)
			}
		}
		id, err := newShortID(graph.Tasks)
		if err != nil {
			return err
		}
		uuid, err := newUUID()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		payload := NewTaskEvent{
			ID:        id,
			UUID:      uuid,
			EpicID:    epicID,
			State:     stateTodo,
			Title:     title,
			Body:      body,
			CreatedAt: formatTime(now),
		}
		if isEpic {
			payload.EpicID = ""
		}
		eventType := "new_task"
		if isEpic {
			eventType = "new_epic"
		}
		event, err := newEvent(eventType, now, payload)
		if err != nil {
			return err
		}
		if err := appendEvents(eventsPath, []Event{event}); err != nil {
			return err
		}
		kind := "task"
		if isEpic {
			kind = "epic"
		}
		output = createOutput{
			Kind:      kind,
			ID:        id,
			UUID:      uuid,
			EpicID:    payload.EpicID,
			State:     stateTodo,
			Title:     title,
			Body:      body,
			CreatedAt: payload.CreatedAt,
		}
		return nil
	})
	if err != nil {
		return createOutput{}, err
	}
	return output, nil
}

// ResultEvidence holds evidence metadata captured when attaching a result.
type ResultEvidence struct {
	Sha256AtAttach    string
	MtimeAtAttach     string
	GitCommitAtAttach string
}

// validateResultPath ensures path is relative, within project root, and exists.
// Returns the cleaned relative path.
func validateResultPath(repoDir, relPath string) (string, error) {
	relPath = filepath.Clean(relPath)

	// Must be relative (no leading /)
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("result path must be relative: %s", relPath)
	}

	// No .. traversal outside project
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, string(filepath.Separator)+"..") {
		return "", fmt.Errorf("result path must be within project: %s", relPath)
	}

	// Must not point into .ergo/
	if strings.HasPrefix(relPath, dataDirName+string(filepath.Separator)) || relPath == dataDirName {
		return "", fmt.Errorf("result path cannot be inside .ergo/: %s", relPath)
	}

	// File must exist
	fullPath := filepath.Join(repoDir, relPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("result file does not exist: %s", relPath)
		}
		return "", fmt.Errorf("cannot access result file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("result path must be a file, not directory: %s", relPath)
	}

	return relPath, nil
}

// captureResultEvidence computes evidence metadata for a result file.
func captureResultEvidence(repoDir, relPath string) (ResultEvidence, error) {
	fullPath := filepath.Join(repoDir, relPath)

	// Read file and compute SHA256
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return ResultEvidence{}, fmt.Errorf("cannot read result file: %w", err)
	}
	hash := sha256.Sum256(content)

	// Get mtime
	info, err := os.Stat(fullPath)
	if err != nil {
		return ResultEvidence{}, fmt.Errorf("cannot stat result file: %w", err)
	}

	evidence := ResultEvidence{
		Sha256AtAttach: fmt.Sprintf("%x", hash),
		MtimeAtAttach:  formatTime(info.ModTime().UTC()),
	}

	// Try to get git HEAD commit (best-effort)
	if gitCommit := getGitHead(repoDir); gitCommit != "" {
		evidence.GitCommitAtAttach = gitCommit
	}

	return evidence, nil
}

// getGitHead returns the current HEAD commit SHA, or empty if not a git repo.
func getGitHead(repoDir string) string {
	// Check if .git exists
	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return ""
	}

	// Read HEAD
	headPath := filepath.Join(gitDir, "HEAD")
	headContent, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}

	head := strings.TrimSpace(string(headContent))

	// If it's a ref (e.g., "ref: refs/heads/main"), resolve it
	if strings.HasPrefix(head, "ref: ") {
		refPath := filepath.Join(gitDir, strings.TrimPrefix(head, "ref: "))
		refContent, err := os.ReadFile(refPath)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(refContent))
	}

	// Detached HEAD: already a commit SHA
	return head
}

// writeResultEvent attaches a result file reference to a task.
// The file must exist and be within the project root.
func writeResultEvent(dir string, opts GlobalOptions, taskID, summary, relPath string) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	repoDir := filepath.Dir(dir)

	return withLock(lockPath, syscall.LOCK_EX, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		task, ok := graph.Tasks[taskID]
		if !ok {
			return fmt.Errorf("unknown task id %s", taskID)
		}
		if isEpic(task) {
			return errors.New("cannot attach result to epic")
		}
		if err := validateResultSummary(summary); err != nil {
			return err
		}

		// Validate and normalize path
		cleanPath, err := validateResultPath(repoDir, relPath)
		if err != nil {
			return err
		}

		// Capture evidence
		evidence, err := captureResultEvidence(repoDir, cleanPath)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		event, err := newEvent("result", now, ResultEvent{
			TaskID:            taskID,
			Summary:           strings.TrimSpace(summary),
			Path:              cleanPath,
			Sha256AtAttach:    evidence.Sha256AtAttach,
			MtimeAtAttach:     evidence.MtimeAtAttach,
			GitCommitAtAttach: evidence.GitCommitAtAttach,
			TS:                formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}
