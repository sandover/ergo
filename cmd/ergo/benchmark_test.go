// Benchmarks measure representative CLI operations through the built binary.
// Run: go test -bench=. -benchmem
// Run specific: go test -bench=BenchmarkList -benchmem
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// benchList benchmarks the list command with n tasks.
func benchList(b *testing.B, taskCount int) {
	dir := b.TempDir()
	ergo := buildErgoBinary(b)

	// Initialize
	runBenchErgo(b, ergo, dir, "", "init")

	// Create tasks
	created := make([]string, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		body := fmt.Sprintf("Body for task %d\n", i)
		output := runBenchErgo(b, ergo, dir, body, "new", "task", fmt.Sprintf("Task %d", i))
		created = append(created, requireTaskID(b, output))
	}
	requireCreatedTaskCount(b, ergo, dir, created, taskCount)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBenchErgo(b, ergo, dir, "", "list")
	}
}

// BenchmarkList100Tasks measures list with 100 tasks.
func BenchmarkList100Tasks(b *testing.B) { benchList(b, 100) }

// BenchmarkList500Tasks measures list with 500 tasks.
func BenchmarkList500Tasks(b *testing.B) { benchList(b, 500) }

// BenchmarkList1000Tasks measures list with 1000 tasks.
func BenchmarkList1000Tasks(b *testing.B) { benchList(b, 1000) }

// BenchmarkClaim benchmarks the claim hot path.
func BenchmarkClaim(b *testing.B) {
	dir := b.TempDir()
	ergo := buildErgoBinary(b)

	// Initialize and create enough tasks for benchmark iterations
	runBenchErgo(b, ergo, dir, "", "init")
	created := make([]string, 0, b.N)
	for i := 0; i < b.N; i++ {
		body := fmt.Sprintf("Body for task %d\n", i)
		output := runBenchErgo(b, ergo, dir, body, "new", "task", fmt.Sprintf("Task %d", i))
		created = append(created, requireTaskID(b, output))
	}
	requireCreatedTaskCount(b, ergo, dir, created, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBenchErgo(b, ergo, dir, "", "claim", "--agent", "benchmark@local")
	}
}

func TestPerformanceSetupUsesCurrentCLI(t *testing.T) {
	dir := t.TempDir()
	ergo := buildErgoBinary(t)

	runTestErgo(t, ergo, dir, "", "init")
	const taskCount = 12
	created := make([]string, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		body := fmt.Sprintf("Body %d\n", i)
		output := runTestErgo(t, ergo, dir, body, "new", "task", fmt.Sprintf("Task %d", i))
		created = append(created, requireTaskID(t, output))
	}
	requireCreatedTaskCount(t, ergo, dir, created, taskCount)
}

// TestConcurrentClaimNoDoubles validates that racing agents don't double-claim.
// This is a correctness test, not a benchmark.
func TestConcurrentClaimNoDoubles(t *testing.T) {
	dir := t.TempDir()
	ergo := buildErgoBinary(t)

	// Initialize
	runTestErgo(t, ergo, dir, "", "init")

	// Create 20 tasks
	taskCount := 20
	for i := 0; i < taskCount; i++ {
		body := fmt.Sprintf("Body for task %d\n", i)
		runTestErgo(t, ergo, dir, body, "new", "task", fmt.Sprintf("Task %d", i))
	}

	// 10 goroutines racing to claim
	agentCount := 10
	var wg sync.WaitGroup
	claimedIDs := make(chan string, agentCount)
	errors := make(chan error, agentCount)

	for i := 0; i < agentCount; i++ {
		wg.Add(1)
		go func(agentNum int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent-%d", agentNum)

			stdout, stderr, exitCode := runTestErgoWithExit(ergo, dir, "", "claim", "--agent", agentID)
			if exitCode == 0 {
				if strings.Contains(stdout, "No ready ergo tasks.") || stdout == "" {
					return
				}
				id := extractClaimedTaskID(stdout)
				if id != "" {
					claimedIDs <- id
					return
				}
				errors <- fmt.Errorf("%s: unexpected output %q", agentID, stdout)
				return
			}
			errors <- fmt.Errorf("%s: unexpected exit %d stderr=%q", agentID, exitCode, stderr)
		}(i)
	}

	wg.Wait()
	close(claimedIDs)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Collect claimed IDs
	claimed := make(map[string]int)
	for id := range claimedIDs {
		claimed[id]++
	}

	// Verify no double-claims
	for id, count := range claimed {
		if count > 1 {
			t.Errorf("task %s was claimed %d times (should be 1)", id, count)
		}
	}

	if len(claimed) != agentCount {
		t.Errorf("claimed %d tasks, want exactly %d", len(claimed), agentCount)
	}

	t.Logf("Successfully claimed %d tasks with %d agents, no double-claims", len(claimed), agentCount)
}

// --- Helpers ---

func buildErgoBinary(tb testing.TB) string {
	tb.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		tb.Fatalf("get working directory: %v", err)
	}
	binary := filepath.Join(tb.TempDir(), "ergo-bench")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("build ergo benchmark binary: %v\n%s", err, output)
	}
	return binary
}

func runBenchErgo(b *testing.B, binary, dir, stdin string, args ...string) string {
	b.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = bytes.NewBufferString(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		b.Fatalf("ergo %s: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

func runTestErgo(t *testing.T, binary, dir, stdin string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode := runTestErgoWithExit(binary, dir, stdin, args...)
	if exitCode != 0 {
		t.Fatalf("ergo %s: exit %d\nstderr: %s", strings.Join(args, " "), exitCode, stderr)
	}
	return stdout
}

func runTestErgoWithExit(binary, dir, stdin string, args ...string) (string, string, int) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = bytes.NewBufferString(stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
		fmt.Fprintf(&errBuf, "start command: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func extractTaskID(output string) string {
	fields := strings.Fields(output)
	if len(fields) == 1 {
		return fields[0]
	}
	return ""
}

func extractClaimedTaskID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(line, `id: "`); ok {
			if id, ok := strings.CutSuffix(value, `"`); ok {
				return id
			}
		}
	}
	return ""
}

func requireTaskID(tb testing.TB, output string) string {
	tb.Helper()
	id := extractTaskID(output)
	if id == "" {
		tb.Fatalf("task creation returned an invalid ID: %q", output)
	}
	return id
}

func requireCreatedTaskCount(tb testing.TB, binary, dir string, created []string, want int) {
	tb.Helper()
	if len(created) != want {
		tb.Fatalf("setup recorded %d task IDs, want %d", len(created), want)
	}
	cmd := exec.Command(binary, "list", "--all")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tb.Fatalf("verify setup task count: %v\nstderr: %s", err, stderr.String())
	}
	found := 0
	for _, id := range created {
		if strings.Count(stdout.String(), id) != 1 {
			tb.Fatalf("setup task %s does not appear exactly once\n%s", id, stdout.String())
		}
		found++
	}
	if found != want {
		tb.Fatalf("setup contains %d verified tasks, want %d", found, want)
	}
}
