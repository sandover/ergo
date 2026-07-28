// Purpose: Prove current Ergo preserves backlogs emitted by every released major.
// Role: Immutable compatibility and compaction coverage independent of current constructors.
package ergo

import (
	"os"
	"path/filepath"
	"testing"
)

type releasedFixture struct {
	name        string
	file        string
	logName     string
	epicID      string
	firstID     string
	secondID    string
	secondState string
	secondClaim string
	resultTitle string
	messageText string
	blockText   string
}

var releasedFixtures = []releasedFixture{
	{
		name: "v1", file: "v1.0.0-events.jsonl", logName: oldEventsFileName,
		epicID: "RHMKMK", firstID: "TSDLBT", secondID: "S3BKFM",
		secondState: stateError, secondClaim: "fixture@v1.0.0", resultTitle: "go.mod",
	},
	{
		name: "v2", file: "v2.0.0-plans.jsonl", logName: plansFileName,
		epicID: "QZ43EH", firstID: "YDSBYY", secondID: "HAVLMC",
		secondState: stateBlocked, resultTitle: "v2.0.0 evidence",
	},
	{
		name: "v3", file: "v3.0.0-plans.jsonl", logName: plansFileName,
		epicID: "27P2GS", firstID: "RJZVUO", secondID: "KMLJS2",
		secondState: stateBlocked, resultTitle: "go.mod",
		messageText: "v3.0.0 completed", blockText: "v3.0.0 waiting",
	},
	{
		name: "v4", file: "v4.0.0-backlog.jsonl", logName: backlogFileName,
		epicID: "NX5XPB", firstID: "NNZXBA", secondID: "64EBIM",
		secondState: stateBlocked, resultTitle: "go.mod",
		messageText: "v4.0.0 completed", blockText: "v4.0.0 waiting",
	},
}

func TestCompatibilityReleasedBacklogs(t *testing.T) {
	for _, fixture := range releasedFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			ergoDir := t.TempDir()
			copyReleasedFixture(t, fixture.file, filepath.Join(ergoDir, fixture.logName))

			graph, err := loadGraph(ergoDir)
			if err != nil {
				t.Fatalf("load released fixture: %v", err)
			}
			assertReleasedFixture(t, graph, fixture)
		})
	}
}

func TestCompatibilityReleasedBacklogsCompactWithoutSemanticLoss(t *testing.T) {
	for _, fixture := range releasedFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			events, err := readEvents(filepath.Join("testdata", "compatibility", fixture.file))
			if err != nil {
				t.Fatalf("read released fixture: %v", err)
			}
			before, err := replayEvents(events)
			if err != nil {
				t.Fatalf("replay released fixture: %v", err)
			}
			compacted, err := compactEvents(before)
			if err != nil {
				t.Fatalf("compact released fixture: %v", err)
			}
			after, err := replayEvents(compacted)
			if err != nil {
				t.Fatalf("replay compacted fixture: %v", err)
			}
			assertGraphStateEqual(t, before, after)
		})
	}
}

func assertReleasedFixture(t *testing.T, graph *Graph, fixture releasedFixture) {
	t.Helper()
	if len(graph.Tasks) != 3 {
		t.Fatalf("task count = %d, want 3", len(graph.Tasks))
	}
	epic := graph.Tasks[fixture.epicID]
	first := graph.Tasks[fixture.firstID]
	second := graph.Tasks[fixture.secondID]
	if epic == nil || first == nil || second == nil {
		t.Fatalf("missing released tasks: epic=%v first=%v second=%v", epic != nil, first != nil, second != nil)
	}
	if !isContainer(epic, graph) {
		t.Fatalf("%s was not derived as an epic", fixture.epicID)
	}
	if first.EpicID != fixture.epicID || second.EpicID != fixture.epicID {
		t.Fatalf("children are not assigned to %s: first=%q second=%q", fixture.epicID, first.EpicID, second.EpicID)
	}
	if first.State != stateDone || len(first.Results) != 1 {
		t.Fatalf("first task state/results = %s/%d, want done/1", first.State, len(first.Results))
	}
	result := first.Results[0]
	if result.Path != "go.mod" || result.Summary != fixture.resultTitle || result.Sha256AtAttach == "" {
		t.Fatalf("first result = %#v", result)
	}
	if second.State != fixture.secondState || second.ClaimedBy != fixture.secondClaim {
		t.Fatalf("second state/claim = %s/%q, want %s/%q",
			second.State, second.ClaimedBy, fixture.secondState, fixture.secondClaim)
	}
	if len(second.Deps) != 1 || second.Deps[0] != fixture.firstID {
		t.Fatalf("second dependencies = %v, want [%s]", second.Deps, fixture.firstID)
	}
	if fixture.messageText == "" {
		if len(first.Messages) != 0 {
			t.Fatalf("unexpected messages in %s fixture: %#v", fixture.name, first.Messages)
		}
	} else if len(first.Messages) != 1 ||
		first.Messages[0].Kind != "done" ||
		first.Messages[0].Text != fixture.messageText {
		t.Fatalf("first messages = %#v", first.Messages)
	}
	if fixture.blockText == "" {
		if len(second.Messages) != 0 {
			t.Fatalf("unexpected second-task messages in %s fixture: %#v", fixture.name, second.Messages)
		}
	} else if len(second.Messages) != 1 ||
		second.Messages[0].Kind != "block" ||
		second.Messages[0].Text != fixture.blockText {
		t.Fatalf("second messages = %#v", second.Messages)
	}
}

func copyReleasedFixture(t *testing.T, sourceName, destination string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "compatibility", sourceName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0644); err != nil {
		t.Fatal(err)
	}
}
