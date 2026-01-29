// Round-trip tests for compacting the event log.
// Ensures compact output faithfully represents current state.
package ergo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"
)

type graphSnapshot struct {
	Tasks map[string]taskSnapshot `json:"tasks"`
}

type taskSnapshot struct {
	ID        string         `json:"id"`
	UUID      string         `json:"uuid"`
	EpicID    string         `json:"epic_id"`
	IsEpic    bool           `json:"is_epic"`
	State     string         `json:"state"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Worker    Worker         `json:"worker"`
	ClaimedBy string         `json:"claimed_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Deps      []string       `json:"deps"`
	RDeps     []string       `json:"rdeps"`
	Results   []resultSnap   `json:"results"`
	Invariant invariantState `json:"invariant"`
}

type resultSnap struct {
	Summary           string    `json:"summary"`
	Path              string    `json:"path"`
	Sha256AtAttach    string    `json:"sha256_at_attach"`
	MtimeAtAttach     string    `json:"mtime_at_attach"`
	GitCommitAtAttach string    `json:"git_commit_at_attach"`
	CreatedAt         time.Time `json:"created_at"`
}

type invariantState struct {
	ClaimOk bool   `json:"claim_ok"`
	Error   string `json:"error,omitempty"`
}

func snapshotGraphState(graph *Graph) graphSnapshot {
	out := graphSnapshot{Tasks: map[string]taskSnapshot{}}
	if graph == nil {
		return out
	}
	for id, task := range graph.Tasks {
		if task == nil {
			continue
		}
		out.Tasks[id] = snapshotTaskState(task)
	}
	return out
}

func snapshotTaskState(task *Task) taskSnapshot {
	snap := taskSnapshot{
		ID:        task.ID,
		UUID:      task.UUID,
		EpicID:    task.EpicID,
		IsEpic:    task.IsEpic,
		State:     task.State,
		Title:     task.Title,
		Body:      task.Body,
		Worker:    task.Worker,
		ClaimedBy: task.ClaimedBy,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
		Deps:      append([]string(nil), task.Deps...),
		RDeps:     append([]string(nil), task.RDeps...),
		Results:   snapshotResults(task.Results),
	}
	if err := validateClaimInvariant(task.State, task.ClaimedBy); err != nil {
		snap.Invariant = invariantState{ClaimOk: false, Error: err.Error()}
	} else {
		snap.Invariant = invariantState{ClaimOk: true}
	}
	return snap
}

func snapshotResults(results []Result) []resultSnap {
	if len(results) == 0 {
		return nil
	}
	out := make([]resultSnap, 0, len(results))
	for _, r := range results {
		out = append(out, resultSnap(r))
	}
	return out
}

func assertGraphStateEqual(t *testing.T, before, after *Graph) {
	t.Helper()
	sb := snapshotGraphState(before)
	sa := snapshotGraphState(after)
	if reflect.DeepEqual(sb, sa) {
		return
	}

	ids := make([]string, 0, len(sb.Tasks)+len(sa.Tasks))
	seen := map[string]struct{}{}
	for id := range sb.Tasks {
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for id := range sa.Tasks {
		if _, ok := seen[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	for _, id := range ids {
		bTask, bOk := sb.Tasks[id]
		aTask, aOk := sa.Tasks[id]
		if bOk != aOk || !reflect.DeepEqual(bTask, aTask) {
			b, _ := json.MarshalIndent(bTask, "", "  ")
			a, _ := json.MarshalIndent(aTask, "", "  ")
			t.Fatalf("graph state changed for %s after compaction roundtrip:\n--- before\n%s\n--- after\n%s", id, string(b), string(a))
		}
	}
	t.Fatal("graph state changed after compaction roundtrip (no per-task diff found)")
}

func mustNewEventT(t *testing.T, eventType string, ts time.Time, payload interface{}) Event {
	t.Helper()
	event, err := newEvent(eventType, ts, payload)
	if err != nil {
		t.Fatalf("newEvent(%s) failed: %v", eventType, err)
	}
	return event
}

func newTaskEvent(t *testing.T, ts time.Time, id, uuid, epicID, title, body string, worker Worker, isEpic bool) Event {
	t.Helper()
	eventType := "new_task"
	if isEpic {
		eventType = "new_epic"
		epicID = ""
	}
	return mustNewEventT(t, eventType, ts, NewTaskEvent{
		ID:        id,
		UUID:      uuid,
		EpicID:    epicID,
		State:     stateTodo,
		Title:     title,
		Body:      body,
		Worker:    string(worker),
		CreatedAt: formatTime(ts),
	})
}

func stateEvent(t *testing.T, ts time.Time, id, newState string) Event {
	t.Helper()
	return mustNewEventT(t, "state", ts, StateEvent{
		ID:       id,
		NewState: newState,
		TS:       formatTime(ts),
	})
}

func claimEvent(t *testing.T, ts time.Time, id, agentID string) Event {
	t.Helper()
	return mustNewEventT(t, "claim", ts, ClaimEvent{
		ID:      id,
		AgentID: agentID,
		TS:      formatTime(ts),
	})
}

func unclaimEvent(t *testing.T, ts time.Time, id string) Event {
	t.Helper()
	return mustNewEventT(t, "unclaim", ts, UnclaimEvent{
		ID: id,
		TS: formatTime(ts),
	})
}

func workerEvent(t *testing.T, ts time.Time, id string, worker Worker) Event {
	t.Helper()
	return mustNewEventT(t, "worker", ts, WorkerEvent{
		ID:     id,
		Worker: string(worker),
		TS:     formatTime(ts),
	})
}

func titleEvent(t *testing.T, ts time.Time, id, title string) Event {
	t.Helper()
	return mustNewEventT(t, "title", ts, TitleUpdateEvent{
		ID:    id,
		Title: title,
		TS:    formatTime(ts),
	})
}

func bodyEvent(t *testing.T, ts time.Time, id, body string) Event {
	t.Helper()
	return mustNewEventT(t, "body", ts, BodyUpdateEvent{
		ID:   id,
		Body: body,
		TS:   formatTime(ts),
	})
}

func epicAssignEvent(t *testing.T, ts time.Time, id, epicID string) Event {
	t.Helper()
	return mustNewEventT(t, "epic", ts, EpicAssignEvent{
		ID:     id,
		EpicID: epicID,
		TS:     formatTime(ts),
	})
}

func resultEvent(t *testing.T, ts time.Time, taskID string, summary, path, sha256, mtime, gitCommit string) Event {
	t.Helper()
	return mustNewEventT(t, "result", ts, ResultEvent{
		TaskID:            taskID,
		Summary:           summary,
		Path:              path,
		Sha256AtAttach:    sha256,
		MtimeAtAttach:     mtime,
		GitCommitAtAttach: gitCommit,
		TS:                formatTime(ts),
	})
}

func linkEvent(t *testing.T, ts time.Time, from, to string) Event {
	t.Helper()
	return mustNewEventT(t, "link", ts, LinkEvent{
		FromID: from,
		ToID:   to,
		Type:   dependsLinkType,
	})
}

func unlinkEvent(t *testing.T, ts time.Time, from, to string) Event {
	t.Helper()
	return mustNewEventT(t, "unlink", ts, LinkEvent{
		FromID: from,
		ToID:   to,
		Type:   dependsLinkType,
	})
}

func TestCompactEvents_RoundTrip_TaskEvolvesOverTime(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0).UTC()
	ts := func(i int) time.Time { return t0.Add(time.Duration(i) * time.Minute) }

	events := []Event{
		newTaskEvent(t, ts(0), "E1", "uuid-e1", "", "Epic 1", "", workerAny, true),
		newTaskEvent(t, ts(1), "T1", "uuid-t1", "", "Initial title", "", workerAny, false),
		newTaskEvent(t, ts(2), "T2", "uuid-t2", "", "Task 2", "", workerAny, false),

		epicAssignEvent(t, ts(3), "T1", "E1"),
		epicAssignEvent(t, ts(3), "T2", "E1"),

		// Dep graph churn: link + unlink + link.
		linkEvent(t, ts(4), "T2", "T1"),
		unlinkEvent(t, ts(5), "T2", "T1"),
		linkEvent(t, ts(6), "T2", "T1"),

		// Task 1: claim + doing, body changes, error/retry, worker change, then done + reopen.
		claimEvent(t, ts(7), "T1", "agent-1"),
		stateEvent(t, ts(7), "T1", stateDoing),
		titleEvent(t, ts(8), "T1", "Title"),
		bodyEvent(t, ts(8), "T1", "## Details\nbody v2"),
		stateEvent(t, ts(9), "T1", stateError),
		workerEvent(t, ts(10), "T1", workerHuman),
		stateEvent(t, ts(11), "T1", stateDoing),
		bodyEvent(t, ts(12), "T1", "## Details\nbody v3\n\n- bullets\n- more"),
		stateEvent(t, ts(13), "T1", stateDone),
		stateEvent(t, ts(14), "T1", stateTodo),

		// Task 2: blocked, claimed while blocked, then unclaimed (still blocked).
		stateEvent(t, ts(15), "T2", stateBlocked),
		claimEvent(t, ts(16), "T2", "agent-2"),
		unclaimEvent(t, ts(17), "T2"),

		// Results: ensure optional evidence fields survive compaction.
		resultEvent(t, ts(18), "T1", "First artifact", "docs/a.md", "aaa111", "", ""),
		resultEvent(t, ts(19), "T1", "Second artifact", "docs/b.md", "bbb222", formatTime(ts(19)), "deadbeef"),
	}

	graphBefore, err := replayEvents(events)
	if err != nil {
		t.Fatalf("replayEvents(before) failed: %v", err)
	}
	compacted, err := compactEvents(graphBefore)
	if err != nil {
		t.Fatalf("compactEvents failed: %v", err)
	}
	graphAfter, err := replayEvents(compacted)
	if err != nil {
		t.Fatalf("replayEvents(after) failed: %v", err)
	}

	assertGraphStateEqual(t, graphBefore, graphAfter)
}

type simState struct {
	tasks map[string]*Task
	deps  map[string]map[string]struct{}
}

func newSimState() *simState {
	return &simState{
		tasks: map[string]*Task{},
		deps:  map[string]map[string]struct{}{},
	}
}

func (s *simState) toGraph() *Graph {
	graph := &Graph{
		Tasks: map[string]*Task{},
		Deps:  map[string]map[string]struct{}{},
		RDeps: map[string]map[string]struct{}{},
		Meta:  map[string]*TaskMeta{},
	}
	for id, task := range s.tasks {
		clone := *task
		clone.Deps = nil
		clone.RDeps = nil
		graph.Tasks[id] = &clone
	}
	for from, tos := range s.deps {
		graph.Deps[from] = map[string]struct{}{}
		for to := range tos {
			graph.Deps[from][to] = struct{}{}
		}
	}
	return graph
}

func (s *simState) ensureDepsMap(from string) map[string]struct{} {
	if s.deps[from] == nil {
		s.deps[from] = map[string]struct{}{}
	}
	return s.deps[from]
}

func (s *simState) applyNewTask(id, uuid, epicID string, isEpic bool, title, body string, worker Worker, createdAt time.Time) {
	if isEpic {
		epicID = ""
	}
	s.tasks[id] = &Task{
		ID:        id,
		UUID:      uuid,
		EpicID:    epicID,
		IsEpic:    isEpic,
		State:     stateTodo,
		Title:     title,
		Body:      body,
		Worker:    worker,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func (s *simState) applyState(id, newState string, ts time.Time) {
	task := s.tasks[id]
	if task == nil {
		return
	}
	task.State = newState
	task.UpdatedAt = maxTime(task.UpdatedAt, ts)
	if newState == stateTodo || newState == stateDone || newState == stateCanceled {
		task.ClaimedBy = ""
	}
}

func (s *simState) applyClaim(id, agentID string) {
	task := s.tasks[id]
	if task == nil {
		return
	}
	task.ClaimedBy = agentID
}

func (s *simState) applyUnclaim(id string) {
	task := s.tasks[id]
	if task == nil {
		return
	}
	task.ClaimedBy = ""
}

func (s *simState) applyBody(id, body string, ts time.Time) {
	task := s.tasks[id]
	if task == nil {
		return
	}
	task.Body = body
	task.UpdatedAt = maxTime(task.UpdatedAt, ts)
}

func (s *simState) applyTitle(id, title string, ts time.Time) {
	task := s.tasks[id]
	if task == nil {
		return
	}
	task.Title = title
	task.UpdatedAt = maxTime(task.UpdatedAt, ts)
}

func (s *simState) applyWorker(id string, worker Worker, ts time.Time) {
	task := s.tasks[id]
	if task == nil {
		return
	}
	task.Worker = worker
	task.UpdatedAt = maxTime(task.UpdatedAt, ts)
}

func (s *simState) applyEpic(id, epicID string, ts time.Time) {
	task := s.tasks[id]
	if task == nil || task.IsEpic {
		return
	}
	task.EpicID = epicID
	task.UpdatedAt = maxTime(task.UpdatedAt, ts)
}

func (s *simState) applyResult(taskID string, result Result, ts time.Time) {
	task := s.tasks[taskID]
	if task == nil || task.IsEpic {
		return
	}
	task.Results = append([]Result{result}, task.Results...)
	task.UpdatedAt = maxTime(task.UpdatedAt, ts)
}

func (s *simState) applyLink(from, to string) {
	s.ensureDepsMap(from)[to] = struct{}{}
}

func (s *simState) applyUnlink(from, to string) {
	if s.deps[from] == nil {
		return
	}
	delete(s.deps[from], to)
}

func parsePositiveIntEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func randomEventLog(t *testing.T, seed int64, steps int) []Event {
	t.Helper()
	r := rand.New(rand.NewSource(seed))
	base := time.Unix(1_700_123_456, 0).UTC()
	now := func(i int) time.Time { return base.Add(time.Duration(i) * time.Millisecond) }

	sim := newSimState()
	var events []Event

	nextEpic := 1
	nextTask := 1

	// Ensure at least one epic and a couple tasks.
	{
		epicID := fmt.Sprintf("E%d", nextEpic)
		nextEpic++
		title := "Epic " + epicID
		events = append(events, newTaskEvent(t, now(len(events)), epicID, "uuid-"+epicID, "", title, "", workerAny, true))
		sim.applyNewTask(epicID, "uuid-"+epicID, "", true, title, "", workerAny, now(len(events)-1))
	}
	for i := 0; i < 2; i++ {
		taskID := fmt.Sprintf("T%d", nextTask)
		nextTask++
		epicID := "E1"
		title := fmt.Sprintf("Task %s", taskID)
		worker := []Worker{workerAny, workerAgent, workerHuman}[r.Intn(3)]
		events = append(events, newTaskEvent(t, now(len(events)), taskID, "uuid-"+taskID, epicID, title, "", worker, false))
		sim.applyNewTask(taskID, "uuid-"+taskID, epicID, false, title, "", worker, now(len(events)-1))
	}

	taskIDs := func() []string {
		ids := make([]string, 0, len(sim.tasks))
		for id, task := range sim.tasks {
			if task != nil && !task.IsEpic {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		return ids
	}
	epicIDs := func() []string {
		ids := make([]string, 0, len(sim.tasks))
		for id, task := range sim.tasks {
			if task != nil && task.IsEpic {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		return ids
	}

	choose := func(ids []string) (string, bool) {
		if len(ids) == 0 {
			return "", false
		}
		return ids[r.Intn(len(ids))], true
	}

	for i := 0; i < steps; i++ {
		ts := now(len(events))
		switch r.Intn(10) {
		case 0: // new epic
			epicID := fmt.Sprintf("E%d", nextEpic)
			nextEpic++
			title := fmt.Sprintf("Epic %s", epicID)
			events = append(events, newTaskEvent(t, ts, epicID, "uuid-"+epicID, "", title, "", workerAny, true))
			sim.applyNewTask(epicID, "uuid-"+epicID, "", true, title, "", workerAny, ts)

		case 1: // new task
			taskID := fmt.Sprintf("T%d", nextTask)
			nextTask++
			epics := epicIDs()
			epicID := ""
			if len(epics) > 0 && r.Intn(2) == 0 {
				epicID = epics[r.Intn(len(epics))]
			}
			title := fmt.Sprintf("Task %s", taskID)
			body := fmt.Sprintf("seed=%d step=%d", seed, i)
			worker := []Worker{workerAny, workerAgent, workerHuman}[r.Intn(3)]
			events = append(events, newTaskEvent(t, ts, taskID, "uuid-"+taskID, epicID, title, body, worker, false))
			sim.applyNewTask(taskID, "uuid-"+taskID, epicID, false, title, body, worker, ts)

		case 2: // body update (task or epic)
			id, ok := choose(append(taskIDs(), epicIDs()...))
			if !ok {
				continue
			}
			body := fmt.Sprintf("Body %s\n\nseed=%d step=%d\n- a\n- b", id, seed, i)
			events = append(events, bodyEvent(t, ts, id, body))
			sim.applyBody(id, body, ts)

		case 3: // title update (task or epic)
			id, ok := choose(append(taskIDs(), epicIDs()...))
			if !ok {
				continue
			}
			title := fmt.Sprintf("Title %s seed=%d step=%d", id, seed, i)
			events = append(events, titleEvent(t, ts, id, title))
			sim.applyTitle(id, title, ts)

		case 4: // worker update (task only; epics cannot have workers)
			id, ok := choose(taskIDs())
			if !ok {
				continue
			}
			worker := []Worker{workerAny, workerAgent, workerHuman}[r.Intn(3)]
			events = append(events, workerEvent(t, ts, id, worker))
			sim.applyWorker(id, worker, ts)

		case 5: // state transition + optional claim mechanics (task only)
			id, ok := choose(taskIDs())
			if !ok {
				continue
			}
			task := sim.tasks[id]
			if task == nil {
				continue
			}
			candidates := []string{stateTodo, stateDoing, stateDone, stateBlocked, stateCanceled, stateError}
			newState := candidates[r.Intn(len(candidates))]
			if err := validateTransition(task.State, newState); err != nil {
				continue
			}
			needClaim := newState == stateDoing || newState == stateError
			if needClaim && task.ClaimedBy == "" {
				agent := fmt.Sprintf("agent-%d", r.Intn(5)+1)
				events = append(events, claimEvent(t, ts, id, agent))
				sim.applyClaim(id, agent)
			}
			events = append(events, stateEvent(t, ts, id, newState))
			sim.applyState(id, newState, ts)

		case 6: // epic assignment (task only)
			id, ok := choose(taskIDs())
			if !ok {
				continue
			}
			epics := epicIDs()
			epicID := ""
			if len(epics) > 0 && r.Intn(3) != 0 {
				epicID = epics[r.Intn(len(epics))]
			}
			events = append(events, epicAssignEvent(t, ts, id, epicID))
			sim.applyEpic(id, epicID, ts)

		case 7: // dependency link/unlink (tasks only, cycle-free)
			ids := taskIDs()
			if len(ids) < 2 {
				continue
			}
			from := ids[r.Intn(len(ids))]
			to := ids[r.Intn(len(ids))]
			if from == to {
				continue
			}
			graph := sim.toGraph()
			fromTask := graph.Tasks[from]
			toTask := graph.Tasks[to]
			if fromTask == nil || toTask == nil {
				continue
			}
			if err := validateDepKinds(isEpic(fromTask), isEpic(toTask)); err != nil {
				continue
			}

			// Half the time: unlink if present; otherwise link if safe.
			if sim.deps[from] != nil {
				if _, exists := sim.deps[from][to]; exists && r.Intn(2) == 0 {
					events = append(events, unlinkEvent(t, ts, from, to))
					sim.applyUnlink(from, to)
					continue
				}
			}
			if hasCycle(graph, from, to) {
				continue
			}
			events = append(events, linkEvent(t, ts, from, to))
			sim.applyLink(from, to)

		case 8: // result attachment (task only)
			id, ok := choose(taskIDs())
			if !ok {
				continue
			}
			summary := fmt.Sprintf("result for %s (seed=%d step=%d)", id, seed, i)
			path := fmt.Sprintf("docs/%s-%d.md", id, i)
			sha256 := fmt.Sprintf("%x", r.Uint64())
			mtime := ""
			gitCommit := ""
			if r.Intn(3) == 0 {
				mtime = formatTime(ts)
			}
			if r.Intn(3) == 0 {
				gitCommit = fmt.Sprintf("%x", r.Uint64())
			}
			events = append(events, resultEvent(t, ts, id, summary, path, sha256, mtime, gitCommit))
			sim.applyResult(id, Result{
				Summary:           summary,
				Path:              path,
				Sha256AtAttach:    sha256,
				MtimeAtAttach:     mtime,
				GitCommitAtAttach: gitCommit,
				CreatedAt:         ts,
			}, ts)

		case 9: // claim churn without state changes (task only)
			id, ok := choose(taskIDs())
			if !ok {
				continue
			}
			task := sim.tasks[id]
			if task == nil {
				continue
			}
			// Keep generated histories within the claim/state invariants that ergo enforces.
			// todo/done/canceled must not be claimed; doing/error must remain claimed; blocked may toggle.
			switch task.State {
			case stateTodo, stateDone, stateCanceled:
				if task.ClaimedBy != "" {
					events = append(events, unclaimEvent(t, ts, id))
					sim.applyUnclaim(id)
				}
				continue
			case stateDoing, stateError:
				if task.ClaimedBy == "" {
					agent := fmt.Sprintf("agent-%d", r.Intn(5)+1)
					events = append(events, claimEvent(t, ts, id, agent))
					sim.applyClaim(id, agent)
				}
				continue
			}
			// blocked: toggle claim
			if task.ClaimedBy == "" {
				agent := fmt.Sprintf("agent-%d", r.Intn(5)+1)
				events = append(events, claimEvent(t, ts, id, agent))
				sim.applyClaim(id, agent)
				continue
			}
			events = append(events, unclaimEvent(t, ts, id))
			sim.applyUnclaim(id)
		}
	}

	return events
}

func TestCompactEvents_RoundTrip_Randomized(t *testing.T) {
	defaultSeeds := 50
	defaultSteps := 80
	if testing.Short() {
		defaultSeeds = 10
		defaultSteps = 40
	}
	seeds := parsePositiveIntEnv("ERGO_COMPACT_FUZZ_SEEDS", defaultSeeds)
	steps := parsePositiveIntEnv("ERGO_COMPACT_FUZZ_STEPS", defaultSteps)

	for seed := int64(1); seed <= int64(seeds); seed++ {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			events := randomEventLog(t, seed, steps)

			graphBefore, err := replayEvents(events)
			if err != nil {
				t.Fatalf("replayEvents(before) failed: %v", err)
			}
			compacted, err := compactEvents(graphBefore)
			if err != nil {
				t.Fatalf("compactEvents failed: %v", err)
			}
			graphAfter, err := replayEvents(compacted)
			if err != nil {
				t.Fatalf("replayEvents(after) failed: %v", err)
			}
			assertGraphStateEqual(t, graphBefore, graphAfter)
		})
	}
}
