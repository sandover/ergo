package ergo

import (
	"errors"
	"os"
	"strings"
)

type InitializeRequest struct{ Dir string }
type InitializeResult = InitializeOutcome

func (a *Application) Initialize(request InitializeRequest) (InitializeResult, error) {
	dir := request.Dir
	if dir == "" {
		dir = "."
	}
	outcome, err := InitializeRepository(dir)
	return outcome, classifyRepositoryError(err)
}

type CreateEpicRequest struct {
	Title, FilePath, Body string
}
type CreateEpicOutcome = bulkCreateOutput

func (a *Application) CreateEpic(request CreateEpicRequest) (CreateEpicOutcome, error) {
	if strings.TrimSpace(request.FilePath) == "" || strings.TrimSpace(request.Title) == "" {
		return CreateEpicOutcome{}, classified(ErrorUsage, errors.New(NewEpicUsage))
	}
	tasks, err := ParseEpicFile(request.FilePath)
	if err != nil {
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return CreateEpicOutcome{}, classifyRepositoryError(err)
		}
		return CreateEpicOutcome{}, classified(ErrorUsage, err)
	}
	dir, err := ergoDir(a.repository)
	if err != nil {
		return CreateEpicOutcome{}, classifyRepositoryError(err)
	}
	outcome, err := runBulkCreate(dir, a.repository, strings.TrimSpace(request.Title), request.Body, tasks)
	if err != nil {
		return CreateEpicOutcome{}, classifyRepositoryError(err)
	}
	return outcome, nil
}
