// The drawing of a machine, which Solid draws as SVG from the graph the UI
// reads in Go, laid out by layout.js. Every holder, arrow, chip and card is
// an element of its own, kept from one scene to the next by its key, so
// only what changed is written again.
//
// The drawing is seen through a camera: one group whose transform moves
// and scales the whole, and glides as it changes. The camera frames what
// the drawing is about (the card a page focuses, or the cards a run marks,
// or the whole machine) whenever the scene or the focus changes, until the
// person pans or zooms; Follow gives it back.
import { createMemo, createSignal, deep, onSettled } from "solid-js";
import { html } from "ui";
import { layout, num } from "./layout.js";

// pad is the room the camera leaves round what it frames.
const pad = 24;
// The camera never scales a drawing larger than its size to frame it, nor
// smaller than least, below which a person cannot read it.
const least = 0.5, smallest = 0.02, largest = 4;

// Drawing draws a scene, and draws it again as it changes. focus is the
// path of a card to frame: the names of the cards from the machine drawn
// down to it, joined by "/".
export function Drawing(props) {
  return html`<Show when=${() => props.scene}>${() => html`<${Viewport} scene=${() => props.scene} focus=${() => props.focus} />`}</Show>`;
}

function Viewport(props) {
  // The whole scene is read to lay it out, by record and not by every
  // field, as a page's parts are reconciled record by record.
  const laid = createMemo(() => layout(deep(props.scene)));
  const [size, setSize] = createSignal({ w: 0, h: 0 });
  const [following, setFollowing] = createSignal(true);
  const [placed, setPlaced] = createSignal({ x: 0, y: 0, k: 1 });
  const [moving, setMoving] = createSignal(false);
  const [hot, setHot] = createSignal(null);
  // ready says the camera has been placed once, at once: from then on it
  // glides.
  const [ready, setReady] = createSignal(false);
  // framed is where the camera would be to frame what the drawing is about.
  const framed = createMemo(() => frame(laid(), props.focus, size()));
  const camera = createMemo(() => (following() ? framed() : placed()));
  let el;

  onSettled(() => {
    const seen = new ResizeObserver(([e]) => {
      setSize({ w: e.contentRect.width, h: e.contentRect.height });
      if (!ready()) requestAnimationFrame(() => requestAnimationFrame(() => setReady(true)));
    });
    seen.observe(el);
    return () => seen.disconnect();
  });

  // take moves the camera by hand: it stops following, and moves at once
  // while the person moves it.
  function take(to, now = true) {
    setPlaced(to);
    setFollowing(false);
    setMoving(now);
  }
  function zoom(by, px = size().w / 2, py = size().h / 2, now = false) {
    const c = camera();
    const k = Math.min(largest, Math.max(smallest, c.k * by));
    take({ k, x: px - ((px - c.x) * k) / c.k, y: py - ((py - c.y) * k) / c.k }, now);
  }
  function fit() {
    take(frame(laid(), "", size(), true), false);
  }

  let still;
  function wheel(e) {
    e.preventDefault();
    clearTimeout(still);
    still = setTimeout(() => setMoving(false), 150);
    const c = camera();
    if (e.ctrlKey || e.metaKey) {
      const r = el.getBoundingClientRect();
      zoom(Math.exp(-e.deltaY * 0.004), e.clientX - r.left, e.clientY - r.top, true);
    } else {
      take({ ...c, x: c.x - e.deltaX, y: c.y - e.deltaY });
    }
  }

  // A drag pans. One that moved does not follow the link it began on.
  function press(e) {
    if (e.button !== 0 || e.target.closest(".zoom")) return;
    const start = { x: e.clientX, y: e.clientY }, from = camera();
    let dragged = false;
    const move = m => {
      const dx = m.clientX - start.x, dy = m.clientY - start.y;
      if (!dragged && Math.hypot(dx, dy) < 4) return;
      if (!dragged) el.setPointerCapture(e.pointerId);
      dragged = true;
      take({ ...from, x: from.x + dx, y: from.y + dy });
    };
    const up = () => {
      el.removeEventListener("pointermove", move);
      el.removeEventListener("pointerup", up);
      setMoving(false);
      if (dragged) el.addEventListener("click", c => { c.preventDefault(); c.stopPropagation(); }, { capture: true, once: true });
    };
    el.addEventListener("pointermove", move);
    el.addEventListener("pointerup", up);
  }

  function key(e) {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    switch (e.key) {
      case "+": case "=": zoom(1.25); break;
      case "-": case "_": zoom(0.8); break;
      case "0": fit(); break;
      default: return;
    }
    e.preventDefault();
  }

  // What the pointer is over lights what it joins: a card its arrows and
  // their chips, an arrow or a chip its two cards.
  function over(e) {
    const t = e.target.closest("[data-key]");
    setHot(t ? { key: t.dataset.key, from: t.dataset.from, to: t.dataset.to } : null);
  }

  return html`<div class="viewport" tabindex="0" ref=${e => { el = e; }} onWheel=${wheel} onPointerDown=${press} onKeyDown=${key} onPointerOver=${over} onPointerLeave=${() => setHot(null)} onPointerUp=${() => setMoving(false)}>
    <svg class="machine" role="img" aria-label=${() => laid().label}>
      <defs><marker id="ah" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path class="arrow" d="M0 0L10 5L0 10z" /></marker><marker id="at" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto-start-reverse"><path class="arrow taken" d="M0 0L10 5L0 10z" /></marker></defs>
      <g class=${() => ["camera", { moving: moving() || !ready() }]} style=${() => `transform: translate(${num(camera().x)}px, ${num(camera().y)}px) scale(${camera().k.toFixed(4)})`}>
        <${Parts} of=${() => laid()} prefix="" hot=${() => hot()} />
      </g>
    </svg>
    <div class="zoom">
      <Show when=${() => !following()}><button class="btn sm" type="button" title="frame what the drawing is about again, and go on framing it" onClick=${() => { setMoving(false); setFollowing(true); }}>Follow</button></Show>
      <button class="btn sm" type="button" title="zoom out (-)" aria-label="zoom out" onClick=${() => zoom(0.8)}>−</button>
      <button class="btn sm" type="button" title="zoom in (+)" aria-label="zoom in" onClick=${() => zoom(1.25)}>+</button>
      <button class="btn sm" type="button" title="fit the whole machine (0)" onClick=${fit}>Fit</button>
    </div>
  </div>`;
}

// lit says whether what the pointer is over lights a part: the part
// itself, a card at either end of an arrow or a chip, or an arrow or a chip
// at a card.
const lit = (hot, key, from, to) => !!hot && (hot.key === key || (from !== undefined && (hot.key === from || hot.key === to)) || hot.from === key || hot.to === key);

const byKey = part => part.key;

// Parts draws a drawing's parts in the order they are painted: holders,
// then arrows, then their chips, then the cards. Every part is named by its
// key, after the keys of the cards it is drawn inside.
function Parts(props) {
  const k = key => props.prefix + key;
  return html`<For each=${() => props.of.holders} keyed=${byKey}>${h => html`<${Holder} holder=${() => h()} prefix=${() => props.prefix} hot=${() => props.hot} />`}</For><For each=${() => props.of.edges} keyed=${byKey}>${e => html`<path class=${() => [e().class, { lit: lit(props.hot, k("edge " + e().key), k(e().from), k(e().to)) }]} d=${() => e().d} pathLength=${() => (/\btaken\b/.test(e().class) ? 1 : undefined)} marker-end=${() => `url(#${e().head})`} data-key=${() => k("edge " + e().key)} data-from=${() => k(e().from)} data-to=${() => k(e().to)} />`}</For><For each=${() => props.of.chips} keyed=${byKey}>${c => html`<${Chip} chip=${() => c()} prefix=${() => props.prefix} hot=${() => props.hot} />`}</For><For each=${() => props.of.shapes} keyed=${byKey}>${s => html`<${Shape} shape=${() => s()} prefix=${() => props.prefix} hot=${() => props.hot} />`}</For>`;
}

// Holder is a box round cards, with its name, and for a folded toolkit how
// many tools it has.
function Holder(props) {
  const h = () => props.holder;
  const key = () => props.prefix + h().key;
  return html`<g class=${() => [h().class, { lit: lit(props.hot, key()) }]} data-key=${key}><rect x=${() => num(h().x)} y=${() => num(h().y)} width=${() => num(h().w)} height=${() => num(h().h)} rx="14" /><text class="name" x=${() => num(h().name.x)} y=${() => num(h().name.y)}>${() => h().name.text}</text><Show when=${() => h().sub}>${sub => html`<text class="sub" x=${() => num(sub().x)} y=${() => num(sub().y)}>${() => sub().text}</text>`}</Show><title>${() => h().title ?? ""}</title></g>`;
}

// Chip is the label of an arrow, with the whole label as its tooltip where
// it is cut short.
function Chip(props) {
  const c = () => props.chip;
  const k = key => props.prefix + key;
  return html`<g class=${() => ["arrowchip", { lit: lit(props.hot, k("chip " + c().key), k(c().from), k(c().to)) }]} data-key=${() => k("chip " + c().key)} data-from=${() => k(c().from)} data-to=${() => k(c().to)}><rect class=${() => c().class} x=${() => num(c().x)} y=${() => num(c().y)} width=${() => num(c().w)} height="18" rx="9" /><text class="on" x=${() => num(c().tx)} y=${() => num(c().ty)} text-anchor="middle">${() => c().text}<title>${() => c().title}</title></text></g>`;
}

// Shape is a card, or what is written beside cards: a holder's notes, the
// arrow into the state a level starts in, and the turn an agent is on.
function Shape(props) {
  const s = () => props.shape;
  const kind = createMemo(() => s().kind);
  return html`${() => {
    switch (kind()) {
      case "card":
        return html`<${Card} card=${() => s()} prefix=${() => props.prefix} hot=${() => props.hot} />`;
      case "start":
        return html`<g><circle class="start" cx=${() => num(s().x)} cy=${() => num(s().y)} r="4" /><path class="edge" d=${() => s().d} marker-end="url(#ah)" /></g>`;
      case "note":
        return html`<text class="note" x=${() => num(s().x)} y=${() => num(s().y)}>${() => s().text}</text>`;
      case "tag":
        return html`<text class="tag" x=${() => num(s().x)} y=${() => num(s().y)} text-anchor="start">${() => s().text}</text>`;
    }
  }}`;
}

// Card is a card: a link where it has one, and the drawing unfolded inside
// it, whose parts are named after the card.
function Card(props) {
  const c = () => props.card;
  const key = () => props.prefix + c().key;
  const body = () => html`<g class=${() => [c().class, { lit: lit(props.hot, key()) }]} data-key=${key}>
    <For each=${() => c().behind} keyed=${byKey}>${b => html`<rect class=${() => b().class} x=${() => num(b().x)} y=${() => num(b().y)} width=${() => num(b().w)} height=${() => num(b().h)} rx=${() => num(b().rx)} />`}</For>
    <rect class="box" x=${() => num(c().box.x)} y=${() => num(c().box.y)} width=${() => num(c().box.w)} height=${() => num(c().box.h)} rx=${() => num(c().box.rx)} />
    <text class="title" x=${() => num(c().title.x)} y=${() => num(c().title.y)} text-anchor=${() => c().title.anchor}>${() => c().title.text}</text>
    <For each=${() => c().subs} keyed=${byKey}>${t => html`<text class="sub" x=${() => num(t().x)} y=${() => num(t().y)} text-anchor=${() => t().anchor}>${() => t().text}</text>`}</For>
    <For each=${() => c().notes} keyed=${byKey}>${t => html`<text class=${() => t().class} x=${() => num(t().x)} y=${() => num(t().y)} text-anchor=${() => t().anchor}>${() => t().text}</text>`}</For>
    <Show when=${() => c().tile}>${t => html`<rect class=${() => ["tile", t().icon]} x=${() => num(t().x)} y=${() => num(t().y)} width="28" height="28" rx="7" /><Show when=${() => t().spark} fallback=${() => html`<use class="icon" href=${() => "#g-" + t().icon} x=${() => num(t().x + 4)} y=${() => num(t().y + 7)} width="20" height="14" />`}>${d => html`<path class="spark" d=${d} />`}</Show>`}</Show>
    <Show when=${() => c().fold}>${f => html`<text class="fold" x=${() => num(f().x)} y=${() => num(f().y)} text-anchor="middle">${() => f().text}</text>`}</Show>
    <title>${() => c().tooltip}</title>
  </g>`;
  return html`<Show when=${() => c().href} fallback=${body}>${href => html`<g><a href=${href}>${body()}</a></g>`}</Show><Show when=${() => c().in}>${i => html`<g transform=${() => `translate(${num(i().x)} ${num(i().y)})`}><${Parts} of=${() => i()} prefix=${() => key() + " › "} hot=${() => props.hot} /></g>`}</Show>`;
}

// frame gives where the camera is to frame what a drawing is about, in a
// view of a size: the card a path focuses; or else the cards a run marks as
// going, held or failed, the deepest unfolded ones first, with the machine
// they are drawn in where it can be read whole; or else, and with whole,
// the whole drawing. The camera never scales up to frame.
export function frame(l, focus, view, whole = false) {
  const all = { x: l.left, y: 0, w: l.width, h: l.height };
  if (view.w === 0 || view.h === 0) return { x: 0, y: 0, k: 1 };
  const cards = [];
  collect(l, 0, 0, "", 0, null, cards);
  let region = all, marks = null;
  if (!whole && focus) {
    const card = cards.find(c => c.path === focus);
    if (card) region = card.box;
  } else if (!whole) {
    const marked = cards.filter(c => /\b(now|held|failed)\b/.test(c.class) && !/\bholder\b/.test(c.class));
    const deepest = Math.max(-1, ...marked.map(c => c.depth));
    if (deepest >= 0) {
      const at = marked.filter(c => c.depth === deepest);
      marks = union(at.map(c => c.box));
      region = deepest > 0 ? at[0].within : all;
    }
  }
  const fits = Math.min((view.w - 2 * pad) / region.w, (view.h - 2 * pad) / region.h);
  let k = Math.min(1, fits);
  let around = region;
  if (k < least && marks) {
    k = least;
    around = marks;
  }
  const x = view.w / 2 - k * (around.x + around.w / 2);
  // What fits whole is drawn from the top, as a page is read.
  const y = around === region && k * region.h + 2 * pad <= view.h && region === all ? pad - k * region.y : view.h / 2 - k * (around.y + around.h / 2);
  return { x, y, k };
}

// collect gives every card of a drawing, at every depth: its box on the
// whole drawing, its path, its classes, how deep it is, and the box of the
// card it is drawn inside.
function collect(parts, ox, oy, path, depth, within, out) {
  for (const s of parts.shapes) {
    if (s.kind !== "card") continue;
    const box = { x: s.box.x + ox, y: s.box.y + oy, w: s.box.w, h: s.box.h };
    const at = path ? path + "/" + s.name : s.name;
    out.push({ box, path: at, class: s.class, depth, within });
    if (s.in) collect(s.in, ox + s.in.x, oy + s.in.y, at, depth + 1, box, out);
  }
}

function union(boxes) {
  const x = Math.min(...boxes.map(b => b.x)), y = Math.min(...boxes.map(b => b.y));
  return { x, y, w: Math.max(...boxes.map(b => b.x + b.w)) - x, h: Math.max(...boxes.map(b => b.y + b.h)) - y };
}
