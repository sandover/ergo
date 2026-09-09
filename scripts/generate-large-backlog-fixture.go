// Purpose: Create a deterministic, issue-shaped Ergo fixture in a temp project.
// Inputs: --dir, --tasks, --backlog, and --journal select the output shape.
// Output: JSON statistics for the generated backlog and journal.
// Invariants: The generator only writes the selected project directory and
// never changes Ergo's command or storage implementation.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sandover/ergo/v6/internal/ergo"
)

func main() {
	projectDir := flag.String("dir", "", "project directory to populate")
	taskCount := flag.Int("tasks", 1500, "number of tasks, including epics")
	backlogMode := flag.String("backlog", ergo.LargeBacklogModeFull, "backlog mode: full or compacted")
	journalMode := flag.String("journal", ergo.LargeBacklogModeFull, "journal mode: full or compacted")
	flag.Parse()
	if *projectDir == "" {
		fmt.Fprintln(os.Stderr, "--dir is required")
		os.Exit(2)
	}

	stats, err := ergo.WriteLargeBacklogFixture(*projectDir, ergo.LargeBacklogFixtureOptions{
		TaskCount: *taskCount, BacklogMode: *backlogMode, JournalMode: *journalMode,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(stats); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
