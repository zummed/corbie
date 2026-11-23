// layout lays out the graph of a machine, as the UI reads it in Go, and
// gives what drawing.js draws: where every holder, arrow, chip and card is,
// with its classes, in the order they are painted. It reads nothing else
// and imports nothing, so that it runs wherever JavaScript does.

// The sizes a drawing is laid out by: every card is of one width, with an
// icon, a title, and the lines under it.
const cardW = 232;
const lineH = 15;
const noteH = 19;
const sideGap = 28;
const rowGap = 40;
const margin = 24;
const longest = 26;
// longestChip fits between the mid points of two arrows side by side.
const longestChip = 18;
// cardR is how round the corners of every card are.
const cardR = 10;
// nameW is about how wide a letter of a holder's name is, set in IBM Plex
// Mono at 12px.
const nameW = 7.3;
// The room inside a state that holds states: its name above, and a margin.
const inner = 16;
const innerTop = 38;
// trackGap is how far apart two lines are that run side by side.
const trackGap = 22;
// chanGap is how far apart the lines down the side of a holder are.
const chanGap = 12;
// lineApart is how far a chip is from every arrow but its own.
const lineApart = 12;
// foldedH is how high a folded toolkit is: its name and how many tools it has.
const foldedH = 52;

// layout gives the drawing of a scene: its parts, how far it reaches, and
// the box the whole is drawn in. Every part listed has a key, unique among
// its list, that names it from one layout to the next.
export function layout(scene) {
  const d = drawOf(scene);
  return {
    label: scene.Type + " " + scene.Name,
    left: d.left,
    width: d.w + margin - d.left,
    height: d.h + margin,
    holders: d.holders,
    edges: d.edges,
    chips: d.chips,
    shapes: d.shapes,
  };
}

function drawOf(scene) {
  switch (scene.Type) {
    case "flow":
      return drawFlow(scene.Flow);
    case "statechart":
      return drawChart(scene.Chart);
    case "agent":
      return drawAgent(scene.Agent);
    case "decision":
      return drawDecision(scene.Decision);
  }
  return new Drawn();
}

// runes gives how many letters a text has.
const runes = s => [...s].length;

function clip(s, n) {
  const r = [...s];
  return r.length > n ? r.slice(0, n - 1).join("") + "…" : s;
}

// labelWidth is about how wide a label is, set in IBM Plex Sans at 11.5px.
const labelWidth = label => runes(label) * 6.3;

// chipWidth is how wide a chip is for a label.
const chipWidth = label => labelWidth(clip(label, longestChip)) + 14;

// itemOf gives what a card says from the graph, with the machine unfolded
// inside it laid out.
function itemOf(card) {
  return {
    icon: card.Icon,
    name: card.Name,
    lines: card.Lines ?? [],
    notes: [],
    title: card.Title,
    href: card.Href,
    in: card.In ? drawOf(card.In) : null,
    narrow: card.Narrow,
  };
}

// fits gives how many letters of a name a box has room for.
const fits = it => (it.narrow ? 16 : longest);

// head gives how high a box is above what is unfolded inside it.
function head(it) {
  let h = 44;
  if (it.lines.length > 0) h = 34 + it.lines.length * lineH;
  if (it.notes.length > 0) h += it.notes.length * noteH + 2;
  return h;
}

// size gives how big a box must be to hold an item.
function size(it) {
  const b = { x: 0, y: 0, w: cardW, h: head(it) };
  if (it.narrow) b.w = 132;
  if (it.in) {
    b.w = Math.max(b.w, it.in.w + margin);
    b.h += it.in.h - margin + 16;
  }
  return b;
}

const cx = b => b.x + b.w / 2;
const cy = b => b.y + b.h / 2;
const right = b => b.x + b.w;
const bottom = b => b.y + b.h;

// A Drawn is a drawing being made: its parts in the order they are painted,
// holders first, then arrows, then their chips, then the cards they join,
// and how far it reaches, to the right, below and to the left.
class Drawn {
  constructor() {
    this.holders = [];
    this.edges = [];
    this.chips = [];
    this.shapes = [];
    this.w = 0;
    this.h = 0;
    this.left = 0;
  }

  reach(b) {
    this.w = Math.max(this.w, right(b));
    this.h = Math.max(this.h, bottom(b));
  }

  // holder draws a box round cards, with its name.
  holder(cls, b, name, sub, title) {
    this.holders.push({ key: (cls.includes("toolkit") ? "toolkit " : "holder ") + name, class: cls, x: b.x, y: b.y, w: b.w, h: b.h, name: { x: b.x + 14, y: b.y + 22, text: name }, sub, title });
  }

  // paint draws an item in a box, as a link where it has one, and the
  // drawing unfolded inside it. behind is what is drawn under the box, and
  // key names the card among the drawing's.
  paint(it, b, cls, rx, behind, key) {
    if (it.in) cls += " unfolded";
    let tx = b.x + 50, ty = b.y + 22, anchor = "start";
    if (it.lines.length === 0) ty = b.y + 26;
    if (!it.icon) {
      tx = cx(b);
      anchor = "middle";
    }
    const card = {
      kind: "card",
      key: key ?? "card " + it.name,
      name: it.name,
      class: cls,
      href: it.href || "",
      behind: behind ?? [],
      box: { x: b.x, y: b.y, w: b.w, h: b.h, rx },
      title: { x: tx, y: ty, anchor, text: clip(it.name, fits(it)) },
      subs: [],
      notes: [],
      tile: null,
      fold: null,
      tooltip: it.title || "",
      in: null,
    };
    let y = ty;
    for (const l of it.lines) {
      y += lineH;
      card.subs.push({ key: card.subs.length, x: tx, y, anchor, text: l });
    }
    for (const n of it.notes) {
      y += noteH;
      card.notes.push({ key: card.notes.length, class: n.taken ? "note taken" : "note", x: tx, y, anchor, text: n.text });
    }
    if (it.icon) {
      const tile = { x: b.x + 12, y: b.y + 11, w: 28, h: 28 };
      card.tile = { icon: it.icon, x: tile.x, y: tile.y, spark: it.icon === "model" ? spark(cx(tile), cy(tile), 9) : "" };
    }
    if (it.href) card.fold = { x: right(b) - 15, y: b.y + 24, text: it.in ? "⌄" : "›" };
    if (it.in) {
      const dx = (b.w - it.in.w - margin) / 2;
      // The machine inside is at x, y in the card's drawing, and reaches
      // from left to left + w across and to h down.
      card.in = { x: b.x + dx, y: b.y + head(it) - margin + 8, left: it.in.left, w: it.in.w + margin - it.in.left, h: it.in.h + margin, holders: it.in.holders, edges: it.in.edges, chips: it.in.chips, shapes: it.in.shapes };
    }
    this.shapes.push(card);
    this.reach(b);
  }

  // down draws an arrow straight down, or down to a rail above the box it
  // goes to, along it, and down into the box, so arrows from one box or to
  // one row run together. A label is a chip on the arrow: on the rail, where
  // it is apart from the chips of other arrows into the same row or box.
  // joins names the two cards the arrow joins, by their keys.
  down(from, to, cls, label, joins) {
    const x1 = cx(from), y1 = bottom(from), x2 = cx(to), y2 = to.y;
    let rail = y2 - rowGap / 2;
    if (label) rail = y2 - rowGap;
    const mx = (x1 + x2) / 2;
    let my = rail;
    if (Math.abs(x1 - x2) < 1) my = y2 - 14;
    this.line([{ x: x1, y: y1 }, { x: x1, y: rail }, { x: x2, y: rail }, { x: x2, y: y2 }], cls, joins);
    if (label) this.chip(mx, my, label, cls, joins);
  }

  // line draws an arrow through points, one after another across or down,
  // with its turns rounded, from one card to another.
  line(points, cls, joins) {
    const r = 8;
    const pts = simplify(points);
    let at = pts[0];
    let p = `M${num(at.x)} ${num(at.y)}`;
    const to = q => {
      p += Math.abs(q.y - at.y) < 0.01 ? ` H${num(q.x)}` : ` V${num(q.y)}`;
      at = q;
    };
    const toward = (a, b, n) => {
      const l = Math.hypot(b.x - a.x, b.y - a.y);
      return { x: a.x + ((b.x - a.x) * n) / l, y: a.y + ((b.y - a.y) * n) / l };
    };
    for (let i = 1; i < pts.length - 1; i++) {
      const prev = pts[i - 1], q = pts[i], next = pts[i + 1];
      const n = Math.min(r, Math.min(Math.hypot(q.x - prev.x, q.y - prev.y), Math.hypot(next.x - q.x, next.y - q.y)) / 2);
      to(toward(q, prev, n));
      const end = toward(q, next, n);
      p += ` Q${num(q.x)} ${num(q.y)} ${num(end.x)} ${num(end.y)}`;
      at = end;
    }
    to(pts[pts.length - 1]);
    const classes = ("edge " + cls).trim();
    this.edges.push({ key: this.edges.length, class: classes, d: p, head: cls.split(" ").includes("taken") ? "at" : "ah", from: joins.from, to: joins.to, title: joins.why ?? "" });
    for (const q of pts) {
      this.reach({ x: q.x, y: q.y, w: 0, h: 0 });
      this.left = Math.min(this.left, q.x);
    }
  }

  // chip draws a label in a pill, short enough to stay apart from the chips
  // of the arrows beside it, with the whole label as its tooltip. It has the
  // classes of its arrow, and joins the cards its arrow joins.
  chip(x, y, label, cls, joins) {
    const short = clip(label, longestChip);
    const w = chipWidth(label);
    this.chips.push({ key: this.chips.length, class: ("chip " + cls).trim(), x: x - w / 2, y: y - 9, w, tx: x, ty: y + 4, text: short, title: [short !== label ? label : "", joins.why ?? ""].filter(Boolean).join("\n"), from: joins.from, to: joins.to });
    this.reach({ x: x + w / 2, y: y + 9, w: 0, h: 0 });
    this.left = Math.min(this.left, x - w / 2);
  }
}

// num writes a number as the drawing does: to one place, a half to the
// even place, with no trailing zero.
export function num(f) {
  let s = f.toFixed(1);
  // A half is exact only for a quarter that is odd.
  if (Number.isInteger(f * 4) && !Number.isInteger(f * 2)) {
    let n = Math.floor(f * 10);
    if (n % 2 !== 0) n += 1;
    s = (n / 10).toFixed(1);
  }
  return s.endsWith(".0") ? s.slice(0, -2) : s;
}

// spark is the mark of a model: a star of four points.
function spark(x, y, r) {
  const k = r * 0.22;
  const n = num;
  return `M${n(x)} ${n(y - r)}C${n(x + k)} ${n(y - k)} ${n(x + k)} ${n(y - k)} ${n(x + r)} ${n(y)}` +
    `C${n(x + k)} ${n(y + k)} ${n(x + k)} ${n(y + k)} ${n(x)} ${n(y + r)}` +
    `C${n(x - k)} ${n(y + k)} ${n(x - k)} ${n(y + k)} ${n(x - r)} ${n(y)}` +
    `C${n(x - k)} ${n(y - k)} ${n(x - k)} ${n(y - k)} ${n(x)} ${n(y - r)}Z`;
}

// simplify drops the points of a line that are the same as the one before
// or on the way from it to the next.
function simplify(pts) {
  const out = [];
  for (const p of pts) {
    const n = out.length;
    if (n > 0 && Math.abs(out[n - 1].x - p.x) < 0.01 && Math.abs(out[n - 1].y - p.y) < 0.01) continue;
    if (n > 1) {
      const a = out[n - 2], b = out[n - 1];
      if ((Math.abs(a.x - b.x) < 0.01 && Math.abs(b.x - p.x) < 0.01) || (Math.abs(a.y - b.y) < 0.01 && Math.abs(b.y - p.y) < 0.01)) {
        out[n - 1] = p;
        continue;
      }
    }
    out.push(p);
  }
  return out;
}

const cmp = (a, b) => (a < b ? -1 : a > b ? 1 : 0);

// A flow

function drawFlow(f) {
  const names = f.Stages.map(s => s.Card.Name);
  const stage = new Map(f.Stages.map(s => [s.Card.Name, s]));
  const after = new Map(f.Stages.map(s => [s.Card.Name, s.After ?? []]));
  // A stage's row is the longest chain of stages it waits for.
  const row = new Map();
  const depth = (n, seen) => {
    if (row.has(n)) return row.get(n);
    if (seen.has(n)) return 0; // a stage that waits for itself, which the check refuses
    seen.add(n);
    let c = 0;
    for (const a of after.get(n)) c = Math.max(c, depth(a, seen) + 1);
    row.set(n, c);
    return c;
  };
  const rows = [];
  for (const n of names) {
    const c = depth(n, new Set());
    while (rows.length <= c) rows.push([]);
    rows[c].push(n);
  }
  // Within a row, a stage goes by the places of the stages it waits for, to
  // keep arrows short.
  const place = new Map();
  for (const c of rows) {
    const mean = n => {
      const a = after.get(n);
      if (a.length === 0) return Infinity;
      let sum = 0;
      for (const x of a) sum += place.get(x);
      return sum / a.length;
    };
    c.sort((a, b) => cmp(mean(a), mean(b)));
    c.forEach((n, i) => place.set(n, i - (c.length - 1) / 2));
  }

  const items = new Map(f.Stages.map(s => [s.Card.Name, itemOf(s.Card)]));
  const at = new Map();
  const across = c => {
    let used = (c.length - 1) * sideGap;
    for (const n of c) used += size(items.get(n)).w;
    return used;
  };
  let total = 0;
  for (const c of rows) total = Math.max(total, across(c));
  let y = margin + 8;
  for (const c of rows) {
    let high = 0;
    let x = margin + (total - across(c)) / 2;
    for (const n of c) {
      const b = size(items.get(n));
      b.x = x;
      b.y = y;
      at.set(n, b);
      x += b.w + sideGap;
      high = Math.max(high, b.h);
    }
    y += high + rowGap;
  }

  const d = new Drawn();
  const reach = new Map();
  for (const n of names) {
    for (const a of after.get(n)) {
      if (!through(after, reach, n, a)) d.down(at.get(a), at.get(n), "", "", { from: "stage " + a, to: "stage " + n, why: stage.get(n).Why?.[a] ?? "" });
    }
  }
  for (const n of names) {
    const s = stage.get(n), b = at.get(n);
    let cls = "state stage";
    if (s.Mark) cls += " " + s.Mark;
    if (s.When) cls += " when";
    const behind = [];
    if (s.Stack) for (const off of [8, 4]) behind.push({ key: behind.length, class: "behind", x: b.x + off, y: b.y - off, w: b.w, h: b.h, rx: cardR });
    d.paint(items.get(n), b, cls, cardR, behind, "stage " + n);
    d.reach({ x: b.x, y: b.y, w: b.w + 8, h: b.h });
  }
  return d;
}

// through says a stage waits for another through a third, so an arrow to it
// from the other says nothing more.
function through(after, reach, n, a) {
  return after.get(n).some(b => b !== a && reaches(after, reach, b).has(a));
}

// reaches gives every stage a stage waits for, near or far, once for each
// stage. A stage that waits for itself, which the check refuses, ends it.
function reaches(after, reach, n) {
  if (reach.has(n)) return reach.get(n);
  const all = new Set();
  reach.set(n, all);
  for (const a of after.get(n)) {
    all.add(a);
    for (const b of reaches(after, reach, a)) all.add(b);
  }
  return all;
}

// A statechart

// How a transition is drawn, by where its states are drawn: in its card, as
// no arrow; across to the state beside it; below the row, to a state further
// along it; or down, to a state in a row below.
const none = 0, back = 1, across = 2, lane = 3, down = 4;

const pair = (a, b) => a + "\u0000" + b;

function ruleLabel(r) {
  let label = r.Event;
  if (r.When) label += ", when " + clip(r.When.replace(/^\$\{/, "").replace(/\}$/, ""), 28);
  if (r.Timer) label += ", " + r.Timer;
  return label;
}

// An arrow is every rule from one state to another, and how it is drawn: its
// kind, the level its states are drawn apart at, and the states of that level
// they are in or are.
function arrowLabel(a) {
  const labels = [];
  for (const r of a.rules) {
    const l = ruleLabel(r);
    if (!labels.includes(l)) labels.push(l);
  }
  return labels.join(", ");
}

const taken = a => a.rules.some(r => r.Taken);

// arrowClass gives how an arrow is drawn: dashed for a timer or a send, and
// in petrol where the latest step took it.
function arrowClass(a) {
  const classes = [];
  for (const r of a.rules) {
    if (r.Timer && !classes.includes("timer")) classes.push("timer");
    if (r.Send) classes.push("send");
    if (r.Taken && !classes.includes("taken")) classes.push("taken");
  }
  return classes.join(" ");
}

// A Chart is a statechart being laid out: every state by its name, the state
// that holds each, and where each is drawn.
class Chart {
  constructor(c) {
    this.states = new Map(); // by name: its State from the graph
    this.parent = new Map(); // "" for a state at the top
    this.kids = new Map([["", []]]); // by the state that holds them; "" is the top
    this.start = new Map([["", c.Start]]); // the state each level starts in
    this.at = new Map(); // relative to its holder until placed
    this.items = new Map();
    this.noted = new Set(); // the rules written in a box and drawn as no arrow
    this.all = c.Rules ?? [];
    this.rows = new Map(); // each level's states in rows, top to bottom, each left to right
    this.row = new Map(); // the row of each state among its level's
    this.col = new Map(); // its place in its row
    this.list = [];
    this.room = new Map(); // the gap between two states side by side, for the chips on the arrows between them
    this.sideway = []; // the lines down the side of a holder, as asked for
    this.chans = new Map(); // each line down the side of a holder, by its place out from the states inside
    this.pad = new Map(); // the room each side of a holder for its lines
    this.gapY = new Map(); // the first track below each row of a level, from the level's top
    this.high = new Map(); // how many tracks there are below each row of a level
    this.runs = null; // how far along each track its line runs, while the tracks are counted
    this.track = new Map(); // the track each line runs on, 0 nearest the row
    this.entry = new Map(); // where each arrow enters the state it goes to, from above
    this.stub = new Map(); // where each arrow along a row enters the state it goes to, from below
    this.origin = new Map(); // where each level's rows begin on the drawing
    for (const s of c.States ?? []) {
      this.states.set(s.Name, s);
      this.parent.set(s.Name, s.Parent);
      if (!this.kids.has(s.Parent)) this.kids.set(s.Parent, []);
      this.kids.get(s.Parent).push(s.Name);
      this.items.set(s.Name, itemOf(s.Card));
      if (s.Start) this.start.set(s.Name, s.Start);
    }
  }

  kidsOf(n) {
    return this.kids.get(n) ?? [];
  }

  holds(n) {
    return this.kidsOf(n).length > 0;
  }

  // write writes into its box each rule that goes back: to the state
  // itself, or to a state drawn in a row above.
  write() {
    const backs = [];
    const at = new Map();
    for (const r of this.all) {
      if (r.Send || this.kindOf(r)[0] !== back) continue;
      this.noted.add(r);
      const key = pair(r.From, r.To);
      if (!at.has(key)) {
        at.set(key, backs.length);
        backs.push({ from: r.From, to: r.To, rules: [] });
      }
      backs[at.get(key)].rules.push(r);
    }
    for (const b of backs) {
      const text = b.from === b.to ? arrowLabel(b) + " ↻" : arrowLabel(b) + " ↩ " + b.to;
      this.items.get(b.from).notes.push({ text: clip(text, longest + 6), taken: b.rules.some(r => r.Taken) });
    }
  }

  // arrows gives the rules drawn as arrows, together by the states they
  // join.
  arrows() {
    const all = [];
    const at = new Map();
    for (const r of this.all) {
      if (this.noted.has(r)) continue;
      const key = pair(r.From, r.To);
      if (!at.has(key)) {
        at.set(key, all.length);
        all.push({ from: r.From, to: r.To, rules: [], kind: none, level: "", a: "", b: "", right: false, line: 0, of: 0 });
      }
      all[at.get(key)].rules.push(r);
    }
    return all;
  }

  // kindOf gives how a rule is drawn, by the rows of the level its two
  // states are drawn apart at: the level, and the states of it they are in
  // or are. A rule from a state to one it holds, or to one that holds it,
  // goes back.
  kindOf(r) {
    if (!this.states.has(r.To)) return [none, "", "", ""];
    if (r.From === r.To) return [back, "", "", ""];
    for (let level = this.parent.get(r.From); ; level = this.parent.get(level)) {
      const a = this.member(level, r.From), b = this.member(level, r.To);
      if (a === "" || b === "") {
        if (level === "" || level === undefined) return [none, "", "", ""];
        continue;
      }
      if (a === b || this.row.get(b) < this.row.get(a)) return [back, level, a, b];
      if (this.row.get(b) > this.row.get(a)) return [down, level, a, b];
      if (this.col.get(b) - this.col.get(a) === 1 || this.col.get(a) - this.col.get(b) === 1) return [across, level, a, b];
      return [lane, level, a, b];
    }
  }

  // member gives the state of a level that is the state named or holds it.
  member(level, name) {
    if (!this.states.has(name)) return "";
    while (name !== "" && name !== undefined) {
      if (this.parent.get(name) === level) return name;
      name = this.parent.get(name);
    }
    return "";
  }

  // order puts the states of a level, and of each level inside it, in rows
  // by how many transitions each is from the state the level starts in.
  order(level) {
    const members = this.kidsOf(level);
    for (const m of members) if (this.holds(m)) this.order(m);
    if (members.length === 0) return;
    const next = new Map();
    for (const r of this.all) {
      const a = this.member(level, r.From), b = this.member(level, r.To);
      if (a !== "" && b !== "" && a !== b) {
        if (!next.has(a)) next.set(a, []);
        if (!next.get(a).includes(b)) next.get(a).push(b);
      }
    }
    const rows = [];
    const seen = new Set();
    const visit = (first, base) => {
      seen.add(first);
      const at = new Map([[first, base]]);
      const queue = [first];
      while (queue.length > 0) {
        const n = queue.shift();
        while (rows.length <= at.get(n)) rows.push([]);
        rows[at.get(n)].push(n);
        for (const m of next.get(n) ?? []) {
          if (!seen.has(m)) {
            seen.add(m);
            at.set(m, at.get(n) + 1);
            queue.push(m);
          }
        }
      }
    };
    let start = this.start.get(level);
    if (!members.includes(start)) start = members[0];
    visit(start, 0);
    for (const m of members) if (!seen.has(m)) visit(m, rows.length);
    for (let r = 0; r < rows.length; r++) {
      rows[r] = arrange(rows[r], members, next);
      rows[r].forEach((n, i) => {
        this.row.set(n, r);
        this.col.set(n, i);
      });
    }
    this.rows.set(level, rows);
  }

  rowsOf(level) {
    return this.rows.get(level) ?? [];
  }

  // lay places the states of a level in its rows, each relative to the
  // level, and gives the level's size. The gaps in a row are wide enough for
  // the chips between its states, and the gap below a row has room for its
  // tracks.
  lay(level) {
    const rows = this.rowsOf(level);
    if (rows.length === 0) return [0, 0];
    const sz = new Map();
    for (const c of rows) {
      for (const m of c) {
        const s = this.states.get(m);
        const pad = this.pad.get(m) ?? 0;
        if (this.holds(m)) {
          const [w, h] = this.lay(m);
          sz.set(m, { x: 0, y: 0, w: Math.max(w, cardW) + 2 * (inner + pad), h: h + this.top(m) + inner });
        } else if (s.Final) {
          const b = size(this.items.get(m));
          sz.set(m, { x: 0, y: 0, w: b.w + 8, h: b.h + 8 });
        } else {
          sz.set(m, size(this.items.get(m)));
        }
      }
    }
    const gap = (c, i) => Math.max(sideGap, this.room.get(pair(c[i], c[i + 1])) ?? 0);
    const acrossOf = c => {
      let used = 0;
      c.forEach((n, i) => {
        used += sz.get(n).w;
        if (i < c.length - 1) used += gap(c, i);
      });
      return used;
    };
    let total = 0;
    for (const c of rows) total = Math.max(total, acrossOf(c));
    const gapY = new Array(rows.length).fill(0);
    this.gapY.set(level, gapY);
    let y = 0, height = 0;
    rows.forEach((c, r) => {
      let high = 0;
      let x = (total - acrossOf(c)) / 2;
      c.forEach((n, i) => {
        const b = { ...sz.get(n) };
        b.x = x;
        b.y = y;
        this.at.set(n, b);
        if (i < c.length - 1) x += b.w + gap(c, i);
        high = Math.max(high, b.h);
      });
      const highs = this.high.get(level) ?? [];
      const tracks = r < highs.length ? highs[r] : 0;
      gapY[r] = y + high + rowGap;
      height = y + high;
      if (tracks > 0) height = y + high + rowGap + trackGap * (tracks - 1) + rowGap / 2;
      y += high + 2 * rowGap + trackGap * Math.max(tracks - 1, 0);
    });
    return [total, height];
  }

  // top gives the room above the states a state holds: its name and its
  // notes.
  top(name) {
    return innerTop + this.items.get(name).notes.length * noteH;
  }

  // place turns where each state of a level is drawn into where it is on
  // the drawing, the level being at ox, oy.
  place(level, ox, oy) {
    this.origin.set(level, { x: ox, y: oy });
    for (const n of this.kidsOf(level)) {
      const b = this.at.get(n);
      if (!b) continue;
      b.x += ox;
      b.y += oy;
      if (this.holds(n)) this.place(n, b.x + inner + (this.pad.get(n) ?? 0), b.y + this.top(n));
    }
  }

  // paint draws the states of a level, a holder before what it holds.
  paint(d, level) {
    for (const n of this.kidsOf(level)) {
      const b = this.at.get(n), s = this.states.get(n);
      d.reach(b);
      let cls = ("state " + s.Mark).trim();
      if (s.Send) cls += " sends";
      if (s.Final) {
        cls += " final";
        if (s.Fails && s.Mark !== "failed") cls += " failed";
      }
      if (this.holds(n)) {
        d.holder(cls + " holder", b, n);
        this.items.get(n).notes.forEach((note, i) => {
          d.shapes.push({ kind: "note", key: "note " + d.shapes.length, x: b.x + 14, y: b.y + 22 + (i + 1) * noteH, text: note.text });
        });
      } else if (s.Final) {
        const inside = { x: b.x + 4, y: b.y + 4, w: b.w - 8, h: b.h - 8 };
        d.paint(this.items.get(n), inside, cls, cardR - 4, [{ key: 0, class: "outer", x: b.x, y: b.y, w: b.w, h: b.h, rx: cardR }], "state " + n);
      } else {
        d.paint(this.items.get(n), b, cls, cardR, [], "state " + n);
      }
      if (this.start.get(level) === n) {
        d.shapes.push({ kind: "start", key: "start " + d.shapes.length, x: cx(b), y: b.y - 24, d: `M${num(cx(b))} ${num(b.y - 20)} V${num(b.y)}` });
      }
      this.paint(d, n);
    }
  }
}

function drawChart(c) {
  const ch = new Chart(c);
  ch.order("");
  ch.write();
  ch.list = ch.arrows();
  plan(ch);
  // Where lines run across and how many tracks each gap needs follow from
  // where the states are across, so the states are placed twice: the second
  // time with room for the tracks.
  ch.lay("");
  ch.place("", margin, margin + 30);
  assign(ch);
  ch.lay("");
  ch.place("", margin, margin + 30);

  const routes = ch.list.map(a => (a.kind !== none ? simplify(route(ch, a)) : null));
  const d = new Drawn();
  const chips = chipsOf(ch, routes);
  // A transition just taken is drawn last, over any arrow it shares a line
  // with.
  const order = ch.list.map((_, i) => i);
  order.sort((i, j) => cmp(taken(ch.list[i]) ? 1 : 0, taken(ch.list[j]) ? 1 : 0));
  for (const i of order) {
    if (!routes[i]) continue;
    const a = ch.list[i];
    const cls = arrowClass(a);
    const joins = { from: "state " + a.from, to: "state " + a.to };
    d.line(routes[i], cls, joins);
    d.chip(chips[i].x, chips[i].y, arrowLabel(a), cls, joins);
  }
  ch.paint(d, "");
  return d;
}

// arrange orders a row so that its states joined by transitions are side by
// side: first as few transitions as can be to states not beside, then as few
// lines below the row at any place, then as few states passed, then as few
// transitions to the left between states side by side, so that a chain reads
// left to right. Of orders as good, the one nearest the order of the level's
// states, by name, is taken. A row with no transitions in it stays in the
// order found.
function arrange(found, members, next) {
  const row = [...found].sort((a, b) => members.indexOf(a) - members.indexOf(b));
  const pairs = [];
  row.forEach((a, i) => row.forEach((b, j) => {
    if (i !== j && (next.get(a) ?? []).includes(b)) pairs.push([i, j]);
  }));
  const n = row.length;
  if (pairs.length === 0) return found;
  const cost = seq => {
    const at = new Array(n);
    seq.forEach((i, p) => { at[i] = p; });
    const cover = new Array(n).fill(0);
    const c = [0, 0, 0, 0];
    for (const [x, y] of pairs) {
      const lo = Math.min(at[x], at[y]), hi = Math.max(at[x], at[y]);
      if (hi - lo < 2) {
        if (at[y] < at[x]) c[3]++;
        continue;
      }
      c[0]++;
      c[2] += hi - lo - 1;
      for (let k = lo; k <= hi; k++) {
        cover[k]++;
        c[1] = Math.max(c[1], cover[k]);
      }
    }
    return c;
  };
  const less = (a, b) => {
    for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return a[i] < b[i];
    return false;
  };
  let best = row.map((_, i) => i);
  let least = cost(best);
  if (n <= 7) {
    // Every order, in order from the one found.
    const seq = [], used = new Array(n).fill(false);
    const tryAll = () => {
      if (seq.length === n) {
        const c = cost(seq);
        if (less(c, least)) {
          least = c;
          best = [...seq];
        }
        return;
      }
      for (let i = 0; i < n; i++) {
        if (!used[i]) {
          used[i] = true;
          seq.push(i);
          tryAll();
          seq.pop();
          used[i] = false;
        }
      }
    };
    tryAll();
  } else {
    // Two states side by side change places while that makes it better.
    for (let round = 0, better = true; better && round < n; round++) {
      better = false;
      for (let p = 0; p + 1 < n; p++) {
        [best[p], best[p + 1]] = [best[p + 1], best[p]];
        const c = cost(best);
        if (less(c, least)) {
          least = c;
          better = true;
        } else {
          [best[p], best[p + 1]] = [best[p + 1], best[p]];
        }
      }
    }
  }
  return best.map(i => row[i]);
}

// How the arrows of a statechart run between its states, where every arrow
// has a line of its own:
//
//   - An arrow to a state in a row below runs down from its state, along a
//     track in the gap above that row, and down into the state. The arrows
//     from one state share its line and its track, and divide from them.
//   - Tracks that would overlap are put one below another.
//   - An arrow to the state beside it in a row runs straight across, one line
//     each way; an arrow to a state further along runs on a track below the
//     row.
//   - Where a state inside a holder has states below it, an arrow leaving the
//     holder runs down a line of its own at the side of the holder. An arrow
//     into a state with states above it runs down the holder's right side,
//     clear of the holder's name.
//   - Each chip is on its own arrow, clear of chips and cards, and 12 from
//     other arrows.

// A chan names a line down the side of a holder: its holder, the line, and
// the side, right or left.
const chan = (holder, key, isRight) => holder + "\u0000" + key + "\u0000" + (isRight ? "r" : "l");

// A track names a track: the gap below a row of a level, and the line on it.
const trackKey = (level, gap, key) => level + "\u0000" + gap + "\u0000" + key;

// toRight says b is right of a, or level with it, among the rows of a level.
function toRight(ch, level, a, b) {
  const pos = n => (ch.col.get(n) + 0.5) / ch.rowsOf(level)[ch.row.get(n)].length;
  return pos(b) >= pos(a);
}

// plan settles how each arrow is drawn, the room each gap in a row needs for
// its chips, and the lines down the sides of holders.
function plan(ch) {
  const pairs = new Map();
  for (let i = 0; i < ch.list.length; i++) {
    const a = ch.list[i];
    [a.kind, a.level, a.a, a.b] = ch.kindOf(a.rules[0]);
    const id = a.from + "→" + a.to;
    if (a.kind === across) {
      a.right = ch.col.get(a.b) > ch.col.get(a.a);
      const k = pair(a.from < a.to ? a.from : a.to, a.from < a.to ? a.to : a.from);
      if (!pairs.has(k)) pairs.set(k, []);
      pairs.get(k).push(i);
      let l = a.a, r = a.b;
      if (!a.right) [l, r] = [r, l];
      ch.room.set(pair(l, r), Math.max(ch.room.get(pair(l, r)) ?? 0, chipWidth(arrowLabel(a)) + 24));
    } else if (a.kind === down) {
      a.right = toRight(ch, a.level, a.a, a.b);
      side(ch, a.from, a.level, "out " + a.from, a.right, false);
      side(ch, a.to, a.level, "in " + id, true, true);
    } else if (a.kind === lane) {
      a.right = ch.col.get(a.b) > ch.col.get(a.a);
      side(ch, a.from, a.level, "out " + a.from, a.right, false);
      side(ch, a.to, a.level, "up " + id, !a.right, false);
    }
  }
  // Between two states, the arrow to the right runs above the arrow back.
  for (const list of pairs.values()) {
    list.sort((i, j) => (ch.list[i].right === ch.list[j].right ? 0 : ch.list[i].right ? -1 : 1));
    list.forEach((i, n) => {
      ch.list[i].line = n;
      ch.list[i].of = list.length;
    });
  }
  // The lines down a holder's side: out from the states inside, those
  // leaving lower rows first, then those going into higher rows, so that
  // fewer cross.
  ch.sideway.sort((a, b) => {
    if (a.in !== b.in && !a.in) return -1;
    if (a.in !== b.in) return 1;
    if (a.in) return a.row - b.row;
    return b.row - a.row;
  });
  const count = new Map();
  for (const s of ch.sideway) {
    const sideKey = chan(s.holder, "", s.right);
    const c = count.get(sideKey) ?? 0;
    ch.chans.set(chan(s.holder, s.key, s.right), c);
    count.set(sideKey, c + 1);
    ch.pad.set(s.holder, Math.max(ch.pad.get(s.holder) ?? 0, chanGap * (c + 1)));
  }
}

// side asks for a line down the side of each holder between a state and a
// level, where states are drawn between them: below the state, for a line
// out of it, or above it, for a line in.
function side(ch, s, level, key, isRight, isIn) {
  for (let h = ch.parent.get(s); h !== level; h = ch.parent.get(h)) {
    const r = ch.row.get(ch.member(h, s));
    if ((isIn && r === 0) || (!isIn && r === ch.rowsOf(h).length - 1)) continue;
    if (!ch.sideway.some(w => w.holder === h && w.key === key && w.right === isRight)) {
      ch.sideway.push({ holder: h, key, right: isRight, in: isIn, row: r });
    }
  }
}

const has = (ch, holder, key, isRight) => ch.chans.has(chan(holder, key, isRight));

// chanX gives where a line down the side of a holder is.
function chanX(ch, holder, key, isRight) {
  const b = ch.at.get(holder), out = 14 + chanGap * ch.chans.get(chan(holder, key, isRight));
  const pad = ch.pad.get(holder) ?? 0;
  if (isRight) return right(b) - inner - pad + out;
  return b.x + inner + pad - out;
}

// trackY gives where a track is, and notes how far along it a line runs
// while the tracks are being counted.
function trackY(ch, level, gap, key, ...xs) {
  const k = trackKey(level, gap, key);
  if (ch.runs) {
    let run = ch.runs.get(k)?.run ?? [Infinity, -Infinity];
    for (const x of xs) run = [Math.min(run[0], x), Math.max(run[1], x)];
    ch.runs.set(k, { level, gap, key, run });
  }
  return ch.origin.get(level).y + ch.gapY.get(level)[gap] + trackGap * (ch.track.get(k) ?? 0);
}

// assign settles where each arrow enters its state, and gives each run along
// a gap a track, apart from the tracks of the runs it would overlap.
function assign(ch) {
  entries(ch);
  ch.runs = new Map();
  for (const a of ch.list) if (a.kind !== none) route(ch, a);
  const gaps = new Map();
  for (const t of ch.runs.values()) {
    if (t.run[1] - t.run[0] > 0.5) {
      const g = trackKey(t.level, t.gap, "");
      if (!gaps.has(g)) gaps.set(g, []);
      gaps.get(g).push(t);
    }
  }
  for (const keys of gaps.values()) {
    // Shorter runs nearer the row, so that fewer lines cross.
    keys.sort((a, b) => cmp(a.run[1] - a.run[0], b.run[1] - b.run[0]) || cmp(a.run[0], b.run[0]) || cmp(a.key, b.key));
    const tracks = [];
    for (const k of keys) {
      const run = k.run;
      let j = 0;
      for (; j < tracks.length; j++) {
        if (!tracks[j].some(o => run[0] < o[1] + 16 && o[0] < run[1] + 16)) break;
      }
      if (j === tracks.length) tracks.push([]);
      tracks[j].push(run);
      ch.track.set(trackKey(k.level, k.gap, k.key), j);
    }
    const level = keys[0].level, gap = keys[0].gap;
    const high = ch.high.get(level) ?? [];
    while (high.length < ch.rowsOf(level).length) high.push(0);
    high[gap] = tracks.length;
    ch.high.set(level, high);
  }
  ch.runs = null;
  stubs(ch);
}

// inside says a state is inside a holder, at any depth; every state is
// inside the top, "".
function inside(ch, s, holder) {
  while (s !== "" && s !== undefined) {
    s = ch.parent.get(s);
    if (s === holder) return true;
  }
  return false;
}

const keyOf = a => pair(a.from, a.to);

// entries settles where each arrow from above enters its state: in the
// middle where it is the only one, and otherwise side by side in the order
// they come from, clear of the start's arrow and of lines that pass through.
// Where it can, a set of more than one moves so that one arrow goes straight
// down.
function entries(ch) {
  const into = new Map();
  ch.list.forEach((a, i) => {
    if (a.kind === down) {
      if (!into.has(a.to)) into.set(a.to, []);
      into.get(a.to).push(i);
    }
  });
  const depth = n => {
    let d = 0;
    for (; n !== "" && n !== undefined; n = ch.parent.get(n)) d++;
    return d;
  };
  const targets = [...into.keys()].sort();
  // The states inside a holder first, so that its own arrows keep clear of
  // the lines that pass through its top.
  targets.sort((a, b) => depth(b) - depth(a));
  for (const t of targets) {
    const b = ch.at.get(t);
    const avoid = [];
    if (ch.start.get(ch.parent.get(t)) === t) avoid.push(cx(b));
    for (const a of ch.list) {
      if (a.kind === down && a.to !== t && inside(ch, a.to, t) && inside(ch, t, a.level)) {
        for (const p of enter(ch, a, ch.entry.get(keyOf(a)) ?? 0)) {
          if (Math.abs(p.y - b.y) < 1) avoid.push(p.x);
        }
      }
    }
    const list = into.get(t);
    const from = new Map();
    for (const i of list) from.set(i, approach(ch, ch.list[i]));
    list.sort((i, j) => cmp(from.get(i), from.get(j)));
    const lo = b.x + 12, hi = right(b) - 12;
    const clear = xs => xs.every(x => !(x < lo || x > hi || avoid.some(v => Math.abs(v - x) < 10)));
    // Far enough apart for a chip on a line straight down between two.
    let apart = 20;
    for (const i of list) apart = Math.max(apart, chipWidth(arrowLabel(ch.list[i])) / 2 + lineApart + 2);
    if (list.length > 1) apart = Math.min(apart, (hi - lo) / (list.length - 1));
    const at = shift => list.map((_, n) => cx(b) + (n - (list.length - 1) / 2) * apart + shift);
    let xs = at(0);
    for (let step = 1; !clear(xs) && step < 20; step++) {
      xs = at(Math.trunc((step + 1) / 2) * 10 * (1 - 2 * (step % 2)) * -1);
    }
    let best = Infinity;
    let straight = null;
    list.forEach((i, n) => {
      const shift = from.get(i) - xs[n];
      if (list.length === 1 || from.get(i) < lo || from.get(i) > hi || Math.abs(shift) >= Math.abs(best)) return;
      const moved = xs.map(x => x + shift);
      if (clear(moved)) {
        best = shift;
        straight = moved;
      }
    });
    if (straight) xs = straight;
    list.forEach((i, n) => ch.entry.set(keyOf(ch.list[i]), xs[n]));
  }
}

// approach gives the x an arrow comes from as it enters its state: the line
// down the side of a holder it enters, or the line it leaves its own state
// by.
function approach(ch, a) {
  for (let h = ch.parent.get(a.to); h !== a.level; h = ch.parent.get(h)) {
    const key = "in " + a.from + "→" + a.to;
    if (has(ch, h, key, true)) return chanX(ch, h, key, true);
  }
  const out = exit(ch, a.from, a.level, portX(ch, a.from), "out " + a.from, a.from, a.right);
  return out[out.length - 1].x;
}

// stubs settles where each arrow along a row enters its state from below:
// beside the middle, on the side it comes from, those on lower tracks
// nearer the middle, so that they do not cross.
function stubs(ch) {
  const sides = new Map();
  ch.list.forEach((a, i) => {
    if (a.kind === lane) {
      const s = a.right ? "left" : "right";
      const k = pair(a.to, s);
      if (!sides.has(k)) sides.set(k, { to: a.to, side: s, list: [] });
      sides.get(k).list.push(i);
    }
  });
  for (const { to, side: s, list } of sides.values()) {
    const depth = i => {
      const a = ch.list[i];
      return ch.track.get(trackKey(a.level, ch.row.get(a.a), "lane " + a.from + "→" + a.to)) ?? 0;
    };
    list.sort((i, j) => depth(j) - depth(i));
    const sign = s === "right" ? 1 : -1;
    list.forEach((i, n) => ch.stub.set(keyOf(ch.list[i]), portX(ch, to) + sign * 16 * (n + 1)));
  }
}

// portX gives where the arrows from a state leave its bottom: its middle,
// or for a holder, the nearest place clear of the lines from the states
// inside it that leave through its bottom.
function portX(ch, s) {
  const b = ch.at.get(s);
  if (!ch.holds(s)) return cx(b);
  const used = [];
  for (const a of ch.list) {
    if ((a.kind !== down && a.kind !== lane) || !inside(ch, a.from, s) || !inside(ch, s, a.level) || has(ch, s, "out " + a.from, a.right)) continue;
    const out = exit(ch, a.from, s, portX(ch, a.from), "out " + a.from, a.from, a.right);
    used.push(out[out.length - 1].x);
  }
  let x = cx(b);
  for (let step = 1; used.some(t => Math.abs(t - x) < 16); step++) {
    x = cx(b) + Math.trunc((step + 1) / 2) * 16 * (1 - 2 * (step % 2)) * -1;
  }
  return x;
}

// route gives the points an arrow runs through, from the state it leaves to
// the state it goes to.
function route(ch, a) {
  const id = a.from + "→" + a.to;
  if (a.kind === across) return beside(ch, a);
  if (a.kind === lane) {
    const out = exit(ch, a.from, a.level, portX(ch, a.from), "out " + a.from, a.from, a.right);
    const x = ch.stub.has(keyOf(a)) ? ch.stub.get(keyOf(a)) : cx(ch.at.get(a.to));
    const up = exit(ch, a.to, a.level, x, "up " + id, "up " + id, !a.right);
    const p = out[out.length - 1], q = up[up.length - 1];
    const y = trackY(ch, a.level, ch.row.get(a.a), "lane " + id, p.x, q.x);
    const pts = [...out, { x: p.x, y }, { x: q.x, y }];
    for (let i = up.length - 1; i >= 0; i--) pts.push(up[i]);
    return pts;
  }
  const out = exit(ch, a.from, a.level, portX(ch, a.from), "out " + a.from, a.from, a.right);
  const x = ch.entry.has(keyOf(a)) ? ch.entry.get(keyOf(a)) : cx(ch.at.get(a.to));
  const into = enter(ch, a, x);
  const p = out[out.length - 1], q = into[0];
  const y = trackY(ch, a.level, ch.row.get(a.a), a.from, p.x, q.x);
  return [...out, { x: p.x, y }, { x: q.x, y }, ...into];
}

// exit gives the line down from a state at x to the bottom of the state of
// a level it is in: straight down, or where states are drawn below it in a
// holder, along a track to the side of the holder and down it.
function exit(ch, s, level, x, key, track, isRight) {
  const pts = [{ x, y: bottom(ch.at.get(s)) }];
  for (let h = ch.parent.get(s); h !== level; h = ch.parent.get(h)) {
    const hb = ch.at.get(h), last = pts[pts.length - 1];
    if (!has(ch, h, key, isRight)) {
      pts.push({ x: last.x, y: bottom(hb) });
      continue;
    }
    const x2 = chanX(ch, h, key, isRight);
    const y = trackY(ch, h, ch.row.get(ch.member(h, s)), track, last.x, x2);
    pts.push({ x: last.x, y }, { x: x2, y }, { x: x2, y: bottom(hb) });
  }
  return pts;
}

// enter gives the line from the top of the state of the arrow's level that
// its state is in, down to its state at x: straight down, or where states
// are drawn above it in a holder, down the side of the holder and along a
// track to it.
function enter(ch, a, x) {
  const key = "in " + a.from + "→" + a.to;
  let pts = [{ x, y: ch.at.get(a.to).y }];
  for (let h = ch.parent.get(a.to); h !== a.level; h = ch.parent.get(h)) {
    const hb = ch.at.get(h), first = pts[0];
    if (!has(ch, h, key, true)) {
      pts = [{ x: first.x, y: hb.y }, ...pts];
      continue;
    }
    const x2 = chanX(ch, h, key, true);
    const y = trackY(ch, h, ch.row.get(ch.member(h, a.to)) - 1, key, first.x, x2);
    pts = [{ x: x2, y: hb.y }, { x: x2, y }, { x: first.x, y }, ...pts];
  }
  return pts;
}

// beside gives the line from a state to the state beside it: straight
// across, each line between the two apart from the others, or where the two
// are not level, across, down the gap between them, and across.
function beside(ch, a) {
  const s = ch.at.get(a.from), t = ch.at.get(a.to);
  const off = (a.line - (a.of - 1) / 2) * trackGap;
  let x1 = right(s), x2 = t.x;
  let gx = (right(ch.at.get(a.a)) + ch.at.get(a.b).x) / 2;
  if (!a.right) {
    x1 = s.x;
    x2 = right(t);
    gx = (right(ch.at.get(a.b)) + ch.at.get(a.a).x) / 2;
  }
  const lo = Math.max(s.y, t.y), hi = Math.min(bottom(s), bottom(t));
  const y = (lo + hi) / 2 + off;
  if (y > lo + 8 && y < hi - 8) return [{ x: x1, y }, { x: x2, y }];
  const y1 = cy(s) + off, y2 = cy(t) + off;
  return [{ x: x1, y: y1 }, { x: gx + off / 2, y: y1 }, { x: gx + off / 2, y: y2 }, { x: x2, y: y2 }];
}

// meets says two boxes overlap.
const meets = (a, b) => a.x < right(b) && b.x < right(a) && a.y < bottom(b) && b.y < bottom(a);

// crosses says a line from p to q, across or down, passes through a box.
function crosses(b, p, q) {
  if (p.y === q.y) return p.y > b.y && p.y < bottom(b) && Math.max(p.x, q.x) > b.x && Math.min(p.x, q.x) < right(b);
  return p.x > b.x && p.x < right(b) && Math.max(p.y, q.y) > b.y && Math.min(p.y, q.y) < bottom(b);
}

// chipsOf gives where each arrow's chip goes: on the arrow, as near the
// middle of its last run across as is clear of cards, other arrows and chips
// placed before it; on its last run down where no run across is clear.
function chipsOf(ch, routes) {
  const cards = [];
  const walls = [];
  for (const n of [...ch.at.keys()].sort()) {
    const b = ch.at.get(n);
    if (!ch.holds(n)) {
      cards.push(b);
      continue;
    }
    let h = 28 + nameW * runes(n);
    for (const note of ch.items.get(n).notes) h = Math.max(h, 28 + labelWidth(note.text));
    cards.push({ x: b.x, y: b.y, w: h, h: ch.top(n) - 8 });
    walls.push([{ x: b.x, y: b.y }, { x: right(b), y: b.y }], [{ x: b.x, y: bottom(b) }, { x: right(b), y: bottom(b) }],
      [{ x: b.x, y: b.y }, { x: b.x, y: bottom(b) }], [{ x: right(b), y: b.y }, { x: right(b), y: bottom(b) }]);
  }
  for (const [level, s] of ch.start) {
    const b = ch.at.get(s);
    if (b && ch.kidsOf(level).includes(s)) cards.push({ x: cx(b) - 5, y: b.y - 29, w: 10, h: 29 });
  }
  const lines = [];
  const owner = [];
  routes.forEach((r, i) => {
    if (!r) return;
    for (let k = 1; k < r.length; k++) {
      lines.push([r[k - 1], r[k]]);
      owner.push(i);
    }
  });
  const at = new Array(routes.length).fill(null);
  const placed = [];
  routes.forEach((r, i) => {
    if (!r) return;
    const w = chipWidth(arrowLabel(ch.list[i]));
    // What a chip on this arrow could meet: what is near the arrow.
    let nx = Infinity, ny = Infinity, fx = -Infinity, fy = -Infinity;
    for (const p of r) {
      nx = Math.min(nx, p.x);
      ny = Math.min(ny, p.y);
      fx = Math.max(fx, p.x);
      fy = Math.max(fy, p.y);
    }
    const near = { x: nx - w - 4, y: ny - 24, w: fx - nx + 2 * w + 8, h: fy - ny + 48 };
    const boxes = [...cards, ...placed].filter(b => meets(near, b));
    const isNear = l => Math.max(l[0].x, l[1].x) >= near.x && Math.min(l[0].x, l[1].x) <= right(near) &&
      Math.max(l[0].y, l[1].y) >= near.y && Math.min(l[0].y, l[1].y) <= bottom(near);
    const others = lines.filter((l, k) => owner[k] !== i && isNear(l));
    const sides = walls.filter(isNear);
    // A chip is 2 apart from cards, holders and other chips, and further
    // from other arrows, so that it is read as on its own arrow; where no
    // place is that far, 2 apart from them too.
    const free = apart => c => {
      const rb = { x: c.x - w / 2 - 2, y: c.y - 11, w: w + 4, h: 22 };
      const lb = { x: c.x - w / 2 - apart, y: c.y - 9 - apart, w: w + 2 * apart, h: 18 + 2 * apart };
      return !boxes.some(b => meets(rb, b)) && !sides.some(o => crosses(rb, o[0], o[1])) && !others.some(o => crosses(lb, o[0], o[1]));
    };
    let spotsOf = [];
    for (const isAcross of [true, false]) {
      for (let k = r.length - 1; k > 0; k--) {
        const p = r[k - 1], q = r[k];
        if ((p.y === q.y) === isAcross) spotsOf = spotsOf.concat(spots(p, q, w, k === r.length - 1));
      }
    }
    if (spotsOf.length === 0) {
      const p = r[r.length - 2], q = r[r.length - 1];
      spotsOf = [{ x: (p.x + q.x) / 2, y: (p.y + q.y) / 2 }];
    }
    at[i] = spotsOf.find(free(lineApart)) ?? spotsOf.find(free(2)) ?? spotsOf[0];
    placed.push({ x: at[i].x - w / 2, y: at[i].y - 9, w, h: 18 });
  });
  return at;
}

// spots gives the places along a line from p to q where a chip w wide can
// go, from its middle out; on the last run down of an arrow, from just
// above the state it enters, up.
function spots(p, q, w, last) {
  const out = [];
  if (p.y === q.y) {
    const lo = Math.min(p.x, q.x) + w / 2 + 10, hi = Math.max(p.x, q.x) - w / 2 - 10;
    if (lo > hi) return out;
    const mid = (lo + hi) / 2;
    for (let d = 0; mid - d >= lo || mid + d <= hi; d += 4) {
      if (mid + d <= hi) out.push({ x: mid + d, y: p.y });
      if (d > 0 && mid - d >= lo) out.push({ x: mid - d, y: p.y });
    }
    return out;
  }
  const lo = Math.min(p.y, q.y) + 20, hi = Math.max(p.y, q.y) - 14;
  if (lo > hi) return out;
  if (last && q.y > p.y) {
    for (let y = hi; y >= lo; y -= 4) out.push({ x: p.x, y });
    return out;
  }
  const mid = (lo + hi) / 2;
  for (let d = 0; mid - d >= lo || mid + d <= hi; d += 4) {
    if (mid + d <= hi) out.push({ x: p.x, y: mid + d });
    if (d > 0 && mid - d >= lo) out.push({ x: p.x, y: mid - d });
  }
  return out;
}

// An agent

// toolkitSize gives how big a group of what the model can call is.
function toolkitSize(g) {
  if (g.Folded) return { w: cardW, h: foldedH };
  let w = cardW, h = 0;
  if (g.Toolkit) {
    h = innerTop + inner;
    w = cardW + 2 * inner;
  }
  for (const t of g.items) {
    const b = size(t);
    h += b.h + 12;
    w = Math.max(w, b.w);
  }
  return { w, h: h - 12 };
}

// columnSize gives how big a column of groups is, one below another.
function columnSize(c) {
  let w = 0, h = 0;
  for (const g of c) {
    w = Math.max(w, g.at.w);
    h += g.at.h + 12;
  }
  return [w, h - 12];
}

// drawAgent draws three things, one below another: the model, what it can
// call, and its reply. Each group of what it can call is a column, and the
// toolkits folded are one column after the others.
function drawAgent(a) {
  const groups = (a.Toolkits ?? []).map(g => {
    const items = (g.Tools ?? []).map(t => itemOf(t.Card));
    const group = { ...g, items };
    group.at = { x: 0, y: 0, ...toolkitSize(group) };
    return group;
  });
  const columns = groups.filter(g => !g.Folded).map(g => [g]);
  const folded = groups.filter(g => g.Folded);
  if (folded.length > 0) columns.push(folded);
  const askItem = a.Ask ? itemOf(a.Ask) : null;

  let acrossAll = Math.max(columns.length - 1, 0) * sideGap, high = 0;
  for (const c of columns) {
    const [w, h] = columnSize(c);
    acrossAll += w;
    high = Math.max(high, h);
  }
  if (askItem) {
    acrossAll = Math.max(acrossAll, size(askItem).w);
    if (high > 0) high += 12;
    high += size(askItem).h;
  }
  const hasTools = high > 0;
  const tools = { x: 0, y: 0, w: Math.max(acrossAll + 2 * inner, cardW), h: innerTop + high + inner };
  const total = hasTools ? tools.w : cardW;
  const left = margin;
  const model = itemOf({ Icon: "model", Name: a.Model });
  const top = size(model);
  top.x = left + (total - top.w) / 2;
  top.y = margin + 18;
  const reply = size({ ...itemOf({ Name: "reply" }), narrow: true });
  reply.x = left + (total - reply.w) / 2;

  const d = new Drawn();
  let y = bottom(top);
  let cls = "";
  if (hasTools) {
    tools.x = left;
    tools.y = y + 2 * rowGap;
    d.holder("state holder", tools, "tools");
    d.reach(tools);
    let x = tools.x + inner + (tools.w - 2 * inner - acrossAll) / 2;
    const ty = tools.y + innerTop;
    for (const c of columns) {
      let gy = ty, w = 0;
      for (const g of c) {
        g.at.x = x;
        g.at.y = gy;
        gy += g.at.h + 12;
        w = Math.max(w, g.at.w);
        if (paintGroup(d, g)) cls = "taken";
      }
      x += w + sideGap;
    }
    if (askItem) {
      const b = size(askItem);
      b.x = cx(tools) - b.w / 2;
      b.y = bottom(tools) - inner - b.h;
      d.paint(askItem, b, ("state ask " + a.AskMark).trim(), cardR, [], "ask");
    }
    d.down(top, tools, cls, `${a.Turns} turns`, { from: "model", to: "holder tools" });
    y = bottom(tools);
  }
  reply.y = y + (2 * rowGap) / 1.5;
  d.down(hasTools ? tools : top, reply, "", "", { from: hasTools ? "holder tools" : "model", to: "reply" });
  d.paint(model, top, ("state model " + a.Mark).trim(), cardR, [], "model");
  if (a.Turn > 0) d.shapes.push({ kind: "tag", key: "tag", x: top.x + 4, y: top.y - 8, text: `turn ${a.Turn}` });
  d.paint(itemOf({ Name: "reply" }), reply, "state reply", cardR, [], "reply");
  return d;
}

// paintGroup draws a group of what the model can call, and says whether a
// tool in it is one the agent waits on. A toolkit's name is cut to its
// holder, and its tooltip gives the file it is included from and its
// `when:`. A toolkit or a tool with `when:` has a dashed border.
function paintGroup(d, g) {
  const b = g.at;
  let y = b.y;
  let cls = "state holder toolkit";
  if (g.When) cls += " when";
  const name = clip(g.Name, Math.trunc((b.w - 28) / nameW));
  if (g.Folded) {
    const n = g.items.length;
    d.holder(cls + " folded", b, name, { x: b.x + 14, y: b.y + 39, text: `${n} ${n === 1 ? "tool" : "tools"}` }, titleOf(g.items.map(t => t.name).join(", "), g.When));
    d.reach(b);
    return false;
  }
  if (g.Toolkit) {
    d.holder(cls, b, name, null, titleOf(g.File, g.When));
    y += innerTop;
  }
  d.reach(b);
  let now = false;
  g.Tools.forEach((t, i) => {
    now = now || t.Mark === "now" || t.Mark === "held";
    const at = size(g.items[i]);
    at.x = cx(b) - at.w / 2;
    at.y = y;
    let c = ("state tool " + t.Mark).trim();
    if (t.When) c += " when";
    d.paint(g.items[i], at, c, cardR, [], "tool " + t.Card.Name);
    y += at.h + 12;
  });
  return now;
}

// titleOf gives a tooltip of lines: those given, and `when:` where there is
// a condition.
function titleOf(first, when) {
  const lines = [];
  if (first) lines.push(first);
  if (when) lines.push("when: " + when);
  return lines.join("\n");
}

// A decision

// drawDecision draws two things, one below the other: the model, and the
// answers, with each question's name.
function drawDecision(dec) {
  const model = itemOf({ Icon: "model", Name: dec.Model });
  const answers = itemOf(dec.Answers);
  const top = size(model), below = size(answers);
  const total = Math.max(top.w, below.w);
  top.x = margin + (total - top.w) / 2;
  top.y = margin + 18;
  below.x = margin + (total - below.w) / 2;
  below.y = bottom(top) + 2 * rowGap;
  const d = new Drawn();
  d.down(top, below, "", "", { from: "model", to: "answers" });
  d.paint(model, top, ("state model " + dec.Mark).trim(), cardR, [], "model");
  d.paint(answers, below, "state reply", cardR, [], "answers");
  return d;
}
