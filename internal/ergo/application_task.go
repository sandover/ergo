// Purpose: Define application requests and outcomes for focused task changes.
// Exports: title, body, and move request/outcome types and Application methods.
// Role: Validate public inputs and map them onto the shared locked mutation path.
// Invariants: titles are nonblank; body bytes remain literal.
// Invariants: body append is resolved against repository state under the lock.
package ergo

import (
	"errors"
	"strings"
)

type UpdateTitleRequest struct{ ID, Title string }
type UpdateTitleOutcome struct {
	ID, Title string
	Changed   bool
}

func (a *Application) UpdateTitle(request UpdateTitleRequest) (UpdateTitleOutcome, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return UpdateTitleOutcome{}, classified(ErrorUsage, errors.New("title cannot be empty"))
	}
	dir, err := ergoDir(a.repository)
	if err != nil {
		return UpdateTitleOutcome{}, classifyRepositoryError(err)
	}
	outcome, err := applyTaskMutation(dir, a.repository, request.ID, taskMutation{
		Kind: "title", Title: title, TitleSet: true,
	}, "")
	if err != nil {
		return UpdateTitleOutcome{}, classifyRepositoryError(err)
	}
	return UpdateTitleOutcome{ID: request.ID, Title: title, Changed: len(outcome.ChangedFields) > 0}, nil
}

type UpdateBodyRequest struct {
	ID     string
	Body   []byte
	Append bool
}
type UpdateBodyOutcome struct {
	ID      string
	Bytes   int
	Changed bool
}

func (a *Application) UpdateBody(request UpdateBodyRequest) (UpdateBodyOutcome, error) {
	dir, err := ergoDir(a.repository)
	if err != nil {
		return UpdateBodyOutcome{}, classifyRepositoryError(err)
	}
	outcome, err := applyTaskMutation(dir, a.repository, request.ID, taskMutation{
		Kind: "body", Body: string(request.Body), BodySet: true, BodyAppend: request.Append,
	}, "")
	if err != nil {
		return UpdateBodyOutcome{}, classifyRepositoryError(err)
	}
	return UpdateBodyOutcome{
		ID: request.ID, Bytes: len(outcome.Graph.Tasks[request.ID].Body),
		Changed: len(outcome.ChangedFields) > 0,
	}, nil
}

type MoveRequest struct {
	ID, DestinationID string
	ToRoot            bool
}
type MoveOutcome struct {
	ID, DestinationID string
	ToRoot, Changed   bool
}

func (a *Application) Move(request MoveRequest) (MoveOutcome, error) {
	if request.ToRoot && request.DestinationID != "" {
		return MoveOutcome{}, classified(ErrorUsage, errors.New("move destination and --root are mutually exclusive"))
	}
	if !request.ToRoot && request.DestinationID == "" {
		return MoveOutcome{}, classified(ErrorUsage, errors.New("usage: ergo move <id> <epic-id> | ergo move <id> --root"))
	}
	dir, err := ergoDir(a.repository)
	if err != nil {
		return MoveOutcome{}, classifyRepositoryError(err)
	}
	outcome, err := applyTaskMutation(dir, a.repository, request.ID, taskMutation{
		Kind: "move", EpicID: request.DestinationID, EpicSet: true, ValidateMove: true,
	}, "")
	if err != nil {
		return MoveOutcome{}, classifyRepositoryError(err)
	}
	return MoveOutcome{
		ID: request.ID, DestinationID: request.DestinationID, ToRoot: request.ToRoot,
		Changed: len(outcome.ChangedFields) > 0,
	}, nil
}
