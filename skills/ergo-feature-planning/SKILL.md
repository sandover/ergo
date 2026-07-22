---
name: ergo-feature-planning
description: >-
  Plan and execute multi-step software work with Ergo, a repo-local task dependency graph. Use when work is likely to span 3 or more commits, crosses concerns such as API, UI, tests, migration, or docs, or needs design decisions and dependency ordering before implementation. Skip for small, single-concern changes and routine housekeeping.
---

# Ergo Feature Planning

Turn an accepted goal into a dependency-ordered backlog that another agent can
execute without the original conversation. Keep the plan smaller than the work.

## Bootstrap

1. Expect `ergo` to be installed. If it is missing, ask the user to install it.
2. Run `ergo --help` and `ergo quickstart` before creating or executing a plan.
3. Run `ergo where`. If no graph exists, confirm the repository root and run `ergo init`.

## Resolve decisions before planning

Planning exposes ambiguity. Present concrete options and tradeoffs, ask the user,
and record the decision in the relevant container or task body. Do not hide a
question behind `TBD`, `Consult me`, or a vague future checkpoint.

Use a checkpoint only when an implementation artifact is required to decide.
Name the artifact, the exact question, and the instruction not to continue
without approval. If the user cannot decide yet, create a spike whose output is
the missing knowledge.

Revise earlier tasks as later planning reveals better boundaries. Do not preserve
a weak split merely because it was written first.

## Build the graph

Use one root container for each coherent feature area. Put scope, non-goals,
constraints, decisions, and assumptions in its body. Leave genuinely standalone
tasks at root.

Create a container candidate, then add children with its ID:

```sh
ergo new task '{"title":"Authentication"}'
printf '%s\n' '## Goal' '- Add session validation.' |
  ergo new task '{"title":"Validate sessions","epic":"OFKSTE"}'
```

The first child promotes a clean root todo task to a container. For a prepared
markdown backlog, use `ergo plan --file tasks.md '{"title":"Authentication"}'`.

Add only real ordering constraints. `ergo sequence TASK_A TASK_B` means B waits
for A. Prefer independent tasks and maximize safe parallelism.

## Shape tasks

Make each task one atomic, reviewable change that normally fits one session.
Split on real boundaries: public API, data model, migration, UI, tests, or docs.
Avoid tiny bookkeeping tasks and broad tasks with several reviewable outcomes.

Write for a capable agent with less context and possibly less reasoning ability.
Include the paths, behaviors, edge cases, and runnable proof needed to succeed.

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

## Review the plan

Before presenting the backlog, check:

- Coverage: implementation, tests, docs, migration, compatibility, and release work.
- Sizing: no task is trivial or likely to span several reviewable changes.
- Dependencies: every edge is necessary; parallel work remains parallel.
- Validation: every task has runnable evidence or exact human verification.
- Risk: high-risk unknowns have a spike, mitigation, or explicit checkpoint.
- Decisions: no answerable design call is deferred to an implementation agent.
- Cleanup: the plan leaves no unowned compatibility path or duplicate source of truth.

Fix the graph, then give the user a concise summary of containers, key tasks,
dependencies, decisions, and risks. Get approval before implementation when the
user asked for a plan rather than execution.

## Execute the plan

Claim the oldest ready task or a specific task:

```sh
ergo claim --agent model@host
ergo claim ABCDEF --agent model@host
```

Claim output begins with `---` and then `id: "ABCDEF"`. Read the task ID from
that fixed position. The final `## Next` section gives its exact lifecycle
commands.

Then:

1. Read the full task and relevant repository state.
2. Implement and run its validation gates.
3. Stop for any required checkpoint or material design choice.
4. Commit the reviewable change using repository conventions.
5. Close it with the correct lifecycle command and a brief `-m` completion note.

Lifecycle messages append. Use them for decisions, approach, completion, and
attempt history. Use `body` only when the task specification itself changed;
it replaces the entire body.

Use `--result` only for a concrete existing file produced by the task. Do not use
a commit hash, prose status, or a file created only to satisfy the field.

```sh
ergo done ABCDEF -m "Implemented and verified"
ergo done ABCDEF -m "Accepted specification" --result docs/spec.md
```

Choose other exits by intent:

```sh
# An identified impediment must be resolved.
ergo block ABCDEF -m "Waiting for the staging credential"

# The objective remains valid and another attempt can proceed.
ergo release ABCDEF -m "Partial implementation is ready to continue" --result .scratch/attempt.md

# The objective is no longer wanted.
ergo cancel ABCDEF -m "Superseded by the server-side change"
```

Never leave claimed work in doing. Block records an impediment. Release records
unfinished but retryable work. Cancel records a deliberate stop. After a spike,
update dependent task bodies with what was learned before closing the spike.

Plans remain editable during execution. Use `title`, `body`, `move`, `sequence`,
and `unsequence` to keep the graph true, and note why its shape changed. When
the container is complete, the plan is complete.
