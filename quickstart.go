// quickstart command handler and walkthrough text.
package main

import (
	"errors"
	"fmt"
)

func runQuickstart(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ergo quickstart")
	}
	fmt.Println(quickstartText(stdoutIsTTY()))
	return nil
}

func quickstartText(color bool) string {
	title := "ergo quickstart"
	subtitle := "Guided walkthrough of the core workflow."
	if color {
		title = ansiBold + title + ansiReset
		subtitle = ansiCyan + subtitle + ansiReset
	}

	header := func(s string) string {
		if !color {
			return s
		}
		return ansiCyanBold(s)
	}

	return fmt.Sprintf(`%s
%s

%s
  ergo init
            Initialize ergo in the current directory.
            Creates .ergo/ (events.jsonl + lock).

%s
  ergo where
            ergo discovers .ergo like git:
              1. use --dir <path> if provided
              2. search for .ergo/ in the current directory and ancestors
            If none is found, run: ergo init

%s
  IDs are 6-char human IDs (UUID shown in show).
  States: todo | doing | done | blocked | canceled
  READY = state=todo AND unclaimed AND all deps done/canceled.
  Dependency direction: dep A depends B => B blocks A.
  Epics are containers by default.

%s
  ergo task new --kind epic "Improve Linux support"
            Create an epic. Prints the epic id (e.g., EPIC01).

  ergo task new --epic EPIC01 "Locking: add unix lock abstraction"
            Create a task under an epic. Prints the task id (e.g., ABC123).

  ergo task new --epic EPIC01 --body-file task.md
  cat task.md | ergo task new --epic EPIC01 --body-file -

%s
  ergo dep ABC123 depends DEF456
            Add dependency edge: DEF456 blocks ABC123.
  ergo dep rm ABC123 depends DEF456
            Remove dependency edge.

%s
  ergo ready [--epic EPIC01] [--kind any]
            List READY tasks (todo, unclaimed, deps done/canceled).
  ergo take  [--epic EPIC01] [--kind any]
            Claim oldest READY task, set to doing, print body.
  ergo state ABC123 done
            Mark task done (todo clears claims).

%s
  ergo task new --epic EPIC01 --worker human "Choose API shape"
  ergo worker ABC123 human
            Mark tasks that require human judgment.

%s
  ergo ready --as agent --json
  ergo take  --as agent --json
  If no agent-ready work:
    ergo ready --as human --json

%s
  ergo plan --json
  ergo show ABC123 --json
  ergo ls --ready --json

%s
  ergo take --lock-timeout 30s
  ergo --readonly <command>`,
		title,
		subtitle,
		header("GETTING STARTED"),
		header("DATA LOCATION"),
		header("CONCEPTS"),
		header("CREATING WORK"),
		header("DEPENDENCIES"),
		header("WORKFLOW"),
		header("HUMAN-ONLY TASKS (decision points)"),
		header("AGENT INTEGRATION (recommended protocol)"),
		header("OUTPUT FOR SCRIPTS/AGENTS"),
		header("LOCKING / ROBUSTNESS"),
	)
}
