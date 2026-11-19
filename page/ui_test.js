// The tests of what every page shares, in Chrome: a part sent replaces
// what was drawn of it, a record keeps its element, and a line is lit as it
// changes.
import { test, mount, settle, equal, assert, text } from "/test/page/testdata/test.js";
import { html, parts, receive, lit } from "ui";
import { For } from "solid-js";

function Lines() {
  return html`<ol><For each=${() => parts.lines} keyed=${l => l.ID}>${l => html`<li ref=${lit(() => ({ key: l().ID, data: l(), status: l().Status, ended: l().Status === "done" }))}>${() => l().ID}: ${() => l().Status}</li>`}</For></ol>`;
}

test("page_stream_sends_every_part_then_what_changed", async () => {
  const box = mount(Lines, { lines: [{ ID: "a", Status: "running" }, { ID: "b", Status: "running" }] });
  const first = box.querySelector("li");
  receive({ type: "part", id: "lines", data: [{ ID: "a", Status: "done" }, { ID: "b", Status: "running" }, { ID: "c", Status: "running" }] });
  await settle();
  equal([...box.querySelectorAll("li")].map(text), ["a: done", "b: running", "c: running"], "the lines");
  assert(box.querySelector("li") === first, "a record that stayed lost its element");
  equal([...box.querySelectorAll("li")].map(li => li.className), ["ended", "", "new"], "the lights");
});

test("page_store_lost_is_shown", async () => {
  receive({ type: "store", store: "lost" });
  assert(document.getElementById("live").classList.contains("lost"), "the header's dot is not rowan");
  receive({ type: "store", store: "live" });
  assert(!document.getElementById("live").classList.contains("lost"), "the header's dot stayed rowan");
});
