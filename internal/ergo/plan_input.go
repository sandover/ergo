// Shared types for bulk task creation (containers with children and deps).
// Exports: PlanTaskInput, hasPlanCycle.
// Used by: json_input.go (TaskInput.Tasks validation), commands_create.go (runBulkCreate).
package ergo

// PlanTaskInput describes one child task in a bulk creation payload.
type PlanTaskInput struct {
Title *string  `json:"title,omitempty"` // required
Body  *string  `json:"body,omitempty"`  // optional
After []string `json:"after,omitempty"` // optional: task titles this task depends on
}

// hasPlanCycle detects cycles in a title-based dependency graph using DFS.
func hasPlanCycle(titles map[string]int, depsByTitle map[string][]string) bool {
const (
visitUnseen uint8 = iota
visitActive
visitDone
)
state := map[string]uint8{}

var visit func(node string) bool
visit = func(node string) bool {
switch state[node] {
case visitActive:
return true
case visitDone:
return false
}
state[node] = visitActive
for _, dep := range depsByTitle[node] {
if visit(dep) {
return true
}
}
state[node] = visitDone
return false
}

for title := range titles {
if state[title] != visitUnseen {
continue
}
if visit(title) {
return true
}
}
return false
}
