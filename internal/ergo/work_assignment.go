package ergo

import (
	"fmt"
	"io"
)

func RunClaim(id, agentID string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Claim(ClaimRequest{ID: id, AgentID: agentID})
	if err != nil {
		return err
	}
	RenderClaim(render.writer(), outcome, render.Color)
	return nil
}

func RunClaimOldestReady(agentID string, opts GlobalOptions, render RenderOptions) error {
	outcome, err := NewApplication(opts).Claim(ClaimRequest{AgentID: agentID})
	if err != nil {
		return err
	}
	RenderClaim(render.writer(), outcome, render.Color)
	return nil
}

func RenderClaim(w io.Writer, outcome ClaimOutcome, useColor bool) {
	if outcome.NoReady {
		fmt.Fprintln(w, "No ready ergo tasks.")
		return
	}
	task, graph, repoDir := outcome.Task, outcome.Graph, outcome.ProjectDir
	if task == nil {
		return
	}
	id := task.ID
	next := map[string]string{
		"done":    "ergo done " + id,
		"fail":    "ergo fail " + id,
		"block":   "ergo block " + id,
		"cancel":  "ergo cancel " + id,
		"release": "ergo release " + id,
	}
	printTaskDocument(w, task, graph, outcome.Journal, repoDir, useColor)
	writeGeneratedLine(w, "## Next", colorBold+colorCyan, useColor)
	fmt.Fprintln(w)
	writeNextCommand(w, next["done"], useColor)
	writeNextCommand(w, next["fail"], useColor)
	writeNextCommand(w, next["block"], useColor)
	writeNextCommand(w, next["cancel"], useColor)
	writeNextCommand(w, next["release"], useColor)
}

func writeNextCommand(w io.Writer, command string, useColor bool) {
	fmt.Fprint(w, "- `")
	writeGenerated(w, command, colorGreen, useColor)
	fmt.Fprintln(w, "`")
}

type sequenceEdge struct {
	FromID string
	ToID   string
}

func buildSequenceEdges(order []string) []sequenceEdge {
	if len(order) < 2 {
		return nil
	}
	edges := make([]sequenceEdge, 0, len(order)-1)
	for i := 0; i < len(order)-1; i++ {
		edges = append(edges, sequenceEdge{
			FromID: order[i+1],
			ToID:   order[i],
		})
	}
	return edges
}

func RunSequence(args []string, opts GlobalOptions, render RenderOptions) error {
	return runSequenceChange("sequence", "link", args, opts, render.writer())
}

func RunUnsequence(args []string, opts GlobalOptions, render RenderOptions) error {
	return runSequenceChange("unsequence", "unlink", args, opts, render.writer())
}

func runSequenceChange(command, eventType string, args []string, opts GlobalOptions, w io.Writer) error {
	outcome, err := NewApplication(opts).Sequence(SequenceRequest{
		Command: command, EventType: eventType, IDs: args,
	})
	if err != nil {
		return err
	}
	RenderSequence(w, outcome)
	return nil
}

func RenderSequence(w io.Writer, outcome SequenceOutcome) {
	if len(outcome.Edges) == 0 {
		fmt.Fprintln(w, "No dependency changes.")
		return
	}
	for _, edge := range outcome.Edges {
		if outcome.EventType == "unlink" {
			fmt.Fprintf(w, "%s no longer depends on %s\n", edge.FromID, edge.ToID)
			continue
		}
		fmt.Fprintf(w, "%s depends on %s\n", edge.FromID, edge.ToID)
	}
}
