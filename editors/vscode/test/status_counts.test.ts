import assert from "node:assert/strict";
import test from "node:test";
import { ErgoListDocument } from "../src/listing";
import {
  countWorkspaceBacklog,
  formatBacklogStatus,
  formatBacklogTooltip,
} from "../src/status_counts";

test("counts ready and blocked tasks without counting epics or other states", () => {
  const document: ErgoListDocument = {
    version: 1,
    items: [
      { id: "A", title: "Ready", kind: "task", state: "todo", ready: true },
      { id: "B", title: "Waiting", kind: "task", state: "todo", ready: false },
      { id: "C", title: "Blocked", kind: "task", state: "blocked", ready: false },
      { id: "D", title: "Done", kind: "task", state: "done", ready: false },
      { id: "E", title: "Epic", kind: "epic" },
    ],
  };

  assert.deepEqual(countWorkspaceBacklog("Ergo", document), {
    name: "Ergo",
    ready: 1,
    blocked: 1,
  });
});

test("formats the aggregate and keeps folder details for multi-root workspaces", () => {
  const counts = [
    { name: "API", ready: 3, blocked: 1 },
    { name: "Web", ready: 1, blocked: 0 },
  ];

  assert.equal(formatBacklogStatus(counts), "Ergo: 4 ready · 1 blocked");
  assert.equal(
    formatBacklogTooltip(counts),
    "Open the Ergo backlog\n\nAPI: 3 ready · 1 blocked\nWeb: 1 ready · 0 blocked",
  );
  assert.equal(formatBacklogTooltip([counts[0]]), "Open the Ergo backlog");
});
