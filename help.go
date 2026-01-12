// --help text and colorized section formatting.
package main

import "strings"

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiCyan  = "\x1b[36m"
)

func usageText(color bool) string {
	text := `ergo — fast, minimal plan tracker (tasks + epics + deps). safe for parallel agents.

USAGE
  ergo [global flags] <command> [args]

COMMANDS
  init [dir]                              create .ergo/ in dir (arg overrides --dir)

  new epic <title>                        create epic (prints ID) [title required]
  new task <title> [--epic <id>]          create task (prints ID) [title required]

  list [--epic <id>] [--ready|--blocked] [--epics]
                                          list tasks (default: all)
                                          --epics shows epic summaries too
                                          --as applies only with --ready/--blocked. use --json for agents.
  show <id>                               show epic or task details. respects --json.

  next [--epic <id>] [--peek]             (atomic) pick oldest READY task, claim, set doing, print title+body
                                          filters: --epic and --as apply. --peek does not claim.
                                          exit code 3 if no READY task.

  set <id> key=value [key=value ...]      update fields (see KEYS). rejects empty title.
  dep <A> <B>                             add task dependency: A depends on B (B blocks A). rejects cycles.
  dep rm <A> <B>                          remove task dependency

  where                                   print active .ergo directory path
  compact                                 rewrite log to current state (drops history)
  quickstart                              guided walkthrough

GLOBAL FLAGS
  --dir <path>                            discovery start dir (or explicit .ergo dir)
  --as <any|agent|human>                  filter READY/BLOCKED and next (default: any)
  --agent <id>                            claim identity (default: hostname:pid)
  --json                                  JSON output (default: text), recommended for agents
  --readonly                              block commands that write
  --lock-timeout <duration>               default 30s; 0 = fail fast
  -h, --help                              show help
  -V, --version                           print version

READY DEFINITION (tasks only)
  state=todo, unclaimed, worker matches --as, and all deps are done or canceled.

BLOCKED DEFINITION (tasks only)
  state=blocked OR (state=todo AND unmet deps).

KEYS (for 'set')
  Common:
    title=<text>                          required; cannot be cleared
    body=@-                               read body from stdin
    body=@editor                          edit body in $EDITOR

  Task-only:
    epic=<epic_id> | epic=                assign / unassign
    worker=any|agent|human
    state=todo|doing|done|blocked|canceled
                                          state applied last; todo/done/canceled clear claim; doing requires claim
    claim=<agent_id>                      set claim and (by default) state=doing
    claim=                                clear claim (no state change)

  Epic-only:
    (epics ignore state/worker/claim/dep)`

	if !color {
		return text
	}

	lines := strings.Split(text, "\n")
	if len(lines) > 0 {
		lines[0] = ansiBold + lines[0] + ansiReset
	}
	text = strings.Join(lines, "\n")

	for _, header := range []string{
		"USAGE",
		"COMMANDS",
		"GLOBAL FLAGS",
		"READY DEFINITION (tasks only)",
		"BLOCKED DEFINITION (tasks only)",
		"KEYS (for 'set')",
	} {
		text = strings.ReplaceAll(text, header, ansiCyanBold(header))
	}

	return text
}

func ansiCyanBold(s string) string {
	return ansiBold + ansiCyan + s + ansiReset
}
