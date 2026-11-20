// The tests of the runs the Machines page counts, in Chrome, on a house
// whose keyed room has four runs: two open, one closed, and one that ended
// failed today. The UI acts as the controller and as the client that
// started them, and reads the store again only when the list of runs
// changes.
import { test, until, assert, equal, text } from "/test/page/testdata/test.js";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

// countOn gives the count drawn on a card, by its path, or "" for none.
const countOn = path => text($(`#drawing [data-path="${CSS.escape(path)}"] .count text`));

test("machines_location_row_says_its_counts", () => {
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
  // A run started at key 5 is open: the list of runs changes, and the page
  // is sent the room's counts at once, though it reads only hourly.
  const res = await fetch("/started", { method: "POST", body: new URLSearchParams({ key: "5" }) });
  assert(res.ok, "the run was not started: " + (await res.text()));
  await until(() => countOn("open") === "3", "the count of open with the new run");
  const row = $$(".place .named > span").find(s => text(s) === "location").nextElementSibling;
  equal(text(row), "/house/room · 4 running · 1 failed today", "the location's row");
});
