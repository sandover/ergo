import { execFile } from "node:child_process";

export const minimumErgoVersion = "6.0.0";
export const ergoReleasesUrl = "https://github.com/sandover/ergo/releases/latest";
export const windowsInstallCommand = "winget install --id Sandover.Ergo --exact";

export class ErgoCommandError extends Error {
  constructor(message: string, readonly recoverable = false) {
    super(message);
    this.name = "ErgoCommandError";
  }
}

export function listArguments(folder: string): string[] {
  return ["--dir", folder, "list", "--json"];
}

export async function runErgo(
  args: readonly string[],
  executable = "ergo",
): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      executable,
      [...args],
      { encoding: "utf8", maxBuffer: 16 * 1024 * 1024 },
      (error, stdout, stderr) => {
        if (!error) {
          resolve(stdout);
          return;
        }
        const systemError = error as NodeJS.ErrnoException;
        reject(new ErgoCommandError(
          commandErrorMessage(systemError, stderr),
          isExecutableAccessError(systemError),
        ));
      },
    );
  });
}

const compatibleExecutables = new Map<string, Promise<void>>();

export async function runCompatibleErgo(
  args: readonly string[],
  executable = "ergo",
): Promise<string> {
  let check = compatibleExecutables.get(executable);
  if (!check) {
    check = checkCompatibility(executable);
    compatibleExecutables.set(executable, check);
  }
  try {
    await check;
  } catch (error) {
    compatibleExecutables.delete(executable);
    throw error;
  }
  return runErgo(args, executable);
}

export function clearCompatibilityCache(): void {
  compatibleExecutables.clear();
}

export function parseErgoVersion(output: string): string | undefined {
  return output.match(/(?:^|\s)v?(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)(?:\s|$)/)?.[1];
}

export function isSupportedVersion(version: string): boolean {
  const current = version.split(/[+-]/, 1)[0].split(".").map(Number);
  const minimum = minimumErgoVersion.split(".").map(Number);
  return current.every((part, index) =>
    part === minimum[index]
      ? true
      : current.slice(0, index).every((prior, priorIndex) => prior === minimum[priorIndex])
        ? part > minimum[index]
        : true,
  );
}

async function checkCompatibility(executable: string): Promise<void> {
  let output: string;
  try {
    output = await runErgo(["--version"], executable);
  } catch (error) {
    if (error instanceof ErgoCommandError) {
      throw new ErgoCommandError(
        `${error.message} ${installationGuidance(executable)}`,
        true,
      );
    }
    throw error;
  }
  const version = parseErgoVersion(output);
  if (!version) {
    throw new ErgoCommandError(
      `The Ergo executable "${executable}" returned an unrecognized version. ${installationGuidance(executable)}`,
      true,
    );
  }
  if (!isSupportedVersion(version)) {
    throw new ErgoCommandError(
      `Ergo ${version} is too old; Ergo Backlog requires ${minimumErgoVersion} or later. ${installationGuidance(executable)}`,
      true,
    );
  }
}

export function commandErrorMessage(error: NodeJS.ErrnoException, stderr: string): string {
  if (error.code === "ENOENT") {
    return "Ergo was not found.";
  }
  if (error.code === "EACCES") {
    return "The configured Ergo executable is not executable.";
  }
  return stderr.trim() || error.message;
}

export function isExecutableAccessError(error: NodeJS.ErrnoException): boolean {
  return error.code === "ENOENT" || error.code === "EACCES";
}

export function installationGuidance(
  executable: string,
  platform: NodeJS.Platform = process.platform,
): string {
  let source: string;
  if (platform === "win32") {
    source = `On Windows, install with \`${windowsInstallCommand}\`. If WinGet does not find Ergo, download the Windows ZIP from ${ergoReleasesUrl} and extract ergo.exe.`;
  } else if (platform === "darwin") {
    source = `On macOS, install with \`brew install sandover/tap/ergo\`, or download a macOS release from ${ergoReleasesUrl}.`;
  } else if (platform === "linux") {
    source = `On Linux, download the appropriate release archive from ${ergoReleasesUrl} and put ergo on PATH.`;
  } else {
    source = `Install Ergo from ${ergoReleasesUrl}.`;
  }
  return `${source} Select a compatible executable or update ergo.executablePath (attempted "${executable}").`;
}
