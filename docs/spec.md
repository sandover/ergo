# Ergo public CLI contract

This document defines Ergo's stable command, behavior, output, and storage
contract. `ergo --help` and `ergo quickstart` are the user manual. Generated
command help provides syntax and options only.

## Backlog model

Ergo stores tasks. A leaf task has an ID, title, body, lifecycle state, optional
claim, dependencies, lifecycle messages, results, and timestamps.

| State | Meaning |
| --- | --- |
| `todo` | Open and unclaimed. |
| `doing` | Claimed by one agent. |
| `blocked` | An identified impediment requires action. |
| `done` | The objective was satisfied. |
| `canceled` | The objective is no longer wanted. |

The claim invariant is exact:

```text
claimed_by is present if and only if state is doing
```

A task is ready when it is todo and every direct and inherited dependency is
complete. A todo task with unmet dependencies is waiting, not blocked.

An epic is a root task with children. It has no lifecycle state, claim,
lifecycle message, or result. Its completion is derived when every child is done
or canceled. Epics cannot be nested.

## Command surface

```text
init [dir]
new task "<title>" [--epic <id>]
new epic "<title>" --file <path>
list [--epic <id>] [--ready | --all] [--json]
show <id> [--body]
claim [<id>] --agent <identity>
done <id> [-m <message>] [--result <path>]
block <id> [-m <message>] [--result <path>]
cancel <id> [-m <message>] [--result <path>]
release <id> [-m <message>] [--result <path>]
title <id> <title>
body <id> [--append]
move <id> <epic-id>
move <id> --root
sequence <A> <B> [<C>...]
unsequence <A> <B> [<C>...]
where
info
prune [--yes]
compact
quickstart
version
```

Global flags are `--dir <path>`, `--color <mode>`, `--help`, and `--version`.
The color modes are `auto`, `always`, and `never`; the default is `auto`.
`--agent` belongs to `claim`.

## Creation

`new task` requires one nonblank positional title. It creates an unclaimed todo
task. Optional piped stdin becomes the literal initial body. No pipe or an empty
pipe creates an empty body.

`--epic <id>` places the task in an existing epic or in a clean, unclaimed root
todo task with no results. The latter becomes an epic when it receives its first
child. Unknown, nested, claimed, closed, or result-bearing destinations fail.

Successful task creation prints only the generated six-character ID.

`new epic` requires one nonblank positional title and a nonempty `--file`. The
file contains one or more Markdown chunks separated by a line that is exactly
`---`. Each chunk starts with `# Title`; the remaining text is the child body.
Titles are unique within the file. File order adds no dependencies.

Optional piped stdin becomes the literal epic body. The file is fully parsed and
validated before one atomic write. An empty or malformed file or duplicate
title fails without partial state. Successful output names the epic ID and
title, every child ID and title, and task and dependency counts.

For both creation commands, a positional operand that parses as a JSON object
containing `title`, `epic`, `state`, `claim`, or `result` is reserved for an
actionable syntax error. Malformed brace-prefixed text and JSON objects without
those keys remain valid titles.

## Claim and lifecycle

Without an ID, claim selects the oldest ready todo leaf. With an ID, claim
accepts any readable leaf state and may resume work that automatic readiness
would not select. It establishes doing plus the supplied identity under one
lock. Repeating a claim by its owner is a no-op; another identity conflicts.

Claim output is the complete task document followed by exact lifecycle commands.
When no automatic candidate exists, claim exits successfully without mutation.

Done, block, and cancel accept any readable leaf state. Each establishes its
named state and clears the claim. Release accepts unfinished readable work,
establishes unclaimed todo, and rejects done and canceled work. Repeating an
established postcondition may still append a message or result.

Lifecycle commands do not read stdin. Each `-m` value is trimmed and must be
nonblank. Repeated values join with a blank line into one append-only message
recorded with its command kind and time.

`--result` must name an existing regular file inside the project and outside
`.ergo`. Ergo resolves the project and target through symlinks before checking
containment. An in-project symlink to an in-project regular file is accepted,
and the cleaned caller-supplied project-relative path is retained. Ergo records
that path, the opened file's SHA-256 and mtime, and the git commit when
available. Results append as attempt history.

A lifecycle receipt names the task ID and title, resulting state, cleared claim,
appended message, attached result, and oldest ready task when each fact applies.
An event-free success is explicit.

## Focused mutations

`title` trims surrounding whitespace and rejects an empty value. `body` requires
piped stdin and replaces the body literally by default; an empty pipe clears it.
With `--append`, the piped bytes are added literally to the current stored body
under the write lock. Ergo adds no separator or newline. Empty append input is a
no-op. Both modes work on leaves and epics. Writes that produce the current body
append no event and report no change.

Move accepts leaves only. Its destination follows the same promotion rules as
`new task --epic`. Moving to the current parent or root is an explicit no-op.
Epics cannot move or nest. Placement changes reject ancestry dependency
conflicts.

`sequence A B` creates the edge where B depends on A. Longer sequences connect
each adjacent pair. `unsequence` removes the same edges. Existing or absent
edges are no-ops. Every chain validates before writing, so failure leaves no
partial change.

Self-dependencies, ancestry edges, and cycles fail. A dependency on an epic
completes when all its children are done or canceled. A child inherits external
dependencies assigned to its epic.

## Read output

Ergo prints readable text.

Color is presentation metadata carried by ANSI escape sequences. It does not
change the semantic text of output. `--color=auto` enables color only when
stdout is a terminal, `NO_COLOR` is absent, and `TERM` is not `dumb`.
`--color=always` enables color regardless of those conditions, and
`--color=never` disables it. Consequently, pipes and redirects are plain by
default.

Removing Ergo-added ANSI decoration from a colored `show` or `claim` document
yields the exact `--color=never` document. Color sequences decorate only
synthesized presentation; stored task and epic bodies remain literal.
`show --body` emits the exact stored bytes without adding ANSI decoration in
any color mode.

- `list` prints a compact tree with state icons, terse `@agent` ownership, and
  actionable blocker names or counts.
- `show <id>` prints a synthesized document: YAML front matter followed by
  Markdown content and relationships. A leaf document includes its task
  relationships. An epic document includes its children.
- `show <id> --body` projects only the stored body of either a leaf or an epic.
  The projection is byte-preserving: it adds no formatting or final newline,
  preserves every stored trailing newline, and emits zero bytes for an empty
  body.
- `claim` prints the leaf task document followed by exact lifecycle commands.
- focused writes print tangible resulting values or explicit no-op facts.
- `init` reports whether it initialized, repaired, or found a graph and prints
  its resolved absolute `.ergo` path.
- `where` prints the resolved `.ergo` path. `info` prints the running
  executable and version together with the resolved project, `.ergo`, and
  selected backlog paths.
- `compact` reports its resolved log path, source record count, and resulting
  snapshot record count.
- prune prints a preview or applied summary.

Default list omits done and canceled work. `--all` includes it. `--ready` selects
ready leaves and conflicts with `--all`. `--epic <id>` selects one epic and its
children.

`list --json` applies the same filters and semantic order, then writes one
newline-terminated JSON document. Version 1 has this shape:

```json
{
  "version": 1,
  "items": [
    {
      "id": "ABCDEF",
      "title": "Add login",
      "kind": "task",
      "state": "todo",
      "ready": true,
      "epic_id": "GHIJKL"
    }
  ]
}
```

Every item has `id`, `title`, and `kind`. Task items also have `state` and
`ready`; child tasks have `epic_id`. Fields that do not apply are omitted.
This projection intentionally excludes task bodies, graph relationships,
terminal layout, summary text, icons, and ANSI decoration.

`info` uses ordinary repository discovery from the current directory or
`--dir`. It therefore reports the standard missing-backlog error outside an
Ergo project; integrations probe executable compatibility with `--version`
before requesting repository diagnostics.

Success exits zero. Failure exits nonzero and writes an actionable message to
stderr. Unsupported commands or flags and reserved creation JSON produce
one-line current-command guidance and write no graph events.

## Storage and locking

The active repository contains:

```text
.ergo/
├── backlog.jsonl
└── lock
```

The JSONL file contains transaction records and, after compaction, a snapshot
block. New repositories use `.ergo/backlog.jsonl`. Existing repositories with
`.ergo/plans.jsonl` or `.ergo/events.jsonl` continue to use that file in place.
Exactly one supported log may exist; opening fails if more than one is present.
Opening a backlog does not rename or rewrite its selected log.

Replay constructs current tasks, epics, dependencies, messages, results,
metadata, and tombstones. It accepts every stored event shape supported by the
repository fixtures. A stored `error` state remains readable; current commands
do not create it. Claim or a lifecycle command normalizes only its target task.

Prune is logical deletion. Without `--yes`, it is a dry-run. With `--yes`, it
tombstones done and canceled leaves, then epics left empty. Pruned IDs cannot be
read, changed, or used as dependency targets and no longer block dependents.

Compact replaces the selected log with a deterministic snapshot block of
current live state. It removes superseded records and pruned history without
changing the current backlog.

Repository views and updates acquire `.ergo/lock`. An update loads the current
graph, validates its full event batch on an isolated copy, and appends the batch
as one transaction record while holding the lock. List and show use coherent
views. Oldest-ready selection and claim occur in one update, so concurrent
agents cannot claim the same task.
