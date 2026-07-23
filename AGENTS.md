This repository inherits global agent policy from `/Users/brandonharvey/AGENTS.md`.
Keep this file limited to ergo-specific deltas.

# Project goals
- Ergo manages a dependency-aware backlog. User-facing language says backlog,
  task, dependency, and epic. `container` is only an internal implementation
  term.
- The agent manual has three authoritative layers:
  - `help.txt` is the root front door: mental model, first workflow, command
    inventory, global flags, and navigation.
  - Cobra command metadata owns exact command usage, operands, flags, stdin
    behavior, preconditions, and examples.
  - `quickstart.txt` owns the complete cross-command model and operating guide.
- These layers are complete together. Keep each succinct, accurate, humane,
  and nonredundant while preserving full tool-surface coverage.
- README, the shipped skill, specification, errors, and release guidance consume
  this contract; they do not become competing manuals.
