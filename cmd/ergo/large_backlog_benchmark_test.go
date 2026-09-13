// Purpose: Benchmark end-to-end CLI latency on the issue-shaped fixture.
// Coverage: list, ready list, JSON projections, show, claim, and lifecycle
// writes across full/compacted backlog and journal combinations.
// Run: go test ./cmd/ergo -run '^$' -bench LargeBacklog -benchmem.
package main

import (
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
		name  string
		args  []string
		reset []string
	}{
		{"list", []string{"list"}, nil},
		{"list-ready", []string{"list", "--ready"}, nil},
		{"list-json", []string{"list", "--json"}, nil},
		{"list-ready-json", []string{"list", "--ready", "--json"}, nil},
		{"show", []string{"show", "T00001"}, nil},
		{"claim", []string{"claim", "T01174", "--agent", "benchmark@local"}, []string{"open", "T01174"}},
		{"lifecycle-write", []string{"done", "T01174"}, []string{"claim", "T01174", "--agent", "benchmark@local"}},
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
					if command.reset != nil {
						b.StopTimer()
						// Establish a real transition before every measured mutation.
						reset := exec.Command(ergoBinary, command.reset...)
						reset.Dir = dir
						if output, err := reset.CombinedOutput(); err != nil {
							b.Fatalf("ergo %v: %v\n%s", command.reset, err, output)
						}
						b.StartTimer()
					}
					cmd := exec.Command(ergoBinary, command.args...)
					cmd.Dir = dir
					cmd.Stdout = io.Discard
					cmd.Stderr = io.Discard
					if err := cmd.Run(); err != nil {
						b.Fatalf("ergo %v: %v", command.args, err)
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
