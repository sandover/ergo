package ergo

import (
	"os"
	"path/filepath"
)

type CompactOutcomeResult = CompactOutcome

func (a *Application) Compact() (CompactOutcomeResult, error) {
	session, err := a.OpenSession()
	if err != nil {
		return CompactOutcome{}, err
	}
	defer session.Close()
	return session.Compact()
}

type WhereOutcome struct{ Path string }

func (a *Application) Where() (WhereOutcome, error) {
	start, err := os.Getwd()
	if err != nil {
		return WhereOutcome{}, classifyRepositoryError(err)
	}
	if a.repository.StartDir != "" {
		start = a.repository.StartDir
	}
	dir, err := resolveErgoDir(start)
	if err != nil {
		return WhereOutcome{}, classifyRepositoryError(err)
	}
	path, err := filepath.Abs(dir)
	if err != nil {
		return WhereOutcome{}, classifyRepositoryError(err)
	}
	return WhereOutcome{Path: path}, nil
}

type PruneRequest struct{ Confirm bool }
type PruneOutcome struct {
	Confirmed      bool
	Items          []PruneItem
	JournalEntries int
}

func (a *Application) Prune(request PruneRequest) (PruneOutcome, error) {
	session, err := a.OpenSession()
	if err != nil {
		return PruneOutcome{}, err
	}
	defer session.Close()
	return session.Prune(request)
}
