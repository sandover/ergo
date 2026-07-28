package ergo

// Compatibility helpers keep low-level persistence access repository-owned
// while older focused tests transition to the Repository API.
func loadGraph(dir string) (*Graph, error) {
	return repositoryLoadGraph(dir)
}

func appendEvents(path string, events []Event) error {
	return repositoryAppendEvents(path, events)
}

func withLock(path string, opts GlobalOptions, fn func() error) error {
	return repositoryWithLock(path, opts, fn)
}

func (r *Repository) update(fn func(*Graph) ([]Event, error)) error {
	_, err := r.Update(fn)
	return err
}
