// The tests of the runs of testdata/trouble, each on its run's page, in
// Chrome: why the run is where it is, the row it opens on, what a job
// logged and how long it took, an event with no rule, and how Steps, the
// drawing and replay point at one another.
import { test, fixture, settle, until, click, copy, words, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";
import { streamed } from "/test/runs/streamed.js";

const which = await fixture("which");
const steps = () => [...document.querySelectorAll("#steps .step")];
const selected = () => document.querySelector("#steps .step.sel");
const rowOf = said => steps().find(r => text(r.querySelector(".what")).startsWith(said));
const why = () => words(document.getElementById("why"));
// facts gives the facts under the line, by their names.
const facts = () => Object.fromEntries([...document.querySelectorAll("dl.why-facts dt")].map(dt => [text(dt), dt.nextElementSibling]));
const card = path => document.querySelector(`#view .machine [data-path="${CSS.escape(path)}"]`);
const tab = (bar, name) => [...document.querySelectorAll(bar + " .tabs a")].find(a => text(a) === name);
const play = () => document.querySelector("#steps [data-play]");
const key = name => document.body.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true }));

if (which === "fails") {
  test("runs_why_says_where_a_run_failed", async () => {
    await streamed();
    equal(why(), "failed (error) at stage check", "the line");
    equal(Object.keys(facts()), ["error"], "the facts under it");
    assert(text(facts().error).startsWith("no such person: ada"), `the error is ${text(facts().error)}`);
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

  test("runs_replay_from_the_last_row_starts_at_the_first", async () => {
    click(steps().at(-1));
    await until(() => selected() === steps().at(-1), "the last row to be selected");
    click(play());
    await until(() => selected() === steps()[0] || steps().indexOf(selected()) === 1, "replay to start at the first row", 2500);
    equal(text(play()), "Faster", "the button while it plays");
    key(" ");
    await settle();
  });
}

if (which === "denied") {
  test("runs_why_gives_what_refused_a_job_and_the_fix", async () => {
    await streamed();
    equal(why(), "failed (denied) at stage read", "the line");
    equal(Object.keys(facts()), ["refused", "by", "fix"], "the facts under it, each once");
    const fix = facts().fix;
    assert(text(fix).startsWith("add the line fs.read:/etc/hostname to the allow list"), `the fix is ${text(fix)}`);
    assert(fix.scrollWidth <= fix.clientWidth + 1 && fix.getClientRects().length > 0, "the fix is cut off");
  });
}

if (which === "slow-child") {
  test("runs_why_names_the_child_that_failed", async () => {
    await streamed();
    equal(why(), "failed (error) at stage wait", "the line");
    equal(Object.keys(facts()), ["error", "timeout", "child"], "the facts under it");
    equal(text(facts().timeout), "1s", "the stage's timeout");
    equal(text(facts().child), "statechart stuck failed (timeout)", "the child");
    await until(() => selected(), "a row to be selected");
    equal(text(selected().querySelector(".what")).split(" · ")[0], "stage wait failed", "the row selected");
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
}

if (which === "slow") {
  test("runs_waiting_on_gives_a_jobs_facts_and_logs", async () => {
    await streamed();
    assert(/^waiting \d+s on job work on runner$/.test(why()), `the line says ${why()}`);
    click(tab(".split > .panel", "Waiting on"));
    const wait = () => words(document.querySelector("#waits .wait"));
    assert(/^job work on runner · attempt 2 · sent \S+, \d+s ago · due \S+ · restart 1/.test(wait()), `the wait says ${wait()}`);
    assert(wait().includes("working 0"), `the wait gives no log: ${wait()}`);
    const logged = () => document.querySelectorAll("#waits .wait .logs li").length;
    const was = logged();
    await until(() => logged() > was, "the job's logs to grow", 4000);
  });
}

if (which === "unruled") {
  test("runs_why_says_a_statechart_waits_on_an_event", async () => {
    await streamed();
    equal(why(), "waiting on an event in resting", "the line");
  });

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
