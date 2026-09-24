import assert from "node:assert/strict";
import test from "node:test";
import {
  commandErrorMessage,
  installationGuidance,
  isExecutableAccessError,
  isSupportedVersion,
  listArguments,
  parseErgoVersion,
  windowsInstallCommand,
} from "../src/ergo";

test("passes workspace paths as one argument without shell interpolation", () => {
  const folder = "/tmp/Project with spaces; echo unsafe";
  assert.deepEqual(listArguments(folder), [
    "--dir",
    folder,
    "list",
    "--json",
  ]);
});

test("maps missing executables and CLI failures to concise messages", () => {
  const missing = Object.assign(new Error("spawn ergo ENOENT"), { code: "ENOENT" });
  assert.equal(
    commandErrorMessage(missing, ""),
    "Ergo was not found.",
  );
  assert.equal(
    commandErrorMessage(new Error("exit 1"), "error: no .ergo directory found\n"),
    "error: no .ergo directory found",
  );
});

test("parses and compares Ergo semantic versions", () => {
  assert.equal(parseErgoVersion("ergo version 6.0.0\n"), "6.0.0");
  assert.equal(
    parseErgoVersion("ergo version v4.1.0-3-g9f95e3b-dirty\n"),
    "4.1.0-3-g9f95e3b-dirty",
  );
  assert.equal(parseErgoVersion("ergo version dev\n"), undefined);
  assert.equal(isSupportedVersion("4.1.9"), false);
  assert.equal(isSupportedVersion("5.0.9"), false);
  assert.equal(isSupportedVersion("6.0.0"), true);
  assert.equal(isSupportedVersion("6.0.0-rc.1"), true);
  assert.equal(isSupportedVersion("6.10.0"), true);
});

test("distinguishes a non-executable configured path", () => {
  const denied = Object.assign(new Error("spawn EACCES"), { code: "EACCES" });
  assert.equal(
    commandErrorMessage(denied, ""),
    "The configured Ergo executable is not executable.",
  );
});

test("only missing or inaccessible executables trigger CLI recovery", () => {
  const withCode = (code: string): NodeJS.ErrnoException =>
    Object.assign(new Error(code), { code });
  assert.equal(isExecutableAccessError(withCode("ENOENT")), true);
  assert.equal(isExecutableAccessError(withCode("EACCES")), true);
  assert.equal(isExecutableAccessError(withCode("EPERM")), false);
  assert.equal(isExecutableAccessError(withCode("EIO")), false);
});

test("gives installation guidance for each supported desktop platform", () => {
  const windows = installationGuidance("ergo", "win32");
  assert.ok(windows.includes(windowsInstallCommand));
  assert.ok(windows.includes("Windows ZIP"));
  assert.ok(windows.includes("ergo.exe"));

  const macOS = installationGuidance("ergo", "darwin");
  assert.ok(macOS.includes("brew install sandover/tap/ergo"));
  assert.ok(!macOS.includes("winget"));

  const linux = installationGuidance("ergo", "linux");
  assert.ok(linux.includes("appropriate release archive"));
  assert.ok(linux.includes("put ergo on PATH"));
  assert.ok(!linux.includes("winget"));
});
