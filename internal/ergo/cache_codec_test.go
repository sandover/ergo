// Purpose: Prove the disposable cache preserves raw reducer state exactly.
// Coverage: deterministic round trips, legacy evidence, tombstones, format
// versioning, record bounds, and whole-file integrity failures.
// Invariants: decoding never turns malformed cache bytes into usable state.
package ergo

import (
	"bytes"
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
	source := cacheSource{Name: backlogFileName, Identity: "unix:1:2", Bytes: 1234, Lines: 18, Records: 17, ModifiedNS: now.UnixNano()}

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
	data, err := marshalCache(graph, cacheSource{Name: backlogFileName, Identity: "unix:1:2", Bytes: 42, Lines: 1, Records: 1})
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(data, []byte("\n"))

	tests := map[string][]byte{
		"empty":           nil,
		"missing commit":  bytes.Join(lines[:len(lines)-2], []byte("\n")),
		"missing newline": bytes.TrimSuffix(data, []byte("\n")),
		"bad checksum":    bytes.Replace(data, []byte(`"title":"Task"`), []byte(`"title":"Changed"`), 1),
		"bad version":     bytes.Replace(data, []byte(`"version":1`), []byte(`"version":2`), 1),
		"unknown field":   bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"surprise":true`), 1),
		"suffix":          append(append([]byte(nil), data...), []byte("{}\n")...),
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeCache(bytes.NewReader(candidate)); err == nil {
				t.Fatalf("decode accepted invalid cache: %q", strings.TrimSpace(string(candidate)))
			}
		})
	}
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
