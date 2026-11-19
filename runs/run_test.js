// The tests of a run's page, in Chrome, on the page of a lookup whose
// stage started a review that waits on its ticket: its head, Steps, the
// drawing and the details, how the selection moves and follows, and how it
// lays out.
import { test, fixture, settle, wait, assert, equal, text, overlap } from "/test/page/testdata/test.js";
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

const steps = () => [...document.querySelectorAll("#steps .step")];
const visibleSteps = () => steps().filter(r => r.offsetParent !== null);
const selected = () => document.querySelector("#steps .step.sel");
const rowOf = words => steps().find(r => text(r.querySelector(".what")).startsWith(words));
const click = el => el.dispatchEvent(new MouseEvent("click", { bubbles: true }));
const key = name => document.body.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true }));

// gate holds the UI's answers for a row's drawing until it is opened, so a
// test can see the page while one is on its way.
function gate() {
  const real = window.fetch;
  let open;
  const opened = new Promise(r => (open = r));
  window.fetch = async (...args) => {
    if (String(args[0]).includes("/at?")) await opened;
    return real(...args);
  };
  return () => { open(); window.fetch = real; };
}

async function answered() {
  for (let i = 0; i < 100 && document.getElementById("view").classList.contains("pending"); i++) await wait(20);
  await settle();
}

test("runs_own_run_shown_in_full", async () => {
  await wait(700); // the stream's first message
  await settle();
  const ids = await fixture("ids");
  const head = document.getElementById("summary");
  assert(words(head).startsWith("lookup flow running"), `the head is ${words(head)}`);
  equal(head.querySelector(".acts button").dataset.cmd, "corbie run cancel " + ids.lookup, "Cancel's command");
  equal([...document.querySelectorAll(".split > .panel > .tabs a")].map(text), ["Steps", "Waiting on", "Output", "Children"], "the tabs");
  equal(text(document.querySelector("#steps .facts")), String(parts.steps.Count), "the count of rows");
  equal(steps().length, parts.steps.Count, "the rows");
  const check = rowOf("stage check started review");
  assert(check?.closest("li").matches(".group:not(.folded)"), "the running child is not an open group");
  assert(rowOf("entered drafting")?.closest("li.group") === check.closest("li.group").querySelector("li")?.closest("li.group"), "the child's rows are not under its row");
  equal([...document.querySelectorAll("#path a")].map(words), ["lookup", "review"], "the path");
});

test("runs_follow_returns_to_the_newest_row", async () => {
  const newest = steps().at(-1);
  assert(newest.classList.contains("sel"), `the newest row is not selected: ${text(selected())}`);
  assert(!document.querySelector("#steps .acts button[title^='select the newest']"), "Follow is offered while following");
  click(steps()[0]);
  await settle();
  assert(steps()[0].classList.contains("sel"), "a click does not select its row");
  const follow = [...document.querySelectorAll("#steps .acts button")].find(b => text(b) === "Follow");
  assert(follow, "Follow is not offered after a row is chosen");
  await answered();
  click(follow);
  await settle();
  assert(steps().at(-1).classList.contains("sel"), "Follow does not select the newest row");
  assert(![...document.querySelectorAll("#steps .acts button")].some(b => text(b) === "Follow"), "Follow stays after it is used");
  await answered();
});

test("runs_selection_lands_before_the_drawing", async () => {
  const before = words(document.querySelector("#view .head"));
  const open = gate();
  click(rowOf("stage find done"));
  await settle();
  assert(rowOf("stage find done").classList.contains("sel"), "the selection waits for the drawing");
  assert(document.getElementById("details").hidden === false, "the details are not shown");
  equal(words(document.querySelector("#view .head")), before, "the drawing changed before its answer");
  assert(document.getElementById("view").classList.contains("pending"), "the drawing does not show that its answer is on its way");
  open();
  await answered();
  assert(words(document.querySelector("#view .head")).includes("as of step 2"), `the drawing is ${words(document.querySelector("#view .head"))}`);
  equal(text(document.querySelector("#details .head .facts")), "stage find done", "the details");
});

test("runs_a_step_shows_its_run_as_it_was", async () => {
  click(rowOf("entered drafting"));
  await answered();
  const head = words(document.querySelector("#view .head"));
  assert(head.startsWith("review statechart") && head.includes("as of step 1"), `the drawing's head is ${head}`);
  // The page's run is drawn, with the review inside the card of the stage
  // that started it.
  const svg = document.querySelector("#view svg.machine");
  const names = [...svg.querySelectorAll("text.title")].map(text);
  for (const n of ["find", "check", "tell", "drafting", "waiting"]) assert(names.includes(n), `the drawing has no ${n}: ${names}`);
  const check = [...svg.querySelectorAll("g.state")].find(g => text(g.querySelector("text.title")) === "check");
  const drafting = [...svg.querySelectorAll("g.state")].find(g => text(g.querySelector("text.title")) === "drafting");
  const a = check.querySelector("rect.box").getBoundingClientRect(), b = drafting.querySelector("rect.box").getBoundingClientRect();
  assert(b.left >= a.left && b.right <= a.right && b.top >= a.top && b.bottom <= a.bottom, "the review is not inside the check's card");
  assert(drafting.classList.contains("now"), "the review is not marked as in drafting");
  assert(check.classList.contains("now"), "the check is not marked as running");
  equal([...document.querySelectorAll("#path a")].map(a => [words(a), a.classList.contains("sel")]), [["lookup", false], ["review", true]], "the path's light");
  // The record tab gives the step's entries.
  [...document.querySelectorAll("#details .tabs a")].find(a => text(a) === "Record").click();
  await settle();
  const record = text(document.querySelector("#details .record"));
  assert(record.includes("review") && /<- start/.test(document.querySelector("#details .record").textContent), `the record is ${record}`);
});

test("runs_keyboard_moves_the_selection", async () => {
  click(rowOf("stage check started review"));
  await answered();
  key("ArrowDown");
  await settle();
  equal(text(selected().querySelector(".what")), "entered drafting", "ArrowDown selects");
  key("ArrowUp");
  await settle();
  assert(text(selected().querySelector(".what")).startsWith("stage check started review"), "ArrowUp selects");
  key("ArrowLeft");
  await settle();
  assert(rowOf("stage check started review").closest("li").classList.contains("folded"), "ArrowLeft does not fold the group");
  assert(rowOf("entered drafting").offsetParent === null, "a folded group's rows are shown");
  key("ArrowRight");
  await settle();
  assert(!rowOf("stage check started review").closest("li").classList.contains("folded"), "ArrowRight does not open the group");
  await answered();
});

test("runs_folding_a_group_selects_its_head", async () => {
  click(rowOf("entered drafting"));
  await settle();
  rowOf("stage check started review").querySelector("button.fold").click();
  await settle();
  assert(selected() === rowOf("stage check started review"), `folding selects ${text(selected())}`);
  rowOf("stage check started review").querySelector("button.fold").click();
  await settle();
  equal(visibleSteps().length, steps().length, "the group stays folded");
  await answered();
});

test("runs_a_step_that_arrives_is_followed", async () => {
  [...document.querySelectorAll("#steps .acts button")].find(b => text(b) === "Follow")?.click();
  await answered();
  const next = copy(parts.steps);
  const last = next.Rows.at(-1);
  next.Rows.push({ ...copy(last), Key: last.Key + "+", At: new Date(Date.parse(last.At) + 1000).toISOString(), What: [{ Text: "a step that arrived" }], Group: null, Trace: null });
  next.Count++;
  receive({ type: "part", id: "steps", data: copy(next) });
  await settle();
  equal(text(selected().querySelector(".what")), "a step that arrived", "the selection does not follow");
  assert(selected().classList.contains("new"), "the row does not slide in");
  equal(text(document.querySelector("#steps .facts")), String(next.Count), "the count");
});

test("runs_the_page_fits_the_window", async () => {
  assert(document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight, "the page scrolls");
  const view = document.querySelector("#view .drawn").getBoundingClientRect();
  const svg = document.querySelector("#view svg.machine").getBoundingClientRect();
  assert(svg.width >= view.width - 2 && svg.height >= view.height - 2, `the drawing ${svg.width}x${svg.height} does not fill its panel ${view.width}x${view.height}`);
  for (const r of visibleSteps()) {
    const box = r.getBoundingClientRect();
    for (const cell of r.querySelectorAll(":scope > span")) assert(cell.getBoundingClientRect().right <= box.right + 0.5, `${text(cell)} leaves its row`);
  }
  const panels = [...document.querySelectorAll(".split > .panel, #view, #details:not([hidden])")].map(p => p.getBoundingClientRect());
  for (let i = 0; i < panels.length; i++) for (let j = i + 1; j < panels.length; j++) assert(!overlap(panels[i], panels[j]), "two panels overlap");
});

test("runs_actions_run_their_commands", async () => {
  document.querySelector("#summary .acts button").click();
  for (let i = 0; i < 50 && !text(document.querySelector("#summary .said")).startsWith("taken"); i++) await wait(20);
  equal(text(document.querySelector("#summary .said")), "taken", "Cancel's answer");
});
