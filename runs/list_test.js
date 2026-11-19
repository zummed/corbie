// The tests of the Runs page, in Chrome, on the page the UI serves: what
// it lists and where, how a line unfolds, how it lays out, and how it
// follows the store.
import { test, settle, wait, assert, equal, text } from "/test/page/testdata/test.js";
import { parts, receive } from "ui";

// copy gives a part's data as plain JSON, to change and send.
const copy = v => JSON.parse(JSON.stringify(v));
// words gives the words a person reads in an element, each element's apart,
// its icons' names left out.
const words = el => {
  if (!el) return "";
  const all = [], walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  for (let n = walk.nextNode(); n; n = walk.nextNode()) if (!n.parentElement.closest("svg.g")) all.push(n.data);
  return all.join(" ").replace(/\s+/g, " ").trim();
};

const section = id => document.getElementById(id);
// lineOf finds a line by its title, the address of its root.
const lineOf = name => [...document.querySelectorAll("details.req")].find(d => text(d.querySelector("summary .title")).startsWith("/" + name));

test("runs_list_roots_by_section", async () => {
  await settle();
  equal(text(section("busy").querySelector("h2")), "Busy", "the first section");
  equal(section("busy").querySelectorAll(":scope > details.req").length, 2, "the busy lines");
  equal(text(section("busy").querySelector(".facts")), "2", "the busy count");
  assert(section("idle").hidden && section("singletons").hidden, "an empty section is shown");
  const ended = section("ended").querySelectorAll(":scope > details.req");
  equal(ended.length, 1, "the ended lines");
  assert(text(ended[0]).includes("failed (error)"), `the ended line is ${text(ended[0])}`);
  equal([...section("ended").querySelectorAll(".pills a")].map(text), ["1h", "6h", "24h", "7d"], "the windows");
  equal(section("ended").querySelector(".pills a[aria-current]") && text(section("ended").querySelector(".pills a[aria-current]")), "6h", "the window chosen");
});

test("runs_locations_are_listed_by_tree", async () => {
  const nav = document.getElementById("locations");
  equal(text(nav.querySelector("a[aria-current]")), "every location", "the location chosen");
  equal(nav.querySelectorAll("h2.group").length, 1, "the clients");
  equal(nav.querySelectorAll("a.tree").length, 3, "the trees");
  const running = [...nav.querySelectorAll("a.loc")].filter(a => text(a).includes("1 running"));
  equal(running.length, 2, "the locations with a run running");
  for (const a of nav.querySelectorAll("a.loc, a.tree")) assert(a.getAttribute("href").startsWith("/runs?at="), `a location links to ${a.getAttribute("href")}`);
});

test("runs_line_unfolds_to_the_runs_its_root_started", async () => {
  const line = lineOf("lookup");
  assert(!line.open, "a line is unfolded before a click");
  line.querySelector("button.unfold").click();
  await settle();
  assert(line.open, "the button does not unfold the line");
  const rows = [...line.querySelectorAll(".tree .node")];
  equal(rows.length, 2, "the rows of the lookup");
  assert(rows[0].matches(".own") && text(rows[0]).includes("lookup"), `the first row is ${text(rows[0])}`);
  assert(text(rows[1]).includes("review") && text(rows[1]).includes("ticket verdict"), `the child's row is ${text(rows[1])}`);
  equal(rows[1].querySelector("a.name").getAttribute("href"), "/run/" + parts.busy.Lines.find(l => l.Machine === "lookup").Rows[1].ID, "the child's link");
  // A line stays unfolded when its part is sent again.
  receive({ type: "part", id: "busy", data: copy(parts.busy) });
  await settle();
  assert(lineOf("lookup").open, "the line folded when its part came again");
  line.querySelector("button.unfold").click();
  await settle();
  assert(!line.open, "the button does not fold the line");
});

test("runs_lines_fit_their_row", async () => {
  await settle();
  assert(document.documentElement.scrollWidth <= innerWidth, "the page scrolls across");
  for (const s of document.querySelectorAll("summary.line, .node")) {
    if (s.offsetParent === null) continue;
    const r = s.getBoundingClientRect();
    assert(Math.abs(r.height - 31) <= 1.5, `a line is ${r.height}px high: ${text(s)}`);
    for (const cell of s.children) {
      const c = cell.getBoundingClientRect();
      assert(c.right <= r.right + 0.5 && c.left >= r.left - 0.5, `${text(cell)} leaves its line`);
    }
  }
  const main = document.querySelector("main").getBoundingClientRect();
  for (const p of document.querySelectorAll(".panel")) {
    const r = p.getBoundingClientRect();
    assert(r.right <= main.right + 0.5, `a panel reaches past the page: ${p.id}`);
  }
});

test("runs_a_line_follows_the_store", async () => {
  const busy = copy(parts.busy);
  const review = busy.Lines.find(l => l.Machine === "review");
  // A line that changes is lit; one that is new fades in.
  review.Waits = "ticket verdict";
  busy.Lines.unshift({ ...copy(review), ID: "a-client/new", Name: "/new", Machine: "new", Waits: "", Rows: [] });
  receive({ type: "part", id: "busy", data: copy(busy) });
  await settle();
  assert(lineOf("review").querySelector("summary").classList.contains("moved"), "a changed line is not lit");
  assert(lineOf("new").querySelector("summary").classList.contains("new"), "a new line does not fade in");
  equal(text(section("busy").querySelector(".facts")), "3", "the busy count");
  assert(section("busy").querySelector(".facts").classList.contains("bump"), "the count does not lift");
  // A run that ends moves to Ended, lit in its status's colour.
  const ended = copy(parts.ended);
  const done = busy.Lines.shift();
  ended.Lines.unshift({ ...done, Status: "done", Ended: true, Took: "1.0s" });
  receive({ type: "part", id: "ended", data: copy(ended) });
  receive({ type: "part", id: "busy", data: copy(busy) });
  await settle();
  const line = [...section("ended").querySelectorAll("details.req")].find(d => text(d).includes("new"));
  assert(line?.querySelector("summary").classList.contains("ended"), "a run that ended is not lit as ended");
  assert(!section("busy").querySelector("details.req .title")?.textContent.includes("/new"), `a run that ended stayed in Busy: ${[...section("busy").querySelectorAll(".title")].map(text)} with the part holding ${parts.busy.Lines.map(l => l.Name)}`);
  await wait(10);
});

test("runs_store_lost_keeps_the_page", async () => {
  const before = text(section("busy"));
  receive({ type: "store", store: "lost" });
  await settle();
  assert(document.getElementById("live").classList.contains("lost"), "the dot is not rowan");
  equal(text(section("busy")), before, "the page changed when the store was lost");
  receive({ type: "store", store: "live" });
});
