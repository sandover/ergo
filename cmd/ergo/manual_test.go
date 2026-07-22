// Purpose: Keep the embedded help and quickstart aligned with the public CLI.
// Exports: none.
// Role: Manual-surface coverage without freezing prose or incidental layout.
// Invariants: every public command and important flag is taught in both manuals.
// Invariants: removed v1/v2/v3 forms appear only as migration prose, never examples.
package main

import (
	"strings"
	"testing"

	"github.com/sandover/ergo/internal/ergo"
)

func TestManualSurfaceCoverage(t *testing.T) {
	manuals := map[string]string{
		"help":       ergo.UsageText(false),
		"quickstart": ergo.QuickstartText(false),
	}
	commands := []string{
		"init", "new task", "plan", "list", "show", "claim", "done", "block",
		"cancel", "release", "title", "body", "move", "sequence", "unsequence",
		"where", "prune", "compact", "quickstart", "version",
	}
	flags := []string{
		"--agent", "--dir", "--help", "--version", "--file", "--epic", "--ready",
		"--all", "-m", "--result", "--root", "--yes",
	}
	for name, manual := range manuals {
		for _, command := range commands {
			if !strings.Contains(manual, command) {
				t.Errorf("%s does not teach %q", name, command)
			}
		}
		for _, flag := range flags {
			if !strings.Contains(manual, flag) {
				t.Errorf("%s does not teach %q", name, flag)
			}
		}
		for _, removedExample := range []string{
			"$ ergo --json", "$ ergo sequence rm", "--summary <",
		} {
			if strings.Contains(manual, removedExample) {
				t.Errorf("%s contains removed example %q", name, removedExample)
			}
		}
	}
	t.Logf("rendered word counts: help=%d quickstart=%d",
		len(strings.Fields(manuals["help"])), len(strings.Fields(manuals["quickstart"])))
}

func TestManualExplainsInputAndOutputBoundaries(t *testing.T) {
	for name, manual := range map[string]string{
		"help":       ergo.UsageText(false),
		"quickstart": ergo.QuickstartText(false),
	} {
		for _, concept := range []string{
			"inline JSON", "JSONL", "piped stdin", "lifecycle", "YAML front matter",
			"No ready ergo tasks.", "legacy", "without migration",
		} {
			if !strings.Contains(manual, concept) {
				t.Errorf("%s does not explain %q", name, concept)
			}
		}
	}
}
