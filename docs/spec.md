# ergo public CLI spec (contracts)

This document defines **stable contracts** for ergo’s CLI behavior.
It is not the primary user manual; users should read `ergo --help` and `ergo quickstart`.

## Scope

This spec covers:
- Output and exit-code guarantees (especially `--json`)
- State machine and claim invariants
- Dependency kind rules and cycle prevention
- Prune/compact semantics and “pruned ID” behavior
- Concurrency and locking behavior (user-visible consequences)

## Output and exit code contracts

## Input mode contracts (stdin)

For `new epic`, `new task`, and `set`:

- Default input mode reads a single JSON object from stdin (entire stdin; trailing newlines are allowed).
- When stdin is a TTY (not piped), these commands may be driven entirely by flags (e.g. `--title`, `--body`, `--state`) without any stdin.
- With `--body-stdin`, stdin is treated as literal body text and is **not** parsed as JSON.
  - In this mode, the body comes from stdin and other updates come from flags (e.g. `--title`, `--state`, `--epic`, `--claim`, results).
  - `--body` and `--body-stdin` are mutually exclusive.
  - `new epic --body-stdin` and `new task --body-stdin` require `--title`.

### General

- Success returns exit code `0`.
- Failures return a non-zero exit code and print an informative error to stderr.
- When `--json` is set, commands should avoid emitting non-JSON noise on stdout.

### `list` (human output)

These rules govern the human-oriented output of `ergo list` when `--json` is not set.

Root rows (no epic):
- Root task rows are rendered as plain list rows (no tree/connector glyphs in the left margin).
- Root epic rows are rendered as plain list rows (no tree/connector glyphs in the left margin).
- Root rows start with the state/epic icon (or other explicit root prefix), then the title, with the ID right-aligned.
- Root rows must not include `├`, `└`, or `│` in their left margin/prefix area.

Hierarchy:
- Child tasks under an epic are rendered with tree/connector glyphs (`├`, `└`, `│`) to indicate membership and ordering.
- Child indentation must make epic membership unambiguous (no child line can be mistaken for a root row).

Result attachment lines:
- Result lines (`→ file:///...`) are visually associated with their task.
- Root task result lines must not imply hierarchy to an unrelated header or epic.
- Child task result lines must maintain the same epic association as their parent task.

Summary line:
- Summary scope always matches the view that was rendered.
- Summary buckets:
  - Default (`ergo list`): `ready · in progress · blocked · error` (active tasks only).
  - `--ready`: `N ready` only.
  - `--all`: `ready · in progress · blocked · error · done · canceled`.
  - `--epic <id>`: same buckets, scoped to that epic’s children.
- `blocked` includes explicit `blocked` plus `todo` with unmet deps; `error` is counted separately.
- Summaries are suppressed when `--quiet` is set.

Empty states:
- Never silently empty: if the selected view renders zero tasks, print a full-sentence empty-state message.
- Exact empty-state strings:
  - `No tasks.`
  - `No active tasks.`
  - `No ready tasks.`
  - `No tasks in this epic.`
  - `No ready tasks in this epic.`
  - `No epics.`
- Contextual summaries may be printed after empty-state messages to explain why the view is empty:
  - `No active tasks.` → `N done · M canceled`.
  - `No ready tasks.` → `M in progress · K blocked · E error`.
  - Epic-scoped equivalents apply for `--epic <id>`.
- In `--quiet` mode, the primary empty-state message still prints; summaries and hints are suppressed.

Mixed-mode layout:
- When both root tasks and epics exist, there is no blank-line separator by default (consult before changing).

Flag conflicts:
- `--ready` and `--all` are mutually exclusive.
- `--epics` cannot be combined with `--ready`, `--all`, or `--epic <id>`.

`list --epics` (human output):
- Epics-only view renders each epic as a root row using the same list visual language (includes `Ⓔ` and right-aligned ID).
- When there are no epics, print `No epics.`.

#### `list --epic <id>` (human output)

When `--epic <id>` is provided and `--json` is not set:
- Output is an epic-focused view: show the epic header line plus its child tasks only.
- Orphan tasks are excluded.
- The epic header is always shown, even if no children match the current filters.
- Invalid epic IDs are errors (non-zero exit) with a clear stderr message (e.g., `no such epic: <id>`).
- By default, epic-focused view shows **all** tasks within the epic (including `done`/`canceled`).
- `--ready` filters to ready tasks within the epic.
- `--all` is accepted but redundant in epic-focused view.
- Explicit epic targeting disables auto-collapse of fully done epics (show the epic line regardless).
- The stderr hint (`agents: use 'ergo --json list'...`) continues to print for human output.
- Empty-state messages and summaries follow the rules above, scoped to the epic’s children.

### `--json` contract

When `--json` is set and a command succeeds:
- Exactly **one JSON value** is written to stdout (object or array).
- JSON must be parseable by strict decoders (no trailing non-JSON).
- Schema changes should be additive when possible.

Commands that are expected to be machine-used should offer a useful JSON shape:
- `list --json`: array of items
- `show --json`: object (or object-with-children for epics)
- Mutations (`new`, `set`, `sequence`, `prune`, `compact`, `claim`): JSON object(s)

### “No ready tasks” for `claim`

When `ergo claim` (oldest-ready mode) finds no claimable tasks:
- This is a **successful** outcome (exit code `0`).
- Human output prints a clear message.
- With `--json`, output is a JSON object that explicitly indicates “no ready”.

When `ergo claim` succeeds and `--json` is set:
- Output includes an additional `reminder` string:
  - Exact value: `When you have completed this claimed task, you MUST mark it done.`

## State machine and claim invariants

### States

Tasks have a state in:
`todo | doing | done | blocked | canceled | error`.

Epics are structural and do not have state.

### Claim invariants

- `doing` requires a claim.
- `error` requires a claim (to show who failed).
- `todo`, `done`, `canceled` must have **no** claim.
- `blocked` may have a claim or not.

### Transition rules

Transitions are constrained by a fixed state machine (see code for full matrix).
Notably:
- `done` and `canceled` can be reopened to `todo`.
- `error` can transition to `doing` (retry), `todo` (reassign), or `canceled` (give up).

## IDs and entities

- IDs are 6-character, uppercase, short identifiers.
- Entities:
  - **Task**: can be claimed and has state.
  - **Epic**: cannot be claimed and has no state; acts as a grouping node.

## Dependency rules

- Task→task dependencies are allowed.
- Epic→epic dependencies are allowed.
- Task↔epic dependencies are forbidden.
- Self-dependencies are forbidden.
- Creating a dependency that would introduce a cycle is rejected.

## Prune and compact (deletion model)

ergo deletion is **two-phase**:

1) `prune` performs **logical deletion** (tombstones).
2) `compact` performs **physical deletion** (rewrites the event log).

### Prune policy

Policy-based (no per-ID selection):
- Tasks in `done` or `canceled` are eligible.
- Tasks in `todo`, `doing`, `blocked`, `error` are preserved.
- After pruning eligible tasks, any epic with no remaining children is pruned.

### Pruned ID behavior (tombstones)

When an ID is pruned, it is treated as **non-existent**:
- It does not appear in `list`.
- It cannot be used as a dependency endpoint.
- It cannot be claimed or updated.

Dependencies to/from pruned IDs are dropped; a pruned dependency must not keep other work blocked.

### Post-compact behavior

After `compact`, the history of pruned IDs may be removed from the log:
- `show <id>` for a previously pruned ID may no longer be distinguishable from “never existed”.

## Concurrency and locking (user-visible behavior)

- Mutations are serialized by an advisory lock on `.ergo/lock`.
- Lock acquisition is fail-fast:
  - When the lock is held by another process, mutations fail quickly with a “lock busy” error.
  - Callers should retry.
- `claim` oldest-ready is race-safe: only one process can claim a given task.
