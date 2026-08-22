import assert from "node:assert/strict";
import test from "node:test";
import {
  commandErrorMessage,
  isSupportedVersion,
  listArguments,
  parseErgoVersion,
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
