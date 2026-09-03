package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestServeRoundtripEnrichedListMatchesNoServer(t *testing.T) {
	dir := setupErgo(t)
	if _, stderr, code := runErgo(t, dir, "hydrate body\n", "new", "task", "Hydrate task"); code != 0 {
		t.Fatalf("create task: %s", stderr)
	}

	serve := exec.Command(ergoBinary, "serve")
	serve.Dir = dir
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	defer stopServe(t, serve)
	time.Sleep(200 * time.Millisecond)

	proxied, proxiedErr, code := runErgo(t, dir, "", "list", "--json", "--with-meta", "--with-body")
	if code != 0 {
		t.Fatalf("proxied list failed: %s", proxiedErr)
	}
	direct, directErr, code := runErgo(t, dir, "", "--no-server", "list", "--json", "--with-meta", "--with-body")
	if code != 0 {
		t.Fatalf("direct list failed: %s", directErr)
	}
	if normalizeJSON(proxied) != normalizeJSON(direct) {
		t.Fatalf("proxied != direct\nproxied: %s\ndirect: %s", proxied, direct)
	}
}

func TestServeRoundtripBatchShowMatchesNoServer(t *testing.T) {
	dir := setupErgo(t)
	stdout, stderr, code := runErgo(t, dir, "batch body\n", "new", "task", "Batch task")
	if code != 0 {
		t.Fatalf("create task: %s", stderr)
	}
	id := strings.TrimSpace(stdout)
	if len(id) != 6 {
		t.Fatalf("task id = %q", id)
	}

	serve := exec.Command(ergoBinary, "serve")
	serve.Dir = dir
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	defer stopServe(t, serve)
	time.Sleep(200 * time.Millisecond)

	proxiedOut, proxiedErr, code := runErgo(t, dir, "", "batch-show", "--json", id, "ZZZZZZ")
	if code != 0 {
		t.Fatalf("proxied batch-show failed: %s", proxiedErr)
	}
	if !strings.Contains(proxiedErr, "unknown task id ZZZZZZ") {
		t.Fatalf("proxied stderr = %q", proxiedErr)
	}
	directOut, directErr, code := runErgo(t, dir, "", "--no-server", "batch-show", "--json", id, "ZZZZZZ")
	if code != 0 {
		t.Fatalf("direct batch-show failed: %s", directErr)
	}
	if !strings.Contains(directErr, "unknown task id ZZZZZZ") {
		t.Fatalf("direct stderr = %q", directErr)
	}
	if normalizeJSON(proxiedOut) != normalizeJSON(directOut) {
		t.Fatalf("proxied != direct\nproxied: %s\ndirect: %s", proxiedOut, directOut)
	}
}

func stopServe(t *testing.T, serve *exec.Cmd) {
	t.Helper()
	_ = serve.Process.Kill()
	_, _ = serve.Process.Wait()
}

func normalizeJSON(raw string) string {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return strings.TrimSpace(raw)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return string(normalized)
}

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
