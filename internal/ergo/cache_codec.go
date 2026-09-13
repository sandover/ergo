// Purpose: Encode and decode the disposable backlog replay checkpoint.
// Exports: none; repository loading owns all cache use and publication.
// Invariants: backlog history remains authoritative; the cache stores raw
// pre-migration reducer state as one deterministic, checksummed document.
// Any cache-format problem is returned to the caller as a cache miss.
package ergo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	cacheFileName = "cache.json"
	cacheVersion  = 2
)

type cacheEnvelope struct {
	Checkpoint json.RawMessage `json:"checkpoint"`
	SHA256     string          `json:"sha256"`
}

type cacheDocument struct {
	Version      int               `json:"version"`
	Source       cacheSource       `json:"source"`
	Tasks        []cacheTask       `json:"tasks"`
	Dependencies []cacheDependency `json:"dependencies,omitempty"`
	Tombstones   []cacheTombstone  `json:"tombstones,omitempty"`
	LegacyEpics  []string          `json:"legacy_epics,omitempty"`
}

type cacheSource struct {
	Name       string `json:"name"`
	Identity   string `json:"identity"`
	Bytes      int64  `json:"bytes"`
	Lines      int    `json:"lines"`
	Records    int    `json:"records"`
	ModifiedNS int64  `json:"modified_ns"`
}

type cacheTask struct {
	ID        string         `json:"id"`
	UUID      string         `json:"uuid,omitempty"`
	EpicID    string         `json:"epic_id,omitempty"`
	State     string         `json:"state"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	ClaimedBy string         `json:"claimed_by,omitempty"`
	ClaimedNS int64          `json:"claimed_ns,omitempty"`
	CreatedNS int64          `json:"created_ns"`
	UpdatedNS int64          `json:"updated_ns"`
	Results   []cacheResult  `json:"results,omitempty"`
	Messages  []cacheMessage `json:"messages,omitempty"`
}

type cacheResult struct {
	Summary           string `json:"summary"`
	Path              string `json:"path,omitempty"`
	SHA256AtAttach    string `json:"sha256_at_attach,omitempty"`
	MtimeAtAttach     string `json:"mtime_at_attach,omitempty"`
	GitCommitAtAttach string `json:"git_commit_at_attach,omitempty"`
	CreatedNS         int64  `json:"created_ns"`
}

type cacheMessage struct {
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	CreatedNS int64  `json:"created_ns"`
}

type cacheDependency struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
}

type cacheTombstone struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id,omitempty"`
	AtNS    int64  `json:"at_ns"`
}

type cacheCheckpoint struct {
	graph  *Graph
	source backlogSource
}

func marshalCache(graph *Graph, source backlogSource) ([]byte, error) {
	if graph == nil {
		graph = newGraph()
	}
	document := cacheDocument{
		Version: cacheVersion,
		Source:  cacheSource(source),
		Tasks:   make([]cacheTask, 0, len(graph.Tasks)),
	}
	for _, task := range sortedTasks(graph.Tasks) {
		record := cacheTask{
			ID: task.ID, UUID: task.UUID, EpicID: task.EpicID, State: task.State,
			Title: task.Title, Body: task.Body, ClaimedBy: task.ClaimedBy,
			ClaimedNS: cacheTime(task.ClaimedAt), CreatedNS: cacheTime(task.CreatedAt), UpdatedNS: cacheTime(task.UpdatedAt),
			Results: make([]cacheResult, 0, len(task.Results)), Messages: make([]cacheMessage, 0, len(task.Messages)),
		}
		for _, result := range task.Results {
			record.Results = append(record.Results, cacheResult{
				Summary: result.Summary, Path: result.Path, SHA256AtAttach: result.Sha256AtAttach,
				MtimeAtAttach: result.MtimeAtAttach, GitCommitAtAttach: result.GitCommitAtAttach,
				CreatedNS: cacheTime(result.CreatedAt),
			})
		}
		for _, message := range task.Messages {
			record.Messages = append(record.Messages, cacheMessage{Kind: message.Kind, Text: message.Text, CreatedNS: cacheTime(message.CreatedAt)})
		}
		document.Tasks = append(document.Tasks, record)
	}
	for _, from := range sortedMapKeys(graph.Deps) {
		for _, to := range sortedKeys(graph.Deps[from]) {
			document.Dependencies = append(document.Dependencies, cacheDependency{FromID: from, ToID: to})
		}
	}
	for _, id := range sortedValueMapKeys(graph.Tombstones) {
		info := graph.Tombstones[id]
		document.Tombstones = append(document.Tombstones, cacheTombstone{ID: id, AgentID: info.AgentID, AtNS: cacheTime(info.At)})
	}
	document.LegacyEpics = sortedValueMapKeys(graph.legacyEmptyEpics)

	payload, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	encoded, err := json.Marshal(cacheEnvelope{Checkpoint: payload, SHA256: hex.EncodeToString(sum[:])})
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func decodeCache(reader io.Reader) (cacheCheckpoint, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return cacheCheckpoint{}, err
	}
	if len(data) == 0 {
		return cacheCheckpoint{}, errors.New("empty cache")
	}
	if data[len(data)-1] != '\n' {
		return cacheCheckpoint{}, errors.New("cache is not newline terminated")
	}
	var envelope cacheEnvelope
	if err := decodeStrictJSON(data, &envelope); err != nil {
		return cacheCheckpoint{}, fmt.Errorf("invalid cache envelope: %w", err)
	}
	if len(envelope.Checkpoint) == 0 || envelope.SHA256 == "" {
		return cacheCheckpoint{}, errors.New("incomplete cache envelope")
	}
	sum := sha256.Sum256(envelope.Checkpoint)
	if hex.EncodeToString(sum[:]) != envelope.SHA256 {
		return cacheCheckpoint{}, errors.New("cache integrity mismatch")
	}
	var document cacheDocument
	if err := decodeStrictJSON(envelope.Checkpoint, &document); err != nil {
		return cacheCheckpoint{}, fmt.Errorf("invalid cache checkpoint: %w", err)
	}
	if document.Version != cacheVersion {
		return cacheCheckpoint{}, fmt.Errorf("unsupported cache version %d", document.Version)
	}
	if document.Source.Name == "" || document.Source.Identity == "" || document.Source.Bytes < 0 ||
		document.Source.Lines < 0 || document.Source.Records < 0 {
		return cacheCheckpoint{}, errors.New("invalid cache source")
	}

	graph := newGraph()
	graph.Tasks = make(map[string]*Task, len(document.Tasks))
	graph.Deps = make(map[string]map[string]struct{}, len(document.Dependencies))
	graph.Tombstones = make(map[string]TombstoneInfo, len(document.Tombstones))
	graph.legacyEmptyEpics = make(map[string]struct{}, len(document.LegacyEpics))
	lastID := ""
	for _, record := range document.Tasks {
		if record.ID == "" || record.ID <= lastID || !isReadableState(record.State) ||
			(record.ClaimedBy == "") != (record.ClaimedNS == 0) || record.CreatedNS == 0 || record.UpdatedNS == 0 {
			return cacheCheckpoint{}, errors.New("cache tasks are invalid or unordered")
		}
		task := &Task{
			ID: record.ID, UUID: record.UUID, EpicID: record.EpicID, State: record.State,
			Title: record.Title, Body: record.Body, ClaimedBy: record.ClaimedBy,
			ClaimedAt: fromCacheTime(record.ClaimedNS), CreatedAt: fromCacheTime(record.CreatedNS), UpdatedAt: fromCacheTime(record.UpdatedNS),
			Results: make([]Result, 0, len(record.Results)), Messages: make([]Message, 0, len(record.Messages)),
		}
		for _, result := range record.Results {
			if result.CreatedNS == 0 || validateResultSummary(result.Summary) != nil {
				return cacheCheckpoint{}, fmt.Errorf("cache task %s has invalid result", record.ID)
			}
			task.Results = append(task.Results, Result{Summary: result.Summary, Path: result.Path,
				Sha256AtAttach: result.SHA256AtAttach, MtimeAtAttach: result.MtimeAtAttach,
				GitCommitAtAttach: result.GitCommitAtAttach, CreatedAt: fromCacheTime(result.CreatedNS)})
		}
		for _, message := range record.Messages {
			if message.CreatedNS == 0 || validateMessageKind(message.Kind) != nil {
				return cacheCheckpoint{}, fmt.Errorf("cache task %s has invalid message", record.ID)
			}
			task.Messages = append(task.Messages, Message{Kind: message.Kind, Text: message.Text, CreatedAt: fromCacheTime(message.CreatedNS)})
		}
		graph.Tasks[record.ID] = task
		lastID = record.ID
	}
	lastDependency := ""
	for _, record := range document.Dependencies {
		key := record.FromID + "\x00" + record.ToID
		if record.FromID == "" || record.ToID == "" || key <= lastDependency {
			return cacheCheckpoint{}, errors.New("cache dependencies are invalid or unordered")
		}
		if graph.Deps[record.FromID] == nil {
			graph.Deps[record.FromID] = make(map[string]struct{})
		}
		graph.Deps[record.FromID][record.ToID] = struct{}{}
		lastDependency = key
	}
	lastID = ""
	for _, record := range document.Tombstones {
		if record.ID == "" || record.ID <= lastID || record.AtNS == 0 || graph.Tasks[record.ID] != nil {
			return cacheCheckpoint{}, errors.New("cache tombstones are invalid or unordered")
		}
		graph.Tombstones[record.ID] = TombstoneInfo{AgentID: record.AgentID, At: fromCacheTime(record.AtNS)}
		lastID = record.ID
	}
	lastID = ""
	for _, id := range document.LegacyEpics {
		if id == "" || id <= lastID || graph.Tasks[id] == nil {
			return cacheCheckpoint{}, errors.New("cache legacy epics are invalid or unordered")
		}
		graph.legacyEmptyEpics[id] = struct{}{}
		lastID = id
	}
	if err := validateReplayInvariants(graph, nil, nil, nil, nil); err != nil {
		return cacheCheckpoint{}, err
	}
	source := backlogSource{Name: document.Source.Name, Identity: document.Source.Identity,
		Bytes: document.Source.Bytes, Lines: document.Source.Lines, Records: document.Source.Records, ModifiedNS: document.Source.ModifiedNS}
	return cacheCheckpoint{graph: graph, source: source}, nil
}

func decodeStrictJSON(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("cache contains trailing data")
	}
	return nil
}

func cacheTime(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}

func fromCacheTime(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}
