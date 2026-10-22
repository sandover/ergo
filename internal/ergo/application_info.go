// Purpose: Report the active Ergo installation and repository paths.
// Role: Application use case for the read-only `ergo info` command.
package ergo

import "path/filepath"

type InfoRequest struct {
	Executable string
	Version    string
}

type InfoOutcome struct {
	Executable string
	Version    string
	Project    string
	Backlog    string
	Journal    string
	Owner      string
	Storage    string
	Source     string
}

func (a *Application) Info(request InfoRequest) (InfoOutcome, error) {
	selected, err := resolveStorage(a.repository, false)
	if err != nil {
		return InfoOutcome{}, classifyRepositoryError(err)
	}
	backlog, err := selectEventsPath(selected.Dir)
	if err != nil {
		return InfoOutcome{}, classifyRepositoryError(err)
	}
	executable, err := filepath.Abs(request.Executable)
	if err != nil {
		return InfoOutcome{}, classifyRepositoryError(err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	return InfoOutcome{
		Executable: executable,
		Version:    request.Version,
		Project:    selected.Project,
		Backlog:    backlog,
		Journal:    filepath.Join(selected.Dir, journalFileName),
		Owner:      selected.Owner,
		Storage:    selected.Dir,
		Source:     selected.Source,
	}, nil
}
