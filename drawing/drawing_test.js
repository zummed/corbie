// The tests of what Drawing draws, in Chrome: the SVG it makes of the graphs
// drawing_test.go reads in Go, the camera it shows them through, and what
// the pointer lights.
import { test, fixture, mount, settle, wait, assert, equal, text } from "/test/page/testdata/test.js";
import { html, parts, receive } from "ui";
import { Drawing } from "/drawing/drawing.js";

// draw mounts a drawing of a fixture in a box of a size, and gives its
// viewport. focus is the path of a card for the camera to frame.
async function draw(name, size = { width: 900, height: 700 }, focus) {
  const box = mount(() => html`<div class="drawn" style="flex: 1"><${Drawing} scene=${() => parts.scene} focus=${() => parts.focus} /></div>`, { scene: await fixture(name), focus }, size);
  await Promise.all(["600 13px 'IBM Plex Sans'", "500 10.5px 'IBM Plex Sans'", "400 10.5px 'IBM Plex Mono'", "500 12px 'IBM Plex Mono'"].map(f => document.fonts.load(f)));
  await settle(3);
  const v = box.querySelector(".viewport");
  assert(v, `${name} is not drawn`);
  return v;
}

// show puts another graph in place of the one drawn, as a page is sent it.
async function show(name) {
  receive({ type: "part", id: "scene", data: await fixture(name) });
  await settle(3);
}

// card gives the group of a card by its name, as its title writes it.
function card(v, name) {
  const t = [...v.querySelectorAll("text.title")].find(t => t.textContent === name);
  assert(t, `${name} is not drawn as a card`);
  return t.closest("g");
}

// classes gives a part's classes as the graph marks it, without what the
// pointer lights.
const classes = el => [...el.classList].filter(c => c !== "lit").join(" ");

const all = (v, sel) => [...v.querySelectorAll(sel)];
const texts = (v, sel) => all(v, sel).map(t => t.textContent);

// camera gives where the camera is: how far it moves the drawing, and how
// much it scales it.
function camera(v) {
  const m = /translate\(([-0-9.]+)px, ([-0-9.]+)px\) scale\(([0-9.]+)\)/.exec(v.querySelector("g.camera").getAttribute("style"));
  assert(m, "the camera has no transform");
  return { x: +m[1], y: +m[2], k: +m[3] };
}

// seen gives where a card's box is in the viewport, as the camera shows it.
function seen(v, g) {
  const r = g.querySelector("rect.box").getBoundingClientRect(), o = v.getBoundingClientRect();
  return { x: r.left - o.left, y: r.top - o.top, w: r.width, h: r.height };
}

const within = (b, v) => b.x >= -0.5 && b.y >= -0.5 && b.x + b.w <= v.clientWidth + 0.5 && b.y + b.h <= v.clientHeight + 0.5;

test("drawing_statechart_arrows_name_event_condition_and_timer", async () => {
  let v = await draw("review");
  const chips = texts(v, "text.on").map(t => t.replace(/\n.*/s, ""));
  for (const label of ["done", "failed", "verdict, when eve…", "too_long, 48h"]) assert(all(v, "text.on").some(t => t.firstChild.textContent === label), `no arrow is labelled ${label}: ${chips}`);
  assert(texts(v, "text.note").includes("verdict ↩ drafting"), "the transition back to an earlier state is not written in its card");
  assert(all(v, "text.on title").some(t => t.textContent === "verdict, when event.data.ok"), "a chip's whole label is not its tooltip");
  equal(all(v, "path.edge").length, 5, "the arrows: four forward transitions and the start");
  equal(all(v, "path.edge.timer").length, 1, "the dashed arrows: the timer's");
  assert(texts(v, "text.sub").includes("ticket verdict") && texts(v, "text.sub").includes("timer too_long"), "the ticket and timer of waiting are not written in it");
  equal(classes(card(v, "gave_up")), "state final failed", "gave_up, which ends failed");
  equal(all(v, "rect.tile").length, 1, "the icons: only drafting's job is a script");

  v = await draw("testdata/levels.yaml");
  const notes = texts(v, "text.note");
  assert(notes.includes("again ↩ first") && !notes.some(n => n.endsWith("↩ report") || n.endsWith("↩ deep")), `the rules back are not the rules to a row above: ${notes}`);
  for (const label of ["ok", "done", "out", "quick", "bad"]) assert(all(v, "text.on").some(t => t.firstChild.textContent === label), `no arrow is labelled ${label}`);
});

test("drawing_run_is_marked_on_the_drawing", async () => {
  let v = await draw("review-running");
  equal(["waiting", "drafting", "approved"].map(n => classes(card(v, n))), ["state now", "state done", "state never final"], "the states");
  v = await draw("review-ended");
  equal(classes(card(v, "approved")), "state done final", "the state a run ended in");
  v = await draw("flow-running");
  equal(["a", "b", "c"].map(n => classes(card(v, n))), ["state stage done", "state stage now", "state stage never"], "the stages");
  v = await draw("librarian-running");
  equal([classes(card(v, "shelf")), classes(card(v, "helper"))], ["state tool now", "state tool never"], "the tools");
  assert(texts(v, "text.tag").includes("turn 2"), "the agent's turn is not written beside its model");
  assert(texts(v, "text.sub").includes("machine: ./two.yaml"), "the tool whose job is a machine does not say which");
  v = await draw("review-passed");
  equal(classes(card(v, "drafting")), "state done", "the state the run started in and left in its first step");
  v = await draw("helper-asking");
  equal(["tell", "look", "idle"].map(n => classes(card(v, n))), ["state tool held", "state tool done", "state tool never"], "the tools of an agent waiting on its ask");
  const ask = card(v, "ask");
  equal([classes(ask), text(ask.querySelector("text.sub"))], ["state ask now", "flow: approval"], "the ask");
  equal(all(v, 'a[href="/run/a-client/2"]').length, 1, "the links to the run the ask started");
  v = await draw("flow-cancelled");
  equal(classes(card(v, "b")), "state stage cancelled", "the stage running when its run was cancelled");
  v = await draw("librarian-failed");
  equal(classes(card(v, "claude-sonnet-5-5")), "state model failed", "the model of an agent whose turn failed");
});

test("drawing_transition_just_taken_is_marked", async () => {
  let v = await draw("review-running");
  const taken = all(v, "path.edge.taken");
  equal(taken.length, 1, "the arrows marked as taken");
  equal(taken[0].getAttribute("pathLength"), "1", "the taken arrow's length, by which it is drawn as it is taken");
  const chips = all(v, ".chip.taken");
  equal([chips.length, chips[0]?.nextElementSibling.firstChild.textContent], [1, "done"], "the chip marked as taken");
  v = await draw("review-back");
  assert(all(v, "text.note.taken").length === 1 && all(v, "path.edge.taken").length === 0, "the transition back is not marked in its card alone");
});

test("drawing_statechart_send_is_a_card", async () => {
  const v = await draw("listen");
  assert(all(v, "path.edge.send").length === 1, "the send's arrow is not dashed");
  assert(all(v, ".chip.send").some(c => c.nextElementSibling.firstChild.textContent === "message"), "the send's arrow is not labelled with the event");
  assert(classes(card(v, "chat#a")).includes("sends"), "the send's card is not marked as one");
  assert(texts(v, "text.note").includes("again ↻") && !all(v, "text.on").some(t => t.firstChild.textContent === "again"), "a rule back to the same state is not written in its card alone");
});

test("drawing_agent_is_a_loop_of_model_and_tools", async () => {
  let v = await draw("helper");
  for (const want of ["messages", "tools"]) assert(texts(v, ".holder text.name").includes(want), `no holder is named ${want}`);
  assert(texts(v, "text.on").includes("7 turns"), "the chip into the tools does not give the turns");
  assert(texts(v, "text.sub").includes("approve: true") && texts(v, "text.sub").includes("machine: ./approval.yaml"), "the tools' lines are not written");
  assert(text(card(v, "tell").querySelector("title")) === "tells someone", "a tool's description is not its tooltip");
  equal(all(v, "path.edge").length, 2, "the arrows: model to tools, and tools to reply");
  v = await draw("helper-corbie-tool");
  assert(!v.textContent.includes("blob_read"), "a tool of Corbie's own is drawn, and no definition wrote it");
  v = await draw("helper-included");
  const titles = all(v, ".holder").map(h => `${h.querySelector("text.name").textContent}: ${h.querySelector("title")?.textContent ?? ""}`);
  for (const want of ["messages: messages.yaml", "notes: notes.yaml"]) assert(titles.includes(want), `a toolkit's tooltip does not give its file: ${titles}`);
});

test("drawing_machine_unfolds_in_its_card", async () => {
  let v = await draw("nested");
  const hrefs = all(v, "a").map(a => a.getAttribute("href"));
  equal(hrefs.sort(), ["/go?to=here", "/go?to=outer"], "the links of cards whose job names a machine");
  assert(!texts(v, "text.title").includes("inner"), "a card that is not unfolded shows what is inside it");
  assert(texts(v, "text.sub").includes("flow: written") && texts(v, "text.sub").includes("flow: b"), "a card whose job names a machine does not say its type and name");
  v = await draw("nested-outer");
  assert(texts(v, "text.title").includes("inner") && !texts(v, "text.title").includes("deep"), "one card unfolded does not show one level");
  assert(all(v, 'a[href="/go?to=outer/inner"]').length === 1, "a card inside an unfolded card has no link by its path");
  v = await draw("nested-deep");
  for (const want of ["deep", "there"]) assert(texts(v, "text.title").includes(want), `${want} is not drawn when its cards are unfolded`);
  v = await draw("nested-inner-alone");
  assert(!texts(v, "text.title").includes("deep"), "a card inside a folded card is drawn");
  v = await draw("nested-run");
  equal(all(v, "a").map(a => a.getAttribute("href")), ["/run/a-client/1"], "the links on a run's page");
  assert(texts(v, "text.sub").includes("flow: b"), "a card whose job started a run does not say what that run is");
  v = await draw("nested-machines");
  assert(texts(v, "text.sub").includes("flow: b") && all(v, "a").length === 0, "a card whose job started no run does not say its machine");
});

test("drawing_unused_toolkit_is_folded", async () => {
  let v = await draw("toolkits-running");
  const holder = name => all(v, ".holder").find(h => h.querySelector("text.name").textContent === name);
  equal(["bins", "servers", "home", "net"].map(n => classes(holder(n))), ["state holder toolkit folded", "state holder toolkit folded", "state holder toolkit", "state holder toolkit"], "the toolkits");
  for (const hidden of ["days", "put", "disk"]) assert(!texts(v, "text.title").includes(hidden), `a tool of a folded toolkit is drawn as a card: ${hidden}`);
  assert(text(holder("bins").querySelector("text.sub")) === "2 tools" && text(holder("bins").querySelector("title")) === "days, put", "a folded toolkit does not give how many tools it has and name them");
  v = await draw("toolkits");
  assert(all(v, ".folded").length === 0 && texts(v, "text.title").includes("disk"), "a drawing of no run folds a toolkit");
});

test("drawing_call_and_send_jobs_are_cards", async () => {
  let v = await draw("trip");
  for (const want of ["call: services/weather", "send: set", "to: devices/boiler", "to: rooms/hall#${input.k}"]) assert(texts(v, "text.sub").includes(want), `the drawing does not hold ${want}`);
  equal(all(v, "a").length, 0, "the links of a call that started no run, and a send");
  v = await draw("trip-called");
  equal(all(v, "a").map(a => a.getAttribute("href")), ["/run/lab/7"], "the links of a call that started a run");
  assert(texts(v, "text.sub").includes("flow: weather") && texts(v, "text.sub").includes("call: services/weather"), "a call's card does not say the run it called");
});

test("drawing_conditions_approvals_and_calls_are_marked", async () => {
  let v = await draw("conditions");
  const holder = name => all(v, ".holder").find(h => h.querySelector("text.name").textContent === name);
  equal([classes(holder("orders")), text(holder("orders").querySelector("title"))], ["state holder toolkit when", "orders.yaml when: input.asker.scopes.includes('orders')"], "a toolkit with when:");
  equal(classes(holder("notes")), "state holder toolkit", "a toolkit with no when:");
  equal([classes(card(v, "look")), card(v, "look").querySelector("title").textContent], ["state tool when", "looks\nwhen: input.look"], "a tool with when:");
  equal(classes(card(v, "jot")), "state tool", "a tool with no when:");
  v = await draw("conditions-folded");
  const folded = all(v, ".holder.folded.when");
  assert(folded.length === 1 && folded[0].querySelector("title").textContent.startsWith("read\nwhen: input.asker"), "a folded toolkit with when: is not dashed with its condition");
  v = await draw("deploy-running");
  equal(texts(v, "text.sub").filter(t => t === "approve: true").length, 2, "the jobs that say approve: true");
  equal([classes(card(v, "ship")), classes(card(v, "pipeline"))], ["state stage held", "state stage now"], "the stages held and going");
  v = await draw("deploys-held");
  assert(classes(card(v, "shipping")) === "state held" && texts(v, "text.sub").includes("approve: true"), "a state whose job is held is not marked held");
});

test("drawing_decision_is_its_model_and_its_questions", async () => {
  const v = await draw("triage");
  for (const want of ["jev-latest", "answers"]) assert(texts(v, "text.title").includes(want), `the decision does not say ${want}`);
  for (const want of ["urgent", "team"]) assert(v.textContent.includes(want), `the decision does not name the question ${want}`);
});

// Every element of a drawing is SVG, and what a card, a chip and a holder
// write fits inside it in the real fonts.
test("drawing_text_fits_its_card", async () => {
  const faults = [];
  for (const name of [...(await fixture("examples")), "helper", "toolkits-running", "conditions", "trip-called", "nested-deep", "triage", "deploy-running"]) {
    const v = await draw(name);
    const svg = v.querySelector("svg");
    for (const el of svg.querySelectorAll("*")) if (el.namespaceURI !== "http://www.w3.org/2000/svg") faults.push(`${name}: <${el.localName}> is not SVG`);
    for (const g of svg.querySelectorAll("g.state:not(.holder)")) {
      const box = g.querySelector(":scope > rect.box").getBBox();
      for (const t of g.querySelectorAll(":scope > text")) {
        const b = t.getBBox();
        if (b.x < box.x || b.x + b.width > box.x + box.width + 0.5) faults.push(`${name}: "${t.textContent}" overruns its card`);
      }
    }
    for (const c of svg.querySelectorAll("g.label")) {
      const r = c.querySelector("rect").getBBox(), t = c.querySelector("text").getBBox();
      if (t.x < r.x - 0.5 || t.x + t.width > r.x + r.width + 0.5) faults.push(`${name}: the chip "${c.querySelector("text").firstChild.textContent}" overruns its pill: ${t.width.toFixed(1)} in ${r.width.toFixed(1)}`);
    }
    for (const h of svg.querySelectorAll("g.holder")) {
      const r = h.querySelector("rect").getBBox(), t = h.querySelector("text.name").getBBox();
      if (t.x + t.width > r.x + r.width) faults.push(`${name}: the name "${h.querySelector("text.name").textContent}" overruns its holder`);
    }
  }
  equal(faults, [], "the faults");
});

// No two cards of a drawing overlap, but for a card drawn inside another.
test("drawing_cards_do_not_overlap", async () => {
  const faults = [];
  for (const name of [...(await fixture("examples")), "helper", "toolkits-running", "trip", "nested-deep", "lookup-in-child"]) {
    const v = await draw(name);
    const boxes = all(v, "g.state:not(.holder) > rect.box").map(r => ({ r, b: r.getBoundingClientRect() }));
    boxes.forEach((a, i) => {
      for (const o of boxes.slice(i + 1)) {
        if (a.r.closest("g[transform]") !== o.r.closest("g[transform]")) continue;
        const x = Math.min(a.b.right, o.b.right) - Math.max(a.b.left, o.b.left), y = Math.min(a.b.bottom, o.b.bottom) - Math.max(a.b.top, o.b.top);
        if (x > 1 && y > 1) faults.push(`${name}: two cards overlap`);
      }
    });
  }
  equal(faults, [], "the faults");
});

// The camera frames what the drawing is about: the whole of a machine that
// fits, from its top, at its size and never larger.
test("drawing_camera_fits_the_whole_machine", async () => {
  const v = await draw("chain");
  const c = camera(v);
  equal(c.k, 1, "the scale of a machine that fits");
  const svg = v.querySelector("svg"), drawnBox = svg.querySelector("g.camera").getBoundingClientRect(), o = v.getBoundingClientRect();
  assert(drawnBox.top - o.top < 60 && within({ x: drawnBox.left - o.left, y: drawnBox.top - o.top, w: drawnBox.width, h: drawnBox.height }, v), "the machine is not drawn whole from its top");
  const big = await draw("big-chart", { width: 700, height: 500 });
  assert(camera(big).k < 1, "a machine larger than its view is not scaled down to fit");
});

// On a run's page the camera frames the cards the run marks, and when the
// run goes into a child drawn inside its card, the camera moves into that
// card, which frames the child's machine.
test("drawing_camera_follows_the_run", async () => {
  const v = await draw("lookup-running", { width: 520, height: 360 });
  const before = camera(v);
  assert(within(seen(v, card(v, "find")), v), "the card the run marks is not in view");
  await show("lookup-in-child");
  const after = camera(v);
  const inner = card(v, "b");
  assert(classes(inner) === "state stage now", "the child's running stage is not drawn inside its card");
  assert(within(seen(v, inner), v), "the stage the child runs is not in view");
  const check = card(v, "check").querySelector("rect.box").getBBox();
  const shown = seen(v, card(v, "check"));
  assert(after.k >= before.k * 0.999 && shown.w <= v.clientWidth, "the camera did not frame the child's card");
  assert(Math.abs(shown.x + shown.w / 2 - v.clientWidth / 2) < 2, `the child's card is not in the middle of the view: ${JSON.stringify(shown)} of ${v.clientWidth}`);
  assert(check.width > 232, "the child's card does not hold its drawing");
  // The camera glides: its transform has a transition.
  assert(parseFloat(getComputedStyle(v.querySelector("g.camera")).transitionDuration) > 0, "the camera does not glide");
});

// A page can focus a card by its path, and the camera frames it.
test("drawing_camera_frames_the_card_focused", async () => {
  const v = await draw("nested-deep", { width: 600, height: 400 }, "outer/inner");
  const inner = seen(v, card(v, "inner"));
  assert(within(inner, v), "the card focused is not in view");
  assert(Math.abs(inner.x + inner.w / 2 - v.clientWidth / 2) < 2, "the card focused is not in the middle of the view");
});

// A wheel pans, ctrl-wheel zooms round the pointer, and both stop the camera
// following, which Follow gives back; + - and 0 zoom and fit.
test("drawing_camera_is_moved_by_hand", async () => {
  const v = await draw("big-chart", { width: 700, height: 500 });
  const framed = camera(v);
  assert(!v.querySelector(".zoom").textContent.includes("Follow"), "Follow is offered while the camera follows");
  v.dispatchEvent(new WheelEvent("wheel", { deltaX: 30, deltaY: 50, bubbles: true, cancelable: true }));
  await settle();
  let c = camera(v);
  equal([Math.round(framed.x - c.x), Math.round(framed.y - c.y), c.k], [30, 50, framed.k], "the pan of a wheel");
  const r = v.getBoundingClientRect();
  v.dispatchEvent(new WheelEvent("wheel", { deltaY: -100, ctrlKey: true, clientX: r.left + 100, clientY: r.top + 80, bubbles: true, cancelable: true }));
  await settle();
  const z = camera(v);
  assert(z.k > c.k, "ctrl-wheel did not zoom in");
  assert(Math.abs((100 - z.x) / z.k - (100 - c.x) / c.k) < 0.01, "the zoom is not round the pointer");
  const follow = [...v.querySelectorAll(".zoom button")].find(b => b.textContent === "Follow");
  assert(follow, "Follow is not offered once the camera was moved by hand");
  follow.click();
  await settle();
  equal(camera(v), framed, "the camera after Follow");
  v.focus();
  v.dispatchEvent(new KeyboardEvent("keydown", { key: "+", bubbles: true }));
  await settle();
  assert(camera(v).k > framed.k, "+ did not zoom in");
  v.dispatchEvent(new KeyboardEvent("keydown", { key: "0", bubbles: true }));
  await settle();
  const fit = camera(v);
  const whole = v.querySelector("g.camera").getBBox();
  assert(fit.k * whole.height <= v.clientHeight + 1 && fit.k * whole.width <= v.clientWidth + 1, "0 does not fit the whole machine");
});

// A drag pans, and does not follow the link it began on.
test("drawing_camera_is_dragged", async () => {
  const v = await draw("nested", { width: 500, height: 400 });
  const link = v.querySelector("a");
  const was = camera(v);
  let followed = false;
  link.addEventListener("click", e => { if (!e.defaultPrevented) followed = true; e.preventDefault(); });
  const at = link.getBoundingClientRect();
  const fire = (type, x, y) => link.dispatchEvent(new PointerEvent(type, { clientX: x, clientY: y, button: 0, pointerId: 1, bubbles: true, cancelable: true }));
  fire("pointerdown", at.left + 5, at.top + 5);
  fire("pointermove", at.left + 45, at.top + 25);
  fire("pointerup", at.left + 45, at.top + 25);
  link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
  await settle();
  const c = camera(v);
  equal([Math.round(c.x - was.x), Math.round(c.y - was.y)], [40, 20], "the pan of a drag");
  assert(!followed, "a drag followed the link it began on");
});

// The pointer on a card lights its arrows and their chips; on an arrow or a
// chip it lights the two cards it joins.
test("drawing_pointer_lights_what_it_joins", async () => {
  const v = await draw("review");
  const over = el => { el.dispatchEvent(new PointerEvent("pointerover", { bubbles: true })); };
  over(card(v, "waiting").querySelector("rect.box"));
  await settle();
  const lit = all(v, ".lit");
  assert(lit.some(e => e.matches("path.edge")) && lit.some(e => e.matches("g.label")), "a card does not light its arrows and their chips");
  for (const e of all(v, "path.edge.lit")) assert(e.dataset.from.endsWith("state waiting") || e.dataset.to.endsWith("state waiting"), "an arrow that does not join the card is lit");
  const chip = all(v, "g.label").find(c => c.querySelector("text").firstChild.textContent === "done");
  over(chip.querySelector("rect"));
  await settle();
  equal(all(v, "g.state.lit").map(g => g.querySelector("text.title").textContent).sort(), ["drafting", "waiting"], "the cards the chip done lights");
  v.dispatchEvent(new PointerEvent("pointerleave"));
  await settle();
  equal(all(v, ".lit").length, 0, "what is lit once the pointer left");
});

// The camera keeps the cards it draws as the run moves on: a card keeps its
// element, and only what changed is written.
test("drawing_keeps_its_elements_as_the_run_moves_on", async () => {
  const v = await draw("review");
  const waiting = card(v, "waiting");
  await show("review-running");
  assert(card(v, "waiting") === waiting, "a card lost its element as the run moved on");
  equal(classes(waiting), "state now", "the card as the run moved on");
});
