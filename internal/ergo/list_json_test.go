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
	outcome := ListOutcome{Roots: []*treeNode{{
		task: &Task{ID: "EPIC01", Title: "An epic"}, isEpic: true,
		children: []*treeNode{
			{task: ready, isReady: true},
			{task: done},
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
	if document.Version != 1 || len(document.Items) != 3 {
		t.Fatalf("document = %#v", document)
	}
	if got := []string{document.Items[0].ID, document.Items[1].ID, document.Items[2].ID}; strings.Join(got, ",") != "EPIC01,TASK01,TASK02" {
		t.Fatalf("item order = %v", got)
	}
	if document.Items[0].Kind != "epic" || document.Items[0].State != "" || document.Items[0].Ready != nil {
		t.Fatalf("epic fields = %#v", document.Items[0])
	}
	if document.Items[1].Kind != "task" || document.Items[1].State != stateTodo ||
		document.Items[1].Ready == nil || !*document.Items[1].Ready || document.Items[1].EpicID != "EPIC01" {
		t.Fatalf("ready task fields = %#v", document.Items[1])
	}
	if document.Items[2].Ready == nil || *document.Items[2].Ready {
		t.Fatalf("done task readiness = %#v", document.Items[2])
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
