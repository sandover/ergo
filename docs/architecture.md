# Ergo architecture

This document explains the implementation boundaries that maintainers must
preserve. `ergo --help` and `ergo quickstart` are the user manual.
`docs/spec.md` defines stable public behavior.

## System shape

Ergo is a repository-local, dependency-aware backlog. It uses a JSONL file and
an advisory lock instead of a database or daemon. Each command discovers a
repository, reconstructs a task graph, performs one use case, and writes readable
output.

The runtime dependencies flow in one direction:

```text
process capabilities
    → fresh Cobra command tree
    → typed application request
    → repository
    → record and event codecs
    → reducer and graph rules
    → typed application outcome
    → renderer
    → injected output stream
```

The command tree owns argument and flag parsing, stdin policy, terminal
capabilities, error hints, and exit status. The application layer owns use
cases. The repository owns persistence and locking. Renderers turn outcomes
into the public readable output contract.

## Repository discovery

Ergo searches the selected directory and its ancestors for `.ergo/`. Without
`--dir`, the selected directory is the process working directory.

An initialized repository contains a lock and exactly one supported log:

```text
.ergo/
├── backlog.jsonl
└── lock
```

New repositories create `backlog.jsonl`. Existing `plans.jsonl` and
`events.jsonl` logs remain supported in place. If exactly one supported log
exists, Ergo uses it. If more than one exists, opening fails and asks the user
to reconcile them. Ergo does not choose one by precedence, rename a selected
log, or create `backlog.jsonl` beside an existing supported log.

An opened `Repository` selects its data, log, and lock paths once. Its `View`,
`Update`, and `Compact` operations provide the persistence boundary used by the
application.

## Physical log records

The selected log is JSONL. Current mutations append a versioned `transaction`
record containing the command's complete, nonempty event batch. A transaction
is one physical line, so replay either observes the complete batch or ignores
an interrupted partial line. Transaction records and snapshot records are
individually limited to 10 MiB.

Released repositories may contain standalone event records. The record codec
continues to decode them, while current writers emit transaction records.
Decoded events retain their source path, physical line, and transaction-event
index so corruption reports identify the responsible input.

The event codec maps every supported wire kind to a typed payload before domain
reduction. Current and legacy event registries are explicit and disjoint.
`new_epic` is normalized to task creation with legacy explicit-epic identity.
Unknown event kinds and unsupported record versions fail closed. Additive JSON
payload fields are tolerated.

## Reading and recovery

Log inspection accepts blank lines and a valid final record without a newline.
Malformed JSON in an unterminated final record is treated as an interrupted
write and excluded from replay. Before the next append, Ergo truncates that
partial tail. If the final record is valid but lacks a newline, append inserts
the missing separator.

Recovery is deliberately narrow. A complete JSON value with an unsupported
version or invalid semantics is corruption. An incomplete or invalid snapshot
is also corruption. Complete malformed lines report their path and line
instead of being silently discarded.

This policy provides replay-visible transaction atomicity, not rollback of
every operating-system write. A command that reports a late write or sync
failure may have placed bytes on disk. The next read validates what is present,
and the interrupted-tail rule repairs only an incomplete final JSON record.

## Locking and repository updates

`View` acquires `.ergo/lock`, loads the selected log once, and returns a coherent
graph. List and show therefore cannot observe the middle of a transaction.

`Update` holds the same lock for its entire operation:

1. Load the current graph.
2. Ask the use case to build its complete event batch from that graph.
3. Apply the batch to an isolated graph copy with the pure reducer.
4. Reject the operation if reduction or invariant validation fails.
5. Append the batch as one transaction record.
6. Sync the log and return the already validated candidate graph.

The repository does not reload after append. Validation completes before any
new bytes are written. Short writes are retried until the transaction is
complete or writing stops making progress.

The lock operation retries with jitter for up to ten seconds. The lock file is
a synchronization inode, not application state. The operating system releases
the advisory lock when a process exits.

Oldest-ready selection and claim occur inside one `Update`, so concurrent
agents cannot claim the same task. Confirmed prune also computes its targets
and creates all tombstones inside one update, avoiding a plan/apply race.

## Reduction and canonical graph state

The reducer is the only event-to-state transition mechanism. Full replay starts
with an empty graph. Transaction validation applies events to a clone, leaving
the caller's graph unchanged on both success and failure. After reducing all
events, replay validates cross-event invariants and rebuilds derived indexes.

The canonical in-memory state consists of:

- tasks, including bodies, lifecycle data, results, and messages;
- forward dependency edges; and
- tombstone metadata.

Reverse dependencies and children grouped by epic are derived indexes.
Legacy explicit empty-epic identity is compatibility metadata. Results and
lifecycle messages are ordered newest first.

A tombstone removes its task and every incident dependency from the live graph.
Later events for that ID do not restore it. Tombstones remain in an
event-replayed graph so pruned IDs cannot be reused or targeted.

Replay validates event payloads, timestamps, task and parent existence,
placement depth, dependency endpoints, ancestry rules, and lifecycle
invariants. Released compatibility forms are handled explicitly rather than
weakening current write rules.

## Tasks, epics, and lifecycle

Task is the only stored entity. A root task becomes an epic when another task's
parent ID refers to it. Legacy `new_epic` records preserve empty epics that were
explicitly created by older versions.

A leaf task has one of the current forward states: `todo`, `doing`, `blocked`,
`done`, or `canceled`. The historical `error` state remains readable but cannot
be written by current commands. Forward writes enforce:

```text
state=doing  <=>  claimed_by is nonempty
```

Released histories may contain claimed `blocked` or `error` tasks. Replay
accepts those compatibility forms. Claim and lifecycle commands state their
target postcondition directly, so acting on one of those tasks normalizes it
without an intermediate command.

The shared mutation path handles lifecycle state, claim ownership, title, body,
placement, lifecycle messages, and result attachment. It suppresses true
same-value changes. A message, result, or required legacy claim cleanup still
produces events when the requested state itself is unchanged.

Epics remain at the root and cannot nest or move. A clean, unclaimed root
`todo` task with no results may be promoted when it receives its first child.
An epic has no direct lifecycle, claim, result, or lifecycle-message behavior.
Its completion is derived from its children.

## Dependencies and readiness

The graph stores `A depends on B` as a forward edge from A to B. Writes reject
self-dependencies, cycles, and edges between an epic and its own child.

A leaf is ready when it is unclaimed, in `todo`, and all direct and inherited
dependencies are complete. A child inherits dependencies assigned to its epic.
A dependency on an epic is complete when every child is `done` or `canceled`.
An explicitly `blocked` task is distinct from a `todo` task waiting on a
dependency.

Ready work is ordered by creation time and then ID. Automatic claim selects the
first item in that order while holding the repository lock.

## Prune and snapshots

Prune is logical deletion. A dry run computes a deterministic plan under a
coherent read lock. Confirmed prune selects done and canceled leaves and then
epics with no remaining children. It appends the resulting tombstones as one
transaction.

Compact is the only operation that replaces the selected log. While holding
the lock, it loads the current graph and creates a deterministic snapshot block:

1. A manifest records format version, record counts, and a SHA-256 integrity
   digest.
2. Ordered task records follow.
3. Ordered result and message records follow.
4. Ordered dependency records finish the block.

Each record remains independently bounded. The decoder checks the manifest,
counts, ordering, referential integrity, and digest before accepting the
snapshot. Transactions may follow a snapshot and replay onto it normally.

A snapshot represents current live state, not reconstructed history.
Compaction intentionally garbage-collects tombstones and superseded events.
It preserves current tasks, explicit empty-epic identity, dependencies,
messages, results, and readable compatibility lifecycle state. Replacement
writes and syncs a temporary file, renames it over the selected log, and syncs
the containing directory.

## Application boundary and errors

`Application` is the stream- and terminal-independent use-case facade. It is
bound to repository options; request-specific data such as claim identity stays
on each typed request. Each operation returns a typed outcome for rendering.
`WithRepository` copies the base application so each command tree can own its
repository selection without mutating shared state.

Application errors carry a stable classification while preserving the detailed
wrapped error:

- `usage`
- `not_found`
- `conflict`
- `busy`
- `corruption`
- `internal`

The Cobra adapter uses the classification to choose recovery hints. Command
success or failure determines the process exit status. Repository code does not
format CLI errors.

The application may use repository-relative filesystem services, such as
reading an epic input file, resolving the working directory, and inspecting a
result. Its independence guarantee concerns process streams, terminal state,
rendering, and mutable command context.

## Results

Result attachment occurs inside the mutation lock. The supplied path must be
local and relative to the project root. Its symlink-resolved target must remain
inside that root, outside `.ergo/`, and be a regular file.

Ergo preserves the accepted caller-relative path and captures its SHA-256
digest and modification time. It also attempts to capture the current Git
commit with a two-second timeout; Git metadata is optional. Renderers derive a
`file:` URL from the project root and stored relative path. Replay keeps
attached results newest first.

## Rendering and the CLI

`main` captures stdin, stdout, stderr, terminal status, terminal width, command
arguments, build version, and process exit. It passes those capabilities into
`NewRootCommand`; package-level command objects and mutable global flags are not
used.

Every execution builds a fresh Cobra tree. The tree owns flags, writers,
version, and repository directory for that execution. Cobra handlers translate
arguments into typed application requests, then pass successful outcomes to
renderers. Errors go to the injected error stream.

Cobra also owns stdin policy:

- `new task` and `new epic` use piped stdin as an optional initial body;
- `body` requires piped stdin, and an empty pipe clears the body; and
- lifecycle commands reject piped stdin.

Renderers accept explicit writers. List and prune also receive explicit color
and width capabilities, with an 80-column fallback. List renders a compact
tree. Show and claim render YAML front matter followed by Markdown.
`show --body` is a lossless projection that writes exactly the stored body,
including an empty body or trailing newlines. Focused mutations render concise
receipts.

Compatibility `Run*` wrappers remain package-internal for older callers and
tests. The production Cobra path uses typed application operations and
renderers directly.

## Help ownership

The help system has two authoritative manual layers:

- `internal/ergo/help.txt` is the root front door. It owns the mental model,
  first workflow, command inventory, global flags, and navigation.
- `internal/ergo/quickstart.txt` owns the complete cross-command model and
  operating guide.

Cobra command metadata supplies command-local syntax, flags, constraints, and
stdin annotations. It does not form a third manual. `README.md`, the shipped
skill, this architecture, release guidance, and error text consume the same
contract without competing for manual ownership.

Behavior, focused tests, the public specification, and the two manual layers
must change together when a user-visible contract changes.

## Code map

- `repository*.go`: discovery, locking, coherent reads, transactional updates,
  storage, bulk creation, and dependency writes.
- `log_codec.go` and `event_codec.go`: physical records, typed event decoding,
  source context, and compatibility normalization.
- `reducer.go` and `graph_queries.go`: state reconstruction, invariant
  validation, derived indexes, readiness, and graph queries.
- `snapshot.go`: deterministic bounded snapshot encoding and validation.
- `model.go`, `mutation.go`, and domain-specific files: entities, write
  invariants, and atomic mutation construction.
- `application*.go`: typed use-case requests, outcomes, and classified errors.
- `list_*`, `render_*`, `maintenance_surface.go`, and `work_assignment.go`:
  presentation models and readable renderers.
- `cmd/ergo`: fresh Cobra composition, process capabilities, stdin policy,
  help routing, error hints, and exit status.
