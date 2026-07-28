// Purpose: Own repository discovery, log selection, locking, and graph loading.
// Exports: Repository.
// Role: Stable persistence boundary used by application commands.
// Invariants: A repository has exactly one selected supported log path.
// Notes: The update seam retains standalone events until transaction records land.
package ergo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Repository is an opened Ergo repository.
//
// Its paths are selected once at open time. Each View or update acquires the
// repository lock before reading the selected log.
type Repository struct {
	dir        string
	eventsPath string
	lockPath   string
	opts       GlobalOptions
	io         repositoryIO
}

type UpdateOutcome struct {
	Graph *Graph
}

type CompactOutcome struct {
	Path            string
	SourceRecords   int
	SnapshotRecords int
}

type InitializeOutcome struct {
	Path   string
	Status string
}

func InitializeRepository(dir string) (InitializeOutcome, error) {
	target := filepath.Join(dir, dataDirName)
	_, dirErr := os.Stat(target)
	eventsPath, err := selectEventsPath(target)
	if err != nil {
		return InitializeOutcome{}, err
	}
	_, eventsErr := os.Stat(eventsPath)
	lockPath := filepath.Join(target, "lock")
	_, lockErr := os.Stat(lockPath)
	if err := os.MkdirAll(target, 0755); err != nil {
		return InitializeOutcome{}, err
	}
	if err := ensureFileExists(eventsPath, 0644); err != nil {
		return InitializeOutcome{}, err
	}
	if err := ensureFileExists(lockPath, 0644); err != nil {
		return InitializeOutcome{}, err
	}
	resolved, err := filepath.Abs(target)
	if err != nil {
		return InitializeOutcome{}, err
	}
	status := "existing"
	if os.IsNotExist(dirErr) {
		status = "initialized"
	} else if os.IsNotExist(eventsErr) || os.IsNotExist(lockErr) {
		status = "repaired"
	}
	return InitializeOutcome{Path: resolved, Status: status}, nil
}

// repositoryIO contains the deliberately small set of write operations that
// persistence tests must be able to fail deterministically. Production code
// uses the operating-system implementations below.
type repositoryIO struct {
	openFile  func(string, int, os.FileMode) (*os.File, error)
	write     func(*os.File, []byte) (int, error)
	sync      func(*os.File) error
	postWrite func() error
}

func systemRepositoryIO() repositoryIO {
	return repositoryIO{
		openFile: os.OpenFile,
		write: func(file *os.File, data []byte) (int, error) {
			return file.Write(data)
		},
		sync: func(file *os.File) error {
			return file.Sync()
		},
		postWrite: func() error {
			return nil
		},
	}
}

// Open discovers and opens the Ergo repository selected by opts.
func (r *Repository) Open(opts GlobalOptions) error {
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	return r.openAt(dir, opts, systemRepositoryIO())
}

func (r *Repository) openAt(dir string, opts GlobalOptions, io repositoryIO) error {
	eventsPath, err := selectEventsPath(dir)
	if err != nil {
		return err
	}
	if io.openFile == nil || io.write == nil || io.sync == nil || io.postWrite == nil {
		return errors.New("repository I/O is incomplete")
	}
	r.dir = dir
	r.eventsPath = eventsPath
	r.lockPath = filepath.Join(dir, "lock")
	r.opts = opts
	r.io = io
	return nil
}

// View loads a coherent snapshot while holding the repository lock.
func (r *Repository) View() (*Graph, error) {
	if r == nil || r.eventsPath == "" {
		return nil, errors.New("repository is not open")
	}
	var graph *Graph
	err := withLock(r.lockPath, r.opts, func() error {
		var err error
		graph, err = r.load()
		return err
	})
	return graph, err
}

// Update is the mutation boundary. The callback builds a complete event batch
// against the locked snapshot; the pure reducer validates its resulting graph
// before the transaction is appended.
func (r *Repository) Update(fn func(*Graph) ([]Event, error)) (UpdateOutcome, error) {
	if r == nil || r.eventsPath == "" {
		return UpdateOutcome{}, errors.New("repository is not open")
	}
	var outcome UpdateOutcome
	err := withLock(r.lockPath, r.opts, func() error {
		graph, err := r.load()
		if err != nil {
			return err
		}
		base := cloneGraph(graph)
		events, err := fn(graph)
		if err != nil {
			return err
		}
		candidate, err := applyTransaction(base, events)
		if err != nil {
			return err
		}
		if err := r.append(events); err != nil {
			return err
		}
		outcome.Graph = candidate
		return nil
	})
	return outcome, err
}

func (r *Repository) Compact() (CompactOutcome, error) {
	if r == nil || r.eventsPath == "" {
		return CompactOutcome{}, errors.New("repository is not open")
	}
	var outcome CompactOutcome
	err := withLock(r.lockPath, r.opts, func() error {
		read, err := inspectEventLog(r.eventsPath)
		if err != nil {
			return err
		}
		graph, err := r.load()
		if err != nil {
			return err
		}
		data, stats, err := marshalSnapshot(graph)
		if err != nil {
			return err
		}
		path, err := filepath.Abs(r.eventsPath)
		if err != nil {
			return err
		}
		if err := replaceLogAtomically(r.eventsPath, data); err != nil {
			return err
		}
		outcome = CompactOutcome{Path: path, SourceRecords: read.recordCount, SnapshotRecords: stats.Records}
		return nil
	})
	return outcome, err
}

func (r *Repository) Dir() string        { return r.dir }
func (r *Repository) ProjectDir() string { return filepath.Dir(r.dir) }

func (r *Repository) load() (*Graph, error) {
	read, err := inspectEventLog(r.eventsPath)
	if err != nil {
		var pathError *os.PathError
		if !errors.As(err, &pathError) {
			return nil, &corruptionError{err: err}
		}
		return nil, err
	}
	if read.snapshot != nil {
		graph, err := replayEventsOnto(read.snapshot, read.events)
		if err != nil {
			return nil, &corruptionError{err: err}
		}
		return graph, nil
	}
	graph, err := replayEvents(read.events)
	if err != nil {
		return nil, &corruptionError{err: err}
	}
	return graph, nil
}

func (r *Repository) append(events []Event) error {
	data, err := marshalTransaction(events)
	if err != nil || len(data) == 0 {
		return err
	}
	return appendTransaction(r.eventsPath, data, r.io)
}

func appendTransaction(path string, transaction []byte, io repositoryIO) error {
	read, err := inspectEventLog(path)
	if err != nil {
		return err
	}
	file, err := io.openFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if read.truncatedTail {
		if err := file.Truncate(read.validBytes); err != nil {
			return fmt.Errorf("repair incomplete transaction tail: %w", err)
		}
	}
	if _, err := file.Seek(0, 2); err != nil {
		return err
	}
	if read.needsSeparator && !read.truncatedTail {
		transaction = append([]byte{'\n'}, transaction...)
	}
	if err := writeAllWith(file, transaction, io.write); err != nil {
		return err
	}
	if err := io.postWrite(); err != nil {
		return err
	}
	if err := io.sync(file); err != nil {
		return fmt.Errorf("sync transaction record: %w", err)
	}
	return nil
}

func selectEventsPath(dir string) (string, error) {
	names := []string{backlogFileName, plansFileName, oldEventsFileName}
	var existing []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		switch {
		case err == nil:
			if info.IsDir() {
				return "", fmt.Errorf("Ergo backlog path is a directory: %s", path)
			}
			existing = append(existing, path)
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return "", fmt.Errorf("cannot inspect Ergo backlog %s: %w", path, err)
		}
	}
	if len(existing) > 1 {
		return "", fmt.Errorf(
			"multiple Ergo backlog files exist: %s; reconcile them so exactly one of %s remains",
			strings.Join(existing, ", "),
			strings.Join(names, ", "),
		)
	}
	if len(existing) == 1 {
		return existing[0], nil
	}
	return filepath.Join(dir, backlogFileName), nil
}
