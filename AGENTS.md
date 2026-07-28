This repository adds Ergo-specific instructions to the shared global policy in
`~/.config/AGENTS.md`.

# Project goals
- Ergo manages a dependency-aware backlog. User-facing language says backlog,
  task, dependency, and epic. `container` is only an internal implementation
  term.
- The agent manual has two authoritative layers:
  - `help.txt` is the root front door: mental model, first workflow, command
    inventory, global flags, and navigation.
  - `quickstart.txt` owns the complete cross-command model and operating guide.
- Generated command help provides syntax and options only; it is not a third
  manual.
- The two manual layers are complete together. Keep each succinct, accurate, humane,
  and nonredundant while preserving full tool-surface coverage.
- README, the shipped skill, specification, errors, and release guidance consume
  this contract; they do not become competing manuals.
