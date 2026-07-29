import assert from "node:assert/strict";
import test from "node:test";
import { parseListDocument, toPickerItems } from "../src/listing";

test("groups roots and epics into compact searchable picker rows", () => {
  const document = parseListDocument(
    JSON.stringify({
      version: 1,
      items: [
        {
          id: "ROOT01",
          title: "Add support-safe plugin diagnostics to server logs",
          kind: "task",
          state: "doing",
          ready: false,
        },
        { id: "EPIC01", title: "Create one Citation from text that spans pages", kind: "epic" },
        {
          id: "TASK01",
          title: "Define and store multi-page Citation locations",
          kind: "task",
          state: "todo",
          ready: false,
          epic_id: "EPIC01",
        },
      ],
    }),
  );

  const picker = toPickerItems(document);
  assert.deepEqual(picker[0], {
    type: "separator",
    label: "ROOT TASKS",
  });
  assert.deepEqual(picker[1], {
    type: "item",
    label: "$(sync) Add support-safe plugin diagnostics to server logs",
    description: "ROOT01 · doing",
    item: document.items[0],
  });
  assert.deepEqual(picker[2], {
    type: "separator",
    label: "EPICS",
  });
  assert.deepEqual(picker[3], {
    type: "item",
    label: "$(symbol-structure) Create one Citation from text that spans pages",
    description: "EPIC01 · 1 task",
    item: document.items[1],
  });
  assert.deepEqual(picker[4], {
    type: "item",
    label: "    ↳ $(clock) Define and store multi-page Citation locations",
    description: "TASK01 · waiting",
    item: document.items[2],
  });
});

test("rejects malformed and unsupported listings", () => {
  assert.throws(() => parseListDocument("{"), /invalid task listing/);
  assert.throws(
    () => parseListDocument('{"version":2,"items":[]}'),
    /does not support Ergo task listing version 2/,
  );
  assert.throws(
    () => parseListDocument('{"version":1,"items":[{"id":"TASK01","title":"Task","kind":"task"}]}'),
    /invalid task listing/,
  );
});
