# Ergo public CLI contract

This document defines the stable v2 CLI contract. `ergo --help` is the quick
reference. `ergo quickstart` is the complete user manual.

## Domain model

Ergo stores one entity type: a task.

- A leaf task has lifecycle state, body, results, dependencies, and possibly a claim.
- A container is a root task with children. It has no direct lifecycle, claim, or result behavior.
- A container completes when every child is done or canceled.

Forward lifecycle states are:

| State | Meaning |
| --- | --- |
| `todo` | Open and unclaimed. |
| `doing` | Open and claimed by one agent. |
| `blocked` | Open with an identified impediment. |
| `done` | The objective was satisfied. |
| `canceled` | The objective is no longer wanted. |

The claim invariant is exact:

```text
claimed_by is present if and only if state is doing
```

Readiness is derived:

```text
state is todo + every dependency is complete = ready
```

A todo task with unmet dependencies is waiting. It is not explicitly blocked.
The legacy `error` state remains readable but no v2 command creates it.

## Command surface

Existing-task mutation uses these intent-specific commands:

```text
claim [<id>] --agent <identity>
done <id> [--result <path>] [--summary <text>]
block <id> [--result <path>] [--summary <text>]
cancel <id> [--result <path>] [--summary <text>]
release <id> [--result <path>] [--summary <text>]
title <id> <title>
body <id>
move <id> <container-id>
move <id> --root
```

There is no generic field or state mutation command. There is no reopen command.
A specific claim resumes closed work and assigns it atomically. The CLI does not
expose closed-to-unclaimed-todo as a separate operation.

Other commands are:

```text
init [dir]
new task [json]
plan --file <path> [json]
show <id>
list [--epic <id>] [--ready] [--all]
sequence <A> <B> [<C>...]
sequence rm <A> <B>
where
prune [--yes]
compact
quickstart
version
```

Global flags are `--json`, `--agent <identity>`, `--dir <path>`, `--help`, and
`--version`.

## Lifecycle postconditions

Direct lifecycle commands establish a postcondition. Callers do not need to
know or traverse a transition table.

### Claim

- Without an ID, claim selects only the oldest ready todo leaf task.
- With an ID, claim accepts todo, blocked, done, canceled, or legacy error.
- A specific claim may resume work that automatic readiness would not select.
- Claim writes doing and the claim identity in one locked batch.
- Existing body and results remain intact.
- Repeating a claim by the current owner succeeds without an event.
- Claiming work held by another identity is a conflict.
- Containers cannot be claimed.
- Oldest-ready selection and mutation occur under the same lock.

Claim JSON includes exact task-specific `next_commands` for done, block, cancel,
and release. When oldest-ready claim finds no work, it returns exit code 0 and:

```json
{"status":"no_ready","message":"No ready ergo tasks."}
```

### Done, block, and cancel

- Each accepts any readable leaf-task state, including legacy error.
- Each establishes its named state and clears the claim.
- Repeating the command in its postcondition succeeds without a redundant state event.
- A repeated command may still replace the body or attach a result.

### Release

- Release accepts todo, doing, blocked, or legacy error.
- It establishes unclaimed todo and keeps prior body and results.
- Releasing unclaimed todo is a no-op unless the call also supplies body or result data.
- It rejects done and canceled. A specific claim resumes closed work.
- Release represents unfinished work that remains valid. Block records an impediment.

## Content and result input

`new task [json]` accepts one JSON object with these fields:

- `title`: required, nonblank text.
- `epic`: a destination container ID.
- `state`: todo, doing, blocked, done, or canceled.
- `claim`: an agent identity; implies doing.
- `result`: one existing project-relative result file.

Unknown fields fail. New task never accepts the legacy error state. Piped stdin
is the literal initial body. Empty piped stdin creates an empty body.

`plan --file <path> [json]` requires a JSON title for the new container. The
file contains `# Title` task chunks separated by a line that is exactly `---`.
File order creates no dependencies.

`title` trims surrounding whitespace and rejects an empty title. It changes no
other field. `body` requires piped stdin and treats it literally; an empty pipe
clears the body. Both commands accept leaf tasks and containers.

Lifecycle commands treat piped stdin as a body replacement. TTY stdin leaves
the body unchanged. Body, result, state, and claim changes form one locked event
batch.

`--result` must be an existing regular file inside the active project root. It
cannot escape the root or point inside `.ergo/`. `--summary` requires `--result`;
the path is the default summary. Ergo records attachment-time SHA-256, mtime,
and git commit when available. Prior results remain in retry history. Repeat the
current lifecycle command to attach a late result.

## Placement

- `move <id> <container-id>` and `move <id> --root` are mutually exclusive.
- The source must be a leaf task. Containers never move or nest.
- The destination must exist at root.
- An existing container is valid.
- A root todo task with no claim or results may receive its first child and become a container.
- Moving into self or creating a container-child dependency is rejected.
- Moving to the current parent succeeds without an event.

## Dependencies

- `sequence A B` creates the edge where B depends on A.
- Longer sequences create one edge between each adjacent pair.
- `sequence rm A B` removes the edge where B depends on A.
- Self-dependencies and cycles are rejected.
- A task and its container cannot depend on one another.
- A dependency on a container completes when every child is done or canceled.

## Output and exit codes

Success returns exit code 0. Failure returns nonzero and writes an actionable
error to stderr. A missing graph names both recovery paths: initialize with
`ergo init`, or select an existing graph with `ergo --dir <path>`.

With `--json`, every successful command writes exactly one valid JSON value to
stdout and no non-JSON noise. Important shapes are:

- `list`: an array of task items.
- `show`: one task object, or a container object with children.
- direct mutation: an object with `kind`, `id`, `updated_fields`, and current state.
- claim: task details, claim identity and time, plus `next_commands`.
- `new task`, `plan`, `sequence`, `prune`, `compact`, `init`, and `where`: one command-specific object.

`updated_fields` names data that actually changed or was appended. Claim cleanup
is reported as `claim`. A true no-op returns an empty array. `claimed_by` is
present on mutation output only when the resulting task is doing.

Human mutations print the affected task ID. Claim also prints the title, body,
and all four next commands.

## List contract

Default list output omits done and canceled work. `--all` includes it. `--ready`
shows only ready work and conflicts with `--all`. `--epic <id>` scopes the view
to one valid container and its children.

Root rows have no tree connector. Child rows use connectors that make membership
clear. Result lines remain visually attached to their task. Empty selections
print a sentence rather than silent output. Human summaries use the rendered
view's scope. Explicit blocked work and todo work waiting on dependencies both
contribute to the blocked summary; unresolved legacy error remains a separate
compatibility bucket.

## Legacy plans

Ergo reads both `.ergo/plans.jsonl` and the legacy `.ergo/events.jsonl` name.
All historical event types remain replayable. Opening a repository never
rewrites its event log.

Unresolved legacy error and claimed-blocked tasks retain their exact state and
ownership during replay and compact. V2 normalizes them only when an explicit
lifecycle command states new intent:

| Legacy condition | Explicit outcomes |
| --- | --- |
| error with claim | claim to doing; release to todo; block, done, or cancel to named state |
| blocked with claim | claim to doing; release to todo; block clears claim; done or cancel closes |

Older Ergo binaries may append legacy events after v2 has opened a repository.
The next v2 invocation replays them. No migration command, event version, or
eager repository rewrite is required.

## Prune and compact

Prune is logical deletion. Without `--yes`, it returns a dry-run. With `--yes`,
it tombstones done and canceled leaf tasks, then any containers left empty.
Pruned IDs disappear from list, cannot be mutation or dependency targets, and
no longer block dependents.

Compact is physical maintenance. It rewrites the event log to the current live
graph and can remove pruned history. It is lossless for unresolved legacy state
and never guesses a replacement for error or claimed-blocked work.

## Locking

Graph reads and writes acquire `.ergo/lock`. A mutation validates and appends its
complete event batch while holding the lock. List and show return coherent
snapshots. Commands wait briefly before reporting lock contention. The lock file
must not be deleted to recover from contention; deleting it can split processes
across different inodes.
