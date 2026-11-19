// The tests of the Runs page, in Chrome, on the page the UI serves: what
// it lists and where, how a line unfolds, how it lays out, and how it
// follows the store.
import { test, settle, until, click, copy, words, assert, equal, text } from "/test/page/testdata/test.js";
import { parts, receive } from "ui";
import { streamed } from "/test/runs/streamed.js";

const section = id => document.getElementById(id);
// lineOf finds a line by its title, the address of its root.
const lineOf = name => [...document.querySelectorAll("details.req")].find(d => text(d.querySelector("summary .title")).startsWith("/" + name));
// busy gives the Busy part as the store holds it, with one line changed.
const sendBusy = busy => receive({ type: "part", id: "busy", data: copy(busy) });

test("roots_are_listed_by_section", async () => {
  await streamed();
  equal(section("busy").querySelectorAll(":scope > details.req").length, 2, "the busy lines");
  equal(text(section("busy").querySelector(".facts")), "2", "the busy count");
  equal([...section("idle").querySelectorAll(":scope > details.req .title")].map(text), ["/house/room#1"], "the idle lines");
  assert(section("singletons").hidden, "an empty section is shown");
  const ended = [...section("ended").querySelectorAll(":scope > details.req")].map(words);
  equal(ended.length, 2, "the ended lines");
  assert(ended.some(l => l.startsWith("failed (error) /broken")), `the ended lines are ${ended}`);
  equal([...section("ended").querySelectorAll(".pills a")].map(text), ["1h", "6h", "24h", "7d"], "the windows");
  equal(text(section("ended").querySelector(".pills a[aria-current]")), "6h", "the window chosen");
});

test("locations_link_to_their_runs", async () => {
  const nav = document.getElementById("locations");
  equal(text(nav.querySelector("a[aria-current]")), "every location", "the location chosen");
  equal(nav.querySelectorAll("h2.group").length, 1, "the clients");
  equal([...nav.querySelectorAll("a.tree")].map(words).filter(t => t.startsWith("/house")), ["/house tree"], "the submitted tree");
  equal([...nav.querySelectorAll("a.loc")].filter(a => text(a).includes("1 running")).length, 3, "the locations with a run running");
  for (const a of nav.querySelectorAll("a.loc, a.tree")) assert(a.getAttribute("href").startsWith("/runs?at="), `a location links to ${a.getAttribute("href")}`);
});

test("a_line_unfolds_and_stays_unfolded", async () => {
  const line = lineOf("lookup");
  if (line.open) click(line.querySelector("button.unfold"));
  await until(() => !lineOf("lookup").open, "the line to fold");
  click(line.querySelector("button.unfold"));
  await until(() => lineOf("lookup").open, "the button to unfold the line");
  const rows = [...line.querySelectorAll(".tree .node")];
  equal(rows.length, 2, "the rows of the lookup");
  assert(rows[0].matches(".own") && words(rows[0]).includes("lookup"), `the first row is ${words(rows[0])}`);
  assert(words(rows[1]).includes("review") && words(rows[1]).includes("ticket verdict"), `the child's row is ${words(rows[1])}`);
  equal(rows[1].querySelector("a.name").getAttribute("href"), "/run/" + parts.busy.Lines.find(l => l.Machine === "lookup").Rows[1].ID, "the child's link");
  sendBusy(parts.busy);
  await settle();
  assert(lineOf("lookup").open, "the line folded when its part came again");
  click(line.querySelector("button.unfold"));
  await until(() => !lineOf("lookup").open, "the button to fold the line");
});

test("lines_fit_their_row", async () => {
  await settle();
  assert(document.documentElement.scrollWidth <= innerWidth, "the page scrolls across");
  for (const s of document.querySelectorAll("summary.line, .node")) {
    if (s.offsetParent === null) continue;
    const r = s.getBoundingClientRect();
    assert(Math.abs(r.height - 31) <= 1.5, `a line is ${r.height}px high: ${words(s)}`);
    for (const cell of s.children) {
      const c = cell.getBoundingClientRect();
      assert(c.right <= r.right + 0.5 && c.left >= r.left - 0.5, `${words(cell)} leaves its line`);
    }
  }
  const main = document.querySelector("main").getBoundingClientRect();
  for (const p of document.querySelectorAll(".panel")) assert(p.getBoundingClientRect().right <= main.right + 0.5, `a panel reaches past the page: ${p.id}`);
});

test("a_line_is_lit_as_it_changes_and_ends", async () => {
  await streamed();
  const busy = copy(parts.busy);
  const review = busy.Lines.find(l => l.Machine === "review");
  review.Waits = "ticket verdict";
  busy.Lines.unshift({ ...copy(review), ID: "a-client/new", Name: "/new", Machine: "new", Waits: "", Rows: [] });
  sendBusy(busy);
  await until(() => lineOf("new"), "the new line");
  assert(lineOf("review").querySelector("summary").classList.contains("moved"), "a changed line is not lit");
  assert(lineOf("new").querySelector("summary").classList.contains("new"), "a new line does not fade in");
  equal(text(section("busy").querySelector(".facts")), "3", "the busy count");
  assert(section("busy").querySelector(".facts").classList.contains("bump"), "the count does not lift");
  // A run that ends moves to Ended, lit in its status's colour.
  const ended = copy(parts.ended);
  const done = busy.Lines.shift();
  ended.Lines.unshift({ ...done, Status: "done", Ended: true, Took: "1.0s" });
  receive({ type: "part", id: "ended", data: copy(ended) });
  sendBusy(busy);
  await until(() => !section("busy").querySelector(".title")?.textContent.includes("/new"), "the run that ended to leave Busy");
  const line = [...section("ended").querySelectorAll("details.req")].find(d => text(d.querySelector(".title")) === "/new");
  assert(line?.querySelector("summary").classList.contains("ended"), "a run that ended is not lit as ended");
});

test("a_lost_store_keeps_the_page", async () => {
  const before = words(section("busy"));
  receive({ type: "store", store: "lost" });
  await settle();
  assert(document.getElementById("live").classList.contains("lost"), "the dot is not rowan");
  equal(words(section("busy")), before, "the page changed when the store was lost");
  receive({ type: "store", store: "live" });
  await until(() => !document.getElementById("live").classList.contains("lost"), "the dot to turn back");
});
