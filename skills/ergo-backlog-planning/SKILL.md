---
name: ergo-backlog-planning
description: >-
  Shape and execute multi-step work with the Ergo CLI, a repository-local backlog management tool. Use to plan software development, documentation, or general knowledge work as a series or graph of well-defined tasks. Use to define dependencies within that task backlog. Use to group tasks into epics when needed. Skip small tasks and routine housekeeping.
---

# Ergo Backlog Planning

Ergo is a CLI tool for writing and managing a task graph at the repository level. Tasks have state and dependency relationships.

The basic usage model is that you take the feature or goal assigned to you by the user and methodically break it down into tasks, file those tasks in ergo. One or more agents or subagents (perhaps working in parallel) will later claim and implement the tasks.

The tasks should establish *guardrails* that help an implementing agent avoid undesirable outcomes and *speedrails* that help that agent move quickly and confidently.

Follow a principle of parsimony. Add plan complexity only when it helps an agent decide, order, or verify work.

## Bootstrap

1. Expect `ergo` to be installed. If it is missing, ask the user to install it.
2. Run `ergo --help` and `ergo quickstart` to learn the tool.
3. Run `ergo where`. If no backlog exists, confirm the repository root and run `ergo init`.

## Backlog Planning

### Tasks and epics

- Size tasks in a common-sense way. Do not make them trivially small.
- Split tasks on real boundaries such as public API, data model, migration, UI, tests, or docs.
- Mark knowledge-producing tasks with `spike:`.

Standalone tasks do not need an epic. Use an epic for a set of related tasks.

An epic body is optional. Use it for shared scope, non-goals, constraints, decisions, and assumptions. It does not replace instructions needed to execute an individual task.

### Resolve decisions first

If a planning decision is uncertain or hard to reverse, stop and ask the user early. Resolve material choices during planning so the backlog contains few ambiguities or deferred decisions.

Defer a decision only when implementation evidence is required, such as an artifact or spike result that the user needs to judge the next move.

During planning, you can continue to revise earlier tasks as later tasks reveal new insights. You are responsible for the coherence of the part of the backlog you are writing; it must all fit together.

### Writing tasks

Include known facts that help the implementer act or avoid an error, such as paths, behavior, edge cases, local validation, and shared behavior verified later. Do not overspecify the task. Let the implementing agent decide how to do the work within its guardrails.

- Propose the smallest unambiguous validation gate that can establish the task outcome. Do not propose needless CI or QA. Keep the effort proportionate to the risk.
- If no automated validation is possible, propose an exact manual procedure.
- When several tasks affect the same behavior, assign one broader check to one task that runs after them. Do not copy the check into each contributing task. No wasted effort.
- Use an existing integration, compatibility, or release task when possible. Create a final verification task only when no earlier task can check the combined behavior.

Use this body shape. Omit empty sections except `Validation Gates`.

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
A checkpoint stops agent work and asks the user for input.

### Task dependencies

- Keep independent tasks parallel when possible. Do not add needless dependency links.
- If there's a broad validation gate, make the task that runs it depend on every task that can affect it. This ensures that the check runs only after those tasks are complete.

### Review the backlog

Once the backlog is in place, stop, clear your mind, and re-read the backlog as if you did not write it.

- Does it contain all work needed to reach the goal, including necessary tests, docs, migration, compatibility, cleanup, and release work?
- Can an agent execute each task without the original conversation?
- Does the dependency graph make sense?
- Is every open question, compatibility path, and duplicate source of truth resolved or assigned to a task?

Fix any problems. Then summarize the epics, key tasks, dependencies, decisions, and risks. Get approval before implementation when the user asked only for planning or when material decisions remain.

### Execute and adapt

- Each agent claims one task at a time. Independent agents may work on ready tasks in parallel when the user wants parallel execution.
- Always keep task state updated and accurate.
- End every claim with the lifecycle command that matches the outcome. Never leave claimed work in `doing`.
- State the outcome and checks run in the completion message. Attach a result only when the task produced an actual project file.
- After a spike, update dependent task bodies before closing it.
