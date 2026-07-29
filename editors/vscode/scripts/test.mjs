import { readdir } from "node:fs/promises";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const testDirectory = new URL("../out/test/", import.meta.url);
const files = (await readdir(testDirectory))
  .filter((name) => name.endsWith(".test.js"))
  .sort()
  .map((name) => fileURLToPath(new URL(name, testDirectory)));

const child = spawn(process.execPath, ["--test", ...files], {
  stdio: "inherit",
  shell: false,
});
child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exitCode = code ?? 1;
});
