---
name: ergo-backlog-planning
description: >-
  Shape and execute multi-step work with the Ergo CLI, a repository-local backlog management tool. Use to plan software development, documentation, or general knowledge work as a series or graph of well-defined tasks. Use to define dependencies within that task backlog. Use to group tasks into epics when needed. Skip small tasks and routine housekeeping.
---

# Ergo Backlog Planning

Ergo is a CLI tool for writing and managing a task graph at the repository level. Tasks have state and dependency relationships.

The basic usage model is that you take the feature or goal assigned to you by the user and methodically break it down into tasks, file those tasks in ergo. One or more agents or subagents (perhaps working in parallel) will later claim and implement the tasks.

When graph construction takes more than one command, create the new task or
epic with `--draft`. Draft work stays visible for review but never appears in
`ergo list --ready` and cannot be claimed. Add children and dependency edges,
then run `ergo open <id>` once per leaf after its placement and ordering are
safe. Use `open` instead of the removed `release` command; blocked work must be
opened before claim, while finished work retries through a specific claim.

The tasks should establish *guardrails* that help an implementing agent avoid undesirable outcomes and *speedrails* that help that agent move quickly and confidently.

Follow a principle of parsimony. Add plan complexity only when it helps an agent decide, order, or verify work.

## Bootstrap

1. Expect `ergo` to be installed. If it is missing, ask the user to install it.
2. Run `ergo --help` and `ergo quickstart` to learn the tool.
3. Run `ergo where`. If no backlog exists, confirm the repository root and run `ergo init`.

## Backlog Planning

### Establish scope and decisions

Establish the intended outcome, constraints, and completion criteria. Investigate
facts directly. Ask about unresolved decisions that materially affect scope, risk,
or difficult-to-reverse choices. Use reasonable assumptions for routine
implementation details and proceed with authorized work.

### Tasks and epics

- Size tasks in a common-sense way. Do not make them trivially small.
- Split tasks on real boundaries such as public API, data model, migration, UI, tests, or docs.
- Mark knowledge-producing tasks with `spike:`.

Standalone tasks do not need an epic. Use an epic for a set of related tasks.

An epic body is optional. Use it for shared scope, non-goals, constraints, decisions, and assumptions. It does not replace instructions needed to execute an individual task.

### Resolve decisions first

Resolve material choices early using the decision criteria above. Continue independent work while a necessary answer is pending.

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
Include a checkpoint only when a specific user decision or approval is needed.
It pauses dependent work until the user answers.

### Task dependencies

- Keep independent tasks parallel when possible. Do not add needless dependency links.
- If there's a broad validation gate, make the task that runs it depend on every task that can affect it. This ensures that the check runs only after those tasks are complete.

### Review the backlog

Review the backlog for complete scope, executable tasks, sound dependencies, and
unresolved decisions. Simplify structure that does not help implementation or
verification, and fix any gaps. Summarize the tasks, dependencies, decisions, and
material risks.

If the user requested planning only, return the plan. If implementation is
already authorized, continue through its completion criteria; pause dependent
work only for a necessary user decision or approval.

### Execute and adapt

- Each agent claims one task at a time. Independent agents may work on ready tasks in parallel when the user wants parallel execution.
- Always keep task state updated and accurate.
- End every claim with the lifecycle command that matches the outcome. Never leave claimed work in `doing`.
- Use `ergo done` when the objective succeeded, `ergo fail` when the attempt finished unsuccessfully, and `ergo block` only when an impediment prevents the attempt from finishing.
- State the outcome and checks run in the lifecycle message. Use `ergo result <id> "<text>" [--file <path>]` for durable result evidence; attach a file only when the task produced an actual project file.
- After a spike, update dependent task bodies before closing it.
- When adding newly discovered context, pipe only the new text to
  `ergo body <id> --append`, including any desired leading newline. Replace the
  whole body only when intentionally rewriting it.
