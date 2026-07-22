# Ergo public CLI contract

This document defines the stable Ergo 3 command and data contract. `ergo --help`
is the compact manual. `ergo quickstart` is the complete operational guide.

## Domain model

Ergo stores tasks. A leaf task has an ID, title, body, lifecycle state, optional
claim, dependencies, lifecycle messages, results, and timestamps. A container is
a root task with children. It has no lifecycle, claim, message, or result of its
own. It completes when every child is done or canceled.

Leaf states are:

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

A todo task with unmet dependencies is waiting. The legacy `error` state remains
readable, but no current command creates it.

## Command surface

Existing tasks change through focused commands:

```text
claim [<id>] --agent <identity>
done <id> [-m <message>] [--result <path>]
block <id> [-m <message>] [--result <path>]
cancel <id> [-m <message>] [--result <path>]
release <id> [-m <message>] [--result <path>]
title <id> <title>
body <id>
move <id> <container-id>
move <id> --root
sequence <A> <B> [<C>...]
unsequence <A> <B> [<C>...]
```

Other commands are:

```text
init [dir]
new task [json]
plan --file <path> [json]
show <id>
list [--epic <id>] [--ready | --all]
where
prune [--yes]
compact
quickstart
version
```

Global flags are `--agent <identity>`, `--dir <path>`, `--help`, and
`--version`. There is no generic mutation, reopen, or separate output mode.

## Lifecycle postconditions

### Claim

- Without an ID, claim selects the oldest ready todo leaf.
- With an ID, claim accepts todo, blocked, done, canceled, or legacy error.
- A specific claim may resume work that automatic readiness would not select.
- Claim writes doing and its identity in one locked batch.
- Repeating a claim by its owner succeeds without an event.
- Claiming work held by another identity is a conflict.
- Containers cannot be claimed.
- No automatic candidate prints `No ready ergo tasks.` and exits 0.

Claim output starts with `---` and then `id: "ABCDEF"`. This fixed position lets
an agent recover the claimed ID without interpreting prose. The complete task
follows, then a `## Next` section with exact lifecycle commands.

### Done, block, and cancel

- Each accepts any readable leaf state, including legacy error.
- Each establishes its named state and clears the claim.
- Repeating an established postcondition adds no state event.
- A repeated command may still append a message or result.

### Release

- Release accepts todo, doing, blocked, or legacy error.
- It establishes unclaimed todo and preserves prior content and results.
- Releasing todo adds no state event unless the call also appends a message or result.
- It rejects done and canceled. A specific claim resumes closed work.
- Release means unfinished work remains valid. Block means an impediment requires action.

## Content, messages, and results

`new task [json]` accepts one inline JSON object:

- `title`: required, nonblank text.
- `epic`: destination container ID.
- `state`: todo, doing, blocked, done, or canceled.
- `claim`: agent identity; implies doing.
- `result`: one existing project-relative file.

Unknown fields fail. New tasks cannot use legacy error. Piped stdin is the
literal initial body. An empty pipe creates an empty body.

`plan --file <path> [json]` requires `{"title":"..."}`. Its file contains
`# Title` chunks separated by a line that is exactly `---`. File order creates
no dependencies.

`title` trims surrounding whitespace and rejects an empty value. `body` requires
piped stdin and treats it literally; an empty pipe clears the body. Both accept
leaves and containers. `body` is the only existing-task body writer.

Lifecycle commands reject piped stdin, including an empty pipe. Each `-m` value
is trimmed and must be nonblank. Repeated values join with a blank line into one
append-only lifecycle message. The message records its command kind and time.

`--result` must be an existing regular file inside the project and outside
`.ergo/`. New results use the path as their stored identity. Ergo records the
attachment-time SHA-256, mtime, and git commit when available. Results append as
attempt history. Distinct summaries from legacy logs remain readable.

## Placement and dependencies

- A move source must be a leaf. Containers never move or nest.
- A destination must exist at root.
- An existing container is valid.
- A clean, unclaimed root todo task with no results becomes a container on its first child.
- A self-move or container-child dependency conflict fails.
- Moving to the current parent succeeds without an event.

`sequence A B` creates the edge where B depends on A. Longer sequences create
an edge between each adjacent pair. `unsequence` removes the same edges.
Existing or absent edges are no-ops. Every chain validates before writing, so a
failure leaves no partial change.

Self-dependencies and cycles fail. A task and its container cannot depend on one
another. A dependency on a container completes when every child is done or
canceled.

## Output and exit codes

Ergo has one readable output mode:

- `list` prints a compact tree with state and claim labels.
- `show` prints YAML front matter followed by Markdown content and relationships.
- `claim` prints the same task document followed by task-specific next commands.
- `new task` prints the generated ID.
- `plan` prints its container, child IDs and titles, and dependency count.
- focused writes print the affected ID and resulting postcondition.
- `init` and `where` print the active `.ergo` path.
- `compact` prints one completion line; prune prints a preview or applied summary.

Default list omits done and canceled work. `--all` includes it. `--ready` shows
only ready work and conflicts with `--all`. `--epic <id>` selects one valid
container and its children.

Success exits 0. Failure exits nonzero and writes an actionable message to
stderr. Missing-graph errors name `ergo init` and `ergo --dir <path>`.

Inline JSON remains creation input. The append-only event log remains JSONL.
Output `--json` is not a public mode. The removed `--json`, `--summary`, and
`sequence rm` forms fail with their direct replacements.

## Legacy plans

Ergo reads `.ergo/plans.jsonl` and legacy `.ergo/events.jsonl`. Opening a graph
never rewrites or migrates it. Every historical event type remains replayable.

Replay and compact preserve unresolved legacy error, claimed-blocked work,
lifecycle body updates, and result summaries. An explicit claim or lifecycle
command normalizes only its target task. Older binaries may append legacy events;
the next invocation replays them without an event-version conversion.

## Prune, compact, and locking

Prune is logical deletion. Without `--yes`, it is a dry-run. With `--yes`, it
tombstones done and canceled leaves, then containers left empty. Pruned IDs
cannot be inspected, mutated, or used as dependency targets and no longer block
dependents.

Compact is physical maintenance. It rewrites the log to current live state and
can remove pruned history. It preserves unresolved legacy state and does not
guess a replacement.

Reads and writes acquire `.ergo/lock`. A mutation validates and appends its full
event batch while holding the lock. List and show return coherent snapshots.
Oldest-ready selection and claim occur under the same lock, so concurrent agents
cannot claim the same task.
