package ergo

import (
	"errors"
	"strings"
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
	Draft  bool
}

type CreateTaskOutcome struct {
	ID string
}

func (a *Application) CreateTask(request CreateTaskRequest) (CreateTaskOutcome, error) {
	session, err := a.OpenSession()
	if err != nil {
		return CreateTaskOutcome{}, err
	}
	defer session.Close()
	return session.CreateTask(request)
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
	session, err := a.OpenSession()
	if err != nil {
		return ShowOutcome{}, err
	}
	defer session.Close()
	return session.Show(request)
}

func (a *Application) ShowBody(request ShowBodyRequest) (ShowBodyOutcome, error) {
	session, err := a.OpenSession()
	if err != nil {
		return ShowBodyOutcome{}, err
	}
	defer session.Close()
	return session.ShowBody(request)
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
	session, err := a.OpenSession()
	if err != nil {
		return ResultOutcome{}, err
	}
	defer session.Close()
	return session.Result(request)
}

func (a *Application) Lifecycle(request LifecycleRequest) (LifecycleOutcome, error) {
	session, err := a.OpenSession()
	if err != nil {
		return LifecycleOutcome{}, err
	}
	defer session.Close()
	return session.Lifecycle(request)
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
	if strings.TrimSpace(request.AgentID) == "" {
		return ClaimOutcome{}, classified(ErrorUsage, errors.New("claim requires --agent"))
	}
	session, err := a.OpenSession()
	if err != nil {
		return ClaimOutcome{}, err
	}
	defer session.Close()
	return session.Claim(request)
}
