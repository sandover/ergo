# Proposal: `ergo plan` — create an epic + task graph from one JSON document

## Problem

Creating a planned epic today requires N+1 commands:

```bash
printf '%s' '{"title":"Add auth"}' | ergo new epic        # → EPIC01
printf '%s' '{"title":"Add middleware","epic":"EPIC01"}' | ergo new task  # → AAAAAA
printf '%s' '{"title":"Add endpoint","epic":"EPIC01"}' | ergo new task   # → BBBBBB
printf '%s' '{"title":"Write tests","epic":"EPIC01"}' | ergo new task    # → CCCCCC
ergo sequence AAAAAA BBBBBB
ergo sequence BBBBBB CCCCCC
```

A 10-task epic with sequencing requires ~15 subprocess calls. Each call acquires the file lock, replays the event log, appends events, and releases the lock. This is:

- **Slow.** Lock acquisition + full log replay per call. Agents creating plans spend more time on ergo I/O than on thinking.
- **Fragile.** A failure mid-sequence leaves a partial plan — tasks exist but edges don't, or half the tasks are missing. The agent must detect and recover from partial state.
- **Awkward.** The agent must capture IDs from stdout, thread them into subsequent commands, and express sequencing by positional ID — even though the plan exists as a coherent structure in the agent's context before the first call.

## Proposal

Add `ergo plan` — a single command that accepts a structured document (JSON on stdin) describing an entire epic with tasks and sequencing, and creates everything in one mutation transaction.

```bash
printf '%s' '{
  "title": "Add user auth",
  "body": "## Goal\nAdd signup and login.\n\n## Scope\n...",
  "tasks": [
    {"title": "Add auth middleware", "body": "..."},
    {"title": "Add login endpoint", "body": "...", "after": ["Add auth middleware"]},
    {"title": "Add signup endpoint", "body": "...", "after": ["Add auth middleware"]},
    {"title": "Write integration tests", "body": "...", "after": ["Add login endpoint", "Add signup endpoint"]}
  ]
}' | ergo --json plan
```

One call. One lock acquisition. One log replay. The caller gets either a complete plan or an error.

## Input contract

### JSON stdin (default)

```json
{
  "title": "Epic title",
  "body": "Epic body (optional)",
  "tasks": [
    {
      "title": "Task title (required)",
      "body": "Task body (optional)",
      "after": ["Title of predecessor task (optional)"]
    }
  ]
}
```

**Fields:**

| Field | Type | Required | Notes |
|---|---|---|---|
| `title` | string | yes | Epic title |
| `body` | string | no | Epic body (markdown) |
| `tasks` | array | yes | At least one task |
| `tasks[].title` | string | yes | Task title; must be unique within the plan |
| `tasks[].body` | string | no | Task body (markdown) |
| `tasks[].after` | string[] | no | Titles of tasks within this plan that must complete first |

**Constraints:**

- `tasks[].title` values must be unique within the plan (they serve as local references for `after`).
- `after` references must resolve to a task title within the same plan. Dangling references are rejected.
- The `after` graph must be acyclic. Cycles are rejected as `validation_failed` (message may mirror `ergo sequence` wording).
- Unknown keys are rejected as `parse_error` with fuzzy suggestions, consistent with `new`/`set`.
- Task-title references in `after` are exact, case-sensitive matches.

### Out of scope for v1

`--body-stdin` and flags-only authoring can be added later. v1 focuses on JSON stdin because it covers the primary agent workflow and keeps the contract tight.

### Flags-only mode (future, optional)

For simple plans from a TTY:

```bash
ergo plan --title "Quick fix" --task "Do the thing" --task "Verify it"
```

Low priority. Agents will use JSON stdin.

## Output contract

### `--json` (agent mode)

```json
{
  "kind": "plan",
  "epic": {
    "id": "EPIC01",
    "uuid": "...",
    "title": "Add user auth",
    "created_at": "..."
  },
  "tasks": [
    {"id": "AAAAAA", "title": "Add auth middleware"},
    {"id": "BBBBBB", "title": "Add login endpoint"},
    {"id": "CCCCCC", "title": "Add signup endpoint"},
    {"id": "DDDDDD", "title": "Write integration tests"}
  ],
  "edges": [
    {"from_id": "BBBBBB", "to_id": "AAAAAA", "type": "depends"},
    {"from_id": "CCCCCC", "to_id": "AAAAAA", "type": "depends"},
    {"from_id": "DDDDDD", "to_id": "BBBBBB", "type": "depends"},
    {"from_id": "DDDDDD", "to_id": "CCCCCC", "type": "depends"}
  ]
}
```

### Human mode (no `--json`)

Print the epic ID and a compact summary:

```
Created epic EPIC01: Add user auth (4 tasks, 4 dependencies)
```

### Errors

Parse failures and validation failures follow the existing error envelope:

```json
{"error": "validation_failed", "message": "...", "invalid": {...}}
```

Specific error cases:
- Duplicate task title within plan
- Dangling `after` reference (title not found)
- Cycle in `after` graph
- Empty `tasks` array
- Missing required fields
- Unknown keys (`parse_error`, with suggestions when available)
- Malformed JSON / multiple top-level JSON values (`parse_error`)

## Semantics

- **Transaction semantics.** Validation happens before mutation. Under the lock, the command computes the full new event set and commits once. If validation fails, nothing is written.
- **One lock, one replay.** The event log is replayed once per plan command. This is O(1) lock acquisitions regardless of plan size.
- **IDs are generated server-side.** The caller never needs to know or manage IDs during plan creation. The response returns all generated IDs.
- **Event and output ordering.** Task creation events follow input array order, and `tasks` in output preserve that order. IDs remain opaque/random.
- **Dependency edges are derived from `after`.** The `after` field maps title → ID internally. Output edge direction matches `sequence`: `from_id` depends on `to_id`.
- **All tasks belong to the created epic.** There is no option to create orphan tasks via `plan`.
- **Initial state is `todo` for all tasks.** No create-and-claim within `plan`. Agents should `plan` then `claim` — these are separate concerns.

## Implementation sketch

1. **Parse and validate** the input document. Check: required fields present, titles unique, `after` references resolve, `after` graph is acyclic, and unknown keys are rejected.
2. **Acquire lock** (single `flock`).
3. **Replay log** to get current graph (for ID collision checks and invariant validation).
4. **Generate IDs** for epic + all tasks.
5. **Build events:** `new_epic` + N × `new_task` + M × `link`.
6. **Commit once under lock** (transaction-style): write full new event file to temp path, `fsync`, and atomic rename.
7. **Release lock.**
8. **Emit output** (JSON or human).

The core logic is still small (~150-220 lines). Most machinery already exists: ID generation, lock management, replay, JSON parsing, validation helpers, and event-file rewrite utilities.

## Relationship to `show`

`ergo show <epic-id>` renders graph state for review (Markdown in human mode, structured JSON in machine mode). `ergo plan` builds graph state from a structured input document.

Conceptually this is close to a round-trip, but not yet literal:

```
ergo show EPIC01 --json -> transform to plan-input shape -> ergo plan
```

A true round-trip would benefit from a dedicated export shape (for example `show --plan-json`) and, later, an update mode.

## What this does NOT do

- **No natural-language parsing.** The input is structured JSON, not prose. Agents already produce structured output natively.
- **No templates or blueprints.** The plan is fully specified by the caller. Reusable templates are a separate concern.
- **No cross-epic references.** `after` resolves within the plan only. Cross-epic sequencing uses `ergo sequence` after creation.
- **No create-and-claim.** Plan creates work; claim assigns it. Mixing them adds complexity without clear value.
- **No update/diff mode (v1).** First version is create-only. Update-in-place is a natural follow-up.

## Impact

- **Agent planning time:** ~15 subprocess calls → 1. Proportional reduction in lock contention, log replays, and shell overhead.
- **Failure handling:** Validation failures never mutate state; commit path can provide all-or-nothing durability when implemented via temp-file + rename.
- **Cognitive load:** Agents express plans as documents (their native format) instead of imperative command sequences.
- **Zero breaking changes:** New command and output contract; existing event types (`new_epic`, `new_task`, `link`) and existing workflows stay unchanged.

## Open questions

1. **Should `after` support IDs in addition to titles?** Titles are more ergonomic for plan authoring, but IDs would allow referencing tasks outside the plan (cross-epic deps). Recommendation: titles-only for v1, keeping the scope tight.

2. **Should `plan` support `claim` on individual tasks?** An agent might want to create a plan and immediately claim the first task. Counter-argument: `plan` then `claim` is two calls but keeps concerns separate. Recommendation: no claim in v1.

3. **Should output include a title-to-id map?** Returning only `tasks[]` is sufficient, but a `task_ids_by_title` object could simplify downstream scripting. Recommendation: keep v1 minimal (no redundant map).

4. **Do we need a hard cap on plan size?** Current log mechanics can handle large plans, but a soft guardrail (for example, warning over N tasks) may prevent accidental massive writes. Recommendation: no hard cap in v1; consider telemetry-informed limits later.
