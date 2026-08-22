package ergo

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRenderListJSONSchemaAndOrder(t *testing.T) {
	ready := &Task{ID: "TASK01", Title: "Ready task", State: stateTodo, EpicID: "EPIC01"}
	done := &Task{ID: "TASK02", Title: "Done task", State: stateDone, EpicID: "EPIC01"}
	failed := &Task{ID: "TASK03", Title: "Failed task", State: stateFailed, EpicID: "EPIC01"}
	epic := &Task{ID: "EPIC01", Title: "An epic"}
	outcome := ListOutcome{Graph: &Graph{Tasks: map[string]*Task{
		epic.ID: epic, ready.ID: ready, done.ID: done, failed.ID: failed,
	}}, Roots: []*treeNode{{
		task: epic, isEpic: true,
		children: []*treeNode{
			{task: ready, isReady: true},
			{task: done},
			{task: failed},
		},
	}}}

	var output bytes.Buffer
	if err := RenderListJSON(&output, outcome); err != nil {
		t.Fatalf("RenderListJSON: %v", err)
	}
	if !strings.HasSuffix(output.String(), "\n") {
		t.Fatalf("output is not newline-terminated: %q", output.String())
	}
	if strings.Contains(output.String(), "\x1b") {
		t.Fatalf("output contains ANSI: %q", output.String())
	}

	var document struct {
		Version int `json:"version"`
		Items   []struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			State  string `json:"state"`
			Ready  *bool  `json:"ready"`
			EpicID string `json:"epic_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if document.Version != 1 || len(document.Items) != 4 {
		t.Fatalf("document = %#v", document)
	}
	if got := []string{document.Items[0].ID, document.Items[1].ID, document.Items[2].ID, document.Items[3].ID}; strings.Join(got, ",") != "EPIC01,TASK01,TASK02,TASK03" {
		t.Fatalf("item order = %v", got)
	}
	if document.Items[0].Kind != "epic" || document.Items[0].State != "active" || document.Items[0].Ready != nil {
		t.Fatalf("epic fields = %#v", document.Items[0])
	}
	if document.Items[1].Kind != "task" || document.Items[1].State != stateTodo ||
		document.Items[1].Ready == nil || !*document.Items[1].Ready || document.Items[1].EpicID != "EPIC01" {
		t.Fatalf("ready task fields = %#v", document.Items[1])
	}
	if document.Items[2].Ready == nil || *document.Items[2].Ready {
		t.Fatalf("done task readiness = %#v", document.Items[2])
	}
	if document.Items[3].State != stateFailed || document.Items[3].Ready == nil || *document.Items[3].Ready {
		t.Fatalf("failed task fields = %#v", document.Items[3])
	}
}

func TestRenderListJSONDerivesEpicStateFromFullGraph(t *testing.T) {
	epic := &Task{ID: "EPIC01", Title: "An epic"}
	done := &Task{ID: "DONE01", Title: "Done", State: stateDone, EpicID: epic.ID}
	failed := &Task{ID: "FAIL01", Title: "Failed", State: stateFailed, EpicID: epic.ID}
	outcome := ListOutcome{
		Graph: &Graph{Tasks: map[string]*Task{epic.ID: epic, done.ID: done, failed.ID: failed}},
		Roots: []*treeNode{{task: epic, isEpic: true, children: []*treeNode{{task: done}}}},
	}

	var output bytes.Buffer
	if err := RenderListJSON(&output, outcome); err != nil {
		t.Fatalf("RenderListJSON: %v", err)
	}
	var document struct {
		Version int `json:"version"`
		Items   []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if document.Version != 1 || len(document.Items) != 2 || document.Items[0].State != stateFailed {
		t.Fatalf("document = %#v", document)
	}
}

func TestRenderListJSONPreservesDraftStateAndReadiness(t *testing.T) {
	draft := &Task{ID: "DRAFT01", Title: "Draft", State: stateDraft}
	graph := &Graph{Tasks: map[string]*Task{draft.ID: draft}, Deps: map[string]map[string]struct{}{}}
	graph.rebuildIndexes()
	outcome := ListOutcome{Graph: graph, Roots: []*treeNode{{task: draft}}, Options: ListOptions{}}
	var output strings.Builder
	if err := RenderListJSON(&output, outcome); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"state":"draft"`) || strings.Contains(output.String(), `"ready":true`) {
		t.Fatalf("draft JSON = %s", output.String())
	}
}

func TestRenderListJSONEmptyItemsAndWriterError(t *testing.T) {
	var output bytes.Buffer
	if err := RenderListJSON(&output, ListOutcome{}); err != nil {
		t.Fatalf("RenderListJSON: %v", err)
	}
	if output.String() != "{\"version\":1,\"items\":[]}\n" {
		t.Fatalf("empty output = %q", output.String())
	}

	if err := RenderListJSON(failingListJSONWriter{}, ListOutcome{}); err == nil {
		t.Fatal("writer failure was ignored")
	}
}

type failingListJSONWriter struct{}

func (failingListJSONWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
