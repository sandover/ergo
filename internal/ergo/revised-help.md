ergo — fast, minimal plan tracker (tasks + epics + deps). safe for parallel agents.

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

  claim [<id>] [--epic <id>]             (atomic) claim oldest READY task, set doing, print title+body
                                          or claim a specific task by id.
                                          filters: --epic and --as apply. exit code 3 if none.

  set <id> key=value [key=value ...]      update fields (see KEYS). rejects empty title.
  dep <A> <B>                             A depends on B (B blocks A).
                                          tasks can depend on tasks; epics can depend on epics.
                                          rejects cycles; rejects mixed (task↔epic) deps.
  dep rm <A> <B>                          remove dependency

  where                                   print active .ergo directory path
  compact                                 rewrite log to current state (drops history)
  quickstart                              guided walkthrough

GLOBAL FLAGS
  --dir <path>                            discovery start dir (or explicit .ergo dir)
  --as <any|agent|human>                  filter READY/BLOCKED and claim (default: any)
  --agent <id>                            claim identity (default: username@hostname)
  --json                                  JSON output (default: text), recommended for agents
  --readonly                              block commands that write
  --lock-timeout <duration>               default 30s; 0 = fail fast
  -h, --help                              show help
  -V, --version                           print version

READY DEFINITION (tasks only)
  state=todo, unclaimed, worker matches --as, all task deps done|canceled,
  and all epic-deps of the task's epic are complete.

BLOCKED DEFINITION (tasks only)
  state=blocked, OR (state=todo AND unmet task deps), OR task's epic has
  incomplete epic-deps.

EPIC COMPLETION
  An epic is complete when all its tasks are done or canceled.
  Epic-to-epic deps: tasks in epic A are blocked until epic B is complete.

KEYS (for `set`)
  Common:
    title=<text>                          required; cannot be cleared
    body=@-                               read body from stdin

  Task-only:
    epic=<epic_id> | epic=                assign / unassign
    worker=any|agent|human
    state=todo|doing|done|blocked|canceled|error
                                          doing/error require claim; todo/done/canceled clear claim
    claim=<agent_id>                      set claim and (by default) state=doing
    claim=                                clear claim (no state change)

  Epic-only:
    (epics ignore state/worker/claim — use epic-deps instead)

STATE MACHINE
  todo     → doing, done, blocked, canceled
  doing    → todo, done, blocked, canceled, error
  blocked  → todo, doing, done, canceled
  done     → todo (reopen only)
  canceled → todo (reopen only)
  error    → todo, doing, canceled (retry, reassign, or give up)

DEPENDENCY RULES
  task → task: allowed
  epic → epic: allowed
  task ↔ epic: forbidden (no mixing)
  self-dep:    forbidden (A cannot depend on A)
  cycles:      forbidden (A→B→...→A rejected)
