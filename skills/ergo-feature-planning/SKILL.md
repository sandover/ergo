---
name: ergo-feature-planning
description: >-
  Shape and execute multi-step software work with the Ergo CLI, a repository-local, dependency-aware backlog management tool. Use when work is likely to span 3 or more commits, crosses concerns such as API, UI, tests, migration, or docs, or needs design decisions and dependency ordering before implementation. Skip small, single-concern changes and routine housekeeping.
---

# Ergo Feature Planning

Turn any project goal into a dependency-ordered, self-contained backlog that one or more agents can execute without the original conversation. Keep the backlog smaller than the work; don't overcomplicate. Follow a principle of parsimony: add plan complexity only when it helps an agent decide, order, or verify work.

## Bootstrap

1. Expect `ergo` to be installed. If it is missing, ask the user to install it.
2. Run `ergo --help` and `ergo quickstart` to learn ergo.
3. Run `ergo where`. If no backlog exists, confirm the repository root and run `ergo init`.

## Resolve decisions first

If a missing answer could change the plan or be hard to reverse, stop and ask the user. It's their plan. Do not hide a question behind `TBD`, `Consult me`, or a vague future checkpoint.

A backlog can defer a decision when a task must first produce an implementation artifact, spike result, or other evidence.

Revise earlier tasks when later work reveals better boundaries. Do not preserve a weak split merely because it was written first. Make the tasks fit together and reach the goal.

## Shape the backlog

### Organize the work

Use an epic for tasks that deliver one feature or change. Put shared scope, non-goals, constraints, decisions, and assumptions in its body. The epic body coordinates the shared context; it does not replace instructions needed to execute a child.

Genuinely standalone tasks don't need an epic.

### Define tasks

- Make each task one atomic, reviewable change that normally fits one session. Split on real boundaries: public API, data model, migration, UI, tests, or docs.
- Avoid tiny bookkeeping tasks and broad tasks with several reviewable outcomes.
- Prefix knowledge-producing work with `spike:`. State what dependent tasks must learn from it.

### Plan validation

- Give each task the smallest check that can fail when its change is wrong. Do not run full CI when a smaller check covers the changed behavior. If no automated check applies, name an exact inspection or manual procedure.
- When several tasks affect the same behavior, assign one broader check to one task that runs after them. Do not copy the check into each contributing task.
- Use an existing integration, compatibility, or release task when possible. Create a final verification task only when no earlier task can check the combined behavior.

### Write task bodies

Write for a capable agent with less context and possibly less reasoning ability. Include known facts that help the implementer act or avoid an error, such as paths, behavior, edge cases, local validation, and any shared behavior verified later. But try not to overspecify.

Use this body shape. Omit empty sections except `Validation Gates`; every task must name its local check and any shared behavior verified later.

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
- Run: <Smallest task-local test, lint, build, or inspection>
- Deferred: <Behavior> is verified by <task ID or unambiguous title>.
```

A checkpoint asks the user to make a decision and stops work. For human verification, the executor performs the procedure and reports the result without stopping.

### Record the backlog

Build an epic incrementally while its tasks are still changing. Use bulk creation when the backlog is already prepared. Retain the returned task IDs for dependencies and review.

### Add dependencies

- Add a dependency only when one task must finish before another can proceed. Keep independent tasks parallel.
- Add a dependency from every task that can affect a broad gate to the task that runs it. This prevents the gate from running early.

## Review the backlog

Read the backlog as if you did not shape it.

- Does it contain all work needed to reach the goal, including necessary tests, docs, migration, compatibility, cleanup, and release work?
- Can another agent execute each task without the original conversation?
- Does the full dependency graph preserve parallel work and run shared checks after every task that can affect them?
- Is every open question, compatibility path, and duplicate source of truth resolved or assigned to a task?

Fix any problems. Then summarize the epics, key tasks, dependencies, decisions, and risks. Get approval before implementation when the user asked only for backlog shaping.

## Execute and adapt

- Claim a known task when possible. Use automatic claim only when choosing the oldest ready task is intentional.
- If implementation changes the task, its dependencies, or its validation, update the backlog.
- End every claim with the lifecycle command that matches the outcome. Never leave claimed work in doing.
- State the outcome and checks run in the completion message. Attach a result only when the task produced an actual project file.
- After a spike, update dependent task bodies before closing it.
