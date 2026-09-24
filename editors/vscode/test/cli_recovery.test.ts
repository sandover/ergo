import assert from "node:assert/strict";
import test from "node:test";
import {
  cliRecoveryChoices,
  executableSettingScope,
  runWithCliRecovery,
} from "../src/cli_recovery";
import type { CliRecoveryChoice, CliRecoveryEffects } from "../src/cli_recovery";
import { ErgoCommandError, windowsInstallCommand } from "../src/ergo";

const unavailable = new ErgoCommandError("Ergo was not found.", true);

test("offers Windows install, selection, retry, and release actions", () => {
  assert.deepEqual(
    cliRecoveryChoices("win32"),
    [
      { action: "copy-install-command", title: "Copy WinGet command" },
      { action: "select-executable", title: "Select Ergo executable…" },
      { action: "retry-detection", title: "Retry detection" },
      { action: "open-releases", title: "Open Windows release ZIPs" },
    ],
  );
  assert.ok(
    !cliRecoveryChoices("darwin").some((choice) => choice.action === "copy-install-command"),
  );
});

test("copies the exact WinGet command and keeps recovery available", async () => {
  const events: string[] = [];
  let offered: readonly CliRecoveryChoice[] = [];
  let choiceCount = 0;
  const effects = recoveryEffects({
    choose: async (_message, availableChoices) => {
      offered = availableChoices;
      return choiceCount++ === 0 ? "copy-install-command" : "retry-detection";
    },
    copyText: async (text) => {
      events.push(`copy:${text}`);
    },
    notifyCopied: async () => {
      events.push("notify");
    },
  });
  const attempted: string[] = [];
  let attempts = 0;

  const result = await runWithCliRecovery(
    "win32",
    "ergo",
    async (executable) => {
      attempted.push(executable);
      attempts++;
      if (attempts < 3) {
        throw unavailable;
      }
      return "ready";
    },
    effects,
  );

  assert.deepEqual(attempted, ["ergo", "ergo", "ergo"]);
  assert.deepEqual(events, [`copy:${windowsInstallCommand}`, "notify"]);
  assert.ok(offered.some((choice) => choice.action === "retry-detection"));
  assert.deepEqual(result, { status: "completed", value: "ready", executable: "ergo" });
});

test("selecting an executable saves it and retries compatibility detection", async () => {
  const events: string[] = [];
  let attempts = 0;
  const selected = "C:\\Tools\\ergo.exe";

  const result = await runWithCliRecovery(
    "win32",
    "ergo",
    async (executable) => {
      events.push(`run:${executable}`);
      attempts++;
      if (attempts === 1) {
        throw unavailable;
      }
      return "ergo version 6.1.0";
    },
    recoveryEffects({
      choose: async () => "select-executable",
      selectExecutable: async () => selected,
      saveExecutable: async (executable) => {
        events.push(`save:${executable}`);
      },
      clearCompatibilityCache: () => events.push("clear-cache"),
    }),
  );

  assert.deepEqual(events, [
    "run:ergo",
    "clear-cache",
    `run:${selected}`,
    `save:${selected}`,
  ]);
  assert.deepEqual(result, {
    status: "completed",
    value: "ergo version 6.1.0",
    executable: selected,
  });
});

test("does not save a selected executable until it works", async () => {
  let choices = 0;
  let saved = false;
  const result = await runWithCliRecovery(
    "win32",
    "ergo",
    async () => {
      throw unavailable;
    },
    recoveryEffects({
      choose: async () => choices++ === 0 ? "select-executable" : undefined,
      selectExecutable: async () => "C:\\Wrong\\ergo.exe",
      saveExecutable: async () => {
        saved = true;
      },
    }),
  );

  assert.equal(saved, false);
  assert.deepEqual(result, { status: "stopped", message: unavailable.message });
});

test("retry detection clears the compatibility cache before trying again", async () => {
  let attempts = 0;
  let cleared = 0;
  const result = await runWithCliRecovery(
    "linux",
    "ergo",
    async () => {
      attempts++;
      if (attempts === 1) {
        throw unavailable;
      }
      return "ready";
    },
    recoveryEffects({
      choose: async () => "retry-detection",
      clearCompatibilityCache: () => cleared++,
    }),
  );

  assert.equal(attempts, 2);
  assert.equal(cleared, 1);
  assert.deepEqual(result, { status: "completed", value: "ready", executable: "ergo" });
});

test("replaces the configuration scope that supplied the executable", () => {
  assert.equal(executableSettingScope({}), "global");
  assert.equal(
    executableSettingScope({ workspaceValue: "C:\\Tools\\ergo.exe" }),
    "workspace",
  );
  assert.equal(
    executableSettingScope({
      workspaceValue: "C:\\Tools\\ergo.exe",
      workspaceFolderValue: "C:\\Project\\ergo.exe",
    }),
    "workspace-folder",
  );
});

function recoveryEffects(overrides: Partial<CliRecoveryEffects>): CliRecoveryEffects {
  return {
    choose: async (_message, choices) => choices[0]?.action,
    copyText: async () => undefined,
    notifyCopied: async () => undefined,
    selectExecutable: async () => undefined,
    saveExecutable: async () => undefined,
    openReleases: async () => undefined,
    clearCompatibilityCache: () => undefined,
    ...overrides,
  };
}
