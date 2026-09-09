// Purpose: Benchmark end-to-end CLI latency on the issue-shaped fixture.
// Coverage: list, ready list, JSON projections, show, claim, and lifecycle
// writes across full/compacted backlog and journal combinations.
// Run: go test ./cmd/ergo -run '^$' -bench LargeBacklog -benchmem.
package main

import (
	"fmt"
	"io"
	"os/exec"
	"testing"

	"github.com/sandover/ergo/v6/internal/ergo"
)

func BenchmarkLargeBacklogCLI(b *testing.B) {
	representations := []struct {
		name, backlog, journal string
	}{
		{"full-full", ergo.LargeBacklogModeFull, ergo.LargeBacklogModeFull},
		{"compacted-full", ergo.LargeBacklogModeCompacted, ergo.LargeBacklogModeFull},
		{"compacted-compacted", ergo.LargeBacklogModeCompacted, ergo.LargeBacklogModeCompacted},
	}
	commands := []struct {
		name string
		args func(iteration int) []string
	}{
		{"list", func(int) []string { return []string{"list"} }},
		{"list-ready", func(int) []string { return []string{"list", "--ready"} }},
		{"list-json", func(int) []string { return []string{"list", "--json"} }},
		{"list-ready-json", func(int) []string { return []string{"list", "--ready", "--json"} }},
		{"show", func(int) []string { return []string{"show", "T00001"} }},
		{"claim", func(int) []string { return []string{"claim", "T01174", "--agent", "benchmark@local"} }},
		{"lifecycle-write", func(iteration int) []string {
			return []string{"done", fmt.Sprintf("T%05d", 1174+iteration%303)}
		}},
	}
	for _, representation := range representations {
		for _, command := range commands {
			b.Run(representation.name+"/"+command.name, func(b *testing.B) {
				dir := b.TempDir()
				if _, err := ergo.WriteLargeBacklogFixture(dir, ergo.LargeBacklogFixtureOptions{
					BacklogMode: representation.backlog, JournalMode: representation.journal,
				}); err != nil {
					b.Fatal(err)
				}
				warm := exec.Command(ergoBinary, "list", "--json")
				warm.Dir = dir
				warm.Stdout = io.Discard
				warm.Stderr = io.Discard
				if err := warm.Run(); err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					cmd := exec.Command(ergoBinary, command.args(i)...)
					cmd.Dir = dir
					cmd.Stdout = io.Discard
					cmd.Stderr = io.Discard
					if err := cmd.Run(); err != nil {
						b.Fatalf("ergo %v: %v", command.args(i), err)
					}
				}
			})
		}
	}
}

func BenchmarkProcessStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(ergoBinary, "version")
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
}
