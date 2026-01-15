// Data directory discovery and event storage helpers.
package main

import (
	"bufio"
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

func findDataDir(start string) (string, bool) {
	current := start
	for {
		candidate := filepath.Join(current, dataDirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, true
		}
		if current == filepath.Dir(current) {
			return "", false
		}
		current = filepath.Dir(current)
	}
}

func resolveErgoDir(start string) (string, error) {
	ergoDir, ok := findDataDir(start)
	if ok {
		return ergoDir, nil
	}
	if filepath.Base(start) == dataDirName {
		if info, err := os.Stat(start); err == nil && info.IsDir() {
			return start, nil
		}
	}
	return "", fmt.Errorf("%w (run ergo init)", errNoErgoDir)
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

	var events []Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func appendEvents(path string, events []Event) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

func writeLinkEvent(dir string, opts GlobalOptions, eventType, from, to string) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
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

func writeWorkerEvent(dir string, opts GlobalOptions, id string, worker Worker) error {
	lockPath := filepath.Join(dir, "lock")
	eventsPath := filepath.Join(dir, "events.jsonl")
	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if _, ok := graph.Tasks[id]; !ok {
			return fmt.Errorf("unknown task id %s", id)
		}
		now := time.Now().UTC()
		event, err := newEvent("worker", now, WorkerEvent{
			ID:     id,
			Worker: string(worker),
			TS:     formatTime(now),
		})
		if err != nil {
			return err
		}
		return appendEvents(eventsPath, []Event{event})
	})
}

func createTask(dir string, opts GlobalOptions, epicID string, isEpic bool, body string, worker Worker) (createOutput, error) {
	eventsPath := filepath.Join(dir, "events.jsonl")
	lockPath := filepath.Join(dir, "lock")
	return createTaskWithDir(dir, opts, lockPath, eventsPath, epicID, isEpic, body, worker)
}

func createTaskWithDir(dir string, opts GlobalOptions, lockPath, eventsPath, epicID string, isEpic bool, body string, worker Worker) (createOutput, error) {
	if worker == "" {
		worker = workerAny
	}
	var output createOutput
	err := withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
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
			Body:      body,
			Worker:    string(worker),
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
			Worker:    string(worker),
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

	return withLock(lockPath, syscall.LOCK_EX, opts.LockTimeout, func() error {
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
