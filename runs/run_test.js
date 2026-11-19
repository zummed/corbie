// The tests of a run's page, in Chrome, on the page of a lookup whose
// stage started a review that waits on its ticket: its head, Steps, the
// drawing and the details, how the selection moves and follows, and how it
// lays out. Each test starts from the page following the newest row.
import { test, fixture, settle, until, click, copy, words, assert, equal, text, overlap } from "/test/page/testdata/test.js";
import { parts, receive } from "ui";
import { streamed } from "/test/runs/streamed.js";

const steps = () => [...document.querySelectorAll("#steps .step")];
const selected = () => document.querySelector("#steps .step.sel");
const rowOf = said => steps().find(r => text(r.querySelector(".what")).startsWith(said));
const groupOf = said => rowOf(said).closest("li");
const followButton = () => [...document.querySelectorAll("#steps .acts button")].find(b => text(b) === "Follow");
const view = () => document.getElementById("view");
const viewHead = () => words(document.querySelector("#view .head"));
const key = name => document.body.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true }));

// following brings the page back to following the newest row, with every
// group as it starts: a running child open, a send folded.
async function following() {
  await streamed();
  if (followButton()) click(followButton());
  if (groupOf("stage check started review").classList.contains("folded")) click(rowOf("stage check started review").querySelector("button.fold"));
  await until(() => !followButton() && steps().at(-1).classList.contains("sel") && view() && !view().classList.contains("pending") && viewHead().includes("as of step 2"), "the page to follow the newest row");
}

// failing makes the page's requests for an address fail, until the
// function it gives is called.
function failing(part) {
  const real = window.fetch;
  window.fetch = (url, ...rest) => (String(url).includes(part) ? Promise.resolve(new Response("the store cannot be read", { status: 500 })) : real(url, ...rest));
  return () => { window.fetch = real; };
}

// held holds the page's requests for an address until the function it
// gives is called.
function held(part) {
  const real = window.fetch;
  let open;
  const opened = new Promise(r => (open = r));
  window.fetch = async (url, ...rest) => {
    if (String(url).includes(part)) await opened;
    return real(url, ...rest);
  };
  return () => { open(); window.fetch = real; };
}

test("the_head_tabs_rows_and_path", async () => {
  await following();
  const ids = await fixture("ids");
  const head = document.getElementById("summary");
  assert(words(head).startsWith("lookup flow running"), `the head is ${words(head)}`);
  equal(head.querySelector(".acts button").dataset.cmd, "corbie run cancel " + ids.lookup, "Cancel's command");
  equal([...document.querySelectorAll(".split > .panel > .tabs a")].map(text), ["Steps", "Waiting on", "Output", "Children"], "the tabs");
  equal(text(document.querySelector("#steps .facts")), String(parts.steps.Count), "the count of rows");
  equal(steps().length, parts.steps.Count, "the rows");
  assert(groupOf("stage check started review").matches(".group:not(.folded)"), "the running child is not an open group");
  assert(groupOf("stage check started review").contains(rowOf("entered drafting")), "the child's rows are not under its row");
  equal([...document.querySelectorAll("#path a")].map(words), ["lookup", "review"], "the path");
});

test("follow_selects_the_newest_row", async () => {
  await following();
  click(steps()[0]);
  await until(() => steps()[0].classList.contains("sel") && followButton(), "the first row to be selected and Follow offered");
  click(followButton());
  await until(() => steps().at(-1).classList.contains("sel") && !followButton(), "Follow to select the newest row");
});

test("a_selection_lands_before_its_drawing", async () => {
  await following();
  const before = viewHead();
  const open = held("/at?");
  click(rowOf("stage find done"));
  await until(() => rowOf("stage find done").classList.contains("sel"), "the row to be selected while its drawing is asked for");
  assert(!document.getElementById("details").hidden, "the details are not shown");
  equal(viewHead(), before, "the drawing changed before its answer");
  assert(view().classList.contains("pending"), "the drawing does not show that its answer is on its way");
  open();
  await until(() => !view().classList.contains("pending") && viewHead().includes("as of step 2"), "the drawing of the row");
  equal(text(document.querySelector("#details .head .facts")), "stage find done", "the details");
});

test("a_child_row_draws_the_child_inside_its_card", async () => {
  await following();
  click(rowOf("entered drafting"));
  await until(() => viewHead().startsWith("review statechart") && viewHead().includes("as of step 1"), "the drawing of the review's first step");
  const svg = document.querySelector("#view svg.machine");
  const card = name => [...svg.querySelectorAll("g.state")].find(g => text(g.querySelector("text.title")) === name);
  for (const n of ["find", "check", "tell", "drafting", "waiting"]) assert(card(n), `the drawing has no ${n}`);
  const a = card("check").querySelector("rect.box").getBoundingClientRect(), b = card("drafting").querySelector("rect.box").getBoundingClientRect();
  assert(b.left >= a.left && b.right <= a.right && b.top >= a.top && b.bottom <= a.bottom, "the review is not inside the check's card");
  assert(card("drafting").classList.contains("now") && card("check").classList.contains("now"), "the review is not marked as in drafting inside a running check");
  equal([...document.querySelectorAll("#path a")].map(a => [words(a), a.classList.contains("sel")]), [["lookup", false], ["review", true]], "the path's light");
  click([...document.querySelectorAll("#details .tabs a")].find(a => text(a) === "Record"));
  await until(() => /<- start/.test(document.querySelector("#details .record")?.textContent ?? ""), "the record of the step");
});

test("the_keys_move_and_fold", async () => {
  await following();
  click(rowOf("stage check started review"));
  await until(() => selected() === rowOf("stage check started review"), "the group's row to be selected");
  key("ArrowDown");
  await until(() => text(selected().querySelector(".what")) === "entered drafting", "ArrowDown to select the next row");
  key("ArrowUp");
  await until(() => selected() === rowOf("stage check started review"), "ArrowUp to select the row above");
  key("ArrowLeft");
  await until(() => groupOf("stage check started review").classList.contains("folded"), "ArrowLeft to fold the group");
  assert(rowOf("entered drafting").offsetParent === null, "a folded group's rows are shown");
  key("ArrowRight");
  await until(() => !groupOf("stage check started review").classList.contains("folded"), "ArrowRight to open the group");
});

test("folding_a_group_selects_its_head", async () => {
  await following();
  click(rowOf("entered drafting"));
  await until(() => selected() === rowOf("entered drafting"), "the child's row to be selected");
  click(rowOf("stage check started review").querySelector("button.fold"));
  await until(() => selected() === rowOf("stage check started review"), "folding to select the group's row");
  await until(() => viewHead().startsWith("lookup flow"), "the drawing of the group's row");
});

test("a_step_that_arrives_is_followed", async () => {
  await following();
  const next = copy(parts.steps);
  const last = next.Rows.at(-1);
  next.Rows.push({ ...copy(last), Key: last.Key + "+", At: new Date(Date.parse(last.At) + 1000).toISOString(), What: [{ Text: "a step that arrived" }], Group: null, Trace: null });
  next.Count++;
  next.Follows = last.Key + "+";
  receive({ type: "part", id: "steps", data: copy(next) });
  await until(() => text(selected()?.querySelector(".what")) === "a step that arrived", "the selection to follow the new row");
  assert(selected().classList.contains("new"), "the row does not slide in");
  equal(text(document.querySelector("#steps .facts")), String(next.Count), "the count");
  receive({ type: "part", id: "steps", data: copy({ ...next, Rows: next.Rows.slice(0, -1), Count: next.Count - 1, Follows: last.Key }) });
  await until(() => !rowOf("a step that arrived"), "the row sent to go");
});

test("a_failed_answer_says_why_and_is_asked_again", async () => {
  await following();
  // A row of the review no test before asks for.
  const mend = failing("/at?");
  click(rowOf("left drafting"));
  await until(() => text(document.querySelector("#view > p.failed")).includes("the store cannot be read"), "the error to be shown");
  mend();
  click(rowOf("left drafting"));
  await until(() => viewHead().startsWith("review statechart") && viewHead().includes("as of step 2") && !document.querySelector("#view > p.failed"), "the row to be asked for again");
});

test("the_page_fits_the_window", async () => {
  await following();
  assert(document.documentElement.scrollWidth <= innerWidth && document.documentElement.scrollHeight <= innerHeight, "the page scrolls");
  const drawn = document.querySelector("#view .drawn").getBoundingClientRect();
  const svg = document.querySelector("#view svg.machine").getBoundingClientRect();
  assert(svg.width >= drawn.width - 2 && svg.height >= drawn.height - 2, `the drawing ${svg.width}x${svg.height} does not fill its panel ${drawn.width}x${drawn.height}`);
  for (const r of steps()) {
    if (r.offsetParent === null) continue;
    const box = r.getBoundingClientRect();
    for (const cell of r.querySelectorAll(":scope > span")) assert(cell.getBoundingClientRect().right <= box.right + 0.5, `${words(cell)} leaves its row`);
  }
  const panels = [...document.querySelectorAll(".split > .panel, #view, #details:not([hidden])")].map(p => p.getBoundingClientRect());
  for (let i = 0; i < panels.length; i++) for (let j = i + 1; j < panels.length; j++) assert(!overlap(panels[i], panels[j]), "two panels overlap");
});

test("cancel_shows_its_answer", async () => {
  await following();
  click(document.querySelector("#summary .acts button"));
  await until(() => text(document.querySelector("#summary .said")) === "taken", "Cancel's answer");
});
