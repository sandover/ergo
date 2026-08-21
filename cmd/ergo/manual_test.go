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

	"github.com/sandover/ergo/v4/internal/ergo"
	"github.com/spf13/cobra"
)

var publicCommandPaths = []string{
	"init", "new", "new task", "new epic", "list", "show", "claim", "done",
	"fail", "block", "cancel", "release", "result", "title", "body", "move", "sequence",
	"unsequence", "where", "info", "compact", "prune", "quickstart", "version",
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
		"list [--epic <id>] [--ready | --all] [--json]", "show <id> [--body]", "claim [<id>]", "done <id>",
		"block <id>", "cancel <id>", "release <id>", `result <id> "<text>"`, "title <id> <title>",
		"body <id> [--append]", "move <id> <epic-id>", "sequence <A> <B>",
		"unsequence <A> <B>", "where", "prune [--yes]", "compact",
		"quickstart", "version",
	} {
		if !strings.Contains(help, "\n  "+signature) {
			t.Errorf("root command table lacks exact entry %q", signature)
		}
	}
}

func TestRegisteredCommandInventoryIsExact(t *testing.T) {
	root := newManualTestRoot()
	got := registeredCommandPaths(root)
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
	root := newManualTestRoot()
	rootHelp := ergo.UsageText(false)
	for _, path := range publicCommandPaths {
		command := findCommand(t, root, path)
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
		"--agent", "--dir", "--color", "--help", "--version", "--file", "--epic",
		"--ready", "--all", "--json", "--body", "--append", "-m", "--root", "--yes",
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

func TestColorDocumentationHasClearOwnership(t *testing.T) {
	rootHelp := ergo.UsageText(false)
	if !strings.Contains(rootHelp, "--color <mode>") {
		t.Fatal("root help does not expose the color flag")
	}
	for _, detail := range []string{"NO_COLOR", "TERM=dumb", "ANSI", "redirect"} {
		if strings.Contains(rootHelp, detail) {
			t.Errorf("root help owns detailed color guidance %q", detail)
		}
	}

	root := newManualTestRoot()
	for _, path := range []string{"list", "show", "claim"} {
		help := renderCommandHelp(t, findCommand(t, root, path))
		for _, fact := range []string{"--color mode", "auto, always, or never", "(default auto)"} {
			if !strings.Contains(help, fact) {
				t.Errorf("%s help lacks generated color option fact %q:\n%s", path, fact, help)
			}
		}
	}

	quickstart := strings.Join(strings.Fields(ergo.QuickstartText(false)), " ")
	for _, concept := range []string{
		"--color=auto", "stdout is a terminal", "redirected or piped output plain",
		"NO_COLOR", "TERM=dumb", "--color=always", "--color=never",
		"override terminal detection", "removing Ergo's ANSI decoration",
		"User-authored bodies are never decorated", "without adding ANSI decoration",
	} {
		if !strings.Contains(quickstart, concept) {
			t.Errorf("quickstart lacks color concept %q", concept)
		}
	}

	repositoryRoot := filepath.Join("..", "..")
	surfaces := map[string][]string{
		"README.md": {"--color=always", "--color=never", "NO_COLOR", "TERM=dumb"},
		"docs/spec.md": {"Color is presentation metadata", "stdout is a terminal",
			"Removing Ergo-added ANSI decoration", "without adding ANSI decoration"},
		"CHANGELOG.md": {"easier to scan", "--color=auto|always|never",
			"keeps pipes and redirects plain", "exact, undecorated body projection"},
	}
	for name, concepts := range surfaces {
		data, err := os.ReadFile(filepath.Join(repositoryRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(string(data)), " ")
		for _, concept := range concepts {
			if !strings.Contains(text, concept) {
				t.Errorf("%s lacks color concept %q", name, concept)
			}
		}
	}
}

func TestReaderJourneyHasNoDeadEnd(t *testing.T) {
	commandRoot := newManualTestRoot()
	root := ergo.UsageText(false)
	if !strings.Contains(root, `new task "<title>"`) ||
		!strings.Contains(root, "ergo quickstart") {
		t.Fatal("root help does not orient a fresh reader toward the manual")
	}
	taskHelp := renderCommandHelp(t, findCommand(t, commandRoot, "new task"))
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
	if !strings.Contains(renderCommandHelp(t, findCommand(t, commandRoot, "done")), "-m, --message") {
		t.Fatal("done help does not let the reader continue after the error")
	}
}

func TestCommandsWithStdinInputsRevealThemInHelp(t *testing.T) {
	root := newManualTestRoot()
	expected := map[string]string{
		"new task": "Optional piped stdin becomes the initial task body; no pipe creates an empty body.",
		"new epic": "Optional piped stdin becomes the epic body; --file supplies the child tasks.",
		"body":     "Piped stdin is required. By default it replaces the body; --append adds literal bytes, and empty append input is a no-op.",
	}
	for path, input := range expected {
		help := renderCommandHelp(t, findCommand(t, root, path))
		if !strings.Contains(help, "\nInput:\n  "+input+"\n") {
			t.Errorf("%s help lacks exact stdin contract:\n%s", path, help)
		}
	}
}

func TestCommandHelpRevealsOptionConstraints(t *testing.T) {
	root := newManualTestRoot()
	checks := map[string][]string{
		"new epic": {`ergo new epic "<title>" --file <path>`},
		"list":     {"--ready", "conflicts with --all", "--all", "conflicts with --ready", "--json", "versioned JSON task listing"},
		"claim":    {"--agent", "required"},
		"show":     {"--body", "exact stored body", "byte-for-byte"},
		"body":     {"--append", "Append stdin bytes to the existing body"},
		"done":     {"-m, --message", "repeatable"},
		"result":   {`ergo result <id> "<text>"`, "--file", "project-relative"},
		"move":     {"ergo move <id> <epic-id> | ergo move <id> --root"},
		"prune":    {"--yes", "default is dry-run"},
	}
	for path, facts := range checks {
		help := renderCommandHelp(t, findCommand(t, root, path))
		for _, fact := range facts {
			if !strings.Contains(help, fact) {
				t.Errorf("%s help lacks option constraint %q:\n%s", path, fact, help)
			}
		}
	}
}

func TestShowBodyDocumentationHasClearOwnership(t *testing.T) {
	rootHelp := ergo.UsageText(false)
	if !strings.Contains(rootHelp, "show <id> [--body]") {
		t.Fatal("root command inventory does not expose the body projection")
	}
	for _, detail := range []string{"trailing newline", "temporary file", "synthesized metadata"} {
		if strings.Contains(rootHelp, detail) {
			t.Errorf("root help owns detailed show semantics %q", detail)
		}
	}

	commandHelp := renderCommandHelp(t, findCommand(t, newManualTestRoot(), "show"))
	for _, fact := range []string{"--body", "exact stored body", "byte-for-byte"} {
		if !strings.Contains(commandHelp, fact) {
			t.Errorf("generated show help lacks option fact %q:\n%s", fact, commandHelp)
		}
	}
	if strings.Contains(commandHelp, "mktemp") {
		t.Fatal("generated show help contains operating-guide prose")
	}

	quickstart := ergo.QuickstartText(false)
	normalized := strings.Join(strings.Fields(quickstart), " ")
	for _, concept := range []string{
		"leaf", "epic", "literal stored body", "empty body produces no output",
		"without a final newline", "trailing newlines remain intact",
	} {
		if !strings.Contains(normalized, concept) {
			t.Errorf("quickstart lacks body-projection concept %q", concept)
		}
	}
	assertOrdered(t, "safe body edit", quickstart,
		"tmp=$(mktemp) || exit", `ergo show ABCDEF --body >"$tmp" || exit`,
		`${EDITOR:-vi} "$tmp" || exit`, `ergo body ABCDEF <"$tmp"`)

	repositoryRoot := filepath.Join("..", "..")
	surfaces := map[string][]string{
		"README.md": {"lossless body edit", "ergo show ABCDEF --body", `ergo body ABCDEF <"$tmp"`},
		"docs/spec.md": {"synthesized document", "projects only the stored body",
			"byte-preserving", "emits zero bytes for an empty body"},
		"CHANGELOG.md": {"show <id> --body", "lossless read-edit-write"},
	}
	for name, concepts := range surfaces {
		data, err := os.ReadFile(filepath.Join(repositoryRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(string(data)), " ")
		for _, concept := range concepts {
			if !strings.Contains(text, concept) {
				t.Errorf("%s lacks body-projection concept %q", name, concept)
			}
		}
	}
}

func TestAgentFlagBelongsOnlyToClaim(t *testing.T) {
	root := newManualTestRoot()
	if root.PersistentFlags().Lookup("agent") != nil {
		t.Fatal("--agent remains a global flag")
	}
	if findCommand(t, root, "claim").Flags().Lookup("agent") == nil {
		t.Fatal("claim lacks --agent")
	}
	for _, path := range publicCommandPaths {
		if path == "claim" {
			continue
		}
		if strings.Contains(renderCommandHelp(t, findCommand(t, root, path)), "--agent") {
			t.Errorf("%s help exposes --agent", path)
		}
	}
}

func TestMaintainedSurfacesUseCurrentVocabularyAndSyntax(t *testing.T) {
	root := filepath.Join("..", "..")
	files := []string{
		"README.md", "internal/ergo/help.txt", "internal/ergo/quickstart.txt",
		"skills/ergo-backlog-planning/SKILL.md", "docs/spec.md",
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

func newManualTestRoot() *cobra.Command {
	return NewRootCommand(
		ergo.NewApplication(ergo.RepositoryOptions{}),
		Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, StdinTerminal: true, Width: 80},
		"test",
	)
}

func findCommand(t *testing.T, root *cobra.Command, path string) *cobra.Command {
	t.Helper()
	command := root
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
