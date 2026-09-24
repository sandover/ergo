import * as vscode from "vscode";
import { clearCompatibilityCache } from "./ergo";
import {
  executableSettingScope,
  runWithCliRecovery as runRecovery,
} from "./cli_recovery";
import type { CliRecoveryResult } from "./cli_recovery";

export function runWithCliRecovery<T>(
  resource: vscode.Uri,
  executable: string,
  operation: (executable: string) => Promise<T>,
): Promise<CliRecoveryResult<T>> {
  return runRecovery(process.platform, executable, operation, {
    choose: async (message, choices) => {
      const titles = choices.map((choice) => choice.title);
      const selected = await vscode.window.showErrorMessage(message, ...titles);
      return choices.find((choice) => choice.title === selected)?.action;
    },
    copyText: async (text) => {
      await vscode.env.clipboard.writeText(text);
    },
    notifyCopied: async () => {
      await vscode.window.showInformationMessage(
        "Copied the WinGet command. Paste it in a terminal to install Ergo, then choose Retry detection.",
      );
    },
    selectExecutable: async () => {
      const files = await vscode.window.showOpenDialog({
        title: "Select Ergo executable",
        openLabel: "Select Ergo executable",
        canSelectFiles: true,
        canSelectFolders: false,
        filters: process.platform === "win32" ? { Executable: ["exe"] } : undefined,
      });
      return files?.[0]?.fsPath;
    },
    saveExecutable: async (selected) => {
      const configuration = vscode.workspace.getConfiguration("ergo", resource);
      const scope = executableSettingScope(configuration.inspect("executablePath") ?? {});
      const target = scope === "workspace-folder"
        ? vscode.ConfigurationTarget.WorkspaceFolder
        : scope === "workspace"
          ? vscode.ConfigurationTarget.Workspace
          : vscode.ConfigurationTarget.Global;
      await configuration.update("executablePath", selected, target);
    },
    openReleases: async (url) => {
      await vscode.env.openExternal(vscode.Uri.parse(url));
    },
    clearCompatibilityCache,
  });
}
