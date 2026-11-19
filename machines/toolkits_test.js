// The tests of the Toolkits page in Chrome, on a project open at the
// toolkit "messages", which three agents offer.
import { test, fixture, until, words, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

test("machines_toolkits_show_what_offers_each", async () => {
  const want = await fixture("toolkits");
  const rows = $$(".files .filelist > a");
  equal(rows.length, want.Toolkits.length, "the toolkits listed");
  equal(rows.map(r => text($(".name", r))), want.Toolkits.map(k => k.Name), "the toolkits' names");
  const open = parts.toolkit.Open;
  equal(text($(".files .filelist > a[aria-current] .name")), open.Name, "the toolkit chosen");
  equal(text($(".view .head > span")), open.Name, "the head of the toolkit chosen");
  equal(text($(".view .body .description")), "messages", "its description");
  const offered = $$(".view table.offered tbody tr");
  equal(offered.length, open.Offered.length, "the agents that offer it");
  for (const [i, o] of open.Offered.entries()) {
    const first = $("a.start", offered[i]);
    equal(words(first), o.Name, "an agent that offers it");
    equal(first.getAttribute("href"), "/machines?" + new URLSearchParams({ file: o.File }), `the link to ${o.Name}`);
  }
  const cards = $$(".view .toolcards section.tool");
  equal(cards.map(c => text($(".tname", c))), open.Tools.map(t => t.Name), "the tool cards");
  const tell = cards.find(c => text($(".tname", c)) === "tell");
  assert(text($(".approve", tell)) === "needs approval", "a tool with approve: true does not say it needs approval");
  assert($("p.job a", tell)?.getAttribute("href").startsWith("/machines?"), "the tool's job does not link to its machine");
});

test("machines_toolkits_show_the_problems_in_their_file", () => {
  equal($$(".files .problems .problem").length, parts.list.Problems.length, "the problems above the list");
  equal($$(".view nav.tabs a").map(text), ["Drawing", "Text"], "the tabs");
  equal($$(".view nav.tabs a")[1].getAttribute("href"), "/toolkits?" + new URLSearchParams({ toolkit: parts.toolkit.Open.ID, view: "text" }), "the Text tab's address");
});

test("machines_pages_only_read", () => {
  equal($$("main form, main input, main textarea, main select").length, 0, "the fields on the Toolkits page");
});

test("machines_toolkits_page_follows_the_directory", async () => {
  const own = "agent: own\nmodel: { name: claude-sonnet-5-5, secret: KEY }\nprompt: hello\ntoolkits:\n  messages:\n    description: other messages, as now\n    tools:\n      read: { description: reads, job: { script: ./own.js } }\n";
  const res = await fetch("/written", { method: "POST", body: new URLSearchParams({ file: "own.yaml", text: own }) });
  assert(res.ok, "the file was not written");
  await until(() => $$(".files .filelist > a small").some(s => text(s).includes("other messages, as now")), "the list as the directory is now", 6000);
});

test("machines_toolkits_panels_fit_the_window_and_scroll_inside", () => {
  for (const panel of $$(".ide > .panel")) {
    assert(panel.getBoundingClientRect().bottom <= innerHeight + 1, "a panel is longer than the window");
    equal(getComputedStyle(panel).overflowY, "auto", "how a panel scrolls");
  }
  for (const row of $$(".files .filelist > a")) {
    const box = row.getBoundingClientRect();
    for (const child of row.children) assert(child.getBoundingClientRect().right <= box.right + 1, `${text(row)} reaches out of its row`);
  }
  for (const card of $$(".view .toolcards section.tool")) {
    assert(card.scrollWidth <= card.clientWidth + 1, `the card of ${text($(".tname", card))} is wider than it is drawn`);
  }
});
