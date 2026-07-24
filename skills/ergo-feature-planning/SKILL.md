---
name: ergo-feature-planning
description: >-
  Shape and execute multi-step software work with Ergo, a repository-local,
  dependency-aware backlog. Use when work is likely to span 3 or more commits,
  crosses concerns such as API, UI, tests, migration, or docs, or needs design
  decisions and dependency ordering before implementation. Skip small,
  single-concern changes and routine housekeeping.
---

# Ergo Feature Planning

Turn an accepted goal into a dependency-ordered backlog that another agent can
execute without the original conversation. Keep the backlog smaller than the
work.

## Bootstrap

1. Expect `ergo` to be installed. If it is missing, ask the user to install it.
2. Run `ergo --help` and `ergo quickstart` before shaping or executing work.
3. Run `ergo where`. If no backlog exists, confirm the repository root and run
   `ergo init`.
4. If a command fails, read its error. Use `ergo <command> --help` to check
   syntax and options. Do not guess alternate syntax.

## Resolve decisions first

Backlog shaping exposes ambiguity. Present concrete options and tradeoffs, ask
the user, and record the decision in the relevant epic or task body. Do not hide
a question behind `TBD`, `Consult me`, or a vague future checkpoint.

Use a checkpoint only when an implementation artifact is required to decide.
Name the artifact, exact question, and instruction not to continue without
approval. If the user cannot decide yet, create a spike whose result supplies
the missing knowledge.

Revise earlier tasks when later work reveals better boundaries. Do not preserve
a weak split merely because it was written first.

## Build the backlog

Use one epic for each coherent feature area. Put shared scope, non-goals,
constraints, decisions, and assumptions in its body. Leave genuinely
standalone work at the root.

Build an epic incrementally:

```sh
ergo new task "Authentication"
# prints the task ID, for example OFKSTE

printf '%s\n' '## Goal' '- Add session validation.' |
  ergo new task "Validate sessions" --epic OFKSTE
```

`new task` prints only its ID. Optional piped stdin becomes the literal initial
body. The first child promotes a clean root todo task to an epic.

For a prepared Markdown backlog, use:

```sh
ergo new epic "Authentication" --file tasks.md
```

Optional piped stdin becomes free-form epic context. The command names the epic
and every child it creates; retain those IDs for dependencies and review.

Add only real ordering constraints. `ergo sequence TASK_A TASK_B` means B waits
for A. Prefer independent tasks and preserve safe parallelism.

## Shape tasks

Make each task one atomic, reviewable change that normally fits one session.
Split on real boundaries: public API, data model, migration, UI, tests, or docs.
Avoid tiny bookkeeping tasks and broad tasks with several reviewable outcomes.

Write for a capable agent with less context and possibly less reasoning ability.
Include the paths, behavior, edge cases, and runnable proof needed to succeed.

Use this body shape and omit empty sections:

```md
## Goal
- <Concrete outcome and why it matters>

## Context
- <Relevant decisions, paths, contracts, and constraints>

## Acceptance Criteria
- <Observable behavior and important edge cases>

## Checkpoint
- Produce: <specific artifact>
- Then ask: <specific decision question>
- Do not proceed without approval.

## Validation Gates
- <Exact test, lint, build, or inspection commands>
```

Prefix knowledge-producing work with `spike:`. State what dependent tasks must
learn from it.

## Review the backlog

Before presenting it, check:

- Coverage: implementation, tests, docs, migration, compatibility, and release.
- Sizing: no task is trivial or likely to span several reviewable changes.
- Dependencies: every edge is necessary; independent work remains independent.
- Validation: every task has runnable evidence or exact human verification.
- Risk: high-risk unknowns have a spike, mitigation, or explicit checkpoint.
- Decisions: no answerable design call is deferred to an implementation agent.
- Cleanup: no unowned compatibility path or duplicate source of truth remains.

Fix the backlog, then give the user a concise summary of epics, key tasks,
dependencies, decisions, and risks. Get approval before implementation when the
user asked only for backlog shaping.

## Execute the backlog

Claim a specific task when its ID is known:

```sh
ergo claim ABCDEF --agent model@host
```

Use automatic claim only when choosing the oldest ready task is intentional:

```sh
ergo claim --agent model@host
```

Claim output contains the complete current task and exact lifecycle commands.
Then:

1. Read the task and relevant repository state.
2. Implement and run its validation gates.
3. Stop at any checkpoint or material design choice.
4. Commit the reviewable change using repository conventions.
5. Close it with the lifecycle command that states the outcome.

Lifecycle receipts report the task's resulting state, claim changes, appended
message or result, and ready work when applicable. Use that information before
making another query.

Messages append. Use them for decisions, completion, and attempt history. Use
`body` only when the task specification itself changed; it replaces the entire
body.

Use `--result` only for an existing project file produced by the task. Do not
use a commit hash, prose status, or a file created only to fill the field.

```sh
ergo done ABCDEF -m "Implemented and verified"
ergo done ABCDEF -m "Accepted specification" --result docs/spec.md
```

Choose other exits by intent:

```sh
ergo block ABCDEF -m "Waiting for the staging credential"
ergo release ABCDEF -m "Partial implementation is ready to continue" --result .scratch/attempt.md
ergo cancel ABCDEF -m "Superseded by the server-side change"
```

Never leave claimed work in doing. Block records an impediment. Release records
unfinished but retryable work. Cancel records a deliberate stop. After a spike,
update dependent task bodies with what was learned before closing it.

The backlog remains editable during execution. Use `title`, `body`, `move`,
`sequence`, and `unsequence` to keep it true. When every child is complete, its
epic is complete.
