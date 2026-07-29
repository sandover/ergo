import assert from "node:assert/strict";
import test from "node:test";
import {
  PreviewStore,
  previewIdentity,
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

test("keeps workspace and task preview identities distinct", () => {
  assert.notEqual(
    previewIdentity("/work/one", "ABC123", "task"),
    previewIdentity("/work/two", "ABC123", "task"),
  );
  assert.notEqual(
    previewIdentity("/work/one", "ABC123", "task"),
    previewIdentity("/work/one", "DEF456", "task"),
  );
});

test("names task and epic preview tabs concisely", () => {
  assert.equal(previewName("ABC123", "task"), "Ergo task ABC123");
  assert.equal(previewName("DEF456", "epic"), "Ergo epic DEF456");
});

test("stores preview source without changing any bytes", () => {
  const source = "---\nid: ABC123\n---\n\n# Title\n\nBody without final newline";
  const identity = previewIdentity("/work/one", "ABC123", "task");
  const store = new PreviewStore();
  store.set(identity, source);
  assert.equal(store.get(identity), source);
});
