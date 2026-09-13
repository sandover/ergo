// Purpose: Prove the disposable cache preserves raw reducer state exactly.
// Coverage: deterministic round trips, legacy evidence, tombstones, format
// versioning, strict structure, and whole-file integrity failures.
// Invariants: decoding never turns malformed cache bytes into usable state.
package ergo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCacheCodecRoundTripsRawReplayState(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	graph := newGraph()
	graph.Tasks["EPIC00"] = &Task{ID: "EPIC00", UUID: "epic-uuid", State: stateTodo, Title: "", Body: "# Heading\n\nRecovered title\nOriginal body", CreatedAt: now, UpdatedAt: now}
	graph.Tasks["TASK01"] = &Task{
		ID: "TASK01", UUID: "task-uuid", EpicID: "EPIC00", State: stateDoing,
		Title: "Child", Body: "body", ClaimedBy: "agent@host", ClaimedAt: now.Add(time.Minute),
		CreatedAt: now, UpdatedAt: now.Add(2 * time.Minute),
		Results:  []Result{{Summary: "Verified", Path: "docs/result.md", Sha256AtAttach: "abc", CreatedAt: now.Add(time.Minute)}},
		Messages: []Message{{Kind: "open", Text: "Retry", CreatedAt: now.Add(time.Minute)}},
	}
	graph.Tasks["TASK02"] = &Task{ID: "TASK02", UUID: "task-2", State: stateTodo, Title: "Root", CreatedAt: now, UpdatedAt: now}
	graph.Deps["TASK01"] = map[string]struct{}{"TASK02": {}}
	graph.Tombstones["OLD001"] = TombstoneInfo{AgentID: "prune@host", At: now.Add(3 * time.Minute)}
	graph.legacyEmptyEpics["EPIC00"] = struct{}{}
	source := backlogSource{Name: backlogFileName, Identity: "unix:1:2", Bytes: 1234, Lines: 18, Records: 17, ModifiedNS: now.UnixNano()}

	first, err := marshalCache(graph, source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := marshalCache(graph, source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("cache encoding is not deterministic")
	}
	decoded, err := decodeCache(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.source != source {
		t.Fatalf("source = %+v, want %+v", decoded.source, source)
	}
	if decoded.graph.Tasks["EPIC00"].Title != "" || decoded.graph.Tasks["EPIC00"].Body != graph.Tasks["EPIC00"].Body {
		t.Fatalf("legacy title state was finalized during cache round trip: %+v", decoded.graph.Tasks["EPIC00"])
	}
	if _, ok := decoded.graph.legacyEmptyEpics["EPIC00"]; !ok {
		t.Fatal("legacy explicit epic identity was lost after it gained a child")
	}
	if got := decoded.graph.Tasks["TASK01"]; got.ClaimedBy != "agent@host" || len(got.Results) != 1 || len(got.Messages) != 1 {
		t.Fatalf("task evidence or claim was lost: %+v", got)
	}
	if _, ok := decoded.graph.Tombstones["OLD001"]; !ok {
		t.Fatal("tombstone was lost")
	}
	if _, ok := decoded.graph.Deps["TASK01"]["TASK02"]; !ok {
		t.Fatal("dependency was lost")
	}
}

func TestCacheCodecRejectsInvalidFiles(t *testing.T) {
	graph := newGraph()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	graph.Tasks["TASK01"] = &Task{ID: "TASK01", State: stateTodo, Title: "Task", CreatedAt: now, UpdatedAt: now}
	data, err := marshalCache(graph, backlogSource{Name: backlogFileName, Identity: "unix:1:2", Bytes: 42, Lines: 1, Records: 1})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"empty":           nil,
		"missing payload": []byte("{}\n"),
		"missing newline": bytes.TrimSuffix(data, []byte("\n")),
		"bad checksum":    bytes.Replace(data, []byte(`"title":"Task"`), []byte(`"title":"Changed"`), 1),
		"bad version":     resignCache(t, data, []byte(`"version":2`), []byte(`"version":3`)),
		"unknown field":   resignCache(t, data, []byte(`"version":2`), []byte(`"version":2,"surprise":true`)),
		"unknown parent": rewriteCache(t, data, func(document *cacheDocument) {
			document.Tasks[0].EpicID = "MISSING"
		}),
		"dangling dependency": rewriteCache(t, data, func(document *cacheDocument) {
			document.Dependencies = []cacheDependency{{FromID: "TASK01", ToID: "MISSING"}}
		}),
		"suffix": append(append([]byte(nil), data...), []byte("{}\n")...),
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCache(bytes.NewReader(candidate)); err == nil {
				t.Fatalf("decode accepted invalid cache: %q", strings.TrimSpace(string(candidate)))
			}
		})
	}
}

func rewriteCache(t *testing.T, data []byte, mutate func(*cacheDocument)) []byte {
	t.Helper()
	var envelope cacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	var document cacheDocument
	if err := json.Unmarshal(envelope.Checkpoint, &document); err != nil {
		t.Fatal(err)
	}
	mutate(&document)
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	envelope.Checkpoint = payload
	envelope.SHA256 = hex.EncodeToString(sum[:])
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func resignCache(t *testing.T, data, old, replacement []byte) []byte {
	t.Helper()
	var envelope cacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Checkpoint = bytes.Replace(envelope.Checkpoint, old, replacement, 1)
	sum := sha256.Sum256(envelope.Checkpoint)
	envelope.SHA256 = hex.EncodeToString(sum[:])
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func TestRawReplayDefersLegacyTitleMigrationUntilTailApplied(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	created, err := newEvent(eventNewTask, now, NewTaskEvent{ID: "TASK01", State: stateTodo, Body: "Old title\nOld body", CreatedAt: formatTime(now)})
	if err != nil {
		t.Fatal(err)
	}
	body, err := newEvent(eventBody, now.Add(time.Minute), BodyUpdateEvent{ID: "TASK01", Body: "New title\nNew body", TS: formatTime(now.Add(time.Minute))})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := replayEventsOntoRaw(newGraph(), []Event{created})
	if err != nil {
		t.Fatal(err)
	}
	if raw.Tasks["TASK01"].Title != "" {
		t.Fatal("raw replay applied legacy title migration")
	}
	final, err := replayEventsOnto(raw, []Event{body})
	if err != nil {
		t.Fatal(err)
	}
	if final.Tasks["TASK01"].Title != "New title" || final.Tasks["TASK01"].Body != "New body" {
		t.Fatalf("tail did not determine legacy title: %+v", final.Tasks["TASK01"])
	}
}
