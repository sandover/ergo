# When planning
Use ergo for all planning, so that your plans can be stored in the repo. Run "ergo --help" and "ergo quickstart" now to learn this tool.

# When coding
- prefer a lightly functional style and prefer pure functions when it makes sense
- Avoid hidden state and hidden variables -- make inputs and outputs explicit
- Favor independent, testable components with loose coupling
- Each code file should have top matter comments sufficient to allow agents to confidently understand and navigate the code file *without having to read the whole thing*. 

# When writing commit messages
- Use Conventional Commits (type(scope): imperative summary; scope optional)
- Body (a handful of lines) explains what/why/how plus constraints or invariants (and notable risks/tests if relevant)
- When applicable, add trailers (one per line) for traceability: Fixes: #XYZ, Refs: PROJ-9, BREAKING CHANGE: ...

# Invariants
- **CI Stays Green**: Never tag a release without first verifying that the local code passes all linting & formatting checks and tests.
- **Docs are Accurate**: When the code changes, docs (such as README, code comments, built-in help text, etc) must reflect it. Goal is perfect internal coherence in the project at all times.
- **CI Parity**: Run `task ci` before pushing to match CI tool versions and steps.

# Other Guidance
- For temporary work and experiments, use tmp/ or .scratch/, not /tmp

# Project goals
- --help text is incredibly important because it's the front door for agents (our primary users). It seeks to be:
    - as succinct and tight as we can get away with
    - 100% coverage of options
    - beautifully formatted, a great example of information design
    - nutrient-dense for agents -- if it's the only thing they read, they can operate. if they go and read the quickstart, so much the better, they have a deeper understanding.

If you have read these instructions and are keeping them in mind, end each of your messages with this glpyh on its own line: ✠
