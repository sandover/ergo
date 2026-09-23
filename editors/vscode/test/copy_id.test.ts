import assert from "node:assert/strict";
import test from "node:test";
import { copyIdButton, isCopyIdMessage } from "../src/copy_id";

test("copy controls name the item and use a separate action target", () => {
  assert.match(copyIdButton("ABC123", "task"), /data-copy-id="ABC123"/);
  assert.match(copyIdButton("ABC123", "task"), /title="Copy task ID ABC123" aria-label="Copy task ID ABC123"/);
  assert.doesNotMatch(copyIdButton("ABC123", "task"), /data-id=/);
});

test("accepts only exact copy messages with a six-character Ergo ID", () => {
  assert.equal(isCopyIdMessage({ type: "copyId", id: "ABC123" }), true);
  assert.equal(isCopyIdMessage({ type: "copyId", id: "abc123" }), false);
  assert.equal(isCopyIdMessage({ type: "copyId", id: "ABC1234" }), false);
  assert.equal(isCopyIdMessage({ type: "copyId", id: "ABC123", text: "arbitrary text" }), false);
  assert.equal(isCopyIdMessage({ type: "copyId", id: "ABC123", extra: true }), false);
  assert.equal(isCopyIdMessage({ type: "copy", id: "ABC123" }), false);
  assert.equal(isCopyIdMessage(["copyId", "ABC123"]), false);
});
