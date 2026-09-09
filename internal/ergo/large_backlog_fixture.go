// Purpose: Generate deterministic large-backlog data for performance spikes.
// Exports: LargeBacklogFixtureOptions, LargeBacklogFixtureStats,
// WriteLargeBacklogFixture.
// Role: Keep realistic history generation out of Ergo's runtime command path.
// Invariants: The default fixture has 1,500 tasks, 30,380 transactions, and
// issue-shaped journal counts; compacted output represents the same graph.
package ergo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LargeBacklogModeFull      = "full"
	LargeBacklogModeCompacted = "compacted"

	largeBacklogDefaultTasks           = 1500
	largeBacklogDefaultEpics           = 24
	largeBacklogReferenceLeaves        = 1476
	largeBacklogReferenceBodyEvents    = 14000
	largeBacklogReferenceTitleEvents   = 6574
	largeBacklogReferenceOpenEntries   = 1516
	largeBacklogReferenceClaimEntries  = 1380
	largeBacklogReferenceDoneEntries   = 1157
	largeBacklogReferenceCancelEntries = 227
	largeBacklogReferenceFailEntries   = 36
	largeBacklogReferenceBlockEntries  = 14
	largeBacklogReferenceResultEntries = 154
)

// LargeBacklogFixtureOptions selects the physical representation to write.
// TaskCount defaults to the issue-shaped 1,500-task fixture.
type LargeBacklogFixtureOptions struct {
	TaskCount   int
	BacklogMode string
	JournalMode string
}

// LargeBacklogFixtureStats describes the generated logical and physical data.
type LargeBacklogFixtureStats struct {
	TaskCount          int    `json:"task_count"`
	EpicCount          int    `json:"epic_count"`
	LeafCount          int    `json:"leaf_count"`
	DependencyCount    int    `json:"dependency_count"`
	EventCount         int    `json:"event_count"`
	TransactionCount   int    `json:"transaction_count"`
	BacklogRecords     int    `json:"backlog_records"`
	JournalEntries     int    `json:"journal_entries"`
	FullJournalEntries int    `json:"full_journal_entries"`
	BacklogBytes       int64  `json:"backlog_bytes"`
	JournalBytes       int64  `json:"journal_bytes"`
	BacklogMode        string `json:"backlog_mode"`
	JournalMode        string `json:"journal_mode"`
	FinalDone          int    `json:"final_done"`
	FinalCanceled      int    `json:"final_canceled"`
	FinalFailed        int    `json:"final_failed"`
	FinalBlocked       int    `json:"final_blocked"`
	FinalDoing         int    `json:"final_doing"`
	FinalTodo          int    `json:"final_todo"`
	BodyEvents         int    `json:"body_events"`
	TitleEvents        int    `json:"title_events"`
}

type largeBacklogFixture struct {
	events  []Event
	journal []JournalEntry
	graph   *Graph
	stats   LargeBacklogFixtureStats
}

// WriteLargeBacklogFixture writes .ergo/backlog.jsonl, journal.jsonl, and
// lock beneath projectDir. It is intended for temporary benchmark directories.
func WriteLargeBacklogFixture(projectDir string, options LargeBacklogFixtureOptions) (LargeBacklogFixtureStats, error) {
	fixture, err := buildLargeBacklogFixture(options)
	if err != nil {
		return LargeBacklogFixtureStats{}, err
	}

	dataDir := filepath.Join(projectDir, dataDirName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return LargeBacklogFixtureStats{}, err
	}
	backlog, backlogRecords, err := marshalLargeBacklog(fixture)
	if err != nil {
		return LargeBacklogFixtureStats{}, err
	}
	journal, err := fixtureJournalBytes(fixture)
	if err != nil {
		return LargeBacklogFixtureStats{}, err
	}
	if err := os.WriteFile(filepath.Join(dataDir, backlogFileName), backlog, 0644); err != nil {
		return LargeBacklogFixtureStats{}, err
	}
	if err := os.WriteFile(filepath.Join(dataDir, journalFileName), journal, 0644); err != nil {
		return LargeBacklogFixtureStats{}, err
	}
	if err := os.WriteFile(filepath.Join(dataDir, "lock"), nil, 0644); err != nil {
		return LargeBacklogFixtureStats{}, err
	}

	fixture.stats.BacklogBytes = int64(len(backlog))
	fixture.stats.JournalBytes = int64(len(journal))
	fixture.stats.BacklogRecords = backlogRecords
	if fixture.stats.JournalMode == LargeBacklogModeCompacted {
		fixture.stats.JournalEntries = len(compactJournal(fixture.journal, fixture.graph))
	}
	return fixture.stats, nil
}

func buildLargeBacklogFixture(options LargeBacklogFixtureOptions) (largeBacklogFixture, error) {
	taskCount := options.TaskCount
	if taskCount == 0 {
		taskCount = largeBacklogDefaultTasks
	}
	if taskCount < 3 {
		return largeBacklogFixture{}, errors.New("large backlog fixture requires at least three tasks")
	}
	if options.BacklogMode == "" {
		options.BacklogMode = LargeBacklogModeFull
	}
	if options.JournalMode == "" {
		options.JournalMode = LargeBacklogModeFull
	}
	if !validLargeBacklogMode(options.BacklogMode) || !validLargeBacklogMode(options.JournalMode) {
		return largeBacklogFixture{}, fmt.Errorf("large backlog modes must be %q or %q", LargeBacklogModeFull, LargeBacklogModeCompacted)
	}

	epicCount := taskCount * largeBacklogDefaultEpics / largeBacklogDefaultTasks
	if epicCount < 1 {
		epicCount = 1
	}
	if epicCount >= taskCount {
		epicCount = taskCount - 1
	}
	leafCount := taskCount - epicCount
	bodyEvents := scaledLargeBacklogCount(largeBacklogReferenceBodyEvents, taskCount, largeBacklogDefaultTasks)
	titleEvents := scaledLargeBacklogCount(largeBacklogReferenceTitleEvents, taskCount, largeBacklogDefaultTasks)
	openEntries := scaledLargeBacklogCount(largeBacklogReferenceOpenEntries, leafCount, largeBacklogReferenceLeaves)
	claimEntries := scaledLargeBacklogCount(largeBacklogReferenceClaimEntries, leafCount, largeBacklogReferenceLeaves)
	doneEntries := scaledLargeBacklogCount(largeBacklogReferenceDoneEntries, leafCount, largeBacklogReferenceLeaves)
	cancelEntries := scaledLargeBacklogCount(largeBacklogReferenceCancelEntries, leafCount, largeBacklogReferenceLeaves)
	failEntries := scaledLargeBacklogCount(largeBacklogReferenceFailEntries, leafCount, largeBacklogReferenceLeaves)
	blockEntries := scaledLargeBacklogCount(largeBacklogReferenceBlockEntries, leafCount, largeBacklogReferenceLeaves)
	resultEntries := scaledLargeBacklogCount(largeBacklogReferenceResultEntries, leafCount, largeBacklogReferenceLeaves)

	doneFinal := scaledLargeBacklogCount(900, leafCount, largeBacklogReferenceLeaves)
	canceledFinal := scaledLargeBacklogCount(180, leafCount, largeBacklogReferenceLeaves)
	failedFinal := scaledLargeBacklogCount(36, leafCount, largeBacklogReferenceLeaves)
	blockedFinal := scaledLargeBacklogCount(14, leafCount, largeBacklogReferenceLeaves)
	doingFinal := scaledLargeBacklogCount(43, leafCount, largeBacklogReferenceLeaves)
	todoFinal := leafCount - doneFinal - canceledFinal - failedFinal - blockedFinal - doingFinal

	fixture := largeBacklogFixture{
		stats: LargeBacklogFixtureStats{
			TaskCount: taskCount, EpicCount: epicCount, LeafCount: leafCount,
			BacklogMode: options.BacklogMode, JournalMode: options.JournalMode,
			BodyEvents: bodyEvents, TitleEvents: titleEvents,
			FinalDone: doneFinal, FinalCanceled: canceledFinal, FinalFailed: failedFinal,
			FinalBlocked: blockedFinal, FinalDoing: doingFinal, FinalTodo: todoFinal,
		},
	}

	base := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	nextTime := func() time.Time {
		return base.Add(time.Duration(len(fixture.events)) * time.Second)
	}
	addEvent := func(kind string, payload any) (time.Time, error) {
		now := nextTime()
		event, err := newEvent(kind, now, payload)
		if err != nil {
			return time.Time{}, err
		}
		fixture.events = append(fixture.events, event)
		return now, nil
	}
	addJournal := func(taskID, kind, agent, text string, at time.Time, file *JournalFile) {
		entry := newJournalEntry(taskID, kind, agent, text, at)
		entry.File = file
		fixture.journal = append(fixture.journal, entry)
	}
	addState := func(id, state string, withJournal bool, journalKind string) error {
		now := nextTime()
		if _, err := addEvent(eventState, StateEvent{ID: id, NewState: state, TS: formatTime(now)}); err != nil {
			return err
		}
		if withJournal {
			addJournal(id, journalKind, "fixture@local", "", now, nil)
		}
		return nil
	}
	addClaim := func(id string) error {
		now := nextTime()
		if _, err := addEvent(eventClaim, ClaimEvent{ID: id, AgentID: "fixture@local", TS: formatTime(now)}); err != nil {
			return err
		}
		addJournal(id, "claim", "fixture@local", "", now, nil)
		return nil
	}

	epics := make([]string, epicCount)
	leaves := make([]string, leafCount)
	for i := range epics {
		id := fmt.Sprintf("E%05d", i+1)
		epics[i] = id
		now := nextTime()
		if _, err := addEvent(eventNewTask, NewTaskEvent{
			ID: id, UUID: fmt.Sprintf("uuid-%s", strings.ToLower(id)), State: stateTodo,
			Title: fmt.Sprintf("Workstream %02d", i+1), Body: fixtureEpicBody(i), CreatedAt: formatTime(now),
		}); err != nil {
			return largeBacklogFixture{}, err
		}
		addJournal(id, "created", "", "", now, nil)
	}
	for i := range leaves {
		id := fmt.Sprintf("T%05d", i+1)
		leaves[i] = id
		now := nextTime()
		if _, err := addEvent(eventNewTask, NewTaskEvent{
			ID: id, UUID: fmt.Sprintf("uuid-%s", strings.ToLower(id)), EpicID: epics[i%len(epics)], State: stateTodo,
			Title: fmt.Sprintf("Implement queue item %04d", i+1), Body: fixtureBody(i, 0), CreatedAt: formatTime(now),
		}); err != nil {
			return largeBacklogFixture{}, err
		}
		addJournal(id, "created", "", "", now, nil)
	}

	// Churn 500 dependency pairs, then leave a second acyclic edge family in
	// place. This gives replay and graph derivation both historical and live work.
	churnPairs := minInt(500, leafCount-2)
	for i := 0; i < churnPairs; i++ {
		from, to := leaves[i+1], leaves[i]
		if _, err := addEvent(eventLink, LinkEvent{FromID: from, ToID: to, Type: dependsLinkType}); err != nil {
			return largeBacklogFixture{}, err
		}
		if _, err := addEvent(eventUnlink, LinkEvent{FromID: from, ToID: to, Type: dependsLinkType}); err != nil {
			return largeBacklogFixture{}, err
		}
		if _, err := addEvent(eventLink, LinkEvent{FromID: from, ToID: to, Type: dependsLinkType}); err != nil {
			return largeBacklogFixture{}, err
		}
	}
	for i := 0; i < minInt(1000, leafCount-2); i++ {
		if _, err := addEvent(eventLink, LinkEvent{FromID: leaves[i+2], ToID: leaves[i], Type: dependsLinkType}); err != nil {
			return largeBacklogFixture{}, err
		}
	}

	for i := 0; i < bodyEvents; i++ {
		id := leaves[i%len(leaves)]
		now := nextTime()
		if _, err := addEvent(eventBody, BodyUpdateEvent{ID: id, Body: fixtureBody(i%len(leaves), i/len(leaves)+1), TS: formatTime(now)}); err != nil {
			return largeBacklogFixture{}, err
		}
	}
	for i := 0; i < titleEvents; i++ {
		id := leaves[i%len(leaves)]
		now := nextTime()
		if _, err := addEvent(eventTitle, TitleUpdateEvent{ID: id, Title: fmt.Sprintf("Implement queue item %04d revision %04d", (i%len(leaves))+1, i/len(leaves)+1), TS: formatTime(now)}); err != nil {
			return largeBacklogFixture{}, err
		}
	}

	// Keep the journal kind mix close to the issue report. The later final-state
	// pass makes the event stream end in a deterministic active/terminal mix.
	claimedBeforeFinal := claimEntries - doingFinal
	for i := 0; i < claimedBeforeFinal; i++ {
		if err := addClaim(leaves[i%len(leaves)]); err != nil {
			return largeBacklogFixture{}, err
		}
	}
	terminalGroups := []struct {
		count int
		state string
		kind  string
		start int
	}{
		{doneEntries, stateDone, "done", 0},
		{cancelEntries, stateCanceled, "cancel", doneFinal},
		{failEntries, stateFailed, "fail", doneFinal + canceledFinal},
		{blockEntries, stateBlocked, "block", doneFinal + canceledFinal + failedFinal},
	}
	for _, group := range terminalGroups {
		width := maxInt(1, minInt(group.count, leafCount-group.start))
		for i := 0; i < group.count; i++ {
			if err := addState(leaves[group.start+i%width], group.state, true, group.kind); err != nil {
				return largeBacklogFixture{}, err
			}
		}
	}
	for i := 0; i < openEntries; i++ {
		if err := addState(leaves[i%len(leaves)], stateTodo, true, "open"); err != nil {
			return largeBacklogFixture{}, err
		}
	}
	for i := 0; i < resultEntries; i++ {
		id := leaves[i%len(leaves)]
		now := nextTime()
		addJournal(id, "result", "fixture@local", fmt.Sprintf("Attached deterministic artifact %03d", i+1), now, &JournalFile{
			Path:   fmt.Sprintf("docs/fixture-result-%03d.md", i+1),
			SHA256: fmt.Sprintf("fixture-sha-%064d", i+1),
		})
	}

	finalStates := []struct {
		count int
		state string
	}{
		{doneFinal, stateDone},
		{canceledFinal, stateCanceled},
		{failedFinal, stateFailed},
		{blockedFinal, stateBlocked},
		{todoFinal, stateTodo},
	}
	finalIndex := 0
	for _, group := range finalStates {
		for i := 0; i < group.count; i++ {
			if err := addState(leaves[finalIndex], group.state, false, ""); err != nil {
				return largeBacklogFixture{}, err
			}
			finalIndex++
		}
	}
	for i := 0; i < doingFinal; i++ {
		if err := addClaim(leaves[finalIndex+i]); err != nil {
			return largeBacklogFixture{}, err
		}
		if err := addState(leaves[finalIndex+i], stateDoing, false, ""); err != nil {
			return largeBacklogFixture{}, err
		}
	}

	graph, err := replayEvents(fixture.events)
	if err != nil {
		return largeBacklogFixture{}, fmt.Errorf("replay generated fixture: %w", err)
	}
	fixture.graph = graph
	fixture.stats.DependencyCount = countDependencies(graph)
	fixture.stats.EventCount = len(fixture.events)
	fixture.stats.TransactionCount = len(fixture.events)
	fixture.stats.JournalEntries = len(fixture.journal)
	fixture.stats.FullJournalEntries = len(fixture.journal)
	if options.BacklogMode == LargeBacklogModeCompacted {
		// The snapshot writer is exercised here so compacted fixtures cannot drift
		// from the actual compaction format without failing generation.
		if _, _, err := marshalSnapshot(graph); err != nil {
			return largeBacklogFixture{}, err
		}
	}
	return fixture, nil
}

func marshalLargeBacklog(fixture largeBacklogFixture) ([]byte, int, error) {
	if fixture.stats.BacklogMode == LargeBacklogModeCompacted {
		data, stats, err := marshalSnapshot(fixture.graph)
		return data, stats.Records, err
	}
	var output bytes.Buffer
	for _, event := range fixture.events {
		line, err := marshalTransaction([]Event{event})
		if err != nil {
			return nil, 0, err
		}
		output.Write(line)
	}
	return output.Bytes(), len(fixture.events), nil
}

func marshalJournalForLargeBacklog(fixture largeBacklogFixture) ([]byte, error) {
	entries := fixture.journal
	if fixture.stats.JournalMode == LargeBacklogModeCompacted {
		entries = compactJournal(entries, fixture.graph)
	}
	return marshalJournal(entries)
}

func validLargeBacklogMode(mode string) bool {
	return mode == LargeBacklogModeFull || mode == LargeBacklogModeCompacted
}

func scaledLargeBacklogCount(reference, actual, referenceSize int) int {
	if actual == referenceSize {
		return reference
	}
	return reference * actual / referenceSize
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func countDependencies(graph *Graph) int {
	count := 0
	for _, dependencies := range graph.Deps {
		count += len(dependencies)
	}
	return count
}

func fixtureEpicBody(index int) string {
	return fmt.Sprintf("## Workstream %02d\n\nThis deterministic epic groups queue work for the large-backlog spike.\n", index+1)
}

func fixtureBody(taskIndex, revision int) string {
	const targetBytes = 1400
	var body strings.Builder
	fmt.Fprintf(&body, "## Queue item %04d\n\nRevision %04d records the current implementation context.\n\n", taskIndex+1, revision)
	body.WriteString("### Evidence\n\n")
	body.WriteString("- Replayed from a deterministic file-backed fixture.\n- Body history intentionally changes while the final graph remains stable.\n\n")
	for body.Len() < targetBytes {
		body.WriteString("The worker preserves ordering, records the observed transition, and leaves the next agent a small piece of actionable context. ")
	}
	return body.String()[:targetBytes]
}

// fixtureJournalBytes is kept separate so the script and tests use exactly the
// same journal-mode projection. It also avoids making the writer's mode branch
// silently disagree with compacted benchmark setup.
func fixtureJournalBytes(fixture largeBacklogFixture) ([]byte, error) {
	return marshalJournalForLargeBacklog(fixture)
}
