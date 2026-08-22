import { execFile } from "node:child_process";

export const minimumErgoVersion = "6.0.0";

export class ErgoCommandError extends Error {
  constructor(message: string) {
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
        reject(new ErgoCommandError(commandErrorMessage(systemError, stderr)));
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
      throw new ErgoCommandError(`${error.message} ${installationGuidance(executable)}`);
    }
    throw error;
  }
  const version = parseErgoVersion(output);
  if (!version) {
    throw new ErgoCommandError(
      `The Ergo executable "${executable}" returned an unrecognized version. ${installationGuidance(executable)}`,
    );
  }
  if (!isSupportedVersion(version)) {
    throw new ErgoCommandError(
      `Ergo ${version} is too old; Ergo Backlog requires ${minimumErgoVersion} or later. ${installationGuidance(executable)}`,
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

function installationGuidance(executable: string): string {
  const source = process.platform === "darwin"
    ? "Install it with `brew install sandover/tap/ergo`"
    : "Install it from https://github.com/sandover/ergo/releases";
  return `${source}, or set ergo.executablePath to the intended executable (attempted "${executable}").`;
}
