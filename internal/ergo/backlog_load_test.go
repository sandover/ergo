// Purpose: Guard the ownership and parsing boundaries of backlog loading.
// Coverage: full/tail repair equivalence and conditional raw-state retention.
// Invariants: cache presence preserves authoritative errors and repair metadata;
// final migration never changes the optional checkpoint retained for publishing.
package ergo

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestBacklogFullAndTailParsingAgree(t *testing.T) {
	event, err := marshalTransaction([]Event{cacheTestNewTask(t, "TASK02", "Second")})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]string{
		"empty":                   "",
		"record":                  string(event),
		"blank-lines":             "\n \t\n" + string(event) + "\n",
		"crlf":                    strings.ReplaceAll(string(event), "\n", "\r\n"),
		"unterminated-record":     strings.TrimSuffix(string(event), "\n"),
		"partial-json":            string(event) + `{"type":"transaction"`,
		"crlf-partial-json":       strings.ReplaceAll(string(event), "\n", "\r\n") + `{"type":"transaction"`,
		"terminated-bad-json":     "{broken\n",
		"invalid-transaction":     `{"type":"transaction","version":99,"events":[]}`,
		"snapshot-in-tail":        "{\"type\":\"snapshot\",\"version\":1}\n",
		"unterminated-whitespace": "  ",
	}
	for name, tail := range tests {
		t.Run(name, func(t *testing.T) {
			_, r := cacheTestRepository(t, []Event{cacheTestNewTask(t, "TASK01", "First")})
			writeCacheForRepository(t, r)
			f, err := os.OpenFile(r.eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tail); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			got, loaded, gotErr := r.loadWithRead()
			full, fullErr := inspectEventLog(r.eventsPath)
			if fullErr == nil {
				_, fullErr = replayEvents(full.events)
			}
			if gotErr != nil || fullErr != nil {
				if gotErr == nil || fullErr == nil || gotErr.Error() != fullErr.Error() {
					t.Fatalf("cached error %v; full error %v", gotErr, fullErr)
				}
				return
			}
			if !loaded.cacheHit {
				t.Fatal("valid tail unexpectedly fell back")
			}
			want, err := replayEvents(full.events)
			if err != nil {
				t.Fatal(err)
			}
			assertGraphStateEqual(t, want, got)
			actual := loaded.log
			if actual.validBytes != full.validBytes || actual.needsSeparator != full.needsSeparator || actual.truncatedTail != full.truncatedTail || actual.lineCount != full.lineCount || actual.recordCount != full.recordCount {
				t.Fatalf("cached repair metadata %+v; full %+v", actual, full)
			}
			if len(actual.events) != len(full.events)-1 || (len(actual.events) > 0 && !reflect.DeepEqual(actual.events, full.events[1:])) {
				t.Fatal("tail event provenance differs from full scan")
			}
			if (full.truncatedTail || full.needsSeparator) && loaded.checkpoint != nil {
				t.Fatal("unterminated tail retained a publication candidate")
			}
		})
	}
}

func TestBacklogFinalizationRetainsRawStateOnlyForRefresh(t *testing.T) {
	source := backlogSource{Identity: "test", Bytes: cacheCreateBytes, Lines: 1, Records: 1}
	for _, test := range []struct {
		name   string
		source backlogSource
		base   *backlogSource
		retain bool
	}{
		{"create", source, nil, true},
		{"small", backlogSource{Identity: "test", Bytes: 10}, nil, false},
		{"unchanged", source, &source, false},
		{"ineligible", backlogSource{}, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := cacheTestNewTask(t, "TASK01", "")
			raw, err := replayEventsOntoRaw(newGraph(), []Event{event})
			if err != nil {
				t.Fatal(err)
			}
			raw.Tasks["TASK01"].Body = "Derived title\nOriginal body"
			raw.Tombstones["OLD001"] = TombstoneInfo{}
			graph, loaded := finishBacklogLoad(raw, eventLogRead{source: test.source}, test.base)
			if graph != raw {
				t.Fatal("command did not retain ownership of the loaded graph")
			}
			if graph.Tasks["TASK01"].Title != "Derived title" || graph.Tasks["TASK01"].Body != "Original body" {
				t.Fatal("legacy migration did not finalize command graph")
			}
			if (loaded.checkpoint != nil) != test.retain {
				t.Fatalf("checkpoint retained=%v, want %v", loaded.checkpoint != nil, test.retain)
			}
			if loaded.checkpoint != nil {
				saved := loaded.checkpoint.graph
				if saved == graph || saved.Tasks["TASK01"].Title != "" || saved.Tasks["TASK01"].Body != "Derived title\nOriginal body" {
					t.Fatal("checkpoint was finalized or aliases command state")
				}
				graph.Tasks["TASK01"].Body = "changed"
				delete(graph.Tombstones, "OLD001")
				if saved.Tasks["TASK01"].Body != "Derived title\nOriginal body" || len(saved.Tombstones) != 1 {
					t.Fatal("command mutation leaked into checkpoint")
				}
			}
		})
	}
}
