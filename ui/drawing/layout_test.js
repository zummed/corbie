// The tests of how a machine's graph is laid out, in Chrome: where layout.js
// puts each card, arrow and chip of the graphs drawing_test.go reads in Go.
import { test, fixture, assert, equal } from "/test/page/testdata/test.js";
import { layout } from "/drawing/layout.js";

const laid = async name => layout(await fixture(name));

// cards gives every card of a drawing that is drawn at its top level, by
// its name: its box.
const cards = l => new Map(l.shapes.filter(s => s.kind === "card").map(s => [s.name, s.box]));
const mid = b => ({ x: b.x + b.w / 2, y: b.y + b.h / 2 });

test("drawing_flow_stage_follows_what_it_waits_for", async () => {
  const c = cards(await laid("fan"));
  const y = n => mid(c.get(n)).y;
  assert(y("fetch") < y("sum") && y("fetch") < y("translate"), "sum and translate are not drawn below fetch");
  assert(y("sum") < y("send") && y("translate") < y("send"), "send is not drawn below sum and translate, which it waits for");
  equal(y("sum"), y("translate"), "the row of translate, beside sum");
});

test("drawing_flow_has_one_arrow_for_each_wait_that_says_something", async () => {
  equal((await laid("chain")).edges.length, 2, "the arrows: c waits for a through b, and needs none from a");
});

test("drawing_chips_of_arrows_into_one_state_are_apart", async () => {
  const l = await laid("two");
  const at = text => l.chips.find(c => c.text === text).tx;
  assert(at("ok") !== at("fine"), "two chips are at one x");
});

// A segment is a straight part of an arrow, across or down.
function segments(d) {
  const out = [];
  let at = null;
  for (const [, cmd, args] of d.matchAll(/([MHVQ])([-0-9. ]+)/g)) {
    const v = args.trim().split(/\s+/).map(Number);
    switch (cmd) {
      case "M": at = { x: v[0], y: v[1] }; break;
      case "H": out.push({ p: at, q: { x: v[0], y: at.y } }); at = { x: v[0], y: at.y }; break;
      case "V": out.push({ p: at, q: { x: at.x, y: v[0] } }); at = { x: at.x, y: v[0] }; break;
      case "Q": at = { x: v[2], y: v[3] }; break;
    }
  }
  return out;
}

// passes says a segment goes through the inside of a box, grown by grow.
function passes(b, s, grow) {
  const x0 = b.x - grow, y0 = b.y - grow, x1 = b.x + b.w + grow, y1 = b.y + b.h + grow;
  if (s.p.y === s.q.y) return s.p.y > y0 && s.p.y < y1 && Math.max(s.p.x, s.q.x) > x0 && Math.min(s.p.x, s.q.x) < x1;
  return s.p.x > x0 && s.p.x < x1 && Math.max(s.p.y, s.q.y) > y0 && Math.min(s.p.y, s.q.y) < y1;
}

// overlap says two segments run along one line for more than a point.
function overlap(a, b) {
  if (a.p.y === a.q.y && b.p.y === b.q.y && Math.abs(a.p.y - b.p.y) < 1)
    return Math.min(Math.max(a.p.x, a.q.x), Math.max(b.p.x, b.q.x)) - Math.max(Math.min(a.p.x, a.q.x), Math.min(b.p.x, b.q.x)) > 1;
  if (a.p.x === a.q.x && b.p.x === b.q.x && Math.abs(a.p.x - b.p.x) < 1)
    return Math.min(Math.max(a.p.y, a.q.y), Math.max(b.p.y, b.q.y)) - Math.max(Math.min(a.p.y, a.q.y), Math.min(b.p.y, b.q.y)) > 1;
  return false;
}

const on = (p, s) => p.x >= Math.min(s.p.x, s.q.x) - 0.5 && p.x <= Math.max(s.p.x, s.q.x) + 0.5 && p.y >= Math.min(s.p.y, s.q.y) - 0.5 && p.y <= Math.max(s.p.y, s.q.y) + 0.5;

const onBorder = (p, boxes) =>
  boxes.some(b => {
    const inside = p.x >= b.x - 0.5 && p.x <= b.x + b.w + 0.5 && p.y >= b.y - 0.5 && p.y <= b.y + b.h + 0.5;
    const edge = Math.abs(p.x - b.x) < 0.5 || Math.abs(p.x - b.x - b.w) < 0.5 || Math.abs(p.y - b.y) < 0.5 || Math.abs(p.y - b.y - b.h) < 0.5;
    return inside && edge;
  });

// boxes gives a drawing's cards, each as its outermost box, and its holders.
function boxes(l) {
  const out = [];
  for (const s of l.shapes.filter(s => s.kind === "card")) {
    const outer = s.behind.find(b => b.class === "outer");
    out.push(outer ?? s.box);
  }
  return { cards: out, holders: l.holders.map(h => ({ x: h.x, y: h.y, w: h.w, h: h.h })) };
}

// Every arrow of a statechart has a line of its own, from the card it
// leaves to the card it goes to, behind no card, and each chip is clear of
// other chips and cards, and 12 from other arrows. Arrows from one card
// share the line they leave it by.
test("drawing_arrows_and_chips_are_apart", async () => {
  const names = await fixture("examples");
  assert(names.length >= 8, "the examples in testdata are not there");
  const faults = [];
  for (const name of names) {
    const l = await laid(name);
    const lines = l.edges.map(e => segments(e.d));
    const chips = l.chips.map(c => ({ x: c.x, y: c.y, w: c.w, h: 18 }));
    const { cards, holders } = boxes(l);
    if (chips.length !== lines.length) faults.push(`${name}: ${chips.length} chips for ${lines.length} arrows`);
    lines.forEach((a, i) => {
      if (!onBorder(a[0].p, [...cards, ...holders]) || !onBorder(a[a.length - 1].q, [...cards, ...holders])) faults.push(`${name}: the arrow ${l.edges[i].d} does not run from one card to another`);
      for (const s of a) for (const c of cards) if (passes(c, s, -1)) faults.push(`${name}: the arrow ${l.edges[i].d} runs behind the card at ${JSON.stringify(c)}`);
      for (const b of lines.slice(i + 1)) {
        if (a[0].p.x === b[0].p.x && a[0].p.y === b[0].p.y) continue;
        for (const s of a) for (const o of b) if (overlap(s, o)) faults.push(`${name}: two arrows run on one line at ${JSON.stringify(s)}`);
      }
    });
    chips.forEach((c, i) => {
      for (const o of chips.slice(i + 1)) if (c.x < o.x + o.w && o.x < c.x + c.w && c.y < o.y + o.h && o.y < c.y + c.h) faults.push(`${name}: the chips ${l.chips[i].text} and another overlap`);
      for (const b of cards) if (c.x < b.x + b.w && b.x < c.x + c.w && c.y < b.y + b.h && b.y < c.y + c.h) faults.push(`${name}: the chip ${l.chips[i].text} is on a card`);
      const m = { x: c.x + c.w / 2, y: c.y + c.h / 2 };
      lines.forEach((a, j) => {
        for (const s of a) {
          if (j === i && passes(c, s, 0) && !on(m, s)) faults.push(`${name}: the chip ${l.chips[i].text} is on its arrow twice`);
          if (j !== i && passes(c, s, 11.5)) faults.push(`${name}: the chip ${l.chips[i].text} is within 12 of another arrow`);
        }
      });
    });
  }
  equal(faults, [], "the faults");
});

test("drawing_holder_makes_room_for_its_notes", async () => {
  const l = await laid("held");
  const notes = l.shapes.filter(s => s.kind === "note").map(s => s.y);
  const c = cards(l);
  assert(notes.length > 0 && Math.max(...notes) + 20 < c.get("one").y + 22, "the last note overlaps the state inside");
  const busy = l.holders.find(h => h.name.text === "busy");
  assert(busy, "busy is not drawn round its states");
  assert(busy.y + busy.h >= c.get("two").y + 22 + 16, "the state inside is below the holder's bottom");
});

test("drawing_statechart_send_is_a_card", async () => {
  const c = cards(await laid("listen"));
  assert(c.get("chat#a").y > c.get("hearing").y, "the send's card is not below the state");
});

test("drawing_agent_is_a_loop_of_model_and_tools", async () => {
  const c = cards(await laid("helper"));
  const y = n => c.get(n).y;
  assert(y("claude-sonnet-5-5") < y("tell") && y("claude-sonnet-5-5") < y("look") && y("tell") < y("approval") && y("approval") < y("reply") && y("look") < y("reply"), "the model, the tools, the hook and the reply are not drawn top to bottom");
  assert(mid(c.get("tell")).x === mid(c.get("look")).x && y("look") < y("tell"), "the tools are not one below another");
});

test("drawing_call_and_send_jobs_are_cards", async () => {
  const c = cards(await laid("trip"));
  assert(c.get("heat").y > c.get("weather").y, "a send whose data reads a stage is not drawn below it");
});

test("drawing_machine_unfolds_in_its_card", async () => {
  const folded = await laid("nested"), one = await laid("nested-outer");
  assert(one.height > folded.height, "an unfolded card does not make room for what it holds");
  const outer = one.shapes.find(s => s.name === "outer");
  assert(outer.in && outer.in.shapes.some(s => s.name === "inner"), "the unfolded card does not hold its machine");
});

test("drawing_of_a_big_machine_is_quick", async () => {
  for (const name of ["big-flow", "big-chart"]) {
    const scene = await fixture(name);
    const began = performance.now();
    layout(scene);
    const took = performance.now() - began;
    assert(took < 1500, `${name} took ${Math.round(took)}ms to lay out`);
  }
});
