# Ergo release guide

This is the maintainer checklist for a tag-driven release. User behavior belongs
in `ergo --help`, `ergo quickstart`, and `docs/spec.md`.

## Prepare

- Start from a clean, current main branch.
- Update `CHANGELOG.md` with user-visible behavior and upgrade notes.
- Confirm help, quickstart, spec, architecture, README, and shipped skill agree.
- Run `task ci`.
- Run `task build` and smoke `./bin/ergo --help`, quickstart, JSON output, and one lifecycle loop.
- Run `goreleaser check`.
- Run `goreleaser release --snapshot --clean` and inspect every configured target.

For a version candidate, inject the same linker variable as GoReleaser:

```sh
go build -ldflags "-s -w -X main.version=2.0.0" -o .scratch/release/ergo-v2-candidate ./cmd/ergo
.scratch/release/ergo-v2-candidate version
```

The command must print `ergo 2.0.0`. The release tag includes the `v` prefix:
`v2.0.0`.

## Versioning

- Patch releases fix defects without changing contracts.
- Minor releases add compatible commands, flags, or JSON fields.
- Major releases remove or change public commands, behavior, or JSON semantics.

Breaking releases must map old workflows to new commands and state what is no
longer exposed. Legacy storage compatibility must be tested against copied
event logs rather than assumed from unit tests alone.

## Publish

1. Record the exact release commit and passing CI run.
2. Get explicit approval to publish the tag.
3. Create and push the immutable version tag.
4. Watch the tag-specific release workflow to completion.
5. Verify the GitHub Release is final and contains checksums plus every configured archive.

Never move or replace a published version tag. Correct a failed release with a
new version.

## Verify delivery

- Download the archives and verify them against `checksums.txt`.
- Run the released binary's version, help, and quickstart commands.
- Verify one JSON lifecycle from claim through an exit.
- Verify one copied legacy log containing error or claimed-blocked state.
- Install through Homebrew and invoke `$(brew --prefix)/bin/ergo` explicitly.
- Verify WinGet too when its publisher is configured.

The release is complete only when source, artifacts, and package-manager installs
all report the intended version and accepted CLI contract.
