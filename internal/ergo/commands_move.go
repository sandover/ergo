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
		return errors.New("usage: ergo move <id> <container-id> | ergo move <id> --root")
	}
	dir, err := ergoDir(opts)
	if err != nil {
		return err
	}
	_, err = applyTaskMutation(dir, opts, id, taskMutation{
		Kind: "move", EpicID: destinationID, EpicSet: true, ValidateMove: true,
	})
	if err != nil {
		return err
	}
	if toRoot {
		fmt.Printf("%s moved to root\n", id)
		return nil
	}
	fmt.Printf("%s moved to %s\n", id, destinationID)
	return nil
}

func validateMovePlacement(graph *Graph, task *Task, destinationID string) error {
	if isContainer(task, graph) {
		return fmt.Errorf("cannot move container %s", task.ID)
	}
	if destinationID == "" {
		return nil
	}
	if destinationID == task.ID {
		return errors.New("cannot move a task into itself")
	}
	destination := graph.Tasks[destinationID]
	if destination == nil {
		return fmt.Errorf("unknown container id %s", destinationID)
	}
	if destination.EpicID != "" {
		return fmt.Errorf("cannot nest under task %s: containers must remain at root", destinationID)
	}
	if !isContainer(destination, graph) {
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
			return errors.New("task cannot depend on its destination container")
		}
	}
	if graph.Deps[destinationID] != nil {
		if _, ok := graph.Deps[destinationID][task.ID]; ok {
			return errors.New("destination container cannot depend on its child")
		}
	}
	return nil
}
