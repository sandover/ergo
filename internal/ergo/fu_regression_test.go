package ergo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderClaimNoReadyOutcome(t *testing.T) {
	var output bytes.Buffer
	RenderClaim(&output, ClaimOutcome{NoReady: true}, false)
	if got, want := output.String(), "No ready ergo tasks.\n"; got != want {
		t.Fatalf("RenderClaim() = %q, want %q", got, want)
	}
}

func TestApplicationSequenceRejectsRemovedRMForm(t *testing.T) {
	_, err := NewApplication(RepositoryOptions{}).Sequence(SequenceRequest{
		Command: "sequence", EventType: "link", IDs: []string{"rm", "A", "B"},
	})
	if err == nil || !strings.Contains(err.Error(), "use ergo unsequence") {
		t.Fatalf("Sequence() error = %v", err)
	}
	requireApplicationError(t, err, ErrorUsage)
}

func TestApplicationCreateEpicClassifiesContentAndFileErrors(t *testing.T) {
	app := NewApplication(RepositoryOptions{})

	missing := filepath.Join(t.TempDir(), "missing.md")
	_, err := app.CreateEpic(CreateEpicRequest{Title: "Epic", FilePath: missing})
	requireApplicationError(t, err, ErrorInternal)

	invalid := filepath.Join(t.TempDir(), "invalid.md")
	if err := os.WriteFile(invalid, []byte("not a task chunk"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = app.CreateEpic(CreateEpicRequest{Title: "Epic", FilePath: invalid})
	requireApplicationError(t, err, ErrorUsage)
}
