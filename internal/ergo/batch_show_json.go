package ergo

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	batchShowJSONVersion = 1
	batchShowMaxIDs      = 256
)

type BatchShowRequest struct {
	IDs      []string `json:"ids"`
	WithBody bool     `json:"with_body"`
}

type BatchShowOutcome struct {
	Graph    *Graph
	Tasks    map[string]taskJSONProjection
	Missing  []string
	WithBody bool
}

type batchShowJSONDocument struct {
	Version int                              `json:"version"`
	Tasks   map[string]taskJSONProjection `json:"tasks"`
}

func (a *Application) BatchShow(request BatchShowRequest) (BatchShowOutcome, error) {
	session, err := a.OpenSession()
	if err != nil {
		return BatchShowOutcome{}, err
	}
	defer session.Close()
	return session.BatchShow(request)
}

func (s *Session) BatchShow(request BatchShowRequest) (BatchShowOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batchShowLocked(request)
}

func (s *Session) batchShowLocked(request BatchShowRequest) (BatchShowOutcome, error) {
	ids, err := normalizeBatchShowIDs(request.IDs)
	if err != nil {
		return BatchShowOutcome{}, err
	}
	if err := s.ensureFreshLocked(); err != nil {
		return BatchShowOutcome{}, err
	}
	withBody := request.WithBody
	if !withBody {
		withBody = true
	}
	graph := s.graph
	tasks := make(map[string]taskJSONProjection, len(ids))
	var missing []string
	for _, id := range ids {
		if _, pruned := graph.Tombstones[id]; pruned {
			missing = append(missing, id)
			continue
		}
		task := graph.Tasks[id]
		if task == nil {
			missing = append(missing, id)
			continue
		}
		tasks[id] = projectTaskJSON(graph, task, withBody)
	}
	return BatchShowOutcome{
		Graph:    graph,
		Tasks:    tasks,
		Missing:  missing,
		WithBody: withBody,
	}, nil
}

func normalizeBatchShowIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, classified(ErrorUsage, fmt.Errorf("usage: ergo batch-show [--json] <id> [<id>…]"))
	}
	if len(ids) > batchShowMaxIDs {
		return nil, classified(ErrorUsage, fmt.Errorf("batch-show accepts at most %d ids", batchShowMaxIDs))
	}
	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return nil, classified(ErrorUsage, fmt.Errorf("usage: ergo batch-show [--json] <id> [<id>…]"))
	}
	return normalized, nil
}

func RenderBatchShowJSON(w io.Writer, outcome BatchShowOutcome) error {
	if outcome.Tasks == nil {
		outcome.Tasks = map[string]taskJSONProjection{}
	}
	document := batchShowJSONDocument{
		Version: batchShowJSONVersion,
		Tasks:   outcome.Tasks,
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(document)
}

func FormatBatchShowWarnings(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	lines := make([]string, len(missing))
	for i, id := range missing {
		lines[i] = formatBatchShowMissingWarning(id)
	}
	return strings.Join(lines, "\n") + "\n"
}
