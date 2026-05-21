---
name: ergo-feature-planning
description: >-
  Plan software tasks with `ergo` (local task dependency graph + containers) when the work spans 3+ commits or involves multiple concerns. Run "ergo --help" to learn it.
---

# Ergo Feature Planning

Turn a feature request into a repo-local plan tracked by `ergo`: a backlog of well-scoped tasks, with dependency edges, grouped into containers.

## When to use

**Use ergo when** the work would span 3+ commits, touches multiple concerns (API + UI + tests + docs), or has ambiguity that needs resolving before implementation.

**Skip ergo when** the change is straightforward enough to just implement -- a bug fix, a single-concern feature, routine refactoring. Don't plan what you can just do.

**Unsure?** Ask: "Want an ergo plan first, or should I just implement?"

## Bootstrap

1. Expect `ergo` to be globally installed. If missing, ask the user to install it.
2. Run `ergo --help` and `ergo quickstart` to learn the CLI before creating plans.
3. Use `ergo` sequentially and synchronously in the main agent. Do not split `ergo` work across subagents or run concurrent `ergo` mutations; shared plan state is easier to reason about when one agent owns it at a time.

## Planning

Planning naturally surfaces unknowns, ambiguities, and decisions. **Resolve them now, during planning, by asking the user.** Present options clearly with tradeoffs, get an answer, and write the decision into the container body or task AC. Do not write "Consult Me" or "TBD" into task bodies as a way to defer a conversation you could have right now -- that just creates a mid-implementation block for a future agent that has less context than you do.

The test: if you can describe the options and tradeoffs to the user today, ask today. The only decisions that belong in task bodies as checkpoints are ones that literally require an implementation artifact to evaluate, for example "produce a UI mockup, then get approval before proceeding." For those, write a **Checkpoint** section with the specific artifact to produce and the specific question to answer -- not a vague "consult me."

If a task's shape depends on an unresolved decision and the user can't or won't decide yet, make it a spike instead.

Critique continuously as you build the plan. When writing one task reveals that earlier tasks should be merged or split, fix it immediately rather than deferring to a review pass.

### Containers

One per coherent feature area. Body includes scope, non-goals, constraints, and if it makes sense, key decisions and assumptions. Tasks are grouped into containers, but dependencies can cross container boundaries when needed.

Tasks that don't fit a larger feature area can be left ungrouped. Use judgment.

### Tasks

The unit of execution. Each task should be:

- **One atomic, reviewable change** -- completable in a single session. Not trivial, not sprawling.
- **Ideally, automatically verifiable** via acceptance criteria and runnable gates. When human verification is truly needed, include exact instructions so implementing agents know precisely how to verify.
- **Split on real boundaries** only: API surface, data model, migration, tests, docs.
- **Friendly to smaller models** -- the implementing model might not be as smart as you. Before finalizing a task, consider it from the perspective of a less capable model without the context of the full user conversation or the whole backlog. Will they succeed?

**Spikes** produce knowledge, not code. Prefix with `spike:`. Dependent tasks should note what they're waiting to learn from the spike.

Task body template. Trim to fit; omit empty sections.

```md
## Goal
- <1-3 bullets: concrete outcome and why it matters>

## Context
- <Background, links to docs/designs; only when not obvious from the goal>

## Acceptance Criteria
- <Observable behavior, edge cases, definition of done>

## Checkpoint
- Produce: <specific artifact, e.g. "ASCII mockup of the banner layout">
- Then ask: <specific question, e.g. "Does this information hierarchy work?">
- Do not proceed past this point without user approval.

## Validation Gates
- <Exact commands to prove it works: tests, lint, format>
```

### Dependencies

Add edges only for true ordering constraints. Maximize parallelism and task independence.

## Plan review

Before presenting to the user, do a final confirmation pass:

- **Coverage** -- API, tests, docs, migrations, edge cases all accounted for?
- **Sizing** -- anything too small to track separately, or too large to finish in one session?
- **Dependencies** -- missing edges that will cause churn? Unnecessary edges blocking parallelism?
- **Validation** -- every task has runnable gates?
- **Risk** -- 1-3 highest-risk tasks identified, with spikes or mitigation added?
- **Open calls** -- every judgment call resolved with the user, not deferred to task bodies?
- **Cruft** -- will the planned work leave behind unowned debt?

Fix what you find, then present an executive summary to the user for approval. Keep it concise: containers, key tasks, and major decisions in high-level language.

## Executing ergo plans

1. Claim a ready task.
2. Implement it. Stop and consult the user if important questions arise.
3. Commit using repo conventions. Do not include `.ergo/` files in per-task commits.
4. Mark task done. On completion:
   - Update the task body with a brief completion note: decisions made, approach taken, anything non-obvious. Think PR description, not essay.
  - If the task produced a concrete deliverable, attach it with `result` pointing at the file path. Do not create standalone result files just to have a link.
   - After completing a spike, update dependent tasks with what was learned so the knowledge flows forward.
   - If a task can't be completed, mark it blocked or error. Never leave tasks in progress.
5. If the plan needs to change, update the plan and note why. Plans are living documents, not contracts.
6. When a container is done, commit the `.ergo/` state with a message like `plan: complete <feature name>`. Otherwise, go to 1.
