// Purpose: Define application requests and outcomes for focused task changes.
// Exports: title, body, and move request/outcome types and Application methods.
// Role: Validate public inputs and map them onto the shared locked mutation path.
// Invariants: titles are nonblank; body bytes remain literal.
// Invariants: body append is resolved against repository state under the lock.
package ergo

type UpdateTitleRequest struct{ ID, Title string }
type UpdateTitleOutcome struct {
	ID, Title string
	Changed   bool
}

func (a *Application) UpdateTitle(request UpdateTitleRequest) (UpdateTitleOutcome, error) {
	session, err := a.OpenSession()
	if err != nil {
		return UpdateTitleOutcome{}, err
	}
	defer session.Close()
	return session.UpdateTitle(request)
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
	session, err := a.OpenSession()
	if err != nil {
		return UpdateBodyOutcome{}, err
	}
	defer session.Close()
	return session.UpdateBody(request)
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
	session, err := a.OpenSession()
	if err != nil {
		return MoveOutcome{}, err
	}
	defer session.Close()
	return session.Move(request)
}
