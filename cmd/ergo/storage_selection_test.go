package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandover/ergo/v6/internal/ergo"
)

func TestStorageSelectionCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	project, data := t.TempDir(), t.TempDir()
	run := func(args ...string) (string, string, int) {
		var out, stderr bytes.Buffer
		streams := Streams{In: strings.NewReader(""), Out: &out, Err: &stderr, StdinTerminal: true}
		// A supplied application project must survive CLI storage overrides.
		root := NewRootCommand(ergo.NewApplication(ergo.RepositoryOptions{StartDir: project}), streams, "test")
		code := runCommand(root, args, streams)
		return out.String(), stderr.String(), code
	}
	for _, args := range [][]string{
		{"--data-dir", data, "--user", "alice", "init"},
		{"new", "task", "private work", "--data-dir", data, "--user", "alice"},
	} {
		if _, stderr, code := run(args...); code != 0 {
			t.Fatalf("%v: %s", args, stderr)
		}
	}
	out, stderr, code := run("--user=alice", "--data-dir="+data, "list", "--json")
	if code != 0 || !strings.Contains(out, "private work") {
		t.Fatalf("list: %s %s", out, stderr)
	}
	out, stderr, code = run("--data-dir", data, "--user", "alice", "info")
	if code != 0 || !strings.Contains(out, "Owner: alice") || !strings.Contains(out, "Selection: --data-dir") {
		t.Fatalf("info: %s %s", out, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".ergo")); !os.IsNotExist(err) {
		t.Fatalf("source storage exists: %v", err)
	}
	for _, args := range [][]string{
		{"--data-dir", data, "--user", "bob", "list"},
		{"--data-dir", "", "init"},
		{"--user", "", "init"},
		{"--user", "../escape", "init"},
		{"--user", "alice", "--shared", "init"},
	} {
		if _, _, code := run(args...); code == 0 {
			t.Fatalf("accepted %v", args)
		}
	}
	// New global values must not obscure removed-command detection or hints.
	if _, stderr, _ := run("--user", "alice", "--data-dir", data, "plan"); !strings.Contains(stderr, "plan is not accepted") {
		t.Fatal(stderr)
	}
	if got := helpInvocation([]string{"--user", "alice", "--data-dir", data, "new", "task"}); got != "ergo new task" {
		t.Fatal(got)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "ergo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "ergo", "config.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"version"}, {"quickstart"}, {"list", "--help"}} {
		if _, stderr, code := run(args...); code != 0 {
			t.Fatalf("%v with bad config: %s", args, stderr)
		}
	}
	if _, _, code := run("list"); code == 0 {
		t.Fatal("invalid config was ignored")
	}
}
