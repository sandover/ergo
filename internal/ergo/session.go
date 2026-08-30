package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	socketFileName = "ergo.sock"
	pidFileName    = "ergo.pid"
)

type logFingerprint struct {
	backlogDev   uint64
	backlogIno   uint64
	backlogSize  int64
	journalDev   uint64
	journalIno   uint64
	journalSize  int64
	validBytes   int64
}

// Session holds a warm in-memory graph for one opened repository.
type Session struct {
	app         *Application
	repo        Repository
	graph       *Graph
	eventRead   eventLogRead
	journal     []JournalEntry
	fingerprint logFingerprint
	mu          sync.Mutex
}

func (a *Application) OpenSession() (*Session, error) {
	var repo Repository
	if err := repo.Open(a.repository); err != nil {
		return nil, classifyRepositoryError(err)
	}
	session := &Session{app: a, repo: repo}
	if err := session.reload(); err != nil {
		return nil, classifyRepositoryError(err)
	}
	return session, nil
}

func (s *Session) Close() {}

func (s *Session) Repository() *Repository { return &s.repo }

func (s *Session) ProjectDir() string { return s.repo.ProjectDir() }

func SocketPath(dir string) string { return filepath.Join(dir, socketFileName) }

func PidPath(dir string) string { return filepath.Join(dir, pidFileName) }

func (s *Session) reload() error {
	graph, read, err := s.repo.loadWithRead()
	if err != nil {
		return err
	}
	s.graph = graph
	s.eventRead = read
	s.journal = nil
	s.fingerprint, err = captureLogFingerprint(s.repo.eventsPath, s.repo.journalPath, read.validBytes)
	return err
}

func (s *Session) ensureJournal() error {
	if s.journal != nil {
		return nil
	}
	journal, err := s.repo.loadJournal()
	if err != nil {
		return err
	}
	s.journal = mergeLegacyJournal(journal, s.graph)
	hydrateGraphEvidence(s.graph, s.journal)
	return nil
}

func fileStatFingerprint(path string) (dev, ino uint64, size int64, err error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, 0, nil
	}
	if err != nil {
		return 0, 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, info.Size(), nil
	}
	return uint64(stat.Dev), stat.Ino, info.Size(), nil
}

func captureLogFingerprint(eventsPath, journalPath string, validBytes int64) (logFingerprint, error) {
	var fp logFingerprint
	var err error
	fp.backlogDev, fp.backlogIno, fp.backlogSize, err = fileStatFingerprint(eventsPath)
	if err != nil {
		return logFingerprint{}, err
	}
	fp.journalDev, fp.journalIno, fp.journalSize, err = fileStatFingerprint(journalPath)
	if err != nil {
		return logFingerprint{}, err
	}
	fp.validBytes = validBytes
	return fp, nil
}

func (s *Session) ensureFresh() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureFreshLocked()
}

func (s *Session) ensureFreshLocked() error {
	current, err := captureLogFingerprint(s.repo.eventsPath, s.repo.journalPath, s.eventRead.validBytes)
	if err != nil {
		return err
	}
	if current == s.fingerprint {
		return nil
	}
	return s.reload()
}

func (s *Session) updateLoaded(fn func(*Graph) ([]Event, error)) (UpdateOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateLoadedLocked(fn)
}

func (s *Session) updateLoadedLocked(fn func(*Graph) ([]Event, error)) (UpdateOutcome, error) {
	if err := s.ensureFreshLocked(); err != nil {
		return UpdateOutcome{}, err
	}
	outcome, nextRead, err := s.repo.updateLoaded(s.graph, s.eventRead, fn)
	if err != nil {
		return UpdateOutcome{}, err
	}
	s.graph = outcome.Graph
	s.eventRead = nextRead
	s.fingerprint, err = captureLogFingerprint(s.repo.eventsPath, s.repo.journalPath, s.eventRead.validBytes)
	if err != nil {
		return UpdateOutcome{}, err
	}
	return outcome, nil
}

func (s *Session) updateLoadedWithJournal(fn func(*Graph) ([]Event, []JournalEntry, error)) (UpdateOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateLoadedWithJournalLocked(fn)
}

func (s *Session) updateLoadedWithJournalLocked(fn func(*Graph) ([]Event, []JournalEntry, error)) (UpdateOutcome, error) {
	if err := s.ensureFreshLocked(); err != nil {
		return UpdateOutcome{}, err
	}
	outcome, nextRead, journal, err := s.repo.updateLoadedWithJournal(s.graph, s.eventRead, s.journal, fn)
	if err != nil {
		return UpdateOutcome{}, err
	}
	s.graph = outcome.Graph
	s.eventRead = nextRead
	s.journal = journal
	s.fingerprint, err = captureLogFingerprint(s.repo.eventsPath, s.repo.journalPath, s.eventRead.validBytes)
	if err != nil {
		return UpdateOutcome{}, err
	}
	return outcome, nil
}

func (s *Session) applyMutation(id string, mutation taskMutation, agentID string) (mutationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyMutationLocked(id, mutation, agentID)
}

func (s *Session) applyMutationLocked(id string, mutation taskMutation, agentID string) (mutationOutcome, error) {
	var plan mutationPlan
	update, err := s.updateLoadedWithJournalLocked(func(graph *Graph) ([]Event, []JournalEntry, error) {
		var err error
		plan, err = planTaskMutation(graph, id, mutation, agentID)
		if err != nil {
			return nil, nil, err
		}
		if isAutomaticJournalKind(mutation.Kind) || mutation.MessageSet {
			return plan.Events, plan.Journal, nil
		}
		return plan.Events, nil, nil
	})
	if err != nil {
		return mutationOutcome{}, err
	}
	return mutationOutcome{Graph: update.Graph, ChangedFields: plan.ChangedFields, Journal: update.Journal}, nil
}

func planClaimReady(agentID string, chosenID *string) func(*Graph) ([]Event, []JournalEntry, error) {
	return func(graph *Graph) ([]Event, []JournalEntry, error) {
		ready := readyTasks(graph)
		if len(ready) == 0 {
			return nil, nil, nil
		}
		*chosenID = ready[0].ID
		mutation := taskMutation{Kind: "claim", State: stateDoing, StateSet: true, Claim: agentID, ClaimSet: true}
		events, _, err := buildMutationEvents(*chosenID, ready[0], mutation, agentID, time.Now().UTC())
		if err != nil {
			return nil, nil, err
		}
		return events, []JournalEntry{newJournalEntry(*chosenID, "claim", agentID, "", time.Now().UTC())}, nil
	}
}

func (s *Session) List(request ListRequest) (ListOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked(request)
}

func (s *Session) listLocked(request ListRequest) (ListOutcome, error) {
	if request.ReadyOnly && request.ShowAll {
		return ListOutcome{}, classified(ErrorUsage, errors.New("conflicting flags: --ready and --all"))
	}
	if err := s.ensureFreshLocked(); err != nil {
		return ListOutcome{}, err
	}
	if !request.OmitJournal {
		if err := s.ensureJournal(); err != nil {
			return ListOutcome{}, err
		}
	}
	graph := s.graph
	graph.prepareDerivedQueries()
	if request.EpicID != "" {
		epic := graph.Tasks[request.EpicID]
		if epic == nil || !graph.IsEpic(epic.ID) {
			return ListOutcome{}, classified(ErrorNotFound, fmt.Errorf("no such epic: %s", request.EpicID))
		}
	}
	all := collectNonContainerTasks(graph)
	outcome := ListOutcome{
		Options: request, Graph: graph,
		Roots:       buildListRoots(graph, request.ShowAll, request.ReadyOnly, request.EpicID),
		AllTasks:    all,
		ActiveTasks: filterActiveTasks(all), ReadyTasks: filterReadyTasks(all, graph),
	}
	if request.EpicID != "" {
		outcome.EpicChildren = collectEpicChildren(request.EpicID, graph)
		outcome.EpicReady = filterReadyTasks(outcome.EpicChildren, graph)
	}
	return outcome, nil
}

func (s *Session) Show(request ShowRequest) (ShowOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.showLocked(request)
}

func (s *Session) showLocked(request ShowRequest) (ShowOutcome, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return ShowOutcome{}, classified(ErrorUsage, errors.New("usage: ergo show <id>"))
	}
	if err := s.ensureFreshLocked(); err != nil {
		return ShowOutcome{}, err
	}
	if err := s.ensureJournal(); err != nil {
		return ShowOutcome{}, err
	}
	graph := s.graph
	if _, ok := graph.Tombstones[id]; ok {
		return ShowOutcome{}, classified(ErrorNotFound, prunedErr(id))
	}
	task := graph.Tasks[id]
	if task == nil {
		return ShowOutcome{}, classified(ErrorNotFound, fmt.Errorf("unknown task id %s", id))
	}
	var children []*Task
	if graph.IsEpic(id) {
		children = collectEpicChildren(id, graph)
	}
	return ShowOutcome{
		Graph:      graph,
		Task:       task,
		Children:   children,
		ProjectDir: s.repo.ProjectDir(),
		Journal:    s.journal,
	}, nil
}

func (s *Session) ShowBody(request ShowBodyRequest) (ShowBodyOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.showBodyLocked(request)
}

func (s *Session) showBodyLocked(request ShowBodyRequest) (ShowBodyOutcome, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return ShowBodyOutcome{}, classified(ErrorUsage, errors.New("usage: ergo show <id> --body"))
	}
	if err := s.ensureFreshLocked(); err != nil {
		return ShowBodyOutcome{}, err
	}
	graph := s.graph
	if _, ok := graph.Tombstones[id]; ok {
		return ShowBodyOutcome{}, classified(ErrorNotFound, prunedErr(id))
	}
	task := graph.Tasks[id]
	if task == nil {
		return ShowBodyOutcome{}, classified(ErrorNotFound, fmt.Errorf("unknown task id %s", id))
	}
	return ShowBodyOutcome{Body: task.Body}, nil
}

func (s *Session) CreateTask(request CreateTaskRequest) (CreateTaskOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createTaskLocked(request)
}

func (s *Session) createTaskLocked(request CreateTaskRequest) (CreateTaskOutcome, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return CreateTaskOutcome{}, classified(ErrorUsage, errors.New(NewTaskUsage))
	}
	var output createOutput
	update, err := s.updateLoadedWithJournalLocked(func(graph *Graph) ([]Event, []JournalEntry, error) {
		events, journal, planned, err := planCreateTask(graph, request.EpicID, title, request.Body, request.Draft)
		if err != nil {
			return nil, nil, err
		}
		output = planned
		return events, journal, nil
	})
	if err != nil {
		return CreateTaskOutcome{}, err
	}
	if update.Graph == nil || update.Graph.Tasks[output.ID] == nil {
		return CreateTaskOutcome{}, errors.New("internal error: missing created task")
	}
	return CreateTaskOutcome{ID: output.ID}, nil
}

func (s *Session) Result(request ResultRequest) (ResultOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resultLocked(request)
}

func (s *Session) resultLocked(request ResultRequest) (ResultOutcome, error) {
	id := strings.TrimSpace(request.ID)
	text := strings.TrimSpace(request.Text)
	if id == "" {
		return ResultOutcome{}, classified(ErrorUsage, errors.New(`usage: ergo result <id> "<text>" [--file <path>]`))
	}
	if err := validateResultSummary(text); err != nil {
		return ResultOutcome{}, classified(ErrorUsage, err)
	}
	filePath := strings.TrimSpace(request.FilePath)
	if request.FileSet && filePath == "" {
		return ResultOutcome{}, classified(ErrorUsage, errors.New("--file cannot be empty"))
	}
	_, err := s.updateLoadedWithJournalLocked(func(graph *Graph) ([]Event, []JournalEntry, error) {
		if _, pruned := graph.Tombstones[id]; pruned {
			return nil, nil, classified(ErrorNotFound, prunedErr(id))
		}
		task := graph.Tasks[id]
		if task == nil {
			return nil, nil, classified(ErrorNotFound, fmt.Errorf("unknown task id %s", id))
		}
		if graph.IsEpic(id) {
			return nil, nil, classified(ErrorConflict, errors.New("epics cannot have results"))
		}
		entry := newJournalEntry(id, "result", task.ClaimedBy, text, time.Now().UTC())
		if request.FileSet {
			cleanPath, err := validateResultPath(s.repo.ProjectDir(), filePath)
			if err != nil {
				return nil, nil, err
			}
			evidence, err := captureResultEvidence(s.repo.ProjectDir(), cleanPath)
			if err != nil {
				return nil, nil, err
			}
			filePath = cleanPath
			entry.File = &JournalFile{Path: cleanPath, SHA256: evidence.Sha256AtAttach, Mtime: evidence.MtimeAtAttach, GitCommitAtAttach: evidence.GitCommitAtAttach}
		}
		return nil, []JournalEntry{entry}, nil
	})
	if err != nil {
		return ResultOutcome{}, err
	}
	return ResultOutcome{TaskID: id, Text: text, FilePath: filePath}, nil
}

func (s *Session) Lifecycle(request LifecycleRequest) (LifecycleOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lifecycleLocked(request)
}

func (s *Session) lifecycleLocked(request LifecycleRequest) (LifecycleOutcome, error) {
	targetState, err := lifecycleTargetState(request.Kind)
	if err != nil {
		return LifecycleOutcome{}, classified(ErrorUsage, err)
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return LifecycleOutcome{}, classified(ErrorUsage, fmt.Errorf("usage: ergo %s <id> [-m <message>]", request.Kind))
	}
	message, messageSet, err := normalizeLifecycleMessages(request.Messages)
	if err != nil {
		return LifecycleOutcome{}, classified(ErrorUsage, err)
	}
	mutation := taskMutation{
		Kind: request.Kind, State: targetState, StateSet: true,
		MessageKind: request.Kind, MessageText: message, MessageSet: messageSet,
	}
	switch request.Kind {
	case "open":
		mutation.AllowedStates = []string{stateTodo, stateDraft, stateDoing, stateBlocked}
	case "done", "fail", "block":
		mutation.AllowedStates = []string{stateTodo, stateDoing, stateBlocked, stateDone, stateFailed, stateCanceled, stateError}
	case "cancel":
		mutation.AllowedStates = []string{stateTodo, stateDraft, stateDoing, stateBlocked, stateDone, stateFailed, stateCanceled, stateError}
	}
	mutated, err := s.applyMutationLocked(id, mutation, "")
	if err != nil {
		return LifecycleOutcome{}, err
	}
	outcome := LifecycleOutcome{
		Graph: mutated.Graph, Task: mutated.Graph.Tasks[id],
		ChangedFields: mutated.ChangedFields, MessageSet: messageSet && len(mutated.Journal) > 0,
	}
	if ready := readyTasks(mutated.Graph); len(ready) > 0 {
		outcome.Ready = ready[0]
	}
	return outcome, nil
}

func (s *Session) Claim(request ClaimRequest) (ClaimOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimLocked(request)
}

func (s *Session) claimLocked(request ClaimRequest) (ClaimOutcome, error) {
	agentID := strings.TrimSpace(request.AgentID)
	if agentID == "" {
		return ClaimOutcome{}, classified(ErrorUsage, errors.New("claim requires --agent"))
	}
	id := strings.TrimSpace(request.ID)
	if id != "" {
		mutation := taskMutation{
			Kind: "claim", State: stateDoing, StateSet: true,
			Claim: agentID, ClaimSet: true, ClaimConflict: true,
			AllowedStates: []string{stateTodo, stateDoing, stateDone, stateFailed, stateCanceled, stateError},
		}
		mutated, err := s.applyMutationLocked(id, mutation, agentID)
		if err != nil {
			return ClaimOutcome{}, err
		}
		task := mutated.Graph.Tasks[id]
		return ClaimOutcome{Graph: mutated.Graph, Task: task, ProjectDir: s.repo.ProjectDir(), Journal: mutated.Journal}, nil
	}

	var chosenID string
	update, err := s.updateLoadedWithJournalLocked(planClaimReady(agentID, &chosenID))
	if err != nil {
		return ClaimOutcome{}, err
	}
	if chosenID == "" {
		return ClaimOutcome{NoReady: true}, nil
	}
	task := update.Graph.Tasks[chosenID]
	if task == nil {
		return ClaimOutcome{}, classified(ErrorInternal, errors.New("internal error: missing chosen task"))
	}
	return ClaimOutcome{Graph: update.Graph, Task: task, ProjectDir: s.repo.ProjectDir(), Journal: update.Journal}, nil
}

func (s *Session) UpdateTitle(request UpdateTitleRequest) (UpdateTitleOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateTitleLocked(request)
}

func (s *Session) updateTitleLocked(request UpdateTitleRequest) (UpdateTitleOutcome, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return UpdateTitleOutcome{}, classified(ErrorUsage, errors.New("title cannot be empty"))
	}
	outcome, err := s.applyMutationLocked(request.ID, taskMutation{
		Kind: "title", Title: title, TitleSet: true,
	}, "")
	if err != nil {
		return UpdateTitleOutcome{}, err
	}
	return UpdateTitleOutcome{ID: request.ID, Title: title, Changed: len(outcome.ChangedFields) > 0}, nil
}

func (s *Session) UpdateBody(request UpdateBodyRequest) (UpdateBodyOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateBodyLocked(request)
}

func (s *Session) updateBodyLocked(request UpdateBodyRequest) (UpdateBodyOutcome, error) {
	outcome, err := s.applyMutationLocked(request.ID, taskMutation{
		Kind: "body", Body: string(request.Body), BodySet: true, BodyAppend: request.Append,
	}, "")
	if err != nil {
		return UpdateBodyOutcome{}, err
	}
	return UpdateBodyOutcome{
		ID: request.ID, Bytes: len(outcome.Graph.Tasks[request.ID].Body),
		Changed: len(outcome.ChangedFields) > 0,
	}, nil
}

func (s *Session) Move(request MoveRequest) (MoveOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.moveLocked(request)
}

func (s *Session) moveLocked(request MoveRequest) (MoveOutcome, error) {
	if request.ToRoot && request.DestinationID != "" {
		return MoveOutcome{}, classified(ErrorUsage, errors.New("move destination and --root are mutually exclusive"))
	}
	if !request.ToRoot && request.DestinationID == "" {
		return MoveOutcome{}, classified(ErrorUsage, errors.New("usage: ergo move <id> <epic-id> | ergo move <id> --root"))
	}
	outcome, err := s.applyMutationLocked(request.ID, taskMutation{
		Kind: "move", EpicID: request.DestinationID, EpicSet: true, ValidateMove: true,
	}, "")
	if err != nil {
		return MoveOutcome{}, err
	}
	return MoveOutcome{
		ID: request.ID, DestinationID: request.DestinationID, ToRoot: request.ToRoot,
		Changed: len(outcome.ChangedFields) > 0,
	}, nil
}

func (s *Session) Sequence(request SequenceRequest) (SequenceOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sequenceLocked(request)
}

func (s *Session) sequenceLocked(request SequenceRequest) (SequenceOutcome, error) {
	var changed []sequenceEdge
	_, err := s.updateLoadedLocked(func(graph *Graph) ([]Event, error) {
		events, planned, err := planLinkEvents(graph, request.EventType, buildSequenceEdges(request.IDs))
		if err != nil {
			return nil, err
		}
		changed = planned
		return events, nil
	})
	if err != nil {
		return SequenceOutcome{}, err
	}
	return SequenceOutcome{EventType: request.EventType, Edges: changed}, nil
}

func (s *Session) CreateEpic(request CreateEpicRequest) (CreateEpicOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createEpicLocked(request)
}

func (s *Session) createEpicLocked(request CreateEpicRequest) (CreateEpicOutcome, error) {
	tasks, err := ParseEpicFile(request.FilePath)
	if err != nil {
		return CreateEpicOutcome{}, err
	}
	var out bulkCreateOutput
	update, err := s.updateLoadedWithJournalLocked(func(graph *Graph) ([]Event, []JournalEntry, error) {
		events, journal, planned, err := planBulkCreate(graph, strings.TrimSpace(request.Title), request.Body, tasks, request.Draft)
		if err != nil {
			return nil, nil, err
		}
		out = planned
		return events, journal, nil
	})
	if err != nil {
		return CreateEpicOutcome{}, err
	}
	if update.Graph == nil {
		return CreateEpicOutcome{}, errors.New("internal error: missing created epic")
	}
	return out, nil
}

func (s *Session) Compact() (CompactOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compactLocked()
}

func (s *Session) compactLocked() (CompactOutcome, error) {
	if err := s.ensureFreshLocked(); err != nil {
		return CompactOutcome{}, err
	}
	outcome, err := s.repo.Compact()
	if err != nil {
		return outcome, err
	}
	if err := s.reload(); err != nil {
		return CompactOutcome{}, err
	}
	return outcome, nil
}

func (s *Session) Prune(request PruneRequest) (PruneOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pruneLocked(request)
}

func (s *Session) pruneLocked(request PruneRequest) (PruneOutcome, error) {
	dir := s.repo.Dir()
	var plan PrunePlan
	var err error
	if request.Confirm {
		plan, err = RunPruneApply(dir, s.repo.opts)
	} else {
		plan, err = RunPrunePlan(dir)
	}
	if err != nil {
		return PruneOutcome{}, err
	}
	if request.Confirm {
		if err := s.reload(); err != nil {
			return PruneOutcome{}, err
		}
	}
	return PruneOutcome{Confirmed: request.Confirm, Items: plan.Items, JournalEntries: plan.JournalEntries}, nil
}
