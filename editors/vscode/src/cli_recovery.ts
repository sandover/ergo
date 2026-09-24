import {
  ErgoCommandError,
  ergoReleasesUrl,
  windowsInstallCommand,
} from "./ergo";

export type CliRecoveryAction =
  | "copy-install-command"
  | "select-executable"
  | "retry-detection"
  | "open-releases";

export interface CliRecoveryChoice {
  action: CliRecoveryAction;
  title: string;
}

export type CliRecoveryResult<T> =
  | { status: "completed"; value: T; executable: string }
  | { status: "stopped"; message: string };

export type ExecutableSettingScope = "global" | "workspace" | "workspace-folder";

export interface ExecutableSettingValues {
  workspaceFolderValue?: unknown;
  workspaceValue?: unknown;
}

export interface CliRecoveryEffects {
  choose(message: string, choices: readonly CliRecoveryChoice[]): Promise<CliRecoveryAction | undefined>;
  copyText(text: string): Promise<void>;
  notifyCopied(): Promise<void>;
  selectExecutable(): Promise<string | undefined>;
  saveExecutable(executable: string): Promise<void>;
  openReleases(url: string): Promise<void>;
  clearCompatibilityCache(): void;
}

export function cliRecoveryChoices(platform: NodeJS.Platform): CliRecoveryChoice[] {
  const choices: CliRecoveryChoice[] = [];
  if (platform === "win32") {
    choices.push({ action: "copy-install-command", title: "Copy WinGet command" });
  }
  choices.push(
    { action: "select-executable", title: "Select Ergo executable…" },
    { action: "retry-detection", title: "Retry detection" },
    {
      action: "open-releases",
      title: platform === "win32" ? "Open Windows release ZIPs" : "Open Ergo releases",
    },
  );
  return choices;
}

export function executableSettingScope(values: ExecutableSettingValues): ExecutableSettingScope {
  if (values.workspaceFolderValue !== undefined) {
    return "workspace-folder";
  }
  if (values.workspaceValue !== undefined) {
    return "workspace";
  }
  return "global";
}

export async function runWithCliRecovery<T>(
  platform: NodeJS.Platform,
  initialExecutable: string,
  operation: (executable: string) => Promise<T>,
  effects: CliRecoveryEffects,
): Promise<CliRecoveryResult<T>> {
  let executable = initialExecutable;
  let executableToSave: string | undefined;
  const choices = cliRecoveryChoices(platform);

  while (true) {
    try {
      const value = await operation(executable);
      if (executableToSave) {
        await effects.saveExecutable(executableToSave);
      }
      return { status: "completed", value, executable };
    } catch (error) {
      if (!(error instanceof ErgoCommandError) || !error.recoverable) {
        throw error;
      }

      const action = await effects.choose(error.message, choices);
      if (!action || !choices.some((choice) => choice.action === action)) {
        return { status: "stopped", message: error.message };
      }

      if (action === "copy-install-command") {
        await effects.copyText(windowsInstallCommand);
        await effects.notifyCopied();
        continue;
      }
      if (action === "open-releases") {
        await effects.openReleases(ergoReleasesUrl);
        return { status: "stopped", message: error.message };
      }
      if (action === "select-executable") {
        const selected = await effects.selectExecutable();
        if (!selected) {
          return { status: "stopped", message: error.message };
        }
        executable = selected;
        executableToSave = selected;
      }

      effects.clearCompatibilityCache();
    }
  }
}
