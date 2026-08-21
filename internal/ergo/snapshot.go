// Purpose: Encode and decode deterministic bounded snapshots of current state.
// Role: Canonical compaction format; snapshots are state, not reconstructed history.
// Invariants: Records are ordered, individually bounded, counted, and integrity-checked.
// Notes: Compaction intentionally garbage-collects tombstones.
package ergo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	snapshotRecordType           = "snapshot"
	snapshotTaskRecordType       = "snapshot_task"
	snapshotResultRecordType     = "snapshot_result"
	snapshotMessageRecordType    = "snapshot_message"
	snapshotDependencyRecordType = "snapshot_dependency"
	snapshotVersion              = 1
)

type snapshotManifest struct {
	Type         string `json:"type"`
	Version      int    `json:"version"`
	Tasks        int    `json:"tasks"`
	Results      int    `json:"results"`
	Messages     int    `json:"messages"`
	Dependencies int    `json:"dependencies"`
	SHA256       string `json:"sha256"`
}

type snapshotTaskRecord struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	UUID         string `json:"uuid"`
	EpicID       string `json:"epic_id"`
	ExplicitEpic bool   `json:"explicit_epic"`
	State        string `json:"state"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	ClaimedBy    string `json:"claimed_by"`
	ClaimedAt    string `json:"claimed_at"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type snapshotResultRecord struct {
	Type              string `json:"type"`
	TaskID            string `json:"task_id"`
	Ordinal           int    `json:"ordinal"`
	Summary           string `json:"summary"`
	Path              string `json:"path"`
	SHA256AtAttach    string `json:"sha256_at_attach"`
	MtimeAtAttach     string `json:"mtime_at_attach"`
	GitCommitAtAttach string `json:"git_commit_at_attach"`
	CreatedAt         string `json:"created_at"`
}

type snapshotMessageRecord struct {
	Type      string `json:"type"`
	TaskID    string `json:"task_id"`
	Ordinal   int    `json:"ordinal"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type snapshotDependencyRecord struct {
	Type   string `json:"type"`
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
}

type snapshotStats struct {
	Records int
}

func marshalSnapshot(graph *Graph) ([]byte, snapshotStats, error) {
	records, manifest, err := snapshotRecords(graph)
	if err != nil {
		return nil, snapshotStats{}, err
	}
	var payload bytes.Buffer
	hash := sha256.New()
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return nil, snapshotStats{}, err
		}
		if len(line) > maxLogRecordBytes {
			return nil, snapshotStats{}, fmt.Errorf("snapshot record is too long: %d bytes exceeds the %d-byte limit", len(line), maxLogRecordBytes)
		}
		hash.Write(line)
		hash.Write([]byte{'\n'})
		payload.Write(line)
		payload.WriteByte('\n')
	}
	manifest.SHA256 = hex.EncodeToString(hash.Sum(nil))
	header, err := json.Marshal(manifest)
	if err != nil {
		return nil, snapshotStats{}, err
	}
	if len(header) > maxLogRecordBytes {
		return nil, snapshotStats{}, fmt.Errorf("snapshot manifest is too long")
	}
	out := append(header, '\n')
	out = append(out, payload.Bytes()...)
	return out, snapshotStats{Records: 1 + len(records)}, nil
}

func snapshotRecords(graph *Graph) ([]any, snapshotManifest, error) {
	if graph == nil {
		graph = newGraph()
	}
	var records []any
	manifest := snapshotManifest{Type: snapshotRecordType, Version: snapshotVersion}
	tasks := sortedTasks(graph.Tasks)
	manifest.Tasks = len(tasks)
	for _, task := range tasks {
		_, explicit := graph.legacyEmptyEpics[task.ID]
		explicit = explicit && len(graph.Children(task.ID)) == 0
		claimedAt := ""
		if !task.ClaimedAt.IsZero() {
			claimedAt = formatTime(task.ClaimedAt)
		}
		records = append(records, snapshotTaskRecord{
			Type: snapshotTaskRecordType, ID: task.ID, UUID: task.UUID,
			EpicID: task.EpicID, ExplicitEpic: explicit, State: task.State,
			Title: task.Title, Body: task.Body, ClaimedBy: task.ClaimedBy,
			ClaimedAt: claimedAt, CreatedAt: formatTime(task.CreatedAt), UpdatedAt: formatTime(task.UpdatedAt),
		})
	}
	fromIDs := sortedMapKeys(graph.Deps)
	for _, from := range fromIDs {
		toIDs := sortedKeys(graph.Deps[from])
		for _, to := range toIDs {
			records = append(records, snapshotDependencyRecord{
				Type: snapshotDependencyRecordType, FromID: from, ToID: to,
			})
			manifest.Dependencies++
		}
	}
	return records, manifest, nil
}

type snapshotBlockDecoder struct {
	path               string
	line               int
	manifest           snapshotManifest
	seen               int
	hash               bytes.Buffer
	graph              *Graph
	lastTask           string
	lastResultTask     string
	lastMessageTask    string
	lastDependencyFrom string
	lastDependencyTo   string
}

func newSnapshotDecoder(path string, line int, raw []byte) (*snapshotBlockDecoder, error) {
	var manifest snapshotManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%s:%d: invalid snapshot manifest: %w", path, line, err)
	}
	if manifest.Version != snapshotVersion {
		return nil, fmt.Errorf("%s:%d: unsupported snapshot version %d", path, line, manifest.Version)
	}
	if manifest.Tasks < 0 || manifest.Results < 0 || manifest.Messages < 0 || manifest.Dependencies < 0 {
		return nil, fmt.Errorf("%s:%d: snapshot counts cannot be negative", path, line)
	}
	return &snapshotBlockDecoder{
		path: path, line: line, manifest: manifest, graph: newGraph(),
	}, nil
}

func (decoder *snapshotBlockDecoder) total() int {
	return decoder.manifest.Tasks + decoder.manifest.Results + decoder.manifest.Messages + decoder.manifest.Dependencies
}

func (decoder *snapshotBlockDecoder) consume(line int, raw []byte) error {
	decoder.hash.Write(raw)
	decoder.hash.WriteByte('\n')
	index := decoder.seen
	switch {
	case index < decoder.manifest.Tasks:
		var record snapshotTaskRecord
		if err := decodeSnapshotRecord(decoder.path, line, raw, snapshotTaskRecordType, &record); err != nil {
			return err
		}
		if decoder.graph.Tasks[record.ID] != nil || record.ID == "" {
			return fmt.Errorf("%s:%d: invalid or duplicate snapshot task id %q", decoder.path, line, record.ID)
		}
		if decoder.lastTask != "" && record.ID <= decoder.lastTask {
			return fmt.Errorf("%s:%d: snapshot tasks are not in increasing ID order", decoder.path, line)
		}
		decoder.lastTask = record.ID
		createdAt, err := parseTime(record.CreatedAt)
		if err != nil {
			return fmt.Errorf("%s:%d: snapshot task %s has invalid created_at: %w", decoder.path, line, record.ID, err)
		}
		updatedAt, err := parseTime(record.UpdatedAt)
		if err != nil {
			return fmt.Errorf("%s:%d: snapshot task %s has invalid updated_at: %w", decoder.path, line, record.ID, err)
		}
		claimedAt := zeroTime
		if record.ClaimedAt != "" {
			claimedAt, err = parseTime(record.ClaimedAt)
			if err != nil {
				return fmt.Errorf("%s:%d: snapshot task %s has invalid claimed_at: %w", decoder.path, line, record.ID, err)
			}
		}
		if !isReadableState(record.State) {
			return fmt.Errorf("%s:%d: snapshot task %s has invalid state %q", decoder.path, line, record.ID, record.State)
		}
		if record.ClaimedBy == "" && !claimedAt.IsZero() {
			return fmt.Errorf("%s:%d: snapshot task %s has claimed_at without claimed_by", decoder.path, line, record.ID)
		}
		if record.ClaimedBy != "" && claimedAt.IsZero() {
			return fmt.Errorf("%s:%d: snapshot task %s has claimed_by without claimed_at", decoder.path, line, record.ID)
		}
		if record.ExplicitEpic && record.EpicID != "" {
			return fmt.Errorf("%s:%d: explicit snapshot epic %s cannot have a parent", decoder.path, line, record.ID)
		}
		decoder.graph.Tasks[record.ID] = &Task{
			ID: record.ID, UUID: record.UUID, EpicID: record.EpicID, State: record.State,
			Title: record.Title, Body: record.Body, ClaimedBy: record.ClaimedBy, ClaimedAt: claimedAt,
			CreatedAt: createdAt, UpdatedAt: updatedAt,
		}
		if record.ExplicitEpic {
			decoder.graph.legacyEmptyEpics[record.ID] = struct{}{}
		}
	case index < decoder.manifest.Tasks+decoder.manifest.Results:
		var record snapshotResultRecord
		if err := decodeSnapshotRecord(decoder.path, line, raw, snapshotResultRecordType, &record); err != nil {
			return err
		}
		task := decoder.graph.Tasks[record.TaskID]
		if task == nil || record.Ordinal != len(task.Results) {
			return fmt.Errorf("%s:%d: invalid snapshot result task/ordinal %s/%d", decoder.path, line, record.TaskID, record.Ordinal)
		}
		if decoder.lastResultTask != "" && record.TaskID < decoder.lastResultTask {
			return fmt.Errorf("%s:%d: snapshot results are not ordered by task ID", decoder.path, line)
		}
		decoder.lastResultTask = record.TaskID
		createdAt, err := parseTime(record.CreatedAt)
		if err != nil {
			return fmt.Errorf("%s:%d: snapshot result for %s has invalid created_at: %w", decoder.path, line, record.TaskID, err)
		}
		if err := validateResultSummary(record.Summary); err != nil {
			return fmt.Errorf("%s:%d: snapshot result for %s: %w", decoder.path, line, record.TaskID, err)
		}
		task.Results = append(task.Results, Result{
			Summary: record.Summary, Path: record.Path, Sha256AtAttach: record.SHA256AtAttach,
			MtimeAtAttach: record.MtimeAtAttach, GitCommitAtAttach: record.GitCommitAtAttach, CreatedAt: createdAt,
		})
	case index < decoder.manifest.Tasks+decoder.manifest.Results+decoder.manifest.Messages:
		var record snapshotMessageRecord
		if err := decodeSnapshotRecord(decoder.path, line, raw, snapshotMessageRecordType, &record); err != nil {
			return err
		}
		task := decoder.graph.Tasks[record.TaskID]
		if task == nil || record.Ordinal != len(task.Messages) {
			return fmt.Errorf("%s:%d: invalid snapshot message task/ordinal %s/%d", decoder.path, line, record.TaskID, record.Ordinal)
		}
		if decoder.lastMessageTask != "" && record.TaskID < decoder.lastMessageTask {
			return fmt.Errorf("%s:%d: snapshot messages are not ordered by task ID", decoder.path, line)
		}
		decoder.lastMessageTask = record.TaskID
		if err := validateMessageKind(record.Kind); err != nil {
			return fmt.Errorf("%s:%d: snapshot message for %s: %w", decoder.path, line, record.TaskID, err)
		}
		createdAt, err := parseTime(record.CreatedAt)
		if err != nil {
			return fmt.Errorf("%s:%d: snapshot message for %s has invalid created_at: %w", decoder.path, line, record.TaskID, err)
		}
		task.Messages = append(task.Messages, Message{Kind: record.Kind, Text: record.Text, CreatedAt: createdAt})
	default:
		var record snapshotDependencyRecord
		if err := decodeSnapshotRecord(decoder.path, line, raw, snapshotDependencyRecordType, &record); err != nil {
			return err
		}
		if decoder.graph.Deps[record.FromID] == nil {
			decoder.graph.Deps[record.FromID] = map[string]struct{}{}
		}
		if decoder.lastDependencyFrom != "" &&
			(record.FromID < decoder.lastDependencyFrom ||
				(record.FromID == decoder.lastDependencyFrom && record.ToID <= decoder.lastDependencyTo)) {
			return fmt.Errorf("%s:%d: snapshot dependencies are not in increasing endpoint order", decoder.path, line)
		}
		decoder.lastDependencyFrom, decoder.lastDependencyTo = record.FromID, record.ToID
		if _, duplicate := decoder.graph.Deps[record.FromID][record.ToID]; duplicate {
			return fmt.Errorf("%s:%d: duplicate snapshot dependency %s -> %s", decoder.path, line, record.FromID, record.ToID)
		}
		decoder.graph.Deps[record.FromID][record.ToID] = struct{}{}
	}
	decoder.seen++
	return nil
}

var zeroTime time.Time

func decodeSnapshotRecord(path string, line int, raw []byte, want string, destination any) error {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return fmt.Errorf("%s:%d: invalid snapshot record: %w", path, line, err)
	}
	if header.Type != want {
		return fmt.Errorf("%s:%d: snapshot record order: got %q, want %q", path, line, header.Type, want)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return fmt.Errorf("%s:%d: invalid %s record: %w", path, line, want, err)
	}
	return nil
}

func (decoder *snapshotBlockDecoder) finish() (*Graph, error) {
	sum := sha256.Sum256(decoder.hash.Bytes())
	if got := hex.EncodeToString(sum[:]); got != decoder.manifest.SHA256 {
		return nil, fmt.Errorf("%s:%d: snapshot integrity mismatch: got %s, want %s", decoder.path, decoder.line, got, decoder.manifest.SHA256)
	}
	for from, deps := range decoder.graph.Deps {
		for to := range deps {
			if err := validateDepSelf(from, to); err != nil {
				return nil, fmt.Errorf("%s:%d: snapshot dependency %s -> %s: %w", decoder.path, decoder.line, from, to, err)
			}
			if decoder.graph.Tasks[from] == nil || decoder.graph.Tasks[to] == nil {
				return nil, fmt.Errorf("%s:%d: dangling snapshot dependency %s -> %s", decoder.path, decoder.line, from, to)
			}
			if hasCycle(decoder.graph, from, to) {
				return nil, fmt.Errorf("%s:%d: snapshot dependency cycle at %s -> %s", decoder.path, decoder.line, from, to)
			}
		}
	}
	return replayEventsOnto(decoder.graph, nil)
}

func snapshotKind(kind string) bool {
	switch kind {
	case snapshotRecordType, snapshotTaskRecordType, snapshotResultRecordType,
		snapshotMessageRecordType, snapshotDependencyRecordType:
		return true
	default:
		return false
	}
}
