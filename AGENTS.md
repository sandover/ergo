# When coding
- prefer a lightly functional style and prefer pure functions when it makes sense
- Avoid hidden state and hidden variables -- make inputs and outputs explicit
- Favor independent, testable components with loose coupling

# When writing commit messages
- Use Conventional Commits (type(scope): imperative summary; scope optional)
- Body (a handful of lines) explains what/why/how plus constraints or invariants (and notable risks/tests if relevant)
- When applicable, add trailers (one per line) for traceability: Fixes: #XYZ, Refs: PROJ-9, BREAKING CHANGE: ...

# Other Guidance
- don't use /tmp, it prompts me for permissions.  Prefer tmp/ or .scratch/ in the repo
- When the code changes, the docs (such as README, code comments, --help, and "ergo quickstart") must also change to reflect. There must be total internal coherence in the project at all times.

# Project goals
- --help text is incredibly important because it's the front door for agents (our primary users). It seeks to be:
    - as succinct and tight as we can get away with
    - 100% coverage of options
    - beautifully formatted, a great example of information design
    - nutrient-dense for agents -- if it's the only thing they read, they can operate. if they go and read the quickstart, so much the better, they have a deeper understanding.