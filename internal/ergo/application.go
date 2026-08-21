package ergo

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Application is Ergo's process-independent use-case boundary. It owns only
// repository location; request-specific context belongs on each request.
type Application struct {
	repository RepositoryOptions
}

func NewApplication(options RepositoryOptions) *Application {
	return &Application{repository: options}
}

// WithRepository returns an independent application bound to repository
// options owned by one CLI command tree.
func (a *Application) WithRepository(options RepositoryOptions) *Application {
	application := *a
	if options.StartDir != "" {
		application.repository = options
	}
	return &application
}

type VersionRequest struct {
	Version string
}

type VersionOutcome struct {
	Version string
}

func (a *Application) Version(request VersionRequest) VersionOutcome {
	return VersionOutcome(request)
}

type CreateTaskRequest struct {
	Title  string
	EpicID string
	Body   string
}

type CreateTaskOutcome struct {
	ID string
}

func (a *Application) CreateTask(request CreateTaskRequest) (CreateTaskOutcome, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return CreateTaskOutcome{}, classified(ErrorUsage, errors.New(NewTaskUsage))
	}
	dir, err := ergoDir(a.repository)
	if err != nil {
		return CreateTaskOutcome{}, classifyRepositoryError(err)
	}
	created, err := createTask(dir, a.repository, request.EpicID, title, request.Body)
	if err != nil {
		return CreateTaskOutcome{}, classifyRepositoryError(err)
	}
	return CreateTaskOutcome{ID: created.ID}, nil
}

type ShowRequest struct {
	ID string
}

type ShowOutcome struct {
	Graph      *Graph
	Task       *Task
	Children   []*Task
	ProjectDir string
	Journal    []JournalEntry
}

// ShowBodyRequest selects the lossless body projection of one task or epic.
type ShowBodyRequest struct {
	ID string
}

// ShowBodyOutcome contains only the stored body bytes represented as text.
type ShowBodyOutcome struct {
	Body string
}

func (a *Application) Show(request ShowRequest) (ShowOutcome, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return ShowOutcome{}, classified(ErrorUsage, errors.New("usage: ergo show <id>"))
	}
	var repository Repository
	if err := repository.Open(a.repository); err != nil {
		return ShowOutcome{}, classifyRepositoryError(err)
	}
	graph, journal, err := repository.ViewWithJournal()
	if err != nil {
		return ShowOutcome{}, classifyRepositoryError(err)
	}
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
		ProjectDir: repository.ProjectDir(),
		Journal:    journal,
	}, nil
}

func (a *Application) ShowBody(request ShowBodyRequest) (ShowBodyOutcome, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return ShowBodyOutcome{}, classified(ErrorUsage, errors.New("usage: ergo show <id> --body"))
	}
	var repository Repository
	if err := repository.Open(a.repository); err != nil {
		return ShowBodyOutcome{}, classifyRepositoryError(err)
	}
	graph, err := repository.View()
	if err != nil {
		return ShowBodyOutcome{}, classifyRepositoryError(err)
	}
	if _, ok := graph.Tombstones[id]; ok {
		return ShowBodyOutcome{}, classified(ErrorNotFound, prunedErr(id))
	}
	task := graph.Tasks[id]
	if task == nil {
		return ShowBodyOutcome{}, classified(ErrorNotFound, fmt.Errorf("unknown task id %s", id))
	}
	return ShowBodyOutcome{Body: task.Body}, nil
}

type LifecycleRequest struct {
	Kind     string
	ID       string
	Messages []string
}

type LifecycleOutcome struct {
	Graph         *Graph
	Task          *Task
	ChangedFields []string
	MessageSet    bool
	Ready         *Task
}

type ResultRequest struct {
	ID, Text, FilePath string
	FileSet            bool
}

type ResultOutcome struct {
	TaskID, Text, FilePath string
}

func (a *Application) Result(request ResultRequest) (ResultOutcome, error) {
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
	var repository Repository
	if err := repository.Open(a.repository); err != nil {
		return ResultOutcome{}, classifyRepositoryError(err)
	}
	_, err := repository.UpdateWithJournal(func(graph *Graph) ([]Event, []JournalEntry, error) {
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
			cleanPath, err := validateResultPath(repository.ProjectDir(), filePath)
			if err != nil {
				return nil, nil, err
			}
			evidence, err := captureResultEvidence(repository.ProjectDir(), cleanPath)
			if err != nil {
				return nil, nil, err
			}
			filePath = cleanPath
			entry.File = &JournalFile{Path: cleanPath, SHA256: evidence.Sha256AtAttach, Mtime: evidence.MtimeAtAttach, GitCommitAtAttach: evidence.GitCommitAtAttach}
		}
		return nil, []JournalEntry{entry}, nil
	})
	if err != nil {
		return ResultOutcome{}, classifyRepositoryError(err)
	}
	return ResultOutcome{TaskID: id, Text: text, FilePath: filePath}, nil
}

func (a *Application) Lifecycle(request LifecycleRequest) (LifecycleOutcome, error) {
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
	dir, err := ergoDir(a.repository)
	if err != nil {
		return LifecycleOutcome{}, classifyRepositoryError(err)
	}
	mutation := taskMutation{
		Kind: request.Kind, State: targetState, StateSet: true,
		MessageKind: request.Kind, MessageText: message, MessageSet: messageSet,
	}
	if request.Kind == "release" {
		mutation.AllowedStates = []string{stateTodo, stateDoing, stateBlocked, stateError}
	}
	mutated, err := applyTaskMutation(dir, a.repository, id, mutation, "")
	if err != nil {
		return LifecycleOutcome{}, classifyRepositoryError(err)
	}
	outcome := LifecycleOutcome{
		Graph: mutated.Graph, Task: mutated.Graph.Tasks[id],
		ChangedFields: mutated.ChangedFields, MessageSet: messageSet,
	}
	if ready := readyTasks(mutated.Graph); len(ready) > 0 {
		outcome.Ready = ready[0]
	}
	return outcome, nil
}

type ClaimRequest struct {
	ID      string
	AgentID string
}

type ClaimOutcome struct {
	Graph      *Graph
	Task       *Task
	ProjectDir string
	NoReady    bool
	Journal    []JournalEntry
}

func (a *Application) Claim(request ClaimRequest) (ClaimOutcome, error) {
	agentID := strings.TrimSpace(request.AgentID)
	if agentID == "" {
		return ClaimOutcome{}, classified(ErrorUsage, errors.New("claim requires --agent"))
	}
	dir, err := ergoDir(a.repository)
	if err != nil {
		return ClaimOutcome{}, classifyRepositoryError(err)
	}
	id := strings.TrimSpace(request.ID)
	if id != "" {
		mutation := taskMutation{
			Kind: "claim", State: stateDoing, StateSet: true,
			Claim: agentID, ClaimSet: true, ClaimConflict: true,
			AllowedStates: []string{stateTodo, stateDoing, stateBlocked, stateDone, stateFailed, stateCanceled, stateError},
		}
		mutated, err := applyTaskMutation(dir, a.repository, id, mutation, agentID)
		if err != nil {
			return ClaimOutcome{}, classifyRepositoryError(err)
		}
		task := mutated.Graph.Tasks[id]
		return ClaimOutcome{Graph: mutated.Graph, Task: task, ProjectDir: filepath.Dir(dir), Journal: mutated.Journal}, nil
	}

	var repository Repository
	if err := repository.openAt(dir, a.repository, systemRepositoryIO()); err != nil {
		return ClaimOutcome{}, classifyRepositoryError(err)
	}
	var chosenID string
	update, err := repository.UpdateWithJournal(func(graph *Graph) ([]Event, []JournalEntry, error) {
		ready := readyTasks(graph)
		if len(ready) == 0 {
			return nil, nil, nil
		}
		chosenID = ready[0].ID
		mutation := taskMutation{Kind: "claim", State: stateDoing, StateSet: true, Claim: agentID, ClaimSet: true}
		events, _, err := buildMutationEvents(chosenID, ready[0], mutation, agentID, time.Now().UTC())
		if err != nil {
			return nil, nil, err
		}
		return events, []JournalEntry{newJournalEntry(chosenID, "claim", agentID, "", time.Now().UTC())}, nil
	})
	if err != nil {
		return ClaimOutcome{}, classifyRepositoryError(err)
	}
	if chosenID == "" {
		return ClaimOutcome{NoReady: true}, nil
	}
	task := update.Graph.Tasks[chosenID]
	if task == nil {
		return ClaimOutcome{}, classified(ErrorInternal, errors.New("internal error: missing chosen task"))
	}
	return ClaimOutcome{Graph: update.Graph, Task: task, ProjectDir: repository.ProjectDir(), Journal: update.Journal}, nil
}
