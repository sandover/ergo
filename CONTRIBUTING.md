# Contributing

Use the Go version declared in `go.mod` and the Node version required by
`editors/vscode/package.json`. The Windows checks run from PowerShell with:

```powershell
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File .\scripts\validate-windows.ps1
```

The script checks tracked changes for whitespace errors, runs the Windows Go
test suite, builds and starts a temporary `ergo.exe`, then runs the VS Code
extension tests. It installs locked npm dependencies when `node_modules` is
missing. The test commands remain defined by Go and the extension's
`package.json`.

This is a local Windows contributor check, not release qualification. Release
validation still includes the CI module and lint checks, race-enabled tests,
GoReleaser validation and archive builds, and Linux VSIX packaging. Follow
[`docs/release.md`](docs/release.md) before preparing a release.
