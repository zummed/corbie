// The drawing of a machine, which Solid draws as SVG from the graph the UI
// reads in Go, laid out by layout.js. Every holder, arrow, chip and card is
// an element of its own, and a card keeps its element while the scene
// changes, by its name.
import { createMemo } from "solid-js";
import { html } from "ui";
import { layout, num } from "./layout.js";

// Drawing draws a scene, and draws it again as it changes.
export function Drawing(props) {
  const drawn = createMemo(() => (props.scene ? layout(props.scene) : null));
  return html`<Show when=${drawn}>${d => html`<svg class="machine" viewBox=${() => `${num(d().left)} 0 ${num(d().width)} ${num(d().height)}`} width=${() => num(d().width)} height=${() => num(d().height)} role="img" aria-label=${() => d().label}>
    <defs><marker id="ah" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path class="arrow" d="M0 0L10 5L0 10z" /></marker><marker id="at" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto-start-reverse"><path class="arrow taken" d="M0 0L10 5L0 10z" /></marker></defs>
    <${Parts} of=${() => d()} />
  </svg>`}</Show>`;
}

// shapeKey names a shape: a card by its name, anything else by its kind and
// its place among the shapes.
const shapeKey = (s, i) => (s.kind === "card" ? "card " + s.name : s.kind + " " + i);

// Parts draws a drawing's parts in the order they are painted: holders,
// then arrows, then their chips, then the cards.
function Parts(props) {
  const shapes = createMemo(() => props.of.shapes.map((s, i) => ({ key: shapeKey(s, i), s })));
  return html`<For each=${() => props.of.holders} keyed=${h => h.name.text + " " + h.class.includes("folded")}>${h => html`<${Holder} holder=${() => h()} />`}</For><For each=${() => props.of.edges} keyed=${false}>${e => html`<path class=${() => e().class} d=${() => e().d} marker-end=${() => `url(#${e().head})`} />`}</For><For each=${() => props.of.chips} keyed=${false}>${c => html`<${Chip} chip=${() => c()} />`}</For><For each=${shapes} keyed=${k => k.key}>${k => html`<${Shape} shape=${() => k().s} />`}</For>`;
}

// Holder is a box round cards, with its name, and for a folded toolkit how
// many tools it has.
function Holder(props) {
  const h = () => props.holder;
  return html`<g class=${() => h().class}><rect x=${() => num(h().x)} y=${() => num(h().y)} width=${() => num(h().w)} height=${() => num(h().h)} rx="14" /><text class="name" x=${() => num(h().name.x)} y=${() => num(h().name.y)}>${() => h().name.text}</text><Show when=${() => h().sub}>${sub => html`<text class="sub" x=${() => num(sub().x)} y=${() => num(sub().y)}>${() => sub().text}</text>`}</Show><title>${() => h().title ?? ""}</title></g>`;
}

// Chip is the label of an arrow, with the whole label as its tooltip where
// it is cut short.
function Chip(props) {
  const c = () => props.chip;
  return html`<g><rect class=${() => c().class} x=${() => num(c().x)} y=${() => num(c().y)} width=${() => num(c().w)} height="18" rx="9" /><text class="on" x=${() => num(c().tx)} y=${() => num(c().ty)} text-anchor="middle">${() => c().text}<title>${() => c().title}</title></text></g>`;
}

// Shape is a card, or what is written beside cards: a holder's notes, the
// arrow into the state a level starts in, and the turn an agent is on.
function Shape(props) {
  const s = () => props.shape;
  const kind = createMemo(() => props.shape.kind);
  return html`${() => {
    switch (kind()) {
      case "card":
        return html`<${Card} card=${() => s()} />`;
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
// it.
function Card(props) {
  const c = () => props.card;
  const body = () => html`<g class=${() => c().class}>
    <For each=${() => c().behind} keyed=${false}>${b => html`<rect class=${() => b().class} x=${() => num(b().x)} y=${() => num(b().y)} width=${() => num(b().w)} height=${() => num(b().h)} rx=${() => num(b().rx)} />`}</For>
    <rect class="box" x=${() => num(c().box.x)} y=${() => num(c().box.y)} width=${() => num(c().box.w)} height=${() => num(c().box.h)} rx=${() => num(c().box.rx)} />
    <text class="title" x=${() => num(c().title.x)} y=${() => num(c().title.y)} text-anchor=${() => c().title.anchor}>${() => c().title.text}</text>
    <For each=${() => c().subs} keyed=${false}>${t => html`<text class="sub" x=${() => num(t().x)} y=${() => num(t().y)} text-anchor=${() => t().anchor}>${() => t().text}</text>`}</For>
    <For each=${() => c().notes} keyed=${false}>${t => html`<text class=${() => t().class} x=${() => num(t().x)} y=${() => num(t().y)} text-anchor=${() => t().anchor}>${() => t().text}</text>`}</For>
    <Show when=${() => c().tile}>${t => html`<rect class=${() => ["tile", t().icon]} x=${() => num(t().x)} y=${() => num(t().y)} width="28" height="28" rx="7" /><Show when=${() => t().spark} fallback=${() => html`<use class="icon" href=${() => "#g-" + t().icon} x=${() => num(t().x + 4)} y=${() => num(t().y + 7)} width="20" height="14" />`}>${d => html`<path class="spark" d=${d} />`}</Show>`}</Show>
    <Show when=${() => c().fold}>${f => html`<text class="fold" x=${() => num(f().x)} y=${() => num(f().y)} text-anchor="middle">${() => f().text}</text>`}</Show>
    <title>${() => c().tooltip}</title>
  </g>`;
  return html`<Show when=${() => c().href} fallback=${body}>${href => html`<g><a href=${href}>${body()}</a></g>`}</Show><Show when=${() => c().in}>${i => html`<g transform=${() => `translate(${num(i().x)} ${num(i().y)})`}><${Parts} of=${() => i()} /></g>`}</Show>`;
}
