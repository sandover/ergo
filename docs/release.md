# Ergo release guide

This is the maintainer checklist for a tag-driven release. User behavior belongs
in `ergo --help`, `ergo quickstart`, and `docs/spec.md`.

## Prepare

- Start from a clean, current main branch.
- Update `CHANGELOG.md` with user-visible behavior and upgrade notes.
- Manually review `README.md` before tagging or publishing. Confirm its concise
  overview, examples, and links still describe the current CLI and point readers
  to the authoritative quickstart. Keep this as a release check, not an
  automated README contract test.
- Draft deliberate release notes before tagging. Lead with what changed and why,
  then include useful examples, behavioral boundaries, compatibility, and the
  upgrade path. A generated commit list is never acceptable as final notes.
- Confirm help, quickstart, spec, architecture, and shipped skill agree.
- For the 6.0.0 cutover, verify the notes explain draft staging, the `◌`
  presentation, `open` replacing `release`, blocked-work migration, retry by
  specific claim, and incompatibility with older binaries after the first
  draft record.
- Run `task ci`.
- Run `task build` and smoke `./bin/ergo --help`, quickstart, readable output, and one lifecycle loop.
- Run `goreleaser check`.
- During preparation, run `goreleaser build --snapshot --clean --single-target` to check the current host build after relevant changes.
- Run `goreleaser release --snapshot --clean` on the final release candidate and inspect all six configured archives.

Run the full snapshot earlier when `.goreleaser.yaml`, the Go version, dependencies,
or platform-specific code changes. The final full snapshot is required before
every publication.

For a version candidate, inject the same linker variable as GoReleaser:

```sh
release_version=4.0.0
go build -ldflags "-s -w -X main.version=$release_version" -o .scratch/release/ergo-candidate ./cmd/ergo
.scratch/release/ergo-candidate version
```

The command must print the selected version. The release tag uses the same
version with a `v` prefix.

## Versioning

- Patch releases fix defects without changing contracts.
- Minor releases add compatible commands, flags, or output fields.
- Major releases remove or change public commands, behavior, or output semantics.

Breaking releases must map old workflows to new commands and state what is no
longer exposed. Legacy storage compatibility must be tested against copied
event logs rather than assumed from unit tests alone.

## Publish

1. Record the exact release commit and passing CI run.
2. Get explicit approval to publish the tag.
3. Create and push the immutable version tag.
4. Watch the tag-specific release workflow to completion.
5. Replace generated notes with the drafted release notes and inspect the
   rendered release page.
6. Verify the GitHub Release is final and contains checksums plus every configured archive.

Never move or replace a published version tag. Correct a failed release with a
new version.

## Verify delivery

- Download the archives and verify them against `checksums.txt`.
- Run the released binary's version, help, and quickstart commands.
- Verify one staged lifecycle: create draft work, configure it, open the leaf,
  claim it, and finish the attempt.
- Verify one copied legacy log containing error or claimed-blocked state.
- Install through Homebrew and invoke `$(brew --prefix)/bin/ergo` explicitly.
- Verify WinGet too when its publisher is configured.

The release is complete only when its page has useful release notes and source,
artifacts, and package-manager installs all report the intended version and
accepted CLI contract.
