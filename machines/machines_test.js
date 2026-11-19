// The tests of the Machines page in Chrome, on a project open at its agent
// support.yaml: what the page draws, how it is laid out, and what a person
// does on it.
import { test, settle, wait, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";
import { focusOf, focusAt, base } from "/machines/shared.js";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

// until waits for a condition, and fails with what it says when it does not
// hold in time.
async function until(ok, what, ms = 4000) {
  const end = performance.now() + ms;
  while (!ok()) {
    if (performance.now() > end) throw new Error("waited for " + what);
    await wait(20);
  }
  await settle();
}

// click clicks a link as a person does.
const click = el => el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));

const tab = name => $$(".view nav.tabs a").find(a => text(a) === name);

test("machines_list_shows_what_each_item_is", () => {
  const rows = $$(".files .filelist > a").filter(a => !a.closest("h2"));
  const entries = parts.files.Groups.flatMap(g => g.Entries);
  const files = parts.files.Files.filter(f => !f.Path.endsWith("/"));
  equal(rows.length, entries.length + files.length, "the rows of the list");
  for (const e of entries) {
    const row = rows.find(a => a.getAttribute("href") === "/machines?" + new URLSearchParams({ file: e.Path }));
    assert(row, `${e.Path} has no row`);
    equal($("svg.g", row).getAttribute("aria-label"), e.Type || "file", `the icon of ${e.Path}`);
    equal(text($(".name", row)), base(e.Path), `the name of ${e.Path}`);
    equal($(".name", row).title, e.Location, `the location in the tooltip of ${e.Path}`);
    equal(text($("small", row)), e.First + (e.Lifetime ? " · " + e.Lifetime : ""), `what ${e.Path} is`);
  }
  equal($$(".files h2.group").map(text), parts.files.Groups.filter(g => g.Dir !== ".").map(g => g.Dir + "/"), "the directories");
  equal(text($(".files .filelist > a[aria-current]  .name")), "support.yaml", "the machine chosen");
  assert(text($(".files h2.kind")).startsWith("Files"), "the files that are no machine have no heading");
  assert($$(".files .filelist small").some(s => text(s).includes("listed in .corbieignore")), "no file is marked as listed in .corbieignore");
});

test("machines_problems_are_listed_once_at_their_file", () => {
  const problems = $$(".files .problems .problem");
  assert(problems.length > 0 && problems.length === parts.files.Problems.length, `the list shows ${problems.length} problems of ${parts.files.Problems.length}`);
  const broken = problems.find(p => text($("a.at", p)).startsWith("broken.yaml"));
  assert(broken, "the broken machine's problem is not given at its file");
  assert($("a.at", broken).getAttribute("href").includes("view=text"), "a problem does not link to its file's text");
  equal(text($(".problems h2")), `${problems.length} problem${problems.length === 1 ? "" : "s"}`, "the problems' heading");
  assert($(".files .filelist > a[href*='broken.yaml'] .bad"), "the broken machine's row has no count of its problems");
});

test("machines_a_machine_shows_its_location", () => {
  const named = $$(".view section.place .named > span").map(text);
  equal(named[named.indexOf("location") + 1], parts.view.Place.Location, "the location shown");
  assert(named.includes("lifetime") && named.includes("receives") && named.includes("sends to"), "the place lacks a fact: " + named.join(" | "));
});

test("machines_secrets_and_values_used_are_listed", () => {
  const used = $$(".view section.uses .named > span").map(text);
  assert(used.includes("KEY"), "the secret KEY is not listed: " + used.join(", "));
  assert(text($(".view section.uses p")).includes("corbie controller secret set NAME --at " + parts.view.Place.Location), "the page does not say how the secret is set");
});

test("machines_drawing_is_shown", () => {
  equal($$(".view nav.tabs a").map(text), ["Drawing", "Text", "Written out"], "the tabs");
  equal($(".view nav.tabs a[aria-current]") && text($(".view nav.tabs a[aria-current]")), "Drawing", "the tab chosen");
  const svg = $("#drawing svg.machine");
  assert(svg, "the machine is not drawn");
  for (const name of ["tell", "ask"]) assert(text(svg).includes(name), `the drawing has no card ${name}`);
});

test("machines_pages_only_read", () => {
  equal($$("main form, main input, main textarea, main select").length, 0, "the fields on the Machines page");
});

test("machines_text_shows_the_problems_found_in_it", async () => {
  click(tab("Text"));
  await until(() => parts.view.View === "text" && $(".view pre.code"), "the text");
  assert(location.search.includes("view=text"), "the address does not name the tab: " + location.search);
  equal($$(".view pre.code > .l").length, parts.view.Text.length, "the lines of the text");
  equal(text($(".view nav.tabs a[aria-current]")), "Text", "the tab chosen");
  assert(text($(".view pre.code")).includes("agent: support"), "the text is not the file");
});

test("machines_written_out_shows_where_each_value_is_from", async () => {
  click(tab("Written out"));
  await until(() => parts.view.View === "written" && $(".view pre.code.written"), "the machine written out");
  const links = $$(".view pre.code.written a");
  assert(links.length > 0, "no value links to where it is from");
  assert(links.every(a => /^\/machines\?.*view=text.*#L\d+$/.test(a.getAttribute("href"))), "a value links elsewhere than a line of a text: " + links[0].getAttribute("href"));
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

test("machines_unfolding_moves_the_drawing_in_to_the_card", () => {
  equal(focusOf("/machines?file=a.yaml", "/machines?file=a.yaml&open=talking"), "talking", "unfolding a card");
  equal(focusOf("/machines?file=a.yaml&open=talking", "/machines?file=a.yaml&open=talking&open=talking%2Fanswering"), "talking/answering", "unfolding a card inside it");
  equal(focusOf("/machines?file=a.yaml&open=talking&open=talking%2Fanswering", "/machines?file=a.yaml&open=talking"), "talking", "folding the card inside");
  equal(focusOf("/machines?file=a.yaml&open=talking&open=talking%2Fanswering", "/machines?file=a.yaml"), "", "folding the outer card");
  equal(focusOf("/machines?file=a.yaml&open=talking", "/machines?file=a.yaml&open=talking&view=text"), null, "another tab");
  equal(focusAt("/machines?file=a.yaml&open=talking&open=talking%2Fanswering"), "talking/answering", "a page opened at an address");
  equal(focusAt("/machines?file=a.yaml"), "", "a page opened at a machine");
});

test("machines_file_changed_on_disk_is_drawn_again", async () => {
  const toolkit = "description: messages\ntools:\n  notify: { description: tells, job: { machine: .kit/post.yaml, approve: true } }\n";
  const res = await fetch("/written", { method: "POST", body: new URLSearchParams({ file: "toolkits/toolkit.yaml", text: toolkit }) });
  assert(res.ok, "the file was not written: " + (await res.text()));
  const titles = () => $$("#drawing svg.machine text.title").map(text);
  await until(() => titles().includes("notify"), "the drawing of the file as it is now", 6000);
  assert(!titles().includes("tell"), "the drawing still has the tool as it was");
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
