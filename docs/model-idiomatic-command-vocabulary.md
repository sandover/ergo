# Model-idiomatic command vocabulary

Status: accepted target contract. This document describes a breaking CLI cutover that is not implemented yet.

## Goal

Replace the generic `set` command with commands that state intent directly.

An agent should not need to translate "finish this task" into a field mutation, construct JSON for routine work, or learn a transition table before acting. Direct commands may share one internal mutation path. Validation, locking, event generation, and replay remain authoritative in one implementation.

## Model

Ergo keeps three concepts separate:

- State describes the task's current condition.
- A claim is an execution lease held by one agent.
- Results and body notes record attempt history.

A failed attempt is not a task state. It leaves the task available for retry with `release`, or explicitly impeded with `block`. Evidence about the attempt belongs in a result or body note.

### Forward state vocabulary

| State | Meaning |
| --- | --- |
| `todo` | Open and unclaimed. |
| `doing` | Open and claimed by one agent. |
| `blocked` | Open but explicitly unable to proceed. |
| `done` | The objective was satisfied. |
| `canceled` | The objective is no longer wanted. |

`error` is a readable legacy state but is never created by the new command surface.

The claim invariant is exact:

```text
claimed_by is present if and only if state is doing
```

Every command that leaves `doing` clears the claim. Claim and `doing` are one atomic operation and are never independently mutable through the public CLI.

Readiness is derived rather than stored:

```text
state is todo + dependencies are satisfied = ready
```

A task waiting for dependencies remains `todo`. The explicit `blocked` state is reserved for an impediment that an agent or human has identified.

## Command vocabulary

### Existing-task mutations

| Command | Purpose |
| --- | --- |
| `claim [<id>] --agent <identity>` | Claim the oldest ready task or resume a specific task. |
| `done <id> [--result <path>] [--summary <text>]` | Record that the objective was satisfied. |
| `block <id> [--result <path>] [--summary <text>]` | Record that work cannot proceed. |
| `cancel <id> [--result <path>] [--summary <text>]` | Record that the objective is no longer wanted. |
| `release <id> [--result <path>] [--summary <text>]` | Return unfinished work to the unclaimed `todo` pool. |
| `title <id> <title>` | Replace a task's title. |
| `body <id>` | Replace a task's body with piped stdin. |
| `move <id> <container-id>` | Move a task into a container. |
| `move <id> --root` | Remove a task from its container. |

`set` is removed. There is no generic public command for changing arbitrary fields or state.

There is no `reopen` command. A specific `claim` resumes done or canceled work and assigns it in one operation. The CLI does not expose the uncommon intermediate operation of returning closed work to unclaimed `todo`.

### Commands unchanged by this cutover

```text
new task
show
list
sequence
sequence rm
plan
init
where
prune
compact
quickstart
version
```

`new task` still creates one task and `plan` still creates a container with children. Neither command may create the legacy `error` state.

## Lifecycle behavior

Direct lifecycle commands express authoritative intent. They do not require callers to move through intermediate states. Prior state affects selection, claim conflicts, legacy normalization, and idempotency, but does not impose a public transition grammar.

### `claim`

- Without an ID, selects only the oldest ready `todo` leaf task.
- With an ID, accepts a `todo`, `blocked`, `done`, `canceled`, or legacy `error` leaf task.
- A specific claim may resume work even when the task is not automatically ready.
- Sets the claim and `doing` state in one locked mutation.
- Retains prior results and the body.
- Requires `--agent`.
- Claiming a task already held by the same agent is a successful no-op.
- Claiming a task held by another agent is a conflict.
- Containers cannot be claimed.
- Existing oldest-ready ordering and race safety remain unchanged.
- Successful output names every valid way to leave the claim.

Example JSON field:

```json
{
  "next_commands": {
    "done": "ergo --json done ABCDEF",
    "block": "ergo --json block ABCDEF",
    "cancel": "ergo --json cancel ABCDEF",
    "release": "ergo --json release ABCDEF"
  }
}
```

### `done`

- Accepts any leaf-task state, including legacy `error`.
- Sets state to `done` and clears the claim.
- Retains prior results.
- Repeating `done` on an unclaimed done task is a successful no-op.
- A repeated call may still attach a late result or replace the body.

### `block`

- Accepts any leaf-task state, including legacy `error`.
- Sets state to `blocked` and clears the claim.
- Retains prior results.
- Repeating `block` on an unclaimed blocked task is a successful no-op.
- Repeating `block` on a legacy blocked task that still has a claim clears the claim.
- A repeated call may still attach a late result or replace the body.
- A later specific `claim` moves the task to `doing`.

### `cancel`

- Accepts any leaf-task state, including legacy `error`.
- Sets state to `canceled` and clears the claim.
- Retains prior results.
- Repeating `cancel` on an unclaimed canceled task is a successful no-op.
- A repeated call may still attach a late result or replace the body.
- A later specific `claim` resumes the task in `doing`.

### `release`

- Accepts a `doing`, `blocked`, or legacy `error` leaf task and returns it to `todo`.
- Clears the claim.
- Retains prior results.
- Repeating `release` on an unclaimed `todo` task is a successful no-op.
- A repeated call may still attach a result or replace the body without emitting a redundant state event.
- Does not accept `done` or `canceled`; use a specific `claim` when work on a closed task resumes.
- Represents unfinished work that remains valid. Use `block` instead when an impediment must be resolved before another attempt.

## Task content and placement

### `title`

```sh
ergo title ABCDEF "Clarify authentication failure"
```

- The title is a required positional argument.
- Empty or whitespace-only titles are rejected.
- The command changes no other task field.
- Containers and leaf tasks may both be retitled.

### `body`

```sh
printf '%s\n' '## Goal' '- Clarify authentication failure' | ergo body ABCDEF
```

- Piped stdin replaces the body as literal text.
- Empty piped stdin clears the body.
- TTY stdin is an error with an example showing how to pipe body text.
- Containers and leaf tasks may both receive body updates.

### `move`

```sh
ergo move ABCDEF OFKSTE
ergo move ABCDEF --root
```

- The destination must exist.
- The destination may be an existing container.
- A root `todo` task with no claim or results may become a container by receiving its first child.
- Moving a container under another task is rejected.
- Moving a task into itself or violating nesting, ancestry, or dependency invariants is rejected.
- Moving a task to its current parent is a successful no-op.
- `--root` and a destination ID are mutually exclusive.

## Body input on lifecycle commands

`done`, `block`, `cancel`, and `release` use piped stdin in the same way as `body`. This preserves the common atomic operation of recording a note while changing the task's condition.

```sh
printf '%s\n' \
  '## Completion' \
  '- Implemented and verified.' |
  ergo done ABCDEF --result src/feature.ts
```

When stdin is a TTY, the body is unchanged. When stdin is piped, the body replacement, optional result attachment, state change, and claim clearing form one locked event batch.

## Result contract

- `--result` accepts one existing file under the active Ergo project root.
- The path must be relative, must remain inside the project, and must not name a directory or a file under `.ergo/`.
- `--summary` is optional and requires `--result`.
- Without `--summary`, the relative path is used as the summary.
- Result validation and provenance capture remain unchanged.
- Results attach through `done`, `block`, `cancel`, or `release`. There is no separate `attach` or `result` command.
- To attach a late result, repeat the task's current lifecycle command with `--result`.
- A repeated lifecycle command may append a result without changing state.
- A `todo` task may carry results from released attempts. Each result remains part of the retry history.

Examples:

```sh
ergo done ABCDEF --result docs/spec.md
ergo block ABCDEF --result .scratch/investigation.md --summary "Reproduction evidence"
ergo release ABCDEF --result .scratch/failure.md --summary "Timed out on flaky dependency"
```

## Atomicity and command composition

Each command is atomic. Direct lifecycle commands may combine a body replacement, result attachment, state change, and claim clearing in one event batch.

Other combinations use separate commands:

```sh
ergo title ABCDEF "New title"
printf '%s\n' '## Revised scope' | ergo body ABCDEF
ergo move ABCDEF OFKSTE
```

Another process may observe an intermediate state between separate commands. This is deliberate: uncommon multi-field edits do not justify a generic mutation language.

## JSON output

- Every successful mutation emits exactly one JSON object with `--json`.
- Output includes `kind`, `id`, and `updated_fields`.
- Lifecycle output also includes `state` and `claimed_by` when present.
- `updated_fields` names fields that actually changed or received appended data.
- A true no-op returns exit code `0`, an empty `updated_fields` array, and the current state.
- A same-state call with a body, result, or legacy claim cleanup is not a no-op.
- Human output remains concise and prints the task ID on success.

Example:

```json
{"kind":"done","id":"ABCDEF","updated_fields":["body","result","state"],"state":"done"}
```

## Removed `set` mappings

| Former intent | Replacement |
| --- | --- |
| Claim work or set `doing` | `claim` |
| Resume blocked, done, canceled, or legacy-error work | `claim` |
| Set `done` | `done` |
| Set `blocked` | `block` |
| Set `canceled` | `cancel` |
| Return doing, blocked, or legacy-error work to `todo` | `release` |
| Return done or canceled work to unclaimed `todo` | Not exposed; use `claim` when work resumes |
| Record a failed attempt that remains retryable | `release` with an optional body and result |
| Record a failed attempt that needs intervention | `block` with an optional body and result |
| Set `title` | `title` |
| Replace body from stdin | `body` |
| Set or clear `epic` | `move` |
| Attach a result while finishing or releasing | `done`, `block`, `cancel`, or `release --result` |
| Attach a late result | Repeat the task's current lifecycle command with `--result` |

## Legacy Ergo plans and event logs

The cutover must not require users to migrate existing repositories before using the new CLI.

### Read compatibility

- All existing event types remain replayable.
- Historical `error` states remain visible as `error` until a lifecycle command resolves them.
- Legacy blocked tasks that retain a claim remain readable without silently dropping ownership information.
- `show`, `list`, dependency evaluation, and container derivation continue to work on legacy logs.
- Opening a repository never rewrites its event log.

### Lazy normalization

Lifecycle commands normalize legacy state only when the user states a new intent:

| Legacy condition | Normalizing commands |
| --- | --- |
| `error` with a retained claim | `claim` → `doing`; `release` → `todo`; `block` → `blocked`; `done` → `done`; `cancel` → `canceled` |
| `blocked` with a retained claim | `claim` → `doing`; `release` → `todo`; `block` → unclaimed `blocked`; `done` or `cancel` → closed and unclaimed |
| Closed task that needs more work | Specific `claim` → `doing` |

Each normalization is one locked event batch. Existing results and body content remain intact.

`title`, `body`, and `move` do not silently change lifecycle state. A legacy lifecycle condition may therefore remain until a lifecycle command addresses it.

### Compaction

- `compact` is lossless maintenance, not a semantic migration.
- It preserves unresolved legacy `error` and claimed-blocked conditions exactly.
- It must not guess whether a legacy failure should become `todo`, `blocked`, `done`, or `canceled`.
- After a lifecycle command normalizes a task, later compaction writes only the normalized current condition.

### New writes and old automation

- New lifecycle commands never emit `error`.
- `new task` rejects attempts to create `error` and names the valid alternatives.
- Removed `set` syntax fails non-zero and points to the corresponding direct command.
- Logs appended by an older Ergo binary remain readable. The next direct lifecycle command normalizes any legacy condition it encounters.
- No new migration command, event version, or eager log rewrite is introduced.

## Error guidance

Errors must name the valid direct command when intent is clear.

Examples:

```text
error: unknown command "set"
hint: use claim, done, block, cancel, release, title, body, or move
```

```text
error: unknown command "reopen"
hint: use claim <id> --agent <identity> to resume closed work
```

```text
error: result file does not exist: commit f5abd31
hint: --result must be an existing project-relative file; omit it when no file was produced
```

When no `.ergo` directory is found, the error must mention both recovery paths: initialize a graph with `ergo init`, or target an existing graph with `ergo --dir <path>`.

## Implementation constraints

- Direct commands call one shared mutation implementation. They do not duplicate state, claim, result, body, or event logic.
- Command handlers specify postconditions; they do not each implement a transition table.
- Existing event types and append-only storage remain authoritative.
- Same-state lifecycle calls suppress redundant state events while preserving body, result, and claim-cleanup events.
- Every mutation is serialized under the existing repository lock.
- Help and quickstart are updated in the same cutover and remain the complete public manual.
- Parent commands reject unexpected arguments rather than print help and exit successfully.
- Tests cover every command, postcondition, claim conflict, idempotent retry, stdin mode, result rule, container rejection, JSON shape, corrective hint, and legacy case described above.

## Acceptance criteria

- No public workflow requires `set` or positional mutation JSON.
- The primary agent loop is `claim` followed by `done`, `block`, `cancel`, or `release`.
- There is no public `reopen` command or generic state setter.
- Direct lifecycle intent does not require intermediate state choreography.
- Claim and `doing` always move together.
- All non-`doing` forward states are unclaimed.
- Failed attempts are represented by results and either `release` or `block`, not a new `error` state.
- Existing plans open and remain usable without migration.
- Legacy state is preserved until an explicit lifecycle command normalizes it.
- `ergo --help` and `ergo quickstart` cover the complete new surface without presenting equivalent mutation paths.
