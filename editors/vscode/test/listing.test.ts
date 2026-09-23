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
		{
		  id: "FAIL01",
		  title: "Verify the package",
		  kind: "task",
		  state: "failed",
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
    label: "ROOT01  $(sync) Add support-safe plugin diagnostics to server logs",
    description: "in progress",
    item: document.items[0],
  });
  assert.deepEqual(picker[2], {
    type: "separator",
    label: "EPICS",
  });
  assert.deepEqual(picker[3], {
    type: "item",
    label: "EPIC01  $(symbol-structure) Create one Citation from text that spans pages",
    description: "2 tasks",
    item: document.items[1],
  });
  assert.deepEqual(picker[4], {
    type: "item",
    label: "TASK01  ↳ $(clock) Define and store multi-page Citation locations",
    description: "waiting",
    item: document.items[2],
  });
  assert.deepEqual(picker[5], {
    type: "item",
    label: "FAIL01  ↳ $(close) Verify the package",
    description: "failed",
    item: document.items[3],
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

test("marks an epic failed when every child finishes and one fails", () => {
	const document = parseListDocument(JSON.stringify({
	  version: 1,
	  items: [
		{ id: "EPIC01", title: "Ship release", kind: "epic" },
		{ id: "DONE01", title: "Build", kind: "task", state: "done", ready: false, epic_id: "EPIC01" },
		{ id: "FAIL01", title: "Verify", kind: "task", state: "failed", ready: false, epic_id: "EPIC01" },
	  ],
	}));
	const picker = toPickerItems(document);
	assert.deepEqual(picker[1], {
	  type: "item",
	  label: "EPIC01  $(close) Ship release",
	  description: "2 tasks · failed",
	  item: document.items[0],
	});
});

test("keeps draft tasks visible and unavailable in the picker", () => {
  const document = parseListDocument(JSON.stringify({
    version: 1,
    items: [{ id: "DRAFT01", title: "Stage the work", kind: "task", state: "draft", ready: false }],
  }));
  const picker = toPickerItems(document);
  assert.deepEqual(picker[1], {
    type: "item",
    label: "DRAFT01  ◌ Stage the work",
    description: "draft",
    item: document.items[0],
  });
});

test("uses plain status words for every supported task state", () => {
  const document = parseListDocument(JSON.stringify({
    version: 1,
    items: [
      { id: "READY01", title: "Ready", kind: "task", state: "todo", ready: true },
      { id: "WAIT01", title: "Waiting", kind: "task", state: "todo", ready: false },
      { id: "DRAFT01", title: "Draft", kind: "task", state: "draft", ready: false },
      { id: "DOING01", title: "Doing", kind: "task", state: "doing", ready: false },
      { id: "BLOCK01", title: "Blocked", kind: "task", state: "blocked", ready: false },
      { id: "FAIL01", title: "Failed", kind: "task", state: "failed", ready: false },
      { id: "DONE01", title: "Done", kind: "task", state: "done", ready: false },
      { id: "CANCEL01", title: "Canceled", kind: "task", state: "canceled", ready: false },
      { id: "ERROR01", title: "Legacy error", kind: "task", state: "error", ready: false },
    ],
  }));

  assert.deepEqual(
    toPickerItems(document)
      .filter((entry) => entry.type === "item")
      .map((entry) => entry.type === "item" ? entry.description : ""),
    ["ready", "waiting", "draft", "in progress", "blocked", "failed", "done", "canceled", "legacy error"],
  );
});
