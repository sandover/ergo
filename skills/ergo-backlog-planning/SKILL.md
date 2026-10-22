---
name: ergo-backlog-planning
description: >-
  Shape a workstream into tasks, organize tasks into epics, express dependencies among tasks. Use for planning medium and large workstreams; skip for small tasks and routine housekeeping.
---

# Ergo Backlog Planning

Use Ergo to write tasks (work tickets) to be implemented by you or another agent. Each task should include a clear unambiguous goal, essential context and rationale, links to any necessary resources or background, a crisp definition of done (often a list), and, where applicable, suggestions for how to validate the work.

If you do not know the ergo CLI already, read `ergo --help` and `ergo quickstart`. Use `ergo where` to confirm the backlog's location before writing tasks.

- Use reasonable assumptions for routine details. Ask the user when available context cannot resolve a choice that materially changes scope, risk, or what counts as done.
- Split work where an agent can deliver a useful outcome; keep small related steps together.
- Use epics to help organize related tasks.
- Tasks should make sense on their own, but the epic body may provide larger context.
- Assume the implementer is as intelligent as you are, but they lack your conversational context. You don't need to be overly verbose or prescriptive; let the implementer implement.
- Sometimes only a person can judge whether a task is done. If so, say what the human will review and what decision is needed.
- Add a dependency when one task needs another task's result. Leave independent tasks free to run in parallel.
- Keep validation as cheap as possible, checking only what is necessary to establish that the task is done.
- Reuse check results that still apply to the work. Add a combined check only when individual task checks cannot establish the shared outcome.
