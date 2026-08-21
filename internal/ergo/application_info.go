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
}

func (a *Application) Info(request InfoRequest) (InfoOutcome, error) {
	where, err := a.Where()
	if err != nil {
		return InfoOutcome{}, err
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
		Project:    filepath.Dir(where.Path),
		Backlog:    filepath.Join(where.Path, backlogFileName),
		Journal:    filepath.Join(where.Path, journalFileName),
	}, nil
}
