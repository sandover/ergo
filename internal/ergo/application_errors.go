package ergo

import "errors"

var ErrCorruptRepository = errors.New("corrupt repository")

type corruptionError struct{ err error }

func (e *corruptionError) Error() string        { return e.err.Error() }
func (e *corruptionError) Unwrap() error        { return e.err }
func (e *corruptionError) Is(target error) bool { return target == ErrCorruptRepository }

// ErrorKind is the stable application-level classification consumed by CLI
// adapters. The wrapped error retains the precise human-readable message.
type ErrorKind string

const (
	ErrorUsage      ErrorKind = "usage"
	ErrorNotFound   ErrorKind = "not_found"
	ErrorConflict   ErrorKind = "conflict"
	ErrorBusy       ErrorKind = "busy"
	ErrorCorruption ErrorKind = "corruption"
	ErrorInternal   ErrorKind = "internal"
)

type ApplicationError struct {
	Kind ErrorKind
	Err  error
}

func (e *ApplicationError) Error() string { return e.Err.Error() }
func (e *ApplicationError) Unwrap() error { return e.Err }

func classified(kind ErrorKind, err error) error {
	if err == nil {
		return nil
	}
	var existing *ApplicationError
	if errors.As(err, &existing) {
		return err
	}
	return &ApplicationError{Kind: kind, Err: err}
}

// ApplicationErrorKind reports whether err has crossed the application
// boundary with an explicit classification.
func ApplicationErrorKind(err error) (ErrorKind, bool) {
	var applicationError *ApplicationError
	if errors.As(err, &applicationError) {
		return applicationError.Kind, true
	}
	return "", false
}

func classifyRepositoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrLockBusy):
		return classified(ErrorBusy, err)
	case errors.Is(err, ErrNoErgoDir):
		return classified(ErrorNotFound, err)
	case errors.Is(err, ErrCorruptRepository):
		return classified(ErrorCorruption, err)
	default:
		return classified(ErrorInternal, err)
	}
}
