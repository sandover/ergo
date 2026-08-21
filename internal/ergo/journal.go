// The shared journal is the sole durable source for work narrative and results.
// It uses the repository lock but remains a separate JSONL file from graph state.
// Complete malformed records are corruption; only an interrupted final JSON
// record may be discarded on the next append.
package ergo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	journalFileName = "journal.jsonl"
	journalVersion  = 1
)

type JournalFile struct {
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	Mtime             string `json:"mtime,omitempty"`
	GitCommitAtAttach string `json:"git_commit,omitempty"`
}

type JournalEntry struct {
	Version int          `json:"version"`
	TaskID  string       `json:"task_id"`
	Kind    string       `json:"kind"`
	At      string       `json:"at"`
	Agent   string       `json:"agent,omitempty"`
	Text    string       `json:"text,omitempty"`
	File    *JournalFile `json:"file,omitempty"`
}

func newJournalEntry(taskID, kind, agent, text string, at time.Time) JournalEntry {
	return JournalEntry{Version: journalVersion, TaskID: taskID, Kind: kind, At: formatTime(at), Agent: agent, Text: text}
}

func isAutomaticJournalKind(kind string) bool {
	switch kind {
	case "claim", "done", "fail", "block", "cancel", "release":
		return true
	default:
		return false
	}
}

func validateJournalEntry(entry JournalEntry) error {
	if entry.Version != journalVersion {
		return fmt.Errorf("unsupported journal version %d", entry.Version)
	}
	if strings.TrimSpace(entry.TaskID) == "" {
		return errors.New("journal task_id is required")
	}
	switch entry.Kind {
	case "created", "claim", "done", "fail", "block", "cancel", "release", "result":
	default:
		return fmt.Errorf("invalid journal kind %q", entry.Kind)
	}
	if _, err := parseTime(entry.At); err != nil {
		return fmt.Errorf("invalid journal timestamp: %w", err)
	}
	if entry.Kind == "result" {
		if err := validateResultSummary(entry.Text); err != nil {
			return err
		}
	}
	if entry.File != nil {
		if entry.Kind != "result" {
			return errors.New("only result journal entries may attach files")
		}
		if entry.File.Path == "" {
			return errors.New("journal file path is required")
		}
	}
	return nil
}

type journalRead struct {
	entries        []JournalEntry
	validBytes     int64
	truncatedTail  bool
	needsSeparator bool
}

func readJournal(path string) (journalRead, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return journalRead{}, nil
	}
	if err != nil {
		return journalRead{}, err
	}
	endsWithNewline := len(data) > 0 && data[len(data)-1] == '\n'
	var result journalRead
	offset := 0
	lines := bytes.Split(data, []byte{'\n'})
	for index, line := range lines {
		if index == len(lines)-1 && len(line) == 0 {
			break
		}
		lineNo := index + 1
		if len(line) > maxLogRecordBytes {
			return journalRead{}, fmt.Errorf("%s:%d: journal record is too long", path, lineNo)
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			offset += len(line)
			if index < len(lines)-1 {
				offset++
			}
			result.validBytes = int64(offset)
			continue
		}
		var entry JournalEntry
		if err := json.Unmarshal(trimmed, &entry); err != nil {
			if index == len(lines)-1 && !endsWithNewline {
				result.truncatedTail = true
				break
			}
			return journalRead{}, fmt.Errorf("%s:%d: invalid JSON in journal: %w", path, lineNo, err)
		}
		if err := validateJournalEntry(entry); err != nil {
			return journalRead{}, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		result.entries = append(result.entries, entry)
		offset += len(line)
		if index < len(lines)-1 {
			offset++
		}
		result.validBytes = int64(offset)
	}
	if !result.truncatedTail && !endsWithNewline && result.validBytes > 0 {
		result.needsSeparator = true
	}
	return result, nil
}

func marshalJournal(entries []JournalEntry) ([]byte, error) {
	var output bytes.Buffer
	for _, entry := range entries {
		if err := validateJournalEntry(entry); err != nil {
			return nil, err
		}
		line, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		if len(line) > maxLogRecordBytes {
			return nil, errors.New("journal record is too long")
		}
		output.Write(line)
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

func (r *Repository) loadJournal() ([]JournalEntry, error) {
	read, err := readJournal(r.journalPath)
	if err != nil {
		return nil, &corruptionError{err: err}
	}
	return read.entries, nil
}

func (r *Repository) appendJournal(entries []JournalEntry) error {
	data, err := marshalJournal(entries)
	if err != nil || len(data) == 0 {
		return err
	}
	read, err := readJournal(r.journalPath)
	if err != nil {
		return err
	}
	file, err := r.io.openFile(r.journalPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if read.truncatedTail {
		if err := file.Truncate(read.validBytes); err != nil {
			return fmt.Errorf("repair incomplete journal tail: %w", err)
		}
	}
	if _, err := file.Seek(0, 2); err != nil {
		return err
	}
	if read.needsSeparator && !read.truncatedTail {
		data = append([]byte{'\n'}, data...)
	}
	if err := writeAllWith(file, data, r.io.write); err != nil {
		return err
	}
	if err := r.io.postWrite(); err != nil {
		return err
	}
	if err := r.io.sync(file); err != nil {
		return fmt.Errorf("sync journal record: %w", err)
	}
	return nil
}

func (r *Repository) replaceJournal(entries []JournalEntry) error {
	data, err := marshalJournal(entries)
	if err != nil {
		return err
	}
	return replaceLogAtomically(r.journalPath, data)
}

func journalForTask(entries []JournalEntry, taskID string) []JournalEntry {
	var selected []JournalEntry
	for _, entry := range entries {
		if entry.TaskID == taskID {
			selected = append(selected, entry)
		}
	}
	return selected
}

func latestExplicitResult(entries []JournalEntry, taskID string) *JournalEntry {
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].TaskID == taskID && entries[index].Kind == "result" {
			entry := entries[index]
			return &entry
		}
	}
	return nil
}

func mergeLegacyJournal(entries []JournalEntry, graph *Graph) []JournalEntry {
	merged := append([]JournalEntry(nil), entries...)
	seen := make(map[string]struct{}, len(merged))
	for _, entry := range merged {
		seen[journalEntryKey(entry)] = struct{}{}
	}
	for _, task := range sortedTasks(graph.Tasks) {
		for index := len(task.Messages) - 1; index >= 0; index-- {
			message := task.Messages[index]
			entry := newJournalEntry(task.ID, message.Kind, "", message.Text, message.CreatedAt)
			if _, exists := seen[journalEntryKey(entry)]; !exists {
				merged = append(merged, entry)
				seen[journalEntryKey(entry)] = struct{}{}
			}
		}
		for index := len(task.Results) - 1; index >= 0; index-- {
			result := task.Results[index]
			entry := newJournalEntry(task.ID, "result", "", result.Summary, result.CreatedAt)
			entry.File = &JournalFile{Path: result.Path, SHA256: result.Sha256AtAttach, Mtime: result.MtimeAtAttach, GitCommitAtAttach: result.GitCommitAtAttach}
			if _, exists := seen[journalEntryKey(entry)]; !exists {
				merged = append(merged, entry)
				seen[journalEntryKey(entry)] = struct{}{}
			}
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].At != merged[j].At {
			return merged[i].At < merged[j].At
		}
		return merged[i].TaskID < merged[j].TaskID
	})
	return merged
}

func journalEntryKey(entry JournalEntry) string {
	data, _ := json.Marshal(entry)
	return string(data)
}

func compactJournal(entries []JournalEntry, graph *Graph) []JournalEntry {
	latestAutomatic := map[string]int{}
	for index, entry := range entries {
		if entry.Kind != "result" && graph.Tasks[entry.TaskID] != nil {
			latestAutomatic[entry.TaskID] = index
		}
	}
	compacted := make([]JournalEntry, 0, len(entries))
	for index, entry := range entries {
		if graph.Tasks[entry.TaskID] == nil {
			continue
		}
		if entry.Kind == "result" || latestAutomatic[entry.TaskID] == index {
			compacted = append(compacted, entry)
		}
	}
	return compacted
}

func hydrateGraphEvidence(graph *Graph, entries []JournalEntry) {
	for _, task := range graph.Tasks {
		task.Results = nil
		task.Messages = nil
	}
	for _, entry := range entries {
		task := graph.Tasks[entry.TaskID]
		if task == nil {
			continue
		}
		createdAt, err := parseTime(entry.At)
		if err != nil {
			continue
		}
		if entry.Kind == "result" {
			result := Result{Summary: entry.Text, CreatedAt: createdAt}
			if entry.File != nil {
				result.Path = entry.File.Path
				result.Sha256AtAttach = entry.File.SHA256
				result.MtimeAtAttach = entry.File.Mtime
				result.GitCommitAtAttach = entry.File.GitCommitAtAttach
			}
			task.Results = append([]Result{result}, task.Results...)
		} else if entry.Text != "" {
			task.Messages = append([]Message{{Kind: entry.Kind, Text: entry.Text, CreatedAt: createdAt}}, task.Messages...)
		}
	}
}

func journalPathForDir(dir string) string { return filepath.Join(dir, journalFileName) }
