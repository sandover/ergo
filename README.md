# ergo

Minimal multi-agent DAG task planner built on an append-only JSONL event log.

`ergo` is designed for “agents in a repo” workflows: multiple processes can safely claim READY work from a shared task graph, while humans can inspect and steer with simple, pipe-friendly commands.

## Quickstart

```sh
# from your project root
ergo init

# create an epic (multi-line body via stdin)
epic=$(ergo epic new <<'EOF'
Improve Linux support

Motivation:
...

AC:
...

Validation:
...
EOF
)

# create a task under the epic
cat <<'EOF' | ergo task new --epic "$epic"
Locking: add unix lock abstraction for Linux

Motivation:
...

AC:
...

Validation:
...
EOF

# see what’s ready
ergo ls --ready

# atomically claim + move to doing; prints the task body to stdout
ergo take
```

## Installation (local)

Requirements: Go 1.21+

```sh
go build -o ergo .
sudo mv ergo /usr/local/bin/
```

## Installation (Go)

```sh
go install github.com/sandover/ergo@latest
```

## Concepts

- **Storage:** `.ergo/events.jsonl` is the source of truth (append-only JSONL). `.ergo/lock` is used to lock all writes.
- **Tasks & epics:** Epics are tasks with no `epic_id`. Tasks reference their epic via `--epic <id>`.
- **States:** `todo | doing | done | blocked | canceled`
- **Dependencies:** `link A depends B` means “A waits for B”. A dependency is satisfied when the prerequisite is `done` or `canceled`.
- **READY:** `state=todo`, not claimed, and all dependencies satisfied.
- **Claims:** `take` appends a `claim` event and a `state=doing` event under a single lock.
- **Re-queue:** `state <id> todo` clears the current claim (making the task eligible to become READY again).

## Usage

Run `ergo --help` for the full compact manual.

Common commands:

```sh
ergo init [dir]
ergo epic new
ergo task new --epic <epic_id>
ergo edit <id>
ergo state <id> <state>
ergo link <from> depends <to>
ergo unlink <from> depends <to>
ergo ls [--epic <id>] [--ready|--blocked|--all]
ergo take [--epic <id>]
ergo show <id>
ergo deps <id>
ergo rdeps <id>
ergo plan [--epic <id>]
ergo compact
```

### Multi-line bodies

- If stdin is piped, the body is read from stdin.
- Otherwise, `ergo` opens `$EDITOR` (default `nano`).
- For `edit`, piping empty stdin keeps the existing body (useful in some non-interactive shells).

## Data format (events.jsonl)

Events are JSON objects, one per line:

```json
{ "ts":"<RFC3339Nano>", "type":"new_task", "data":{"id":"ABC123","uuid":"...","epic_id":"EPIC01","state":"todo","body":"...","created_at":"..."} }
{ "ts":"<RFC3339Nano>", "type":"new_epic", "data":{"id":"EPIC01","uuid":"...","state":"todo","body":"...","created_at":"..."} }
{ "ts":"<RFC3339Nano>", "type":"state", "data":{"id":"ABC123","new_state":"doing","ts":"..."} }
{ "ts":"<RFC3339Nano>", "type":"link", "data":{"from_id":"ABC123","to_id":"XYZ999","type":"depends"} }
{ "ts":"<RFC3339Nano>", "type":"unlink", "data":{"from_id":"ABC123","to_id":"XYZ999","type":"depends"} }
{ "ts":"<RFC3339Nano>", "type":"claim", "data":{"id":"ABC123","agent_id":"user@host","ts":"..."} }
{ "ts":"<RFC3339Nano>", "type":"edit", "data":{"id":"ABC123","body":"...","ts":"..."} }
```

`ergo` replays this log on each command to reconstruct the task graph in memory.

## Concurrency model

- All writes are serialized via an OS file lock on `.ergo/lock`.
- Read-only commands replay without locking.
- `take` is safe under concurrent use: only one agent will win the claim for a given READY task.

## Compaction

`ergo compact` rewrites `events.jsonl` to the minimal canonical set of events representing the current state and active dependency links.

This **drops history** (past edits/claims/transitions) by design.

## Status / limitations

- Current root discovery is minimal: run commands from the directory containing `.ergo`.
- Output is intentionally plain text / TSV for piping; there is no JSON output mode yet.
- Tested primarily on macOS; Linux is expected to work but is not yet a promised support tier; Windows is not supported yet.

## Roadmap ideas

- Root auto-discovery (walk up like git)
- Optional `--json` output for `ls/plan/show`
- Stronger claim ownership / release workflows (explicit `release` command)
- Linux/Windows support tiers + CI matrix
