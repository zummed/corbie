// The tests of the page of the review the lookup started: its path, and
// the Send it offers as a statechart.
import { test, settle, wait, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";

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

test("runs_path_begins_at_the_root", async () => {
  await settle();
  equal([...document.querySelectorAll("#path a")].map(a => [words(a), a.getAttribute("aria-current")]), [["lookup", null], ["review", "page"]], "the path");
  equal(document.querySelectorAll("#steps .step").length, parts.steps.Count, "the rows");
});

test("runs_actions_run_their_commands", async () => {
  [...document.querySelectorAll(".split > .panel > .tabs a")].find(a => text(a) === "Waiting on").click();
  await settle();
  const waits = text(document.querySelector("#waits .waitlist"));
  assert(waits.includes("ticket verdict") && waits.includes("timer too_long due"), `it waits on ${waits}`);
  const form = [...document.querySelectorAll("#waits form.send")].find(f => text(f.querySelector("button")) === "Send");
  equal(form.querySelector("button").dataset.cmd, `corbie run send ${parts.page.ID} <event> <data>`, "Send's command");
  form.querySelector("[name=event]").value = "verdict";
  form.querySelector("[name=data]").value = '{ "ok": true }';
  form.querySelector("button").click();
  for (let i = 0; i < 50 && !text(form.querySelector(".said")); i++) await wait(20);
  equal(text(form.querySelector(".said")), "taken", "Send's answer");
  equal(form.querySelector("[name=event]").value, "", "Send's fields after it was taken");
});
