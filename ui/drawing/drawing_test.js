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

// show puts another graph in place of the one drawn, as a page is sent it,
// and waits for the camera to glide to it.
async function show(name) {
  receive({ type: "part", id: "scene", data: await fixture(name) });
  await settle(3);
  await wait(450);
}

// card gives the group of a card by its name, as its title writes it.
function card(v, name) {
  const t = [...v.querySelectorAll("text.title")].find(t => t.textContent === name);
  assert(t, `${name} is not drawn as a card`);
  return t.closest("g");
}

// classes gives a part's classes as the graph marks it, without what the
// pointer lights.
const classes = el => [...el.classList].filter(c => c !== "lit" && c !== "at").join(" ");

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
  equal(["tell", "look", "idle"].map(n => classes(card(v, n))), ["state tool held", "state tool done", "state tool never"], "the tools of an agent waiting on a hook");
  const hook = card(v, "approval");
  equal([classes(hook), text(hook.querySelector("text.sub"))], ["state hook when now", "flow: approval"], "the hook");
  equal(all(v, 'a[href="/run/a-client/2"]').length, 1, "the links to the run the hook started");
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
  const hrefs = all(v, "a[href]").map(a => a.getAttribute("href"));
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
  equal(all(v, "a[href]").map(a => a.getAttribute("href")), ["/run/a-client/1"], "the links on a run's page");
  assert(texts(v, "text.sub").includes("flow: b"), "a card whose job started a run does not say what that run is");
  v = await draw("nested-machines");
  assert(texts(v, "text.sub").includes("flow: b") && all(v, "a[href]").length === 0, "a card whose job started no run does not say its machine");
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
  equal(all(v, "a[href]").length, 0, "the links of a call that started no run, and a send");
  v = await draw("trip-called");
  equal(all(v, "a[href]").map(a => a.getAttribute("href")), ["/run/lab/7"], "the links of a call that started a run");
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

// extent gives where a text's letters run across, by its anchor and the
// length of its letters in the font it is set in.
function extent(t) {
  const len = t.getComputedTextLength(), x = +t.getAttribute("x");
  switch (t.getAttribute("text-anchor")) {
    case "middle": return [x - len / 2, x + len / 2];
    case "end": return [x - len, x];
  }
  return [x, x + len];
}

// Every element of a drawing is SVG, and what a card, a chip and a holder
// write fits inside it in the real fonts.
test("drawing_text_fits_its_card", async () => {
  const faults = [];
  for (const name of [...(await fixture("examples")), "helper", "toolkits-running", "conditions", "trip-called", "nested-deep", "triage", "deploy-running"]) {
    const v = await draw(name);
    const svg = v.querySelector("svg");
    for (const el of svg.querySelectorAll("*")) if (el.namespaceURI !== "http://www.w3.org/2000/svg") faults.push(`${name}: <${el.localName}> is not SVG`);
    for (const g of svg.querySelectorAll("g.state:not(.holder)")) {
      const box = g.querySelector(":scope > a.card > rect.box").getBBox();
      for (const t of g.querySelectorAll(":scope > a > text")) {
        const [a, b] = extent(t);
        if (a < box.x || b > box.x + box.width + 0.5) faults.push(`${name}: "${t.firstChild?.textContent}" overruns its card`);
      }
    }
    for (const c of svg.querySelectorAll("g.arrowchip")) {
      const r = c.querySelector("rect").getBBox(), [a, b] = extent(c.querySelector("text"));
      if (a < r.x - 0.5 || b > r.x + r.width + 0.5) faults.push(`${name}: the chip "${c.querySelector("text").firstChild.textContent}" overruns its pill`);
    }
    for (const h of svg.querySelectorAll("g.holder")) {
      const r = h.querySelector("rect").getBBox(), [, b] = extent(h.querySelector("text.name"));
      if (b > r.x + r.width) faults.push(`${name}: the name "${h.querySelector("text.name").textContent}" overruns its holder`);
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
        if (a.r.closest(".in") !== o.r.closest(".in")) continue;
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
  const big = await draw("big-flow", { width: 700, height: 500 });
  assert(camera(big).k < 1, "a machine larger than its view is not scaled down to fit");
});

// inside gives the box of the machine drawn inside a card, as the camera
// shows it in the viewport.
function inside(v, g) {
  const r = g.closest(".at").querySelector(":scope > .in").getBoundingClientRect(), o = v.getBoundingClientRect();
  return { x: r.left - o.left, y: r.top - o.top, w: r.width, h: r.height };
}

// fitted gives the scale at which the whole drawing fits the view.
function fitted(v) {
  const whole = v.querySelector("g.camera").getBBox();
  return Math.min((v.clientWidth - 48) / whole.width, (v.clientHeight - 48) / whole.height);
}

// On a run's page the camera frames the innermost machine the run's marks
// are in: the whole of the run's machine, or a child's machine inside its
// card, which it fills, up to 1.6 times its size.
test("drawing_camera_follows_the_run", async () => {
  const v = await draw("lookup-running", { width: 520, height: 360 });
  assert(within(seen(v, card(v, "find")), v), "the card the run marks is not in view");
  await show("lookup-in-child");
  const inner = card(v, "b");
  assert(classes(inner) === "state stage now", "the child's running stage is not drawn inside its card");
  const child = inside(v, card(v, "check"));
  assert(within(child, v), `the child's machine is not in view: ${JSON.stringify(child)}`);
  // The machine framed holds its margins, which are not drawn.
  assert(Math.abs(child.x + child.w / 2 - v.clientWidth / 2) < 8 && Math.abs(child.y + child.h / 2 - v.clientHeight / 2) < 8, `the child's machine is not in the middle of the view: ${JSON.stringify(child)}`);
  assert(camera(v).k > fitted(v), "the camera did not frame the child's machine more closely than the whole");
  assert(parseFloat(getComputedStyle(v.querySelector("g.camera")).transitionDuration) > 0, "the camera does not glide");
});

// Following a run into a child zooms in to the child's machine, and back
// to the parent's zooms out to the whole of it.
test("drawing_camera_zooms_into_a_child_and_out", async () => {
  const v = await draw("lookup-running", { width: 520, height: 360 });
  const parent = camera(v);
  await show("lookup-in-child");
  const child = camera(v);
  assert(child.k > parent.k, `the scale did not rise into the child: ${parent.k} to ${child.k}`);
  assert(child.k <= 1.6, "the camera scaled the child more than 1.6 times");
  await show("lookup-running");
  equal(camera(v), parent, "the camera back at the parent");
});

// A card kept as the layout changes keeps its element, and glides to its
// new place and size.
test("drawing_cards_glide_as_the_layout_changes", async () => {
  const v = await draw("lookup-running", { width: 900, height: 700 });
  const tell = card(v, "tell"), place = tell.closest(".at"), box = tell.querySelector("rect.box");
  const from = new DOMMatrix(getComputedStyle(place).transform).f;
  receive({ type: "part", id: "scene", data: await fixture("lookup-in-child") });
  await settle(2);
  await wait(100);
  assert(card(v, "tell") === tell, "the card lost its element as the layout changed");
  const to = /translate\([-0-9.]+px, ([-0-9.]+)px\)/.exec(place.getAttribute("style"))[1];
  const now = new DOMMatrix(getComputedStyle(place).transform).f;
  assert(+to > from && now > from && now < +to, `the card did not glide down: from ${from} to ${to}, at ${now} part way`);
  assert(getComputedStyle(place).transitionProperty.includes("transform") && getComputedStyle(box).transitionProperty.includes("height"), "a card's place and size have no transition");
  const check = card(v, "check").querySelector("rect.box");
  assert(parseFloat(getComputedStyle(check).height) < check.getBBox().height + 1, "the card opening has no size");
  await wait(400);
  equal(new DOMMatrix(getComputedStyle(place).transform).f, +to, "the card's place once it glided");
});

// A page can focus a card by its path, and the camera frames it.
test("drawing_camera_frames_the_card_focused", async () => {
  const v = await draw("nested-deep", { width: 600, height: 400 }, "outer/inner");
  const machine = inside(v, card(v, "inner"));
  assert(within(machine, v), "the machine unfolded in the card focused is not in view");
  assert(Math.abs(machine.x + machine.w / 2 - v.clientWidth / 2) < 8, `the machine unfolded in the card focused is not in the middle of the view: ${JSON.stringify(machine)}`);
  assert(camera(v).k > fitted(v), "the camera did not move in to the card focused");
});

// A wheel pans, ctrl-wheel zooms round the pointer, and both stop the camera
// following, which Follow gives back; + - and 0 zoom and fit.
test("drawing_camera_is_moved_by_hand", async () => {
  const v = await draw("big-flow", { width: 700, height: 500 });
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
  // The point of the drawing under the pointer stays under it.
  const p = { x: (100 - c.x) / c.k, y: (80 - c.y) / c.k };
  assert(Math.abs(z.x + p.x * z.k - 100) < 1 && Math.abs(z.y + p.y * z.k - 80) < 1, "the zoom is not round the pointer");
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
  await wait(450);
  const fit = camera(v);
  const whole = v.querySelector("g.camera").getBBox();
  assert(fit.k * whole.height <= v.clientHeight + 1 && fit.k * whole.width <= v.clientWidth + 1, "0 does not fit the whole machine");
});

// A drag pans, and does not follow the link it began on.
test("drawing_camera_is_dragged", async () => {
  const v = await draw("nested", { width: 500, height: 400 });
  const link = v.querySelector("a[href]");
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
  assert(lit.some(e => e.matches("path.edge")) && lit.some(e => e.matches("g.arrowchip")), "a card does not light its arrows and their chips");
  for (const e of all(v, "path.edge.lit")) assert(e.dataset.from.endsWith("state waiting") || e.dataset.to.endsWith("state waiting"), "an arrow that does not join the card is lit");
  const chip = all(v, "g.arrowchip").find(c => c.querySelector("text").firstChild.textContent === "done");
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

// A drawing loaded with a run's marks inside a child's card is framed on
// the child's machine from its first frame, placed at once.
test("drawing_camera_frames_a_child_from_its_first_frame", async () => {
  const box = mount(() => html`<div class="drawn" style="flex: 1"><${Drawing} scene=${() => parts.scene} /></div>`, { scene: await fixture("lookup-in-child") }, { width: 729, height: 394 });
  const v = box.querySelector(".viewport");
  const first = [];
  for (let i = 0; i < 4; i++) {
    await new Promise(r => requestAnimationFrame(r));
    const g = v.querySelector("g.camera");
    first.push({ k: camera(v).k, moving: g.classList.contains("moving") });
  }
  await wait(450);
  const settled = camera(v);
  assert(settled.k > fitted(v), `the camera did not frame the child's machine: ${JSON.stringify(settled)}`);
  const painted = first.find(f => f.k !== 1 || !f.moving);
  equal(painted?.k, settled.k, `the scale of the first frame placed: ${JSON.stringify(first)}`);
});

// drawWith mounts a drawing of a fixture with the props a page gives it:
// lineHref, the paths of cards lit, what a pick does, and counts. lit and
// counts are parts, so a test can change them as a page does.
async function drawWith(name, { lineHref, lit = [], onCard, counts = {} } = {}, size = { width: 900, height: 700 }) {
  const box = mount(() => html`<div class="drawn" style="flex: 1"><${Drawing} scene=${() => parts.scene} lineHref=${lineHref} lit=${() => parts.lit} onCard=${onCard} counts=${() => parts.counts} /></div>`, { scene: await fixture(name), lit, counts }, size);
  await settle(3);
  const v = box.querySelector(".viewport");
  assert(v, `${name} is not drawn`);
  return v;
}

// A card's name links to the line of its file that writes it, where the
// page gives lines an address; the card's own link is on the rest of it.
test("drawing_card_name_links_to_its_line", async () => {
  let v = await drawWith("lookup-in-child", { lineHref: (line, path) => (path.includes("/") ? "" : "/machines?line=" + line) });
  const check = card(v, "check");
  const name = check.querySelector("text.title").closest("a");
  assert(name?.matches("a.line"), "the name of a card is no link to its line");
  equal(name.getAttribute("href"), "/machines?line=" + "4", "the address of check's line");
  equal(check.querySelector("a.card").getAttribute("href"), "/run/a-client/9", "the link of the rest of the card");
  assert(!name.closest("a.card") && !check.querySelector("a.card a"), "a link is inside another");
  v = await drawWith("lookup-in-child");
  assert(!v.querySelector("a.line"), "a name links to its line where the page gives lines no address");
});

// A card's tooltip gives its job's limits, and a flow's arrow why one stage
// waits for the other.
test("drawing_tooltips_are_drawn_on_cards_and_arrows", async () => {
  const v = await drawWith("limited");
  const tip = card(v, "tell").querySelector(":scope > title").textContent;
  equal(tip, "timeout: 5s\nrestart: { after: failure, limit: 2 in 1h, delay: 2s }\ntags: gpu\nallow: network", "the tooltip of tell");
  const why = all(v, "path.edge").map(e => e.querySelector("title")?.textContent).sort();
  equal(why, ["each: { emitted: find }", "follows: tell", "reads find.output"], "the tooltips of the arrows");
});

// The page lights cards by their paths, with their arrows, as the pointer
// lights a card, and as the paths change.
test("drawing_cards_lit_by_the_page", async () => {
  const v = await drawWith("lookup-in-child", { lit: ["check/b"] });
  const litNames = () => all(v, "g.state.lit text.title").map(t => t.textContent).sort();
  equal(litNames(), ["b"], "the cards lit");
  assert(all(v, "path.edge.lit").length === 1, "the arrow into the card lit is not lit");
  receive({ type: "part", id: "lit", data: ["find", "tell"] });
  await settle(2);
  equal(litNames(), ["find", "tell"], "the cards lit after the page changed them");
  receive({ type: "part", id: "lit", data: [] });
  await settle(2);
  equal(litNames(), [], "the cards lit with none");
});

// A card that is no link is picked by a click or by Enter, and the page is
// given its path; a card that is a link is followed, not picked.
test("drawing_card_is_picked_by_the_page", async () => {
  const picked = [];
  const v = await drawWith("lookup-in-child", { onCard: path => picked.push(path) });
  const b = card(v, "b"), check = card(v, "check");
  b.querySelector("rect.box").dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
  card(v, "find").dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
  equal(picked, ["check/b", "find"], "the paths picked");
  equal(b.getAttribute("tabindex"), "0", "a card that can be picked does not take the keyboard");
  assert(!check.hasAttribute("tabindex") && !check.classList.contains("pick"), "a card that is a link can be picked");
});

// A count above 0 is drawn at a card's top right, and goes when it is 0.
test("drawing_card_counts", async () => {
  const v = await drawWith("review", { counts: { waiting: 3, drafting: 0 } });
  equal(all(v, "g.count text").map(t => t.textContent), ["3"], "the counts drawn");
  const badge = card(v, "waiting").querySelector("g.count").getBoundingClientRect(), box = card(v, "waiting").querySelector("rect.box").getBoundingClientRect();
  const k = camera(v).k;
  assert(Math.abs(badge.right - box.right) < 20 * k && Math.abs(badge.top - box.top) < 12 * k, `the count is at ${badge.right - box.right}, ${badge.top - box.top} from the card's top right`);
  receive({ type: "part", id: "counts", data: { drafting: 12 } });
  await settle(2);
  equal(all(v, "g.count text").map(t => t.textContent), ["12"], "the counts after they changed");
});
