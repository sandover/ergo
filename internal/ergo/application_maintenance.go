package ergo

type CompactOutcomeResult = CompactOutcome

func (a *Application) Compact() (CompactOutcomeResult, error) {
	var repository Repository
	if err := repository.Open(a.repository); err != nil {
		return CompactOutcome{}, classifyRepositoryError(err)
	}
	outcome, err := repository.Compact()
	return outcome, classifyRepositoryError(err)
}

type WhereOutcome struct{ Path string }

func (a *Application) Where() (WhereOutcome, error) {
	dir, err := ergoDir(a.repository)
	if err != nil {
		return WhereOutcome{}, classifyRepositoryError(err)
	}
	return WhereOutcome{Path: dir}, nil
}

type PruneRequest struct{ Confirm bool }
type PruneOutcome struct {
	Confirmed      bool
	Items          []PruneItem
	JournalEntries int
}

func (a *Application) Prune(request PruneRequest) (PruneOutcome, error) {
	dir, err := ergoDir(a.repository)
	if err != nil {
		return PruneOutcome{}, classifyRepositoryError(err)
	}
	var plan PrunePlan
	if request.Confirm {
		plan, err = RunPruneApply(dir, a.repository)
	} else {
		plan, err = RunPrunePlan(dir)
	}
	if err != nil {
		return PruneOutcome{}, classifyRepositoryError(err)
	}
	return PruneOutcome{Confirmed: request.Confirm, Items: plan.Items, JournalEntries: plan.JournalEntries}, nil
}
