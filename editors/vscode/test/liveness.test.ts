import assert from "node:assert/strict";
import test from "node:test";
import { RefreshGate } from "../src/refresh_gate";

test("applies only the newest overlapping refresh", async () => {
  const gate = new RefreshGate();
  const applied: string[] = [];
  let finishOld: ((value: string) => void) | undefined;

  const old = gate.run(
    () => new Promise<string>((resolve) => {
      finishOld = resolve;
    }),
    (value) => applied.push(value),
  );
  const latest = gate.run(
    async () => "latest",
    (value) => applied.push(value),
  );
  await latest;
  finishOld?.("old");
  await old;

  assert.deepEqual(applied, ["latest"]);
});

test("does not apply a refresh after disposal", async () => {
  const gate = new RefreshGate();
  const applied: string[] = [];
  let finish: ((value: string) => void) | undefined;
  const refresh = gate.run(
    () => new Promise<string>((resolve) => {
      finish = resolve;
    }),
    (value) => applied.push(value),
  );

  gate.dispose();
  finish?.("late");
  await refresh;
  assert.deepEqual(applied, []);
});

test("ignores a stale refresh failure", async () => {
  const gate = new RefreshGate();
  let rejectOld: ((error: Error) => void) | undefined;
  const old = gate.run(
    () => new Promise<string>((_resolve, reject) => {
      rejectOld = reject;
    }),
    () => assert.fail("stale refresh applied"),
  );
  await gate.run(async () => "latest", () => undefined);
  rejectOld?.(new Error("stale failure"));
  await assert.doesNotReject(old);
});
