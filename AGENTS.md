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