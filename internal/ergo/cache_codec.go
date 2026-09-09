// Purpose: Encode and decode the disposable backlog replay checkpoint.
// Exports: none; repository loading owns all cache use and publication.
// Invariants: backlog history remains authoritative; the cache stores raw
// pre-migration reducer state, is deterministic, bounded, and self-validating.
// Any cache-format problem is returned to the caller as a cache miss.
package ergo

import (
	"bufio"
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
	cacheFileName             = "cache.jsonl"
	cacheVersion              = 1
	cacheManifestRecordType   = "cache"
	cacheTaskRecordType       = "cache_task"
	cacheResultRecordType     = "cache_result"
	cacheMessageRecordType    = "cache_message"
	cacheDependencyRecordType = "cache_dependency"
	cacheTombstoneRecordType  = "cache_tombstone"
	cacheLegacyEpicRecordType = "cache_legacy_epic"
	cacheCommitRecordType     = "cache_commit"
)

type cacheSource struct {
	Name       string
	Identity   string
	Bytes      int64
	Lines      int
	Records    int
	ModifiedNS int64
}

type cacheManifest struct {
	Type          string `json:"type"`
	Version       int    `json:"version"`
	Source        string `json:"source"`
	SourceID      string `json:"source_id"`
	PrefixBytes   int64  `json:"prefix_bytes"`
	PrefixLines   int    `json:"prefix_lines"`
	PrefixRecords int    `json:"prefix_records"`
	ModifiedNS    int64  `json:"modified_ns"`
	Tasks         int    `json:"tasks"`
	Results       int    `json:"results"`
	Messages      int    `json:"messages"`
	Dependencies  int    `json:"dependencies"`
	Tombstones    int    `json:"tombstones"`
	LegacyEpics   int    `json:"legacy_epics"`
}

type cacheTaskRecord struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	UUID      string `json:"uuid"`
	EpicID    string `json:"epic_id"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ClaimedBy string `json:"claimed_by"`
	ClaimedAt string `json:"claimed_at"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type cacheResultRecord struct {
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

type cacheMessageRecord struct {
	Type      string `json:"type"`
	TaskID    string `json:"task_id"`
	Ordinal   int    `json:"ordinal"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type cacheDependencyRecord struct {
	Type   string `json:"type"`
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
}

type cacheTombstoneRecord struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	At      string `json:"at"`
}

type cacheLegacyEpicRecord struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type cacheCommitRecord struct {
	Type   string `json:"type"`
	SHA256 string `json:"sha256"`
}

type decodedCache struct {
	graph  *Graph
	source cacheSource
}

func marshalCache(graph *Graph, source cacheSource) ([]byte, error) {
	if graph == nil {
		graph = newGraph()
	}
	manifest := cacheManifest{
		Type: cacheManifestRecordType, Version: cacheVersion,
		Source: source.Name, SourceID: source.Identity, PrefixBytes: source.Bytes,
		PrefixLines: source.Lines, PrefixRecords: source.Records, ModifiedNS: source.ModifiedNS,
		Tasks: len(graph.Tasks), Tombstones: len(graph.Tombstones), LegacyEpics: len(graph.legacyEmptyEpics),
	}
	for _, task := range graph.Tasks {
		manifest.Results += len(task.Results)
		manifest.Messages += len(task.Messages)
	}
	for _, deps := range graph.Deps {
		manifest.Dependencies += len(deps)
	}

	var output bytes.Buffer
	hash := sha256.New()
	write := func(record any) error {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if len(line) > maxLogRecordBytes {
			return fmt.Errorf("cache record is too long: %d bytes exceeds the %d-byte limit", len(line), maxLogRecordBytes)
		}
		hash.Write(line)
		hash.Write([]byte{'\n'})
		output.Write(line)
		output.WriteByte('\n')
		return nil
	}
	if err := write(manifest); err != nil {
		return nil, err
	}
	for _, task := range sortedTasks(graph.Tasks) {
		claimedAt := ""
		if !task.ClaimedAt.IsZero() {
			claimedAt = formatTime(task.ClaimedAt)
		}
		if err := write(cacheTaskRecord{
			Type: cacheTaskRecordType, ID: task.ID, UUID: task.UUID, EpicID: task.EpicID,
			State: task.State, Title: task.Title, Body: task.Body, ClaimedBy: task.ClaimedBy,
			ClaimedAt: claimedAt, CreatedAt: formatTime(task.CreatedAt), UpdatedAt: formatTime(task.UpdatedAt),
		}); err != nil {
			return nil, err
		}
		for ordinal, result := range task.Results {
			if err := write(cacheResultRecord{
				Type: cacheResultRecordType, TaskID: task.ID, Ordinal: ordinal,
				Summary: result.Summary, Path: result.Path, SHA256AtAttach: result.Sha256AtAttach,
				MtimeAtAttach: result.MtimeAtAttach, GitCommitAtAttach: result.GitCommitAtAttach,
				CreatedAt: formatTime(result.CreatedAt),
			}); err != nil {
				return nil, err
			}
		}
		for ordinal, message := range task.Messages {
			if err := write(cacheMessageRecord{
				Type: cacheMessageRecordType, TaskID: task.ID, Ordinal: ordinal,
				Kind: message.Kind, Text: message.Text, CreatedAt: formatTime(message.CreatedAt),
			}); err != nil {
				return nil, err
			}
		}
	}
	for _, from := range sortedMapKeys(graph.Deps) {
		for _, to := range sortedKeys(graph.Deps[from]) {
			if err := write(cacheDependencyRecord{Type: cacheDependencyRecordType, FromID: from, ToID: to}); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range sortedValueMapKeys(graph.Tombstones) {
		info := graph.Tombstones[id]
		if err := write(cacheTombstoneRecord{Type: cacheTombstoneRecordType, ID: id, AgentID: info.AgentID, At: formatTime(info.At)}); err != nil {
			return nil, err
		}
	}
	for _, id := range sortedValueMapKeys(graph.legacyEmptyEpics) {
		if err := write(cacheLegacyEpicRecord{Type: cacheLegacyEpicRecordType, ID: id}); err != nil {
			return nil, err
		}
	}
	footer, err := json.Marshal(cacheCommitRecord{Type: cacheCommitRecordType, SHA256: hex.EncodeToString(hash.Sum(nil))})
	if err != nil {
		return nil, err
	}
	output.Write(footer)
	output.WriteByte('\n')
	return output.Bytes(), nil
}

func decodeCache(reader io.Reader) (decodedCache, error) {
	tracked := &cacheNewlineReader{reader: reader}
	scanner := bufio.NewScanner(tracked)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogRecordBytes)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return decodedCache{}, err
		}
		return decodedCache{}, errors.New("empty cache")
	}
	first := append([]byte(nil), scanner.Bytes()...)
	var manifest cacheManifest
	if err := decodeCacheRecord(first, cacheManifestRecordType, &manifest); err != nil {
		return decodedCache{}, err
	}
	if manifest.Version != cacheVersion {
		return decodedCache{}, fmt.Errorf("unsupported cache version %d", manifest.Version)
	}
	if manifest.Source == "" || manifest.SourceID == "" || manifest.PrefixBytes < 0 || manifest.PrefixLines < 0 ||
		manifest.PrefixRecords < 0 || manifest.Tasks < 0 || manifest.Results < 0 || manifest.Messages < 0 ||
		manifest.Dependencies < 0 || manifest.Tombstones < 0 || manifest.LegacyEpics < 0 {
		return decodedCache{}, errors.New("invalid cache manifest")
	}

	hash := sha256.New()
	hash.Write(first)
	hash.Write([]byte{'\n'})
	graph := newGraph()
	counts := map[string]int{}
	last := map[string]string{}
	committed := false
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return decodedCache{}, fmt.Errorf("invalid cache record: %w", err)
		}
		if header.Type == cacheCommitRecordType {
			var commit cacheCommitRecord
			if err := decodeCacheRecord(raw, cacheCommitRecordType, &commit); err != nil {
				return decodedCache{}, err
			}
			if got := hex.EncodeToString(hash.Sum(nil)); got != commit.SHA256 {
				return decodedCache{}, errors.New("cache integrity mismatch")
			}
			committed = true
			if scanner.Scan() {
				return decodedCache{}, errors.New("cache has records after commit")
			}
			break
		}
		hash.Write(raw)
		hash.Write([]byte{'\n'})
		counts[header.Type]++
		switch header.Type {
		case cacheTaskRecordType:
			var record cacheTaskRecord
			if err := decodeCacheRecord(raw, header.Type, &record); err != nil {
				return decodedCache{}, err
			}
			if record.ID == "" || graph.Tasks[record.ID] != nil || (last[header.Type] != "" && record.ID <= last[header.Type]) {
				return decodedCache{}, errors.New("cache tasks are invalid or unordered")
			}
			claimedAt, err := parseOptionalCacheTime(record.ClaimedAt)
			if err != nil {
				return decodedCache{}, err
			}
			createdAt, err := parseTime(record.CreatedAt)
			if err != nil {
				return decodedCache{}, err
			}
			updatedAt, err := parseTime(record.UpdatedAt)
			if err != nil {
				return decodedCache{}, err
			}
			if !isReadableState(record.State) || (record.ClaimedBy == "") != claimedAt.IsZero() {
				return decodedCache{}, fmt.Errorf("cache task %s has invalid state or claim", record.ID)
			}
			graph.Tasks[record.ID] = &Task{ID: record.ID, UUID: record.UUID, EpicID: record.EpicID, State: record.State,
				Title: record.Title, Body: record.Body, ClaimedBy: record.ClaimedBy, ClaimedAt: claimedAt,
				CreatedAt: createdAt, UpdatedAt: updatedAt}
			last[header.Type] = record.ID
		case cacheResultRecordType:
			var record cacheResultRecord
			if err := decodeCacheRecord(raw, header.Type, &record); err != nil {
				return decodedCache{}, err
			}
			task := graph.Tasks[record.TaskID]
			if task == nil || record.Ordinal != len(task.Results) {
				return decodedCache{}, errors.New("invalid cache result order")
			}
			createdAt, err := parseTime(record.CreatedAt)
			if err != nil {
				return decodedCache{}, err
			}
			if err := validateResultSummary(record.Summary); err != nil {
				return decodedCache{}, err
			}
			task.Results = append(task.Results, Result{Summary: record.Summary, Path: record.Path,
				Sha256AtAttach: record.SHA256AtAttach, MtimeAtAttach: record.MtimeAtAttach,
				GitCommitAtAttach: record.GitCommitAtAttach, CreatedAt: createdAt})
		case cacheMessageRecordType:
			var record cacheMessageRecord
			if err := decodeCacheRecord(raw, header.Type, &record); err != nil {
				return decodedCache{}, err
			}
			task := graph.Tasks[record.TaskID]
			if task == nil || record.Ordinal != len(task.Messages) || validateMessageKind(record.Kind) != nil {
				return decodedCache{}, errors.New("invalid cache message order or kind")
			}
			createdAt, err := parseTime(record.CreatedAt)
			if err != nil {
				return decodedCache{}, err
			}
			task.Messages = append(task.Messages, Message{Kind: record.Kind, Text: record.Text, CreatedAt: createdAt})
		case cacheDependencyRecordType:
			var record cacheDependencyRecord
			if err := decodeCacheRecord(raw, header.Type, &record); err != nil {
				return decodedCache{}, err
			}
			key := record.FromID + "\x00" + record.ToID
			if last[header.Type] != "" && key <= last[header.Type] {
				return decodedCache{}, errors.New("cache dependencies are unordered")
			}
			if graph.Deps[record.FromID] == nil {
				graph.Deps[record.FromID] = map[string]struct{}{}
			}
			graph.Deps[record.FromID][record.ToID] = struct{}{}
			last[header.Type] = key
		case cacheTombstoneRecordType:
			var record cacheTombstoneRecord
			if err := decodeCacheRecord(raw, header.Type, &record); err != nil {
				return decodedCache{}, err
			}
			if record.ID == "" || graph.Tasks[record.ID] != nil || (last[header.Type] != "" && record.ID <= last[header.Type]) {
				return decodedCache{}, errors.New("cache tombstones are invalid or unordered")
			}
			at, err := parseTime(record.At)
			if err != nil {
				return decodedCache{}, err
			}
			graph.Tombstones[record.ID] = TombstoneInfo{AgentID: record.AgentID, At: at}
			last[header.Type] = record.ID
		case cacheLegacyEpicRecordType:
			var record cacheLegacyEpicRecord
			if err := decodeCacheRecord(raw, header.Type, &record); err != nil {
				return decodedCache{}, err
			}
			if graph.Tasks[record.ID] == nil || (last[header.Type] != "" && record.ID <= last[header.Type]) {
				return decodedCache{}, errors.New("cache legacy epics are invalid or unordered")
			}
			graph.legacyEmptyEpics[record.ID] = struct{}{}
			last[header.Type] = record.ID
		default:
			return decodedCache{}, fmt.Errorf("unknown cache record type %q", header.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return decodedCache{}, err
	}
	if tracked.last != '\n' {
		return decodedCache{}, errors.New("cache is not newline terminated")
	}
	if !committed {
		return decodedCache{}, errors.New("cache commit is missing")
	}
	want := map[string]int{
		cacheTaskRecordType: manifest.Tasks, cacheResultRecordType: manifest.Results,
		cacheMessageRecordType: manifest.Messages, cacheDependencyRecordType: manifest.Dependencies,
		cacheTombstoneRecordType: manifest.Tombstones, cacheLegacyEpicRecordType: manifest.LegacyEpics,
	}
	for kind, total := range want {
		if counts[kind] != total {
			return decodedCache{}, fmt.Errorf("cache %s count is %d, want %d", kind, counts[kind], total)
		}
	}
	if _, err := replayEventsOntoRaw(graph, nil); err != nil {
		return decodedCache{}, err
	}
	return decodedCache{graph: graph, source: cacheSource{Name: manifest.Source, Identity: manifest.SourceID,
		Bytes: manifest.PrefixBytes, Lines: manifest.PrefixLines, Records: manifest.PrefixRecords, ModifiedNS: manifest.ModifiedNS}}, nil
}

func decodeCacheRecord(raw []byte, want string, destination any) error {
	if err := decodeStrictJSON(raw, destination); err != nil {
		return fmt.Errorf("invalid %s record: %w", want, err)
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil || header.Type != want {
		return fmt.Errorf("got cache record %q, want %q", header.Type, want)
	}
	return nil
}

func decodeStrictJSON(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("cache record contains trailing data")
	}
	return nil
}

func parseOptionalCacheTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return parseTime(raw)
}

type cacheNewlineReader struct {
	reader io.Reader
	last   byte
}

func (reader *cacheNewlineReader) Read(buffer []byte) (int, error) {
	n, err := reader.reader.Read(buffer)
	if n > 0 {
		reader.last = buffer[n-1]
	}
	return n, err
}
