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
func (r *Repository) View(fn func(*Graph) error) error {
	if r == nil || r.eventsPath == "" {
		return errors.New("repository is not open")
	}
	return withLock(r.lockPath, r.opts, func() error {
		graph, err := r.load()
		if err != nil {
			return err
		}
		return fn(graph)
	})
}

// repositoryUpdate is the internal mutation boundary. The callback validates
// against the locked snapshot and returns the complete current event batch.
// Transaction envelopes and durability semantics are intentionally delegated
// to the later transaction-record change.
func (r *Repository) update(fn func(*Graph) ([]Event, error)) error {
	if r == nil || r.eventsPath == "" {
		return errors.New("repository is not open")
	}
	return withLock(r.lockPath, r.opts, func() error {
		graph, err := r.load()
		if err != nil {
			return err
		}
		events, err := fn(graph)
		if err != nil {
			return err
		}
		return r.append(events)
	})
}

func (r *Repository) load() (*Graph, error) {
	read, err := inspectEventLog(r.eventsPath)
	if err != nil {
		return nil, err
	}
	if read.snapshot != nil {
		return replayEventsOnto(read.snapshot, read.events)
	}
	return replayEvents(read.events)
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
