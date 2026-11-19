// The tests of the runs of testdata/trouble, each on its run's page, in
// Chrome: why the run is where it is, the row it opens on, what a job
// logged and how long it took, an event with no rule, and how Steps, the
// drawing and replay point at one another.
import { test, fixture, settle, until, click, copy, words, assert, equal, text } from "/test/page/testdata/test.js";
import { parts, receive } from "ui";
import { streamed } from "/test/runs/streamed.js";

const which = await fixture("which");
const steps = () => [...document.querySelectorAll("#steps .step")];
const selected = () => document.querySelector("#steps .step.sel");
const rowOf = said => steps().find(r => text(r.querySelector(".what")).startsWith(said));
const why = () => words(document.getElementById("why"));
const card = path => document.querySelector(`#view .machine [data-path="${CSS.escape(path)}"]`);
const tab = (bar, name) => [...document.querySelectorAll(bar + " .tabs a")].find(a => text(a) === name);
const play = () => document.querySelector("#steps [data-play]");
const key = name => document.body.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true }));

if (which === "fails") {
  test("runs_why_says_where_a_run_failed", async () => {
    await streamed();
    assert(why().startsWith("failed (error) at stage check"), `the line says ${why()}`);
    assert(why().includes("no such person: ada"), `the line does not give the error: ${why()}`);
  });

  test("runs_a_failed_run_opens_on_its_deepest_failure", async () => {
    await until(() => selected(), "a row to be selected");
    equal(text(selected().querySelector(".what")).split(" · ")[0], "stage check failed", "the row selected");
    await until(() => words(document.querySelector("#view .head")).includes("as of step"), "the drawing");
  });

  test("runs_why_selects_its_row", async () => {
    click(rowOf("started"));
    await until(() => selected() === rowOf("started"), "the first row to be selected");
    click(document.querySelector("#why button"));
    await until(() => selected() === rowOf("stage check failed"), "the line to select its row");
  });

  test("runs_a_row_that_ends_a_job_says_how_long_it_took", () => {
    const took = rowOf("stage find done").querySelector(".took");
    assert(took && /^· \d+(\.\d)?s$/.test(text(took)), `the row says ${text(rowOf("stage find done"))}`);
  });

  test("runs_a_row_lights_its_cards", async () => {
    rowOf("stage check failed").dispatchEvent(new MouseEvent("mouseenter"));
    await settle();
    assert(card("check")?.classList.contains("lit"), "the stage's card is not lit");
    assert(!card("find")?.classList.contains("lit"), "another card is lit");
    rowOf("stage check failed").dispatchEvent(new MouseEvent("mouseleave"));
    await settle();
    assert(!card("check").classList.contains("lit"), "the card stays lit");
  });

  test("runs_a_card_selects_its_newest_row", async () => {
    click(card("find"));
    await until(() => selected() === rowOf("stage find done"), "the card to select its row");
  });

  test("runs_replay_steps_through_the_rows", async () => {
    click(rowOf("started"));
    await until(() => selected() === rowOf("started"), "the first row to be selected");
    const rows = steps();
    click(play());
    await settle();
    equal(text(play()), "Faster", "the button while it plays");
    await until(() => rows.indexOf(selected()) >= 1, "the selection to move on", 2500);
    key(" ");
    await settle();
    equal(text(play()), "Play", "the button once Space stopped it");
    const at = rows.indexOf(selected());
    await new Promise(r => setTimeout(r, 1300));
    equal(rows.indexOf(selected()), at, "the row selected after it stopped");
    click(play());
    await settle();
    click(play());
    await settle();
    equal(text(play()), "Stop", "the button while it plays fast");
    await until(() => selected() === rows.at(-1) && text(play()) === "Play", "replay to stop at the last row", 4000);
  });
}

if (which === "denied") {
  test("runs_why_gives_what_refused_a_job_and_the_fix", async () => {
    await streamed();
    assert(why().startsWith(`failed (${parts.summary.Failure}) at stage read`), `the line says ${why()}`);
    const facts = parts.summary.Why.Facts.map(f => f.Name);
    assert(facts.includes("refused") && facts.includes("fix"), `the line gives ${facts}`);
  });
}

if (which === "slow-child") {
  test("runs_why_names_the_child_that_failed", async () => {
    await streamed();
    assert(why().startsWith("failed") && why().includes("in statechart stuck"), `the line says ${why()}`);
    await until(() => selected(), "a row to be selected");
    equal(selected().closest("li.group")?.querySelector(".step .what") && text(selected()).includes("stuck"), true, "the child's row selected");
  });
}

if (which === "stuck") {
  test("runs_why_says_what_a_run_waits_on", async () => {
    await streamed();
    equal(why(), "waiting on ticket verdict", "the line");
  });

  test("runs_waiting_on_gives_the_runs_timeout", () => {
    click(tab(".split > .panel", "Waiting on"));
    assert(words(document.getElementById("waits")).includes("the run times out"), "Waiting on gives no timeout");
  });

  test("runs_waiting_on_gives_a_jobs_facts_and_logs", async () => {
    const began = new Date(Date.now() - 4 * 60 * 1000 - 5000).toISOString();
    const waits = copy(parts.waits);
    waits.Waits = [{ Text: "job fetch", Ticket: "", Value: "", Run: null, Job: { Name: "fetch", Runner: "runner-a", Attempt: 2, Began: began, Sent: "20:29", Due: "20:34", Again: "", Restarts: 0, Logs: [{ Time: "20:30:01", Name: "log", Text: "fetching the page" }] } }];
    receive({ type: "part", id: "waits", data: waits });
    await settle();
    const wait = words(document.querySelector("#waits .wait"));
    assert(wait.includes("job fetch on runner-a · attempt 2 · sent 20:29, 4m ago · due 20:34"), `the wait says ${wait}`);
    assert(wait.includes("20:30:01 fetching the page"), `the wait gives no log: ${wait}`);
  });
}

if (which === "unruled") {
  test("runs_an_event_with_no_rule_says_so", async () => {
    await streamed();
    const row = steps().find(r => text(r).includes("poke from client:"));
    assert(row, "no row of the event");
    assert(text(row.querySelector(".what")).endsWith("· no rule in resting"), `the row says ${text(row)}`);
  });
}

if (which === "logs") {
  test("runs_a_jobs_row_shows_what_it_logged", async () => {
    await streamed();
    click(rowOf("stage count done"));
    click(tab("#details", "Output"));
    await until(() => document.querySelectorAll("#details .logs li").length >= 10, "the job's logs");
    const lines = [...document.querySelectorAll("#details .logs li")].map(words);
    assert(lines[0].endsWith("counted 0"), `the first line is ${lines[0]}`);
    assert(lines.some(l => l.includes("progress 0 of 5")), `no progress in ${lines}`);
  });
}
