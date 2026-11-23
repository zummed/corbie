// What the Machines and Toolkits pages share: a definition as text, and
// the address of a machine on the Machines page.
import { html } from "ui";

// machineHref gives the address of the machine at a location on the
// Machines page, on a tab other than its drawing where view names one.
export const machineHref = (at, view) => "/machines?" + new URLSearchParams(view ? { at, view } : { at });

// opened gives the paths of the cards an address of the Machines page
// unfolds: the names of the cards from the machine drawn down, joined by /.
export const opened = href => new URL(href, "http://ui").searchParams.getAll("open");

// focusOf gives the card a drawing moves in to as the address goes from one
// to another: the card unfolded, or, where a card was folded, the card it
// was in, "" being the whole machine; for another machine, as focusAt gives.
// It gives null where no card was unfolded or folded.
export function focusOf(from, to) {
  const at = href => new URL(href, "http://ui").searchParams.get("at");
  if (at(from) !== at(to)) return focusAt(to);
  const was = opened(from), now = opened(to);
  const deepest = list => list.reduce((a, b) => (b.split("/").length > a.split("/").length ? b : a));
  const unfolded = now.filter(p => !was.includes(p));
  if (unfolded.length) return deepest(unfolded);
  const folded = was.filter(p => !now.includes(p));
  if (!folded.length) return null;
  const top = folded.reduce((a, b) => (b.split("/").length < a.split("/").length ? b : a));
  return top.includes("/") ? top.slice(0, top.lastIndexOf("/")) : "";
}

// focusAt gives the card a drawing moves in to as a page opens at an
// address: the deepest card it unfolds, or the whole machine.
export const focusAt = href => {
  const all = opened(href);
  return all.length ? all.reduce((a, b) => (b.split("/").length > a.split("/").length ? b : a)) : "";
};

function Span(props) {
  const s = () => props.span;
  return html`<Show when=${() => s().Class} fallback=${() => s().Text}><span class=${() => s().Class}>${() => s().Text}</span></Show>`;
}

// Code is a definition as text, read only, a line at a time.
export function Code(props) {
  return html`<pre class="code" aria-label=${() => props.label}><For each=${() => props.lines}>${l => html`<span class="l"><For each=${() => l.Spans}>${s => html`<${Span} span=${() => s} />`}</For></span>`}</For></pre>`;
}
