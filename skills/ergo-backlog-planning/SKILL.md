---
name: ergo-backlog-planning
description: >-
  Shape multi-step work into Ergo tasks with a clear definition of done and a practical way to check it. Use when planning software, documentation, or other project work in an Ergo backlog. Skip small tasks and routine housekeeping.
---

# Ergo Backlog Planning

Use Ergo to write tasks that another agent can pick up later. Each task should
say what outcome to produce, what constraints matter, and how the agent will
know it has finished.

If you do not know the CLI, read `ergo --help` and `ergo quickstart`. Use
`ergo where` to find the backlog. If none exists, confirm the project root
before running `ergo init`.

## Shape the tasks

- Split work where an agent can produce and check a useful outcome. Keep small
  related steps together. Use an epic only when it helps organize related tasks.
- Put the facts and constraints an implementer needs in the task body. A child
  task must make sense when claimed on its own; do not rely on the epic body for
  instructions needed to execute it.
- State what counts as done in observable terms. Include an important edge case
  only when it changes that decision. Leave routine implementation choices to
  the agent.
- Give each task a way to decide whether it is done. Prefer a focused check the
  agent can perform, such as a relevant test or inspection of the result. Name
  the expected outcome. Do not add checks that cannot change the decision.
- Choose human validation deliberately when the agent cannot judge the result.
  Say what the human will review and what decision is needed. Do not substitute
  an unrelated automated check or prescribe detailed review steps without a
  reason.

A short task body often needs only this shape. Add context when it helps:

```md
## Outcome
<What this task must deliver>

## Done when
<Observable result or acceptance criteria>

## Check
<Focused agent check and expected result, or the human review needed>
```

The check may be an inspection, a command, or a human decision. It need not be a
test suite. If human approval is part of the definition of done, keep that
decision visible rather than marking the task complete on implementation alone.

## Connect the work

- Add a dependency only when one task needs another task's result. Leave
  independent tasks free to run in parallel.
- Assign a combined check to one task only when task-level checks cannot
  establish the shared outcome. Do not repeat broad validation across tasks.
  Make a task that checks combined behavior depend on every task whose output
  it checks.
- Resolve choices that affect scope or acceptance before dependent work starts.
  Use reasonable assumptions for routine details; ask the user about choices
  that materially affect scope, risk, or acceptance.

When graph construction takes several commands, create tasks with `--draft`.
Add children and dependencies, then `ergo open <id>` for each leaf when the
plan is ready to claim.

Review the backlog as an implementing agent would: can each task be started,
finished, and checked without guessing? Fix unclear tasks or unnecessary
dependencies. Check that the tasks together cover the requested outcome.
Summarize the plan and any human decisions it still needs.
