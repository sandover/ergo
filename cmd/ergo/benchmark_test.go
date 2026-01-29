// Benchmarks validate performance claims in README.md "Data Representation" section.
// Run: go test -bench=. -benchmem
// Run specific: go test -bench=BenchmarkList -benchmem
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// benchList benchmarks the list command with n tasks.
func benchList(b *testing.B, taskCount int) {
	dir := b.TempDir()
	ergo := buildErgoBinary(b)

	// Initialize
	runBenchErgo(b, ergo, dir, "", "init")

	// Create tasks
	for i := 0; i < taskCount; i++ {
		input := fmt.Sprintf(`{"title":"Task %d","body":"Body for task %d"}`, i, i)
		runBenchErgo(b, ergo, dir, input, "new", "task")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBenchErgo(b, ergo, dir, "", "list")
	}
}

// BenchmarkList100Tasks validates sub-50ms list for 100 tasks.
func BenchmarkList100Tasks(b *testing.B) { benchList(b, 100) }

// BenchmarkList500Tasks validates sub-100ms list for 500 tasks.
func BenchmarkList500Tasks(b *testing.B) { benchList(b, 500) }

// BenchmarkList1000Tasks validates scaling for 1000 tasks.
func BenchmarkList1000Tasks(b *testing.B) { benchList(b, 1000) }

// BenchmarkClaim benchmarks the claim hot path.
func BenchmarkClaim(b *testing.B) {
	dir := b.TempDir()
	ergo := buildErgoBinary(b)

	// Initialize and create enough tasks for benchmark iterations
	runBenchErgo(b, ergo, dir, "", "init")
	for i := 0; i < b.N+100; i++ {
		input := fmt.Sprintf(`{"title":"Task %d","body":"Body for task %d"}`, i, i)
		runBenchErgo(b, ergo, dir, input, "new", "task")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runBenchErgo(b, ergo, dir, "", "claim")
	}
}

// TestPerformance guards against performance regressions.
// These are NOT benchmarks — they're tests that fail if operations exceed
// generous upper bounds. Thresholds are 10x the expected time to allow for
// slow CI machines while still catching major regressions.
//
// Expected: 100 tasks ~3ms, 1000 tasks ~15ms (per README claims)
// Thresholds: 100 tasks <100ms, 1000 tasks <500ms
func TestPerformance_List100Tasks(t *testing.T) {
	assertListUnder(t, 100, 100*time.Millisecond)
}

func TestPerformance_List1000Tasks(t *testing.T) {
	assertListUnder(t, 1000, 500*time.Millisecond)
}

func TestPerformance_Claim(t *testing.T) {
	dir := t.TempDir()
	ergo := buildErgoBinaryForTest(t)

	runTestErgo(t, ergo, dir, "", "init")
	for i := 0; i < 100; i++ {
		input := fmt.Sprintf(`{"title":"Task %d","body":"Body %d"}`, i, i)
		runTestErgo(t, ergo, dir, input, "new", "task")
	}

	start := time.Now()
	runTestErgo(t, ergo, dir, "", "claim")
	elapsed := time.Since(start)

	// Expected ~6ms, threshold 200ms
	if elapsed > 200*time.Millisecond {
		t.Errorf("claim took %v, expected <200ms (regression guard)", elapsed)
	}
	t.Logf("claim: %v", elapsed)
}

func assertListUnder(t *testing.T, taskCount int, maxDuration time.Duration) {
	t.Helper()
	dir := t.TempDir()
	ergo := buildErgoBinaryForTest(t)

	runTestErgo(t, ergo, dir, "", "init")
	for i := 0; i < taskCount; i++ {
		input := fmt.Sprintf(`{"title":"Task %d","body":"Body %d"}`, i, i)
		runTestErgo(t, ergo, dir, input, "new", "task")
	}

	start := time.Now()
	runTestErgo(t, ergo, dir, "", "list")
	elapsed := time.Since(start)

	if elapsed > maxDuration {
		t.Errorf("list %d tasks took %v, expected <%v (regression guard)", taskCount, elapsed, maxDuration)
	}
	t.Logf("list %d tasks: %v", taskCount, elapsed)
}

// TestConcurrentClaimNoDoubles validates that racing agents don't double-claim.
// This is a correctness test, not a benchmark.
func TestConcurrentClaimNoDoubles(t *testing.T) {
	dir := t.TempDir()
	ergo := buildErgoBinaryForTest(t)

	// Initialize
	runTestErgo(t, ergo, dir, "", "init")

	// Create 20 tasks
	taskCount := 20
	for i := 0; i < taskCount; i++ {
		input := fmt.Sprintf(`{"title":"Task %d","body":"Body for task %d"}`, i, i)
		runTestErgo(t, ergo, dir, input, "new", "task")
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

			// Retry on lock busy (with fail-fast locking, agents may lose the race)
			const maxRetries = 3
			for attempt := 0; attempt < maxRetries; attempt++ {
				stdout, _, exitCode := runTestErgoWithExit(ergo, dir, "", "claim", "--agent", agentID)
				if exitCode == 0 && stdout != "" {
					// Successfully claimed
					id := extractTaskID(stdout)
					if id != "" {
						claimedIDs <- id
					}
					return
				} else if exitCode == 1 && attempt < maxRetries-1 {
					// Lock busy, retry
					continue
				} else if exitCode == 3 {
					// No ready tasks, acceptable
					return
				} else if exitCode != 1 {
					// Other error
					errors <- fmt.Errorf("%s: unexpected exit %d", agentID, exitCode)
					return
				}
			}
			// Exhausted retries on lock busy
			errors <- fmt.Errorf("%s: lock busy after %d retries", agentID, maxRetries)
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

	// Should have claimed exactly agentCount tasks (or fewer if race to empty)
	if len(claimed) > agentCount {
		t.Errorf("claimed %d tasks but only had %d agents", len(claimed), agentCount)
	}

	t.Logf("Successfully claimed %d tasks with %d agents, no double-claims", len(claimed), agentCount)
}

// --- Helpers ---

var benchBinaryCache string
var benchBinaryOnce sync.Once

func buildErgoBinary(b *testing.B) string {
	b.Helper()
	benchBinaryOnce.Do(func() {
		benchBinaryCache = buildBinary()
	})
	if benchBinaryCache == "" {
		b.Fatal("failed to build ergo binary")
	}
	return benchBinaryCache
}

func buildErgoBinaryForTest(t *testing.T) string {
	t.Helper()
	benchBinaryOnce.Do(func() {
		benchBinaryCache = buildBinary()
	})
	if benchBinaryCache == "" {
		t.Fatal("failed to build ergo binary")
	}
	return benchBinaryCache
}

func buildBinary() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	binary := filepath.Join(cwd, "ergo-bench")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	if err := cmd.Run(); err != nil {
		return ""
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
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Some commands (like claim with no tasks) exit non-zero
			_ = exitErr
		}
	}
	return string(out)
}

func runTestErgo(t *testing.T, binary, dir, stdin string, args ...string) string {
	t.Helper()
	stdout, _, _ := runTestErgoWithExit(binary, dir, stdin, args...)
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
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func extractTaskID(output string) string {
	// Try JSON first
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(output), &obj); err == nil {
		if id, ok := obj["id"].(string); ok {
			return id
		}
	}
	// Otherwise take first line (plain text output is just the ID)
	lines := bytes.Split([]byte(output), []byte("\n"))
	if len(lines) > 0 {
		return string(bytes.TrimSpace(lines[0]))
	}
	return ""
}
