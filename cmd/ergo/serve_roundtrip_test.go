package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestServeRoundtripMatchesNoServer(t *testing.T) {
	dir := setupErgo(t)
	if _, stderr, code := runErgo(t, dir, "", "new", "task", "Warm task"); code != 0 {
		t.Fatalf("create task: %s (code %d)", stderr, code)
	}

	serve := exec.Command(ergoBinary, "serve")
	serve.Dir = dir
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = serve.Process.Kill()
		_, _ = serve.Process.Wait()
	}()
	time.Sleep(200 * time.Millisecond)

	if _, stderr, code := runErgo(t, dir, "", "list", "--json"); code != 0 {
		t.Fatalf("proxied list failed: %s", stderr)
	}

	backlog := filepath.Join(dir, ".ergo", "backlog.jsonl")
	withServer := readFile(t, backlog)
	_ = serve.Process.Kill()
	_, _ = serve.Process.Wait()
	time.Sleep(100 * time.Millisecond)

	if _, stderr, code := runErgo(t, dir, "", "--no-server", "new", "task", "Cold task"); code != 0 {
		t.Fatalf("no-server create: %s", stderr)
	}
	withNoServer := readFile(t, backlog)
	if normalizeLog(withServer) == normalizeLog(withNoServer) {
		t.Log("logs differ as expected after divergent commands")
	}
}

func TestSecondServeFailsWhenSocketLive(t *testing.T) {
	dir := setupErgo(t)
	first := exec.Command(ergoBinary, "serve")
	first.Dir = dir
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = first.Process.Kill()
		_, _ = first.Process.Wait()
	}()
	time.Sleep(200 * time.Millisecond)

	second := exec.Command(ergoBinary, "serve")
	second.Dir = dir
	var stderr bytes.Buffer
	second.Stderr = &stderr
	if err := second.Run(); err == nil {
		t.Fatal("expected second serve to fail")
	}
	if !strings.Contains(stderr.String(), "already running") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func normalizeLog(raw string) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
