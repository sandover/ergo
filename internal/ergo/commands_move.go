// Purpose: Implement explicit task placement changes and their validation.
// Exports: RunMove.
// Role: Move leaf tasks between root and one-level containers atomically.
// Invariants: containers never nest or move; ancestor dependency edges stay invalid.
// Invariants: only a clean root todo task may gain its first child.
package ergo

import (
	"errors"
	"fmt"
)

func RunMove(id, destinationID string, toRoot bool, opts GlobalOptions) error {
	if toRoot && destinationID != "" {
		return errors.New("move destination and --root are mutually exclusive")
	}
	if !toRoot && destinationID == "" {
		return errors.New("usage: ergo move <id> <epic-id> | ergo move <id> --root")
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	outcome, err := applyTaskMutation(dir, opts, id, taskMutation{
		Kind: "move", EpicID: destinationID, EpicSet: true, ValidateMove: true,
	})
	if err != nil {
		return err
	}
	if len(outcome.ChangedFields) == 0 {
		fmt.Printf("%s placement unchanged\n", id)
		return nil
	}
	if toRoot {
		fmt.Printf("%s moved to root\n", id)
		return nil
	}
	fmt.Printf("%s moved to %s\n", id, destinationID)
	return nil
}

func validateMovePlacement(graph *Graph, task *Task, destinationID string) error {
	if graph.IsEpic(task.ID) {
		return fmt.Errorf("cannot move epic %s", task.ID)
	}
	if destinationID == "" {
		return nil
	}
	if destinationID == task.ID {
		return errors.New("cannot move a task into itself")
	}
	destination := graph.Tasks[destinationID]
	if destination == nil {
		return fmt.Errorf("unknown epic id %s", destinationID)
	}
	if destination.EpicID != "" {
		return fmt.Errorf("cannot nest under task %s: epics must remain at root", destinationID)
	}
	if !graph.IsEpic(destination.ID) {
		switch {
		case destination.ClaimedBy != "":
			return fmt.Errorf("cannot promote task %s: task is claimed by %q", destinationID, destination.ClaimedBy)
		case destination.State != stateTodo:
			return fmt.Errorf("cannot promote task %s: state is %q (must be todo)", destinationID, destination.State)
		case len(destination.Results) > 0:
			return fmt.Errorf("cannot promote task %s: task has results attached", destinationID)
		}
	}
	if graph.Deps[task.ID] != nil {
		if _, ok := graph.Deps[task.ID][destinationID]; ok {
			return errors.New("task cannot depend on its destination epic")
		}
	}
	if graph.Deps[destinationID] != nil {
		if _, ok := graph.Deps[destinationID][task.ID]; ok {
			return errors.New("destination epic cannot depend on its child")
		}
	}
	return nil
}
