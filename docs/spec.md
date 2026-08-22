# Ergo public CLI contract

This specification defines Ergo's public commands, behavior, output, and
storage. `ergo --help` provides the front door to the manual. `ergo quickstart`
provides the complete operating guide. Generated command help provides syntax
and options only.

## Terms and invariants

A leaf task has an ID, title, body, lifecycle state, optional claim,
dependencies, and timestamps. Its journal records work narrative and results.
A root task becomes an epic when it has children. Epics cannot nest.

A finished task satisfies dependencies. Successful work and finished work are
different concepts. Both successful and unsuccessful work can finish.

A task is ready when it has state `todo` and every direct and inherited
dependency has finished. A `todo` task with unfinished dependencies is waiting.
It is not blocked.

`draft` is visible planning work. It is unfinished, never ready, and remains
unclaimable until `open` moves the leaf to `todo`.

The claim invariant applies to every current state:

```text
claimed_by is present if and only if state is doing
```

| State | Meaning | Claimed | Finished | Default list | Prunable |
| --- | --- | --- | --- | --- | --- |
| `draft` | Staged planning work unavailable to agents. | No | No | Yes | No |
| `todo` | Open work. | No | No | Yes | No |
| `doing` | One agent owns the work. | Yes | No | Yes | No |
| `blocked` | An identified impediment prevents completion. | No | No | Yes | No |
| `done` | The objective succeeded. | No | Yes | No | Yes |
| `failed` | The work finished without satisfying the objective. | No | Yes | Yes | Yes |
| `canceled` | The objective is no longer wanted. | No | Yes | No | Yes |
| `error` | A released legacy record. Current commands never write this state. | Compatibility | No | Yes | No |

An epic has no stored lifecycle state, claim, journal result, or lifecycle
operation. An
epic finishes when every child finishes. A finished epic appears failed when
at least one child has state `failed`, canceled when none failed and at least
one was canceled, and done otherwise. Ergo derives this presentation and stores
no epic state.

## Command surface

```text
init [dir]
new task "<title>" [--epic <id>] [--draft]
new epic "<title>" --file <path> [--draft]
list [--epic <id>] [--ready | --all] [--json]
show <id> [--body]
claim [<id>] --agent <identity>
done <id> [-m <message>]
fail <id> [-m <message>]
block <id> [-m <message>]
cancel <id> [-m <message>]
open <id> [-m <message>]
result <id> "<text>" [--file <path>]
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
Color mode accepts `auto`, `always`, or `never`. It defaults to `auto`.
`--agent` belongs to `claim`.

## Repository discovery and initialization

Ergo discovers `.ergo` by walking upward from the current directory or from
`--dir <path>`. `init [dir]` creates the repository at its target. Repeating
`init` preserves an existing valid repository and repairs a missing selected
backlog file.

`where` prints the resolved `.ergo` path. `info` prints the running executable
and version, the project path, the `.ergo` path, and the selected backlog path.
Both commands use ordinary repository discovery. `info` reports the standard
missing-repository error outside an Ergo project.

## Creation

`new task` requires one nonblank positional title. It creates an unclaimed
`todo` leaf, or a visible but unavailable `draft` leaf with `--draft`. Optional
piped stdin becomes the literal body. No pipe or an empty pipe creates an empty
body. Successful creation prints only the generated six-character ID.

`--epic <id>` places the new task in an existing epic or promotes a clean root
`todo` or `draft` leaf. The promotion candidate must have no claim, children,
or results. Unknown, nested, claimed, closed, and result-bearing destinations
fail. The same promotion rule applies to `move`.

`new epic` requires one nonblank positional title and a nonempty `--file`. The
file contains Markdown chunks separated by a line that is exactly `---`. Each
chunk begins with `# Title`; the remaining text becomes the child body. Titles
must be unique within the file. File order creates no dependencies.

Optional piped stdin becomes the literal epic body. Ergo parses and validates
the full file before it writes one atomic batch. Empty files, malformed chunks,
and duplicate titles write nothing. `--draft` gives every child the `draft`
state in that same atomic batch. Success names the epic and every child and
reports task and dependency counts.

For both creation commands, Ergo reserves a positional JSON object containing
`title`, `epic`, `state`, `claim`, or `result` for an actionable syntax error.
Malformed brace-prefixed text and JSON objects without those keys remain valid
titles.

## Claim and lifecycle

Without an ID, `claim` selects the oldest ready `todo` leaf. With an ID, it may
resume `todo`, `doing`, `done`, `failed`, or `canceled` work, including failed
work, even when automatic readiness would not select that task. Draft and
blocked work must be opened first. Claim establishes `doing` and the supplied
identity under one lock. A repeated claim by the same owner is a no-op. A
different identity conflicts. An automatic claim with no candidate succeeds
without a mutation.

Claim output contains the complete task document followed by exact lifecycle
commands.

| Command | Resulting state | Finished | Release dependencies |
| --- | --- | --- | --- |
| `done` | `done` | Yes | Yes |
| `fail` | `failed` | Yes | Yes |
| `block` | `blocked` | No | No |
| `cancel` | `canceled` | Yes | Yes |
| `open` | `todo` | No | No |

`done`, `fail`, and `block` reject draft work; `cancel` and `open` are its only
lifecycle routes. `open` accepts `draft`, `doing`, `blocked`, and `todo`, clears
the claim, and treats `todo` as a no-op. It rejects finished work and legacy
`error`; a specific claim remains the recovery path for legacy error. The
`release` command is removed, but historical release journal entries remain
readable.

A repeated lifecycle postcondition without a new message is a no-op. A repeated
postcondition with a message records another automatic journal entry.

Lifecycle commands do not read stdin. Ergo trims each `-m` value and rejects a
blank value. Repeated values form paragraphs in the command's one automatic
journal entry. A successful lifecycle mutation records its kind and timestamp
even when no message was supplied.

A lifecycle receipt names the task and resulting state. It also names a cleared
claim, appended journal text, and oldest ready task when those facts apply. A
no-op receipt states that no event changed the backlog and writes no journal
entry.

## Journal and results

Every repository has one shared `.ergo/journal.jsonl`. A task journal is the
ordered subset of that file whose `task_id` matches the leaf. The backlog owns
current tasks, dependencies, claims, and lifecycle state. The journal alone
owns work narrative and results.

Each complete line is one versioned JSON object:

```json
{"version":1,"task_id":"ABCDEF","kind":"result","at":"2026-08-20T12:00:00Z","agent":"model@host","text":"Verified the repair","file":{"path":"docs/verification.md","sha256":"...","mtime":"2026-08-20T11:59:00Z","git_commit":"..."}}
```

`version`, `task_id`, `kind`, and `at` are required. `agent`, `text`, and
`file` are optional except that `result` requires nonblank `text`. File evidence
contains the cleaned project-relative `path`, SHA-256, modification time, and
current Git commit when available. Journal order is file order; timestamps use
UTC RFC 3339 with nanoseconds.

The allowed automatic kinds are `created`, `claim`, `done`, `fail`, `block`,
`cancel`, and `open`. Task and epic creation write `created`. A successful
state-changing claim or lifecycle command writes its corresponding kind. Reads,
title and body changes, moves, dependency changes, and true no-ops write
nothing. Automatic entries may name the responsible agent when Ergo knows it.

`result <id> "<text>" [--file <path>]` appends an explicit result to any
readable leaf without changing its lifecycle. Text must contain nonblank,
single-line content. Results may repeat without restriction. The optional file
must be an existing regular project file outside `.ergo`; Ergo applies the same
symlink and containment rules as other project paths and captures the evidence
fields above. Results reject epics and unknown or pruned IDs. No other command
attaches a file.

## Content and placement

`title` trims surrounding whitespace and rejects an empty value.

`body` requires piped stdin. It replaces the stored body literally by default.
An empty pipe clears the body. `--append` adds the piped bytes to the current
body under the write lock. Ergo adds no separator or newline. Empty append input
is a no-op. Leaves and epics both support title and body changes. A write that
produces the current value appends no event.

`move` accepts leaves only. The destination follows the promotion rules for
`new task --epic`. `--root` removes the current parent. Moving to the current
parent or root is a no-op. Epics cannot move or nest. Placement changes reject
ancestry dependency conflicts.

## Dependencies

`sequence A B` creates the edge where B depends on A. A longer sequence connects
each adjacent pair. `unsequence` removes the same edges. Existing links and
absent links are no-ops. Ergo validates the complete chain before writing, so a
failed chain writes no partial edges.

Ergo rejects self-dependencies, ancestry edges, and cycles. A dependency on an
epic finishes when that epic finishes. A child inherits dependencies assigned
to its epic. `done`, `failed`, and `canceled` leaves satisfy dependencies.
`blocked`, `doing`, `todo`, and legacy `error` leaves do not.

## Read output

Ergo prints readable text. Color is presentation metadata. ANSI color changes
presentation only.
`--color=auto` enables color when stdout is a terminal, `NO_COLOR` is absent,
and `TERM` is not `dumb`. `always` and `never` override those conditions.

Removing Ergo-added ANSI decoration from colored `show` or `claim` output produces
the same text as `--color=never`. Ergo decorates only synthesized text. It never
decorates stored bodies.

`list` prints a compact tree with state icons, terse claim ownership, and
actionable blocker names or counts. Results remain in the journal-backed `show`
projection rather than appearing inline in the task tree.
The default list omits `done` and `canceled` work. It includes `failed` work.
`--all` includes every readable state.
`--ready` selects ready leaves and conflicts with `--all`. `--epic <id>` selects
one epic and its children.

`show <id>` prints a synthesized document with YAML front matter, Markdown
content, and relationships. A leaf document includes task relationships. An
epic document includes its children. A leaf document includes its complete
retained journal in chronological order. An epic document shows each child's
current state and newest explicit result, but not the child's full journal. A
failed leaf uses an unmistakable failed presentation. A finished epic appears
failed when any child failed.

`show <id> --body` projects only the stored body of a leaf or epic. The
byte-preserving projection adds no formatting, color, or final newline. It
always writes literal stored bytes without adding ANSI decoration. It
preserves every byte and emits zero bytes for an empty body.

`claim` prints the same complete leaf document followed by exact next commands.
Focused writes print tangible new values or explicit no-op facts. `init`,
`compact`, and prune print their resolved actions and counts.

`list --json` applies the same filters and semantic order as the human list. It
writes one newline-terminated version 1 document:

```json
{
  "version": 1,
  "items": [
    {
      "id": "ABCDEF",
      "title": "Add login",
      "kind": "task",
      "state": "failed",
      "ready": false,
      "epic_id": "GHIJKL"
    }
  ]
}
```

Every item has `id`, `title`, and `kind`. Task items also have `state` and
`ready`. Epic items have their derived `state`. Child tasks have `epic_id`.
Ergo omits fields that do not apply. The
projection excludes bodies, graph relationships, journal entries, icons,
terminal layout, and ANSI decoration. Version 1 carries `failed` in the existing
state string and changes no document shape. Editor integrations use `show` when
they need journal evidence.

Success exits zero. Failure exits nonzero and writes an actionable message to
stderr. Unsupported commands, unsupported flags, and reserved creation JSON
write no graph events.

## Storage and compatibility

An initialized repository contains:

```text
.ergo/
├── backlog.jsonl
├── journal.jsonl
└── lock
```

The selected JSONL file contains transaction records and may begin with a
snapshot block after compaction. New repositories select `backlog.jsonl`.
Existing repositories continue to use `plans.jsonl` or `events.jsonl` in place.
Exactly one supported backlog file may exist. Repository opening does not
rename or rewrite the selected file.

Backlog replay constructs current tasks, epics, dependencies, metadata, and
tombstones. It accepts every released event shape covered by the compatibility
fixtures, including legacy messages and results until compaction migrates them.
A released `error` state remains readable but unwritable. A claim or lifecycle
command normalizes only its target.

Journal reading tolerates a missing file and one truncated final line. A later
journal write recreates a missing file. It rejects malformed complete records,
unsupported versions, invalid kinds, and entries without a task ID or valid
timestamp. Deleting the journal loses evidence but never corrupts or changes
the backlog.

Ergo stores `draft` and `failed` in the existing state string and keeps current
transaction, snapshot, and list JSON versions. Older Ergo binaries may reject a
backlog after a current binary records `draft`; upgrade all agents before using
staging. Ergo adds no dual encoding or automatic downgrade. Ergo 6 removes the
`release` command: migrate `release` to `open`, and migrate a blocked direct
claim to `open` followed by `claim`. Historical release journal entries remain
readable.

## Pruning, compaction, and concurrency

Prune performs logical deletion. Without `--yes`, it reports a deterministic
dry run. With `--yes`, it tombstones `done`, `failed`, and `canceled` leaves,
then epics left empty. Pruned IDs cannot be read, changed, or used as dependency
targets. They no longer block dependents.

Compact replaces the selected backlog with a deterministic snapshot block of
current live state. On the first Ergo 5 compaction, it migrates legacy backlog
messages and results into the journal and omits them from the new backlog
snapshot. Repeated compaction does not duplicate migrated evidence. Journal
compaction preserves every explicit `result` for surviving tasks and only the
newest automatic entry needed to explain each surviving task's current state.
It removes entries for pruned tasks. Explicit results may therefore grow
without limit; Ergo 5 adds no rotation, indexing, or retention policy.

Confirmed prune removes every journal entry for each selected task. Its dry run
reports both selected tasks and the number of journal entries that confirmation
would remove.

Every repository and journal view or update acquires `.ergo/lock`. An update
loads the current graph, validates its complete event batch against an isolated
copy, and appends one backlog transaction while it holds the lock. For mutations
that require an automatic journal entry, Ergo writes the backlog first and then
the journal under that same lock. If the journal append fails, Ergo returns an
explicit partial-success error stating that the backlog changed; it adds no
cross-file transaction or recovery protocol. List and show return coherent
views. Oldest-ready selection and claim occur in one update, so concurrent
agents cannot claim the same task.

Projects may track or ignore `journal.jsonl` independently of backlog policy.
Ergo does not choose that repository policy. Older binaries do not understand
the Ergo 5 split; the change is a clean major-version cutover rather than a
permanent two-source model.
