// Purpose: Render the list use case as a small versioned integration document.
// Role: Stable machine projection beside the human-readable tree renderer.
package ergo

import (
	"encoding/json"
	"io"
)

const (
	listJSONVersion         = 1
	listJSONVersionEnriched   = 2
)

type listJSONDocument struct {
	Version int            `json:"version"`
	Items   []listJSONItem `json:"items"`
}

type listJSONDocumentV2 struct {
	Version int              `json:"version"`
	Items   []map[string]any `json:"items"`
}

type listJSONItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Kind   string `json:"kind"`
	State  string `json:"state,omitempty"`
	Ready  *bool  `json:"ready,omitempty"`
	EpicID string `json:"epic_id,omitempty"`
}

// RenderListJSON writes the filtered list outcome without terminal presentation
// fields. Items follow the same preorder as the readable tree.
func RenderListJSON(w io.Writer, outcome ListOutcome) error {
	if outcome.Options.JSONWithMeta || outcome.Options.JSONWithBody {
		return renderListJSONV2(w, outcome)
	}
	document := listJSONDocument{
		Version: listJSONVersion,
		Items:   make([]listJSONItem, 0),
	}
	appendNodesAsJSON(&document.Items, outcome.Roots, outcome.Graph)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(document)
}

func renderListJSONV2(w io.Writer, outcome ListOutcome) error {
	document := listJSONDocumentV2{
		Version: listJSONVersionEnriched,
		Items:   make([]map[string]any, 0),
	}
	appendNodesAsJSONV2(&document.Items, outcome.Roots, outcome.Graph, outcome.Options)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(document)
}

func appendNodesAsJSON(items *[]listJSONItem, nodes []*treeNode, graph *Graph) {
	for _, node := range nodes {
		if node == nil || node.task == nil {
			continue
		}
		item := listJSONItem{
			ID:    node.task.ID,
			Title: node.task.Title,
			Kind:  "epic",
			State: graph.EpicState(node.task.ID),
		}
		if !node.isEpic {
			ready := node.isReady
			item.Kind = "task"
			item.State = node.task.State
			item.Ready = &ready
			item.EpicID = node.task.EpicID
		}
		*items = append(*items, item)
		appendNodesAsJSON(items, node.children, graph)
	}
}

func appendNodesAsJSONV2(items *[]map[string]any, nodes []*treeNode, graph *Graph, opts ListOptions) {
	for _, node := range nodes {
		if node == nil || node.task == nil {
			continue
		}
		item := map[string]any{
			"id":    node.task.ID,
			"title": node.task.Title,
			"kind":  "epic",
			"state": graph.EpicState(node.task.ID),
		}
		if !node.isEpic {
			item["kind"] = "task"
			item["state"] = node.task.State
			item["ready"] = node.isReady
			if node.task.EpicID != "" {
				item["epic_id"] = node.task.EpicID
			}
		}
		if opts.JSONWithMeta {
			item["created_at"] = formatTime(node.task.CreatedAt)
			item["updated_at"] = formatTime(node.task.UpdatedAt)
			claimedBy, claimedAt := taskClaimedFields(node.task)
			if claimedBy == nil {
				item["claimed_by"] = nil
			} else {
				item["claimed_by"] = *claimedBy
			}
			if claimedAt == nil {
				item["claimed_at"] = nil
			} else {
				item["claimed_at"] = *claimedAt
			}
			item["depends_on"] = taskDependsOnJSON(graph, node.task.ID)
		}
		if opts.JSONWithBody {
			item["body"] = node.task.Body
		}
		*items = append(*items, item)
		appendNodesAsJSONV2(items, node.children, graph, opts)
	}
}
