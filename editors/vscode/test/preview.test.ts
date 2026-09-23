import assert from "node:assert/strict";
import test from "node:test";
import {
  previewName,
  showArguments,
} from "../src/preview";

test("passes show arguments safely and forces plain output", () => {
  const folder = "C:\\Work Projects\\Ergo & Friends";
  assert.deepEqual(showArguments(folder, "ABC123"), [
    "--color=never",
    "--dir",
    folder,
    "show",
    "ABC123",
  ]);
});

test("names task and epic preview tabs concisely", () => {
  assert.equal(previewName("ABC123", "task"), "Ergo task ABC123");
  assert.equal(previewName("DEF456", "epic"), "Ergo epic DEF456");
});
