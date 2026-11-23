// The tests of the Machines page in Chrome, on a project recorded at /bot,
// open at its agent /bot/support: what the page draws, how it is laid out,
// and what a person does on it.
import { test, wait, until, click, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";
import { focusOf, focusAt } from "/machines/shared.js";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

const tab = name => $$(".view nav.tabs a").find(a => text(a) === name);

test("machines_list_shows_what_each_item_is", () => {
  const rows = $$(".files .filelist > a");
  const trees = parts.list.Trees;
  const entries = trees.flatMap(t => t.Entries);
  equal(rows.length, entries.length, "the rows of the list");
  for (const e of entries) {
    const row = rows.find(a => a.getAttribute("href") === "/machines?" + new URLSearchParams({ at: e.Location }));
    assert(row, `${e.Location} has no row`);
    equal($("svg.g", row).getAttribute("aria-label"), e.Type, `the icon of ${e.Location}`);
    equal(text($(".name", row)), e.Name, `the name of ${e.Location}`);
    equal($(".name", row).title, e.Location, `the location in the tooltip of ${e.Location}`);
    equal(text($("small", row)), e.First + " · " + e.Lifetime, `what ${e.Location} is`);
  }
  equal($$(".files h2.group").map(text), trees.map(t => `${t.Prefix}${t.Client} · from ${t.Directory}`), "the trees");
  equal(trees.map(t => t.Prefix), ["/bot"], "the prefixes of the trees");
  equal(text($(".files .filelist > a[aria-current]  .name")), "support", "the machine chosen");
});

test("machines_a_machine_shows_its_location", () => {
  const named = $$(".view section.place .named > span").map(text);
  equal(named[named.indexOf("location") + 1], parts.view.Place.Location, "the location shown");
  assert(named.includes("lifetime") && named.includes("receives") && named.includes("sends to"), "the place lacks a fact: " + named.join(" | "));
});

test("machines_secrets_and_values_used_are_listed", () => {
  const used = $$(".view section.uses .named > span").map(text);
  equal(used, ["KEY", "secret · not set at /bot/support or above"], "the secrets and values the machine uses");
});

test("machines_drawing_is_shown", () => {
  equal($$(".view nav.tabs a").map(text), ["Drawing", "Definition"], "the tabs");
  equal($(".view nav.tabs a[aria-current]") && text($(".view nav.tabs a[aria-current]")), "Drawing", "the tab chosen");
  const svg = $("#drawing svg.machine");
  assert(svg, "the machine is not drawn");
  for (const name of ["tell", "ask"]) assert(text(svg).includes(name), `the drawing has no card ${name}`);
});

test("machines_pages_only_read", () => {
  equal($$("main form, main input, main textarea, main select").length, 0, "the fields on the Machines page");
});

test("machines_definition_shows_where_each_value_is_from", async () => {
  click(tab("Definition"));
  await until(() => parts.view.View === "definition" && $(".view pre.code"), "the machine's definition");
  assert(location.search.includes("view=definition"), "the address does not name the tab: " + location.search);
  equal($$(".view pre.code > .l").length, parts.view.Text.length, "the lines of the definition");
  equal(text($(".view nav.tabs a[aria-current]")), "Definition", "the tab chosen");
  equal(text($(".view pre.code > .l")).replace(/\s+/g, " "), "agent: support # support.yaml:1", "the first line");
  assert($$(".view pre.code .c").some(c => text(c) === "# toolkits/toolkit.yaml:3"), "no value gives the file it was included from");
  equal($$(".view pre.code a").length, 0, "the links in the definition");
  click(tab("Drawing"));
  await until(() => parts.view.View === "drawing" && $("#drawing svg.machine"), "the drawing again");
});

test("machines_drawing_unfolds_as_the_address_says", async () => {
  const link = $$("#drawing svg.machine a").find(a => (a.getAttribute("href") ?? "").includes("open="));
  assert(link, "no card of the drawing unfolds");
  const at = new URLSearchParams(link.getAttribute("href").split("?")[1]).getAll("open");
  const before = $$("#drawing svg.machine text").length;
  const camera = () => $("#drawing g.camera")?.style.transform ?? "";
  const whole = camera();
  click(link);
  await until(() => new URLSearchParams(location.search).getAll("open").length === at.length, "the address naming the card unfolded");
  await until(() => $$("#drawing svg.machine text").length > before, "the machine drawn inside the card");
  assert(JSON.stringify(parts.view.Drawing).includes('"In":'), "the scene holds no machine unfolded inside a card");
  // The camera moves in to the card unfolded: it is in view, and fills
  // more of it than the whole machine did.
  await until(() => camera() !== whole, "the camera moving in to the card", 2000);
  await wait(500);
  const view = $("#drawing").getBoundingClientRect(), card = $("#drawing g.unfolded").getBoundingClientRect();
  const middle = { x: card.left + card.width / 2, y: card.top + card.height / 2 };
  assert(middle.x > view.left && middle.x < view.right && middle.y > view.top && middle.y < view.bottom, "the card unfolded is out of view");
  history.back();
  await until(() => !location.search.includes("open="), "Back folding the card");
  await until(() => $$("#drawing svg.machine text").length === before, "the card folded again");
  await until(() => camera() === whole, "the camera moving back out to the whole machine", 2000);
});

test("machines_focus_is_the_card_unfolded_or_the_card_it_was_in", () => {
  equal(focusOf("/machines?at=/a", "/machines?at=/a&open=talking"), "talking", "unfolding a card");
  equal(focusOf("/machines?at=/a&open=talking", "/machines?at=/a&open=talking&open=talking%2Fanswering"), "talking/answering", "unfolding a card inside it");
  equal(focusOf("/machines?at=/a&open=talking&open=talking%2Fanswering", "/machines?at=/a&open=talking"), "talking", "folding the card inside");
  equal(focusOf("/machines?at=/a&open=talking&open=talking%2Fanswering", "/machines?at=/a"), "", "folding the outer card");
  equal(focusOf("/machines?at=/a&open=talking", "/machines?at=/a&open=talking&view=definition"), null, "another tab");
  equal(focusAt("/machines?at=/a&open=talking&open=talking%2Fanswering"), "talking/answering", "a page opened at an address");
  equal(focusAt("/machines?at=/a"), "", "a page opened at a machine");
});

test("machines_latest_address_asked_for_is_shown", async () => {
  // Two tabs chosen at once, the answer to the first held back so that it
  // comes last: the page shows the tab chosen last.
  const real = window.fetch;
  window.fetch = (u, o) => (String(u).includes("view=definition") ? wait(300).then(() => real(u, o)) : real(u, o));
  try {
    click(tab("Definition"));
    click(tab("Drawing"));
    await until(() => parts.view.View === "drawing" && !location.search.includes("view="), "the tab chosen last");
    await wait(600);
  } finally {
    window.fetch = real;
  }
  equal(parts.view.View, "drawing", "the tab shown once both answers came");
  assert(!location.search.includes("view="), "the address after both answers came: " + location.search);
  assert($("#drawing svg.machine"), "the drawing is not shown");
});

test("machines_a_machine_that_cannot_be_shown_says_why", async () => {
  // The UI's answer for a location with no machine, given here so that the
  // browser logs no failed request; then a request that fails outright.
  const real = window.fetch;
  const here = location.href;
  try {
    for (const [answer, says] of [
      [() => Promise.resolve(new Response("no machine is recorded at /bot/gone", { status: 404 })), "no machine is recorded at /bot/gone"],
      [() => Promise.reject(new TypeError("Failed to fetch")), "TypeError: Failed to fetch"],
    ]) {
      window.fetch = (u, o) => (String(u).startsWith("/machines/view") ? answer() : real(u, o));
      history.pushState(null, "", "/machines?at=/bot/gone");
      dispatchEvent(new PopStateEvent("popstate"));
      await until(() => text($(".view p.failed")) === says, "the failure: " + says);
      equal(parts.view.Open, "/bot/support", "the machine shown after the failure");
    }
  } finally {
    window.fetch = real;
  }
  history.pushState(null, "", here);
  dispatchEvent(new PopStateEvent("popstate"));
  await until(() => !$(".view p.failed"), "the failure gone once the machine is shown again");
});

// row gives the list's row of the machine at a location.
const row = at => $(`.files .filelist > a[href="/machines?${new URLSearchParams({ at })}"]`);

test("machines_choosing_a_machine_shows_it_in_place", async () => {
  // The list is made short, so that it scrolls, and scrolled.
  const list = $(".ide > .files");
  list.style.height = "160px";
  list.scrollTop = 60;
  const scrolled = list.scrollTop;
  assert(scrolled > 0, "the list does not scroll");
  window.samePage = true;
  // The drawing is on the page in every frame while the machine changes.
  let blank = 0, watching = true;
  const watch = () => {
    if (!watching) return;
    if (!$("#drawing svg.machine")) blank++;
    requestAnimationFrame(watch);
  };
  requestAnimationFrame(watch);
  click(row("/bot/router"));
  await until(() => parts.view.Open === "/bot/router" && $$("#drawing svg.machine text.title").map(text).includes("listen"), "the router drawn");
  watching = false;
  equal(blank, 0, "the frames with no drawing while the machine changed");
  assert(window.samePage, "choosing a machine loaded another page");
  equal(location.search, "?at=%2Fbot%2Frouter", "the address");
  equal(list.scrollTop, scrolled, "the list's scroll");
  equal(text($(".files .filelist > a[aria-current] .name")), "router", "the machine highlighted");
  equal(text($(".view > .head > span")), "/bot/router", "the machine shown");
  history.back();
  await until(() => parts.view.Open === "/bot/support" && text($(".files .filelist > a[aria-current] .name")) === "support", "Back showing the agent");
  assert(window.samePage, "Back loaded another page");
  history.forward();
  await until(() => parts.view.Open === "/bot/router", "Forward showing the router again");
  history.back();
  await until(() => parts.view.Open === "/bot/support", "Back showing the agent again");
  list.style.height = "";
});

test("machines_panels_fit_the_window_and_scroll_inside", () => {
  const ide = $(".ide");
  assert(ide.getBoundingClientRect().bottom <= innerHeight + 1, "the page is longer than the window");
  for (const panel of $$(".ide > .panel")) {
    assert(panel.getBoundingClientRect().bottom <= innerHeight + 1, "a panel is longer than the window");
    equal(getComputedStyle(panel).overflowY, "auto", "how a panel scrolls");
  }
  equal(document.scrollingElement.scrollHeight <= innerHeight + 1, true, "the window scrolls");
});

test("machines_rows_do_not_overflow", () => {
  for (const row of $$(".files .filelist > a")) {
    const box = row.getBoundingClientRect();
    for (const child of row.children) {
      const b = child.getBoundingClientRect();
      assert(b.right <= box.right + 1 && b.left >= box.left - 1, `${text(row)}: ${child.className || child.tagName} reaches out of its row`);
    }
  }
});

test("machines_handle_resizes_the_list_and_is_kept", () => {
  const handle = $(".ide > .handle");
  const holder = handle.parentElement;
  const capture = Element.prototype.setPointerCapture;
  Element.prototype.setPointerCapture = () => {};
  try {
    const left = holder.getBoundingClientRect().left;
    handle.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, button: 0, pointerId: 1, clientX: left + 300 }));
    handle.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, pointerId: 1, clientX: left + 420 }));
    handle.dispatchEvent(new PointerEvent("lostpointercapture", { bubbles: true, pointerId: 1 }));
  } finally {
    Element.prototype.setPointerCapture = capture;
  }
  equal(holder.style.getPropertyValue("--files"), "420px", "the list's width");
  equal(Math.round($(".ide > .files").getBoundingClientRect().width), 420, "the list as laid out");
  equal(localStorage.getItem("size:files"), "420px", "the width the browser keeps");
  localStorage.removeItem("size:files");
});

// The last test: it records the project again with its agent changed.
test("machines_a_submit_shows_the_machine_again", async () => {
  window.samePage = true;
  const res = await fetch("/submitted", { method: "POST" });
  assert(res.ok, "the project was not submitted: " + (await res.text()));
  const titles = () => $$("#drawing svg.machine text.title").map(text);
  await until(() => text($(".view .place .description")) === "Answers people." && !titles().includes("tell"), "the agent as it is recorded now", 6000);
  equal(text($(".files .filelist > a[aria-current] small")), "agent: support · transient", "the row of the machine shown");
  equal($(".files .filelist > a[aria-current] small").title, "Answers people.", "the description in its tooltip");
  assert(window.samePage, "the submit loaded another page");
});
