// The tests of the runs the Machines page counts, in Chrome, on a house
// whose keyed room has four runs: two open, one closed, and one that ended
// failed today. The UI acts as the controller and as the client that
// started them.
import { test, until, assert, equal, text, copy } from "/test/page/testdata/test.js";
import { parts, receive } from "ui";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

// countOn gives the count drawn on a card, by its path, or "" for none.
const countOn = path => text($(`#drawing [data-path="${CSS.escape(path)}"] .count text`));

test("machines_a_location_counts_its_runs", () => {
  const row = $$(".place .named > span").find(s => text(s) === "location").nextElementSibling;
  equal(text(row), "/house/room · 3 running · 1 failed today", "the location's row");
  equal($("a", row).getAttribute("href"), "/runs?at=%2Fhouse%2Froom", "the link to the runs there");
});

test("machines_drawing_counts_the_runs_in_each_state", async () => {
  await until(() => countOn("open"), "the counts drawn");
  equal(countOn("open"), "2", "the runs in open");
  equal(countOn("closed"), "1", "the runs in closed");
  equal(countOn("broke"), "", "the runs in broke, where a run ended");
});

test("machines_counts_follow_the_store", async () => {
  const runs = copy(parts.runs);
  runs["/house/room"].In.open = 5;
  runs["/house/room"].Running = 6;
  receive({ type: "part", id: "runs", data: runs });
  await until(() => countOn("open") === "5", "the count of open as sent");
  const row = $$(".place .named > span").find(s => text(s) === "location").nextElementSibling;
  assert(text(row).includes("6 running"), "the location's row is " + text(row));
});
