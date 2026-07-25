// Purpose: Verify the two-layer manual and generated command syntax/input help.
// Exports: none.
// Role: Protect root orientation, quickstart coverage, and terse command usage.
// Invariants: root help and quickstart are complete; command help adds no manual prose.
// Invariants: tests protect meaning and inventory without freezing prose snapshots.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sandover/ergo/internal/ergo"
	"github.com/spf13/cobra"
)

var publicCommandPaths = []string{
	"init", "new", "new task", "new epic", "list", "show", "claim", "done",
	"block", "cancel", "release", "title", "body", "move", "sequence",
	"unsequence", "where", "compact", "prune", "quickstart", "version",
}

func TestRootHelpIsTheFrontDoor(t *testing.T) {
	help := ergo.UsageText(false)
	normalized := strings.Join(strings.Fields(help), " ")
	for _, fact := range []string{
		"dependency-aware backlog", "Tasks are stateful", "ready when",
		"task with children is an epic", "WORKFLOW", "GLOBAL FLAGS",
		"ergo <command> --help", "ergo quickstart",
		"optional stdin sets its body", "optional stdin sets epic body",
	} {
		if !strings.Contains(normalized, fact) {
			t.Errorf("root help lacks %q", fact)
		}
	}
	assertOrdered(t, "root workflow", strings.SplitN(help, "COMMANDS", 2)[0],
		"ergo init", `ergo new task "Add login"`, "ergo list --ready",
		"ergo claim ABCDEF", "ergo done ABCDEF")

	for _, signature := range []string{
		"init [dir]", `new task "<title>"`, `new epic "<title>" --file <path>`,
		"list [--epic <id>]", "show <id>", "claim [<id>]", "done <id>",
		"block <id>", "cancel <id>", "release <id>", "title <id> <title>",
		"body <id>", "move <id> <epic-id>", "sequence <A> <B>",
		"unsequence <A> <B>", "where", "prune [--yes]", "compact",
		"quickstart", "version",
	} {
		if !strings.Contains(help, "\n  "+signature) {
			t.Errorf("root command table lacks exact entry %q", signature)
		}
	}
}

func TestRegisteredCommandInventoryIsExact(t *testing.T) {
	got := registeredCommandPaths(rootCmd)
	want := append([]string(nil), publicCommandPaths...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registered commands:\n got %v\nwant %v", got, want)
	}
	for _, removed := range []string{"plan", "set", "reopen"} {
		if containsExact(got, removed) {
			t.Errorf("removed command %q remains registered", removed)
		}
	}
}

func TestEveryPublicCommandHasSyntaxAndInputOnlyHelp(t *testing.T) {
	rootHelp := ergo.UsageText(false)
	for _, path := range publicCommandPaths {
		command := findCommand(t, path)
		help := renderCommandHelp(t, command)
		if help == rootHelp {
			t.Errorf("%s repeats root help", path)
		}
		for _, required := range []string{"Usage:", "Flags:"} {
			if !strings.Contains(help, required) {
				t.Errorf("%s help lacks %q", path, required)
			}
		}
		if !strings.HasPrefix(help, "Usage:") {
			t.Errorf("%s help starts with prose: %q", path, help)
		}
		if strings.Contains(help, "Examples:") || command.Long != "" || command.Example != "" {
			t.Errorf("%s contains subcommand documentation", path)
		}
	}
}

func TestRootAndQuickstartCoverThePublicContract(t *testing.T) {
	combined := ergo.UsageText(false) + "\n" + ergo.QuickstartText(false)
	normalized := strings.Join(strings.Fields(combined), " ")
	for _, flag := range []string{
		"--agent", "--dir", "--help", "--version", "--file", "--epic",
		"--ready", "--all", "-m", "--result", "--root", "--yes",
	} {
		if !strings.Contains(combined, flag) {
			t.Errorf("documentation system lacks flag %q", flag)
		}
	}
	for _, boundary := range []string{
		"prints only the task ID", "Optional piped stdin", "do not read stdin",
		"Every lifecycle command clears the claim", "failed chain writes no partial",
		"concurrent agents cannot claim the same task",
	} {
		if !strings.Contains(normalized, boundary) {
			t.Errorf("documentation system lacks boundary %q", boundary)
		}
	}
}

func TestReaderJourneyHasNoDeadEnd(t *testing.T) {
	root := ergo.UsageText(false)
	if !strings.Contains(root, `new task "<title>"`) ||
		!strings.Contains(root, "ergo quickstart") {
		t.Fatal("root help does not orient a fresh reader toward the manual")
	}
	taskHelp := renderCommandHelp(t, newTaskCmd)
	if !strings.Contains(taskHelp, `ergo new task "<title>"`) ||
		!strings.Contains(taskHelp, "--epic") ||
		!strings.Contains(taskHelp, "Optional piped stdin becomes the initial task body") {
		t.Errorf("new task help lacks its input contract: %s", taskHelp)
	}
	quickstart := ergo.QuickstartText(false)
	for _, fact := range []string{
		"BACKLOG MODEL", "waiting, not blocked", "CLAIM AND RESUME",
		"DEPENDENCIES", "Piped stdin becomes", "prints only the task ID",
	} {
		if !strings.Contains(quickstart, fact) {
			t.Errorf("quickstart lacks cross-command fact %q", fact)
		}
	}
	err := removedArgumentError([]string{"done", "ABCDEF", "--summary", "note"})
	if err == nil || !strings.Contains(err.Error(), "use -m <message>") {
		t.Fatalf("deliberate error is not recoverable: %v", err)
	}
	if !strings.Contains(renderCommandHelp(t, findCommand(t, "done")), "-m, --message") {
		t.Fatal("done help does not let the reader continue after the error")
	}
}

func TestCommandsWithStdinInputsRevealThemInHelp(t *testing.T) {
	expected := map[string]string{
		"new task": "Optional piped stdin becomes the initial task body; no pipe creates an empty body.",
		"new epic": "Optional piped stdin becomes the epic body; --file supplies the child tasks.",
		"body":     "Piped stdin is required and replaces the body; an empty pipe clears it.",
	}
	for path, input := range expected {
		help := renderCommandHelp(t, findCommand(t, path))
		if !strings.Contains(help, "\nInput:\n  "+input+"\n") {
			t.Errorf("%s help lacks exact stdin contract:\n%s", path, help)
		}
	}
}

func TestCommandHelpRevealsOptionConstraints(t *testing.T) {
	checks := map[string][]string{
		"new epic": {`ergo new epic "<title>" --file <path>`},
		"list":     {"--ready", "conflicts with --all", "--all", "conflicts with --ready"},
		"claim":    {"--agent", "required"},
		"done":     {"-m, --message", "repeatable", "--result"},
		"move":     {"ergo move <id> <epic-id> | ergo move <id> --root"},
		"prune":    {"--yes", "default is dry-run"},
	}
	for path, facts := range checks {
		help := renderCommandHelp(t, findCommand(t, path))
		for _, fact := range facts {
			if !strings.Contains(help, fact) {
				t.Errorf("%s help lacks option constraint %q:\n%s", path, fact, help)
			}
		}
	}
}

func TestAgentFlagBelongsOnlyToClaim(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("agent") != nil {
		t.Fatal("--agent remains a global flag")
	}
	if claimCmd.Flags().Lookup("agent") == nil {
		t.Fatal("claim lacks --agent")
	}
	for _, path := range publicCommandPaths {
		if path == "claim" {
			continue
		}
		if strings.Contains(renderCommandHelp(t, findCommand(t, path)), "--agent") {
			t.Errorf("%s help exposes --agent", path)
		}
	}
}

func TestMaintainedSurfacesUseCurrentVocabularyAndSyntax(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{
		"README.md", "internal/ergo/help.txt", "internal/ergo/quickstart.txt",
		"skills/ergo-feature-planning/SKILL.md", "docs/spec.md",
		"docs/release.md", "docs/suggested-hooks/pre-commit",
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, removed := range []string{
			"ergo new task '{", "ergo new epic --file", "ergo plan ",
			"main.version=3.0.0", "ergo-v3-candidate",
		} {
			if strings.Contains(text, removed) {
				t.Errorf("%s contains removed form %q", name, removed)
			}
		}
		if strings.Contains(text, "container") {
			t.Errorf("%s uses internal container vocabulary", name)
		}
	}
}

func registeredCommandPaths(command *cobra.Command) []string {
	var paths []string
	for _, child := range command.Commands() {
		if child.Hidden {
			continue
		}
		paths = append(paths, strings.TrimPrefix(child.CommandPath(), "ergo "))
		paths = append(paths, registeredCommandPaths(child)...)
	}
	return paths
}

func findCommand(t *testing.T, path string) *cobra.Command {
	t.Helper()
	command := rootCmd
	for _, part := range strings.Fields(path) {
		next, _, err := command.Find([]string{part})
		if err != nil || next == command {
			t.Fatalf("find command %q: %v", path, err)
		}
		command = next
	}
	return command
}

func renderCommandHelp(t *testing.T, command *cobra.Command) string {
	t.Helper()
	var output bytes.Buffer
	previousOut := command.OutOrStdout()
	previousErr := command.ErrOrStderr()
	command.SetOut(&output)
	command.SetErr(&output)
	t.Cleanup(func() {
		command.SetOut(previousOut)
		command.SetErr(previousErr)
	})
	if err := command.Help(); err != nil {
		t.Fatalf("%s help: %v", command.CommandPath(), err)
	}
	return output.String()
}

func assertOrdered(t *testing.T, name, text string, values ...string) {
	t.Helper()
	previous := -1
	for _, value := range values {
		index := strings.Index(text, value)
		if index < 0 || index <= previous {
			t.Errorf("%s does not contain %q in order", name, value)
		}
		previous = index
	}
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
