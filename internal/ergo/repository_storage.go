// Purpose: Manage repository discovery, append-only storage, and result evidence.
// Exports: ResultEvidence.
// Role: Persistence layer used by commands and replay/compact paths.
// Invariants: Writes are append-only under lock; read tolerates truncated final line.
// Notes: Result paths are validated to remain within the repo.
package ergo

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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

type eventLogRead struct {
	events         []Event
	snapshot       *Graph
	recordCount    int
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

func repositoryLoadGraph(dir string) (*Graph, error) {
	eventsPath, err := selectEventsPath(dir)
	if err != nil {
		return nil, err
	}
	read, err := inspectEventLog(eventsPath)
	if err != nil {
		return nil, err
	}
	if read.snapshot != nil {
		return replayEventsOnto(read.snapshot, read.events)
	}
	return replayEvents(read.events)
}

func readEvents(path string) ([]Event, error) {
	read, err := inspectEventLog(path)
	if err != nil {
		return nil, err
	}
	if read.snapshot != nil {
		return nil, fmt.Errorf("%s: snapshot backlog cannot be represented as an event slice", path)
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
	seenRecords := 0
	var snapshotDecoder *snapshotBlockDecoder

	processLine := func(lineNo int, line []byte) error {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		seenRecords++
		result.recordCount++
		if snapshotDecoder != nil && snapshotDecoder.seen < snapshotDecoder.total() {
			if err := snapshotDecoder.consume(lineNo, trimmed); err != nil {
				return err
			}
			if snapshotDecoder.seen == snapshotDecoder.total() {
				graph, err := snapshotDecoder.finish()
				if err != nil {
					return err
				}
				result.snapshot = graph
			}
			return nil
		}
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(trimmed, &header); err != nil {
			return formatEventsParseError(path, lineNo, trimmed, err)
		}
		if header.Type == snapshotRecordType {
			if seenRecords != 1 || result.snapshot != nil || snapshotDecoder != nil {
				return fmt.Errorf("%s:%d: snapshot manifest must be the first and only snapshot", path, lineNo)
			}
			decoder, err := newSnapshotDecoder(path, lineNo, trimmed)
			if err != nil {
				return err
			}
			snapshotDecoder = decoder
			if decoder.total() == 0 {
				graph, err := decoder.finish()
				if err != nil {
					return err
				}
				result.snapshot = graph
			}
			return nil
		}
		if snapshotKind(header.Type) {
			return fmt.Errorf("%s:%d: snapshot data record outside a snapshot block: %q", path, lineNo, header.Type)
		}
		events, err := decodeEventLogRecord(path, lineNo, trimmed)
		if err != nil {
			return err
		}
		result.events = append(result.events, events...)
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
			incompleteSnapshot := snapshotDecoder != nil && snapshotDecoder.seen < snapshotDecoder.total()
			if !incompleteSnapshot && !endsWithNewline && !json.Valid(bytes.TrimSpace(pending)) {
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
	if snapshotDecoder != nil && snapshotDecoder.seen != snapshotDecoder.total() {
		return eventLogRead{}, fmt.Errorf("%s:%d: incomplete snapshot: got %d of %d data records",
			path, snapshotDecoder.line, snapshotDecoder.seen, snapshotDecoder.total())
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

func repositoryAppendEvents(path string, events []Event) error {
	data, err := marshalTransaction(events)
	if err != nil || len(data) == 0 {
		return err
	}
	return appendTransaction(path, data, systemRepositoryIO())
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

func replaceLogAtomically(path string, data []byte) error {
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if err := writeAllWith(file, data, func(file *os.File, chunk []byte) (int, error) {
		return file.Write(chunk)
	}); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
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

func createTask(dir string, opts RepositoryOptions, epicID string, title, body string, draft bool) (createOutput, error) {
	var repository Repository
	if err := repository.openAt(dir, opts, systemRepositoryIO()); err != nil {
		return createOutput{}, err
	}
	var output createOutput
	update, err := repository.UpdateWithJournal(func(graph *Graph) ([]Event, []JournalEntry, error) {
		if epicID != "" {
			epic, ok := graph.Tasks[epicID]
			if !ok {
				return nil, nil, classified(ErrorNotFound, fmt.Errorf("unknown epic id %s", epicID))
			}
			if epic.EpicID != "" {
				return nil, nil, classified(ErrorConflict, fmt.Errorf("task %s is not an epic", epicID))
			}
			// Reject first-child assignment to a dirty leaf: once promoted to a
			// container, leaf-only semantics (state/claim/results) no longer apply.
			if !graph.IsEpic(epic.ID) {
				if err := validateEpicPromotion(epic); err != nil {
					return nil, nil, classified(ErrorConflict, fmt.Errorf("cannot add child to task %s: %w", epicID, err))
				}
			}
		}
		id, err := newShortID(graph.Tasks)
		if err != nil {
			return nil, nil, err
		}
		uuid, err := newUUID()
		if err != nil {
			return nil, nil, err
		}
		now := time.Now().UTC()
		createdAt := formatTime(now)
		state := stateTodo
		if draft {
			state = stateDraft
		}
		payload := NewTaskEvent{
			ID:        id,
			UUID:      uuid,
			EpicID:    epicID,
			State:     state,
			Title:     title,
			Body:      body,
			CreatedAt: createdAt,
		}
		event, err := newEvent("new_task", now, payload)
		if err != nil {
			return nil, nil, err
		}

		output = createOutput{
			ID:        id,
			UUID:      uuid,
			EpicID:    payload.EpicID,
			State:     payload.State,
			Title:     payload.Title,
			Body:      payload.Body,
			CreatedAt: createdAt,
		}
		return []Event{event}, []JournalEntry{newJournalEntry(id, "created", "", "", now)}, nil
	})
	if err != nil {
		return createOutput{}, err
	}
	if update.Graph == nil || update.Graph.Tasks[output.ID] == nil {
		return createOutput{}, errors.New("internal error: missing created task")
	}
	output.State = update.Graph.Tasks[output.ID].State
	return output, nil
}

// ResultEvidence holds evidence metadata captured when attaching a result.
type ResultEvidence struct {
	Sha256AtAttach    string
	MtimeAtAttach     string
	GitCommitAtAttach string
}

// validateResultPath ensures the path resolves to a regular file inside the
// project. It returns the cleaned caller path rather than the resolved target,
// so an accepted in-project symlink remains meaningful in task output.
func validateResultPath(repoDir, relPath string) (string, error) {
	cleanPath, _, err := resolveResultPath(repoDir, relPath)
	return cleanPath, err
}

func resolveResultPath(repoDir, relPath string) (string, string, error) {
	// filepath.IsAbs does not consider drive-relative or root-relative paths
	// absolute on Windows. IsLocal rejects those forms as well as traversal.
	if !filepath.IsLocal(relPath) {
		return "", "", fmt.Errorf("result path must be relative: %s", relPath)
	}
	relPath = filepath.Clean(relPath)

	if strings.HasPrefix(relPath, dataDirName+string(filepath.Separator)) || relPath == dataDirName {
		return "", "", fmt.Errorf("result path cannot be inside .ergo/: %s", relPath)
	}

	resolvedRepo, err := filepath.EvalSymlinks(repoDir)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve project path: %w", err)
	}
	resolvedRepo, err = filepath.Abs(resolvedRepo)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve project path: %w", err)
	}

	fullPath := filepath.Join(resolvedRepo, relPath)
	resolvedTarget, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("result file does not exist: %s", relPath)
		}
		return "", "", fmt.Errorf("cannot resolve result file: %w", err)
	}
	resolvedTarget, err = filepath.Abs(resolvedTarget)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve result file: %w", err)
	}

	resolvedRelative, err := filepath.Rel(resolvedRepo, resolvedTarget)
	if err != nil || !filepath.IsLocal(resolvedRelative) {
		return "", "", fmt.Errorf("result path must resolve within project: %s", relPath)
	}
	if resolvedRelative == dataDirName ||
		strings.HasPrefix(resolvedRelative, dataDirName+string(filepath.Separator)) {
		return "", "", fmt.Errorf("result path cannot resolve inside .ergo/: %s", relPath)
	}

	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return "", "", fmt.Errorf("cannot access result file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("result path must resolve to a regular file: %s", relPath)
	}

	return relPath, resolvedTarget, nil
}

// captureResultEvidence computes evidence metadata for a result file.
func captureResultEvidence(repoDir, relPath string) (ResultEvidence, error) {
	_, resolvedTarget, err := resolveResultPath(repoDir, relPath)
	if err != nil {
		return ResultEvidence{}, err
	}

	file, err := os.Open(resolvedTarget)
	if err != nil {
		return ResultEvidence{}, fmt.Errorf("cannot open result file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ResultEvidence{}, fmt.Errorf("cannot read result file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return ResultEvidence{}, fmt.Errorf("cannot inspect result file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ResultEvidence{}, fmt.Errorf("result path must resolve to a regular file: %s", relPath)
	}

	evidence := ResultEvidence{
		Sha256AtAttach: fmt.Sprintf("%x", hasher.Sum(nil)),
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
