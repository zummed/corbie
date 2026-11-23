// The tests of the Toolkits page in Chrome, on a project open at the
// toolkit "messages", which three agents offer.
import { test, fixture, until, click, words, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

test("machines_toolkits_show_what_offers_each", async () => {
  const want = await fixture("toolkits");
  const rows = $$(".files .filelist > a");
  equal(rows.length, want.Toolkits.length, "the toolkits listed");
  equal(rows.map(r => text($(".name", r))), want.Toolkits.map(k => k.Name), "the toolkits' names");
  const open = parts.toolkit.Open;
  equal($$(".files h2.group").map(text), parts.list.Trees.map(t => t.Prefix), "the trees");
  equal(text($(".files .filelist > a[aria-current] .name")), open.Name, "the toolkit chosen");
  equal(text($(".view .head > span")), open.Name, "the head of the toolkit chosen");
  equal(text($(".view .body .description")), "messages", "its description");
  const offered = $$(".view table.offered tbody tr");
  equal(offered.length, open.Offered.length, "the agents that offer it");
  for (const [i, o] of open.Offered.entries()) {
    const first = $("a.start", offered[i]);
    equal(words(first), o.Name, "an agent that offers it");
    equal(first.getAttribute("href"), "/machines?" + new URLSearchParams({ at: o.At }), `the link to ${o.Name}`);
  }
  const cards = $$(".view .toolcards section.tool");
  equal(cards.map(c => text($(".tname", c))), open.Tools.map(t => t.Name), "the tool cards");
  const tell = cards.find(c => text($(".tname", c)) === "tell");
  assert(text($(".approve", tell)) === "needs approval", "a tool with approve: true does not say it needs approval");
  assert($("p.job a", tell)?.getAttribute("href").startsWith("/machines?"), "the tool's job does not link to its machine");
});

test("machines_pages_only_read", () => {
  equal($$("main form, main input, main textarea, main select").length, 0, "the fields on the Toolkits page");
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

test("machines_toolkits_choosing_a_toolkit_shows_it_in_place", async () => {
  window.samePage = true;
  const first = parts.toolkit.Open.ID;
  const other = $$(".files .filelist > a").find(a => !a.hasAttribute("aria-current"));
  const name = text($(".name", other));
  click(other);
  await until(() => parts.toolkit.Open.Name === name && text($(".files .filelist > a[aria-current] .name")) === name, "the toolkit chosen shown");
  assert(window.samePage, "choosing a toolkit loaded another page");
  equal(text($(".view .head > span")), name, "the head of the toolkit shown");
  history.back();
  await until(() => parts.toolkit.Open.ID === first, "Back showing the toolkit first chosen");
  assert(window.samePage, "Back loaded another page");
});
