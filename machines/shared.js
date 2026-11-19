// What the Machines and Toolkits pages share: the problems Corbie finds, a
// file's text, and the address of a file on the Machines page.
import { html, plural } from "ui";

// machineHref gives the address of a file on the Machines page, on a tab
// other than its drawing where view names one.
export const machineHref = (file, view) => "/machines?" + new URLSearchParams(view ? { file, view } : { file });

// base gives the last name of a path.
export const base = p => p.slice(p.lastIndexOf("/") + 1);

function Problem(props) {
  const p = () => props.problem;
  return html`<div class="problem">
    <Show when=${() => p().Top} fallback=${() => html`<a class="at" href=${() => machineHref(p().File, "text")} title=${() => p().File}>${() => p().File}${() => (p().Line ? ":" + p().Line : "")}</a><p>${() => (p().Place ? html`<span class="place">${() => p().Place}</span>${" "}` : "")}${() => p().Text}</p>`}>
      <p><code>${() => p().File}/</code> is the top of a tree, and its machines read their paths from there: start the UI in <code>${() => p().File}/</code>.</p>
    </Show>
    ${() => (p().Breaks.length ? html`<small title=${() => p().Breaks.join(", ")}>breaks ${() => plural(p().Breaks.length, "machine")}</small>` : "")}
  </div>`;
}

// Problems is the problems Corbie found, each once.
export function Problems(props) {
  return html`<Show when=${() => props.problems.length}>
    <div class="problems"><h2>${() => plural(props.problems.length, "problem")}</h2><For each=${() => props.problems}>${p => html`<${Problem} problem=${() => p} />`}</For></div>
  </Show>`;
}

function Span(props) {
  const s = () => props.span;
  return html`<Show when=${() => s().Href} fallback=${() => (s().Class ? html`<span class=${() => s().Class}>${() => s().Text}</span>` : s().Text)}><a class=${() => s().Class} href=${() => s().Href}>${() => s().Text}</a></Show>`;
}

// Code is a file's text, read only, a line at a time, the lines a problem
// is found at marked.
export function Code(props) {
  return html`<pre class=${() => ["code", { written: props.written }]} aria-label=${() => props.label}><For each=${() => props.lines}>${l => html`<span class=${() => ["l", { err: l.Bad }]} id=${() => l.ID || undefined}><For each=${() => l.Spans}>${s => html`<${Span} span=${() => s} />`}</For></span>`}</For></pre>`;
}
