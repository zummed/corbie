// The tests of the page of the review the lookup started: its path, and
// the Send it offers as a statechart.
import { test, settle, until, click, words, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";

test("a_child_page_paths_from_its_root", async () => {
  await settle();
  equal([...document.querySelectorAll("#path a")].map(a => [words(a), a.getAttribute("aria-current")]), [["lookup", null], ["review", "page"]], "the path");
  equal(document.querySelectorAll("#steps .step").length, parts.steps.Count, "the rows");
});

test("send_shows_its_answer_and_empties_its_fields", async () => {
  click([...document.querySelectorAll(".split > .panel > .tabs a")].find(a => text(a) === "Waiting on"));
  await until(() => document.querySelector("#waits").offsetParent !== null, "the Waiting on tab");
  const waits = text(document.querySelector("#waits .waitlist"));
  assert(waits.includes("ticket verdict") && waits.includes("timer too_long due"), `it waits on ${waits}`);
  const form = [...document.querySelectorAll("#waits form.send")].find(f => text(f.querySelector("button")) === "Send");
  equal(form.querySelector("button").dataset.cmd, `corbie run send ${parts.page.ID} <event> <data>`, "Send's command");
  form.querySelector("[name=event]").value = "verdict";
  form.querySelector("[name=data]").value = '{ "ok": true }';
  click(form.querySelector("button"));
  await until(() => text(form.querySelector(".said")) === "taken", "Send's answer");
  equal(form.querySelector("[name=event]").value, "", "Send's fields after it was taken");
});
