// Purpose: Manage .ergo discovery and append-only event storage.
// Exports: ResultEvidence.
// Role: Persistence layer used by commands and replay/compact paths.
// Invariants: Writes are append-only under lock; read tolerates truncated final line.
// Notes: Result paths are validated to remain within the repo.
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
	"time"
)

const (
	dataDirName       = ".ergo"
	backlogFileName   = "backlog.jsonl"
	plansFileName     = "plans.jsonl"
	oldEventsFileName = "events.jsonl"
	maxLogRecordBytes = 10 * 1024 * 1024
)

const transactionRecordType = "transaction"

type transactionRecord struct {
	Type    string  `json:"type"`
	Version int     `json:"version"`
	Events  []Event `json:"events"`
}

type eventLogRead struct {
	events         []Event
	validBytes     int64
	truncatedTail  bool
	needsSeparator bool
}

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
	return resolveErgoDir(start)
}

func loadGraph(dir string) (*Graph, error) {
	eventsPath, err := selectEventsPath(dir)
	if err != nil {
		return nil, err
	}
	events, err := readEvents(eventsPath)
	if err != nil {
		return nil, err
	}
	return replayEvents(events)
}

func loadGraphLocked(dir string, opts GlobalOptions) (*Graph, error) {
	lockPath := filepath.Join(dir, "lock")
	var graph *Graph
	err := withLock(lockPath, opts, func() error {
		var err error
		graph, err = loadGraph(dir)
		return err
	})
	if err != nil {
		return nil, err
	}
	return graph, nil
}

func readEvents(path string) ([]Event, error) {
	read, err := inspectEventLog(path)
	if err != nil {
		return nil, err
	}
	return read.events, nil
}

func inspectEventLog(path string) (eventLogRead, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return eventLogRead{}, nil
		}
		return eventLogRead{}, err
	}
	defer file.Close()

	endsWithNewline := false
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		last := make([]byte, 1)
		if _, err := file.ReadAt(last, info.Size()-1); err == nil {
			endsWithNewline = last[0] == '\n'
		}
	}

	var result eventLogRead
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogRecordBytes)
	var pending []byte
	pendingNo := 0
	currentNo := 0

	processLine := func(lineNo int, line []byte) error {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(trimmed, &header); err != nil {
			return formatEventsParseError(path, lineNo, trimmed, err)
		}
		if header.Type != transactionRecordType {
			var event Event
			if err := json.Unmarshal(trimmed, &event); err != nil {
				return formatEventsParseError(path, lineNo, trimmed, err)
			}
			event.Source = EventSource{Path: path, Line: lineNo}
			result.events = append(result.events, event)
			return nil
		}
		var record transactionRecord
		if err := json.Unmarshal(trimmed, &record); err != nil {
			return formatEventsParseError(path, lineNo, trimmed, err)
		}
		if record.Version != 1 {
			return fmt.Errorf("%s:%d: unsupported transaction record version %d", path, lineNo, record.Version)
		}
		if len(record.Events) == 0 {
			return fmt.Errorf("%s:%d: transaction record contains no events", path, lineNo)
		}
		for i := range record.Events {
			record.Events[i].Source = EventSource{
				Path:             path,
				Line:             lineNo,
				TransactionIndex: i + 1,
			}
			result.events = append(result.events, record.Events[i])
		}
		return nil
	}

	for scanner.Scan() {
		currentNo++
		line := append([]byte(nil), scanner.Bytes()...) // copy (scanner buffer is reused)
		if pending != nil {
			if err := processLine(pendingNo, pending); err != nil {
				return eventLogRead{}, err
			}
			result.validBytes += int64(len(pending) + 1)
		}
		pending = line
		pendingNo = currentNo
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return eventLogRead{}, fmt.Errorf("%s: event record too long (> %d bytes); file may be corrupted (e.g. missing newlines)", path, maxLogRecordBytes)
		}
		return eventLogRead{}, err
	}

	if pending != nil {
		if err := processLine(pendingNo, pending); err != nil {
			// Only malformed JSON in an unterminated final record is a
			// recoverable interrupted write. A complete JSON value with an
			// unsupported version or invalid semantics remains corruption.
			if !endsWithNewline && !json.Valid(bytes.TrimSpace(pending)) {
				result.truncatedTail = true
				return result, nil
			}
			return eventLogRead{}, err
		}
		result.validBytes += int64(len(pending))
		if endsWithNewline {
			result.validBytes++
		} else if len(pending) > 0 {
			result.needsSeparator = true
		}
	}
	return result, nil
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
	data, err := marshalTransaction(events)
	if err != nil || len(data) == 0 {
		return err
	}
	return appendTransaction(path, data, systemRepositoryIO())
}

func marshalTransaction(events []Event) ([]byte, error) {
	if len(events) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(transactionRecord{
		Type:    transactionRecordType,
		Version: 1,
		Events:  events,
	})
	if err != nil {
		return nil, err
	}
	if len(data) > maxLogRecordBytes {
		return nil, fmt.Errorf("transaction record is too long: %d bytes exceeds the %d-byte limit", len(data), maxLogRecordBytes)
	}
	return append(data, '\n'), nil
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
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func replaceEventsAtomically(path string, events []Event) error {
	tmpPath := path + ".tmp"
	if err := writeEventsFile(tmpPath, events); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func writeAll(w *os.File, data []byte) error {
	return writeAllWith(w, data, func(file *os.File, chunk []byte) (int, error) {
		return file.Write(chunk)
	})
}

func writeAllWith(w *os.File, data []byte, write func(*os.File, []byte) (int, error)) error {
	for len(data) > 0 {
		n, err := write(w, data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return errors.New("event log write made no progress")
		}
		data = data[n:]
	}
	return nil
}

func createTaskWithUpdates(dir string, opts GlobalOptions, epicID string, title, body string, updates map[string]string, agentID string) (createOutput, error) {
	eventsPath, err := selectEventsPath(dir)
	if err != nil {
		return createOutput{}, err
	}
	lockPath := filepath.Join(dir, "lock")
	return createTaskWithDir(dir, opts, lockPath, eventsPath, epicID, title, body, updates, agentID)
}

func createTaskWithDir(dir string, opts GlobalOptions, lockPath, eventsPath, epicID string, title, body string, updates map[string]string, agentID string) (createOutput, error) {
	var output createOutput
	err := withLock(lockPath, opts, func() error {
		graph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		if epicID != "" {
			epic, ok := graph.Tasks[epicID]
			if !ok {
				return fmt.Errorf("unknown epic id %s", epicID)
			}
			if epic.EpicID != "" {
				return fmt.Errorf("task %s is not an epic", epicID)
			}
			// Reject first-child assignment to a dirty leaf: once promoted to a
			// container, leaf-only semantics (state/claim/results) no longer apply.
			if !isContainer(epic, graph) {
				if epic.ClaimedBy != "" {
					return fmt.Errorf("cannot add child to task %s: task is claimed by %q", epicID, epic.ClaimedBy)
				}
				if epic.State != stateTodo {
					return fmt.Errorf("cannot add child to task %s: state is %q (must be todo to promote to epic)", epicID, epic.State)
				}
				if len(epic.Results) > 0 {
					return fmt.Errorf("cannot add child to task %s: task has results attached", epicID)
				}
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
		createdAt := formatTime(now)
		payload := NewTaskEvent{
			ID:        id,
			UUID:      uuid,
			EpicID:    epicID,
			State:     stateTodo,
			Title:     title,
			Body:      body,
			CreatedAt: createdAt,
		}
		event, err := newEvent("new_task", now, payload)
		if err != nil {
			return err
		}

		newTask := &Task{
			ID:        id,
			UUID:      uuid,
			EpicID:    epicID,
			State:     stateTodo,
			Title:     title,
			Body:      body,
			CreatedAt: now,
			UpdatedAt: now,
		}
		graph.Tasks[id] = newTask
		graph.Meta[id] = &TaskMeta{
			CreatedTitle:     title,
			CreatedBody:      body,
			CreatedState:     stateTodo,
			CreatedEpicID:    epicID,
			CreatedEpicIDSet: true,
			CreatedAt:        now,
		}

		events := []Event{event}
		resultPath, hasPath := updates["result.path"]
		resultSummary, hasSummary := updates["result.summary"]
		if hasPath || hasSummary {
			if !hasPath {
				return errors.New("result.summary requires result.path=")
			}
			if !hasSummary {
				resultSummary = resultPath
			}
			resultEvent, err := buildResultEvent(filepath.Dir(dir), graph, id, resultSummary, resultPath, now)
			if err != nil {
				return err
			}
			events = append(events, resultEvent)
			delete(updates, "result.path")
			delete(updates, "result.summary")
		}

		mutation := taskMutation{}
		if state, ok := updates["state"]; ok {
			mutation.State, mutation.StateSet = state, true
			delete(updates, "state")
		}
		if claim, ok := updates["claim"]; ok {
			mutation.Claim, mutation.ClaimSet = claim, true
			delete(updates, "claim")
		}
		createEvents, _, err := buildMutationEvents(id, newTask, mutation, agentID, now)
		if err != nil {
			return err
		}
		if len(updates) > 0 {
			var unknown []string
			for key := range updates {
				unknown = append(unknown, key)
			}
			return fmt.Errorf("unknown keys: %s", strings.Join(unknown, ", "))
		}
		events = append(events, createEvents...)

		if err := appendEvents(eventsPath, events); err != nil {
			return err
		}
		updatedGraph, err := loadGraph(dir)
		if err != nil {
			return err
		}
		task := updatedGraph.Tasks[id]
		if task == nil {
			return errors.New("internal error: missing created task")
		}
		output = createOutput{
			ID:        id,
			UUID:      uuid,
			EpicID:    payload.EpicID,
			State:     task.State,
			Title:     task.Title,
			Body:      task.Body,
			CreatedAt: createdAt,
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
	// filepath.IsAbs does not consider drive-relative or root-relative paths
	// absolute on Windows. IsLocal rejects those forms as well as traversal.
	if !filepath.IsLocal(relPath) {
		return "", fmt.Errorf("result path must be relative: %s", relPath)
	}
	relPath = filepath.Clean(relPath)

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

func buildResultEvent(repoDir string, graph *Graph, taskID, summary, relPath string, now time.Time) (Event, error) {
	if _, ok := graph.Tombstones[taskID]; ok {
		return Event{}, prunedErr(taskID)
	}
	task, ok := graph.Tasks[taskID]
	if !ok {
		return Event{}, fmt.Errorf("unknown task id %s", taskID)
	}
	if isContainer(task, graph) {
		return Event{}, errors.New("cannot attach result to epic")
	}
	if err := validateResultSummary(summary); err != nil {
		return Event{}, err
	}

	cleanPath, err := validateResultPath(repoDir, relPath)
	if err != nil {
		return Event{}, err
	}
	evidence, err := captureResultEvidence(repoDir, cleanPath)
	if err != nil {
		return Event{}, err
	}

	return newEvent("result", now, ResultEvent{
		TaskID:            taskID,
		Summary:           strings.TrimSpace(summary),
		Path:              cleanPath,
		Sha256AtAttach:    evidence.Sha256AtAttach,
		MtimeAtAttach:     evidence.MtimeAtAttach,
		GitCommitAtAttach: evidence.GitCommitAtAttach,
		TS:                formatTime(now),
	})
}
