package ergo

type QuickstartRequest struct{ Color bool }
type QuickstartOutcome struct{ Text string }

func (a *Application) Quickstart(request QuickstartRequest) QuickstartOutcome {
	return QuickstartOutcome{Text: QuickstartText(request.Color)}
}
