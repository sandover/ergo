// Purpose: Render the list use case as a small versioned integration document.
// Role: Stable machine projection beside the human-readable tree renderer.
package ergo

import (
	"encoding/json"
	"io"
)

const listJSONVersion = 1

type listJSONDocument struct {
	Version int            `json:"version"`
	Items   []listJSONItem `json:"items"`
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
	document := listJSONDocument{
		Version: listJSONVersion,
		Items:   make([]listJSONItem, 0),
	}
	appendNodesAsJSON(&document.Items, outcome.Roots)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(document)
}

func appendNodesAsJSON(items *[]listJSONItem, nodes []*treeNode) {
	for _, node := range nodes {
		if node == nil || node.task == nil {
			continue
		}
		item := listJSONItem{
			ID:    node.task.ID,
			Title: node.task.Title,
			Kind:  "epic",
		}
		if !node.isEpic {
			ready := node.isReady
			item.Kind = "task"
			item.State = node.task.State
			item.Ready = &ready
			item.EpicID = node.task.EpicID
		}
		*items = append(*items, item)
		appendNodesAsJSON(items, node.children)
	}
}
