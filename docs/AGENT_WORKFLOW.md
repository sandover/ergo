# Agent Workflow

The simplest pattern is:

1. A human creates tasks/epics and links dependencies.
2. Agents run `ergo ls --ready` to find unblocked work.
3. Each agent runs `ergo take` to claim a task safely.
4. Agents mark tasks `done` (or `blocked`) as they go.

## Suggested AGENTS.md snippet

Add something like:

```text
Use `ergo` for task tracking in this repo.

- Start by running `ergo ls --ready`.
- Claim work with `ergo take` (never pick a task without claiming).
- Mark tasks `done` when finished; mark `blocked` with a clear reason in the body.
```

