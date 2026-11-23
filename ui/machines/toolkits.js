// The Toolkits page: the toolkits the agents of the machines recorded offer,
// by tree, and the toolkit chosen with its tools and what offers and starts
// it. Choosing a toolkit shows it in place, and the page shows the toolkit
// again when a submit changed a tree.
import { createEffect, createSignal } from "solid-js";
import { html, parts, receive, mount, Icon, Handle, plural } from "ui";
import { machineHref } from "/machines/shared.js";

const toolkitHref = id => "/toolkits?" + new URLSearchParams({ toolkit: id });

// failed is why the toolkit could not be shown at the address asked for;
// asked counts the addresses asked for, and only the answer to the latest
// is shown.
const [failed, setFailed] = createSignal("");
let asked = 0;

// show asks the UI for the toolkit an address chooses, and puts it in place as the stream puts a part. A failure is shown in
// the view, and the toolkit stays as it was.
async function show(href, push) {
  const at = new URL(href, location.href);
  const mine = ++asked;
  try {
    const res = await fetch("/toolkits/view" + at.search);
    const answer = res.ok ? await res.json() : (await res.text()).trim() || res.statusText;
    if (mine !== asked) return;
    if (!res.ok) {
      setFailed(answer);
      return;
    }
    setFailed("");
    receive({ type: "part", id: "toolkit", data: answer });
    if (push) history.pushState(null, "", at.href);
  } catch (err) {
    if (mine === asked) setFailed(String(err));
  }
}

// inPlace shows a link to the Toolkits page in place, and follows any other.
function inPlace(e) {
  const a = e.target.closest("a");
  if (!a || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey) return;
  const href = a.getAttribute("href");
  if (!href || new URL(href, location.href).pathname !== "/toolkits") return;
  e.preventDefault();
  show(href, true);
}

// Named is a machine, a file or a location in code: a link to the machine
// recorded at a location on the Machines page, where there is one.
const Named = props => html`<Show when=${() => props.at} fallback=${() => html`<code>${() => props.text}</code>`}><a href=${() => machineHref(props.at)} title=${() => props.at}><code>${() => props.text}</code></a></Show>`;

// base gives the last name of a path.
const base = p => p.slice(p.lastIndexOf("/") + 1);

// Job is what a tool's job is: a call with the input it fixes, a send, or
// the machine or script it runs.
function Job(props) {
  const t = () => props.tool;
  return html`<Show when=${() => t().Call} fallback=${() => html`<Show when=${() => t().Job === "send"} fallback=${() => html`<p class="job">${() => t().Job} ${() => (t().File ? html`<${Named} at=${() => t().At} text=${() => base(t().File)} />` : "")}</p>`}>
      <p class="job">send <code>${() => t().Event}</code> to <${Named} at=${() => t().At} text=${() => t().To} /></p>
    </Show>`}>
    <p class="job">call <${Named} at=${() => t().At} text=${() => t().Call} /></p>
    ${() => (t().Fixed.length ? html`<p class="job">input: ${() => t().Fixed.join(", ")}</p>` : "")}
  </Show>`;
}

function Tool(props) {
  const t = () => props.tool;
  return html`<section class=${() => ["tool", { when: t().When }]} title=${() => (t().When ? "when: " + t().When : undefined)}>
    <div class="bar"><span class="tname">${() => t().Name}</span>${() => (t().Approve ? html`<span class="approve">needs approval</span>` : "")}</div>
    <p title=${() => t().Description}>${() => t().Description}</p>
    <${Job} tool=${t} />
    ${() => (t().When ? html`<p class="job">when: ${() => t().When}</p>` : "")}
  </section>`;
}

// Starter is an agent or a machine that starts one: a link to the machine
// recorded at its location, or its name alone where it has none.
const Starter = props => html`<Show when=${() => props.at} fallback=${() => html`<span class="start" title=${() => props.file}><${Icon} type=${() => props.type} />${() => props.name}</span>`}><a class="start" href=${() => machineHref(props.at)} title=${() => props.at}><${Icon} type=${() => props.type} />${() => props.name}</a></Show>`;

function Offer(props) {
  const o = () => props.offer;
  return html`<tr>
    <td><${Starter} at=${() => o().At} file=${() => o().File} type="agent" name=${() => o().Name} />${" "}<small>${() => o().At || o().File}</small>${() => (o().When ? html`${" "}<code class="when">when: ${() => o().When}</code>` : "")}</td>
    <td class="wrap"><For each=${() => o().Starts}>${s => html`<${Starter} at=${() => s.At} file=${() => s.File} type=${() => s.Type} name=${() => s.Name} />`}</For>${() => (o().Lifetime ? html`<span class="dim">${() => o().Lifetime}: ${() => o().Started}</span>` : "")}</td>
  </tr>`;
}

// Body is the toolkit chosen: its description, its instructions, what
// offers and starts it, and its tools.
function Body(props) {
  const k = () => props.toolkit;
  return html`<div class="body">
    <p class="description">${() => k().Description}</p>
    ${() => (k().InstructionsFile ? html`<p class="instructions">instructions <code>${() => k().InstructionsFile}</code></p>` : k().Instructions ? html`<p class="instructions" title="instructions">${() => k().Instructions}</p>` : "")}
    <table class="list offered"><thead><tr><th>Offered by</th><th>Started by</th></tr></thead><tbody><For each=${() => k().Offered} keyed=${o => o.File + " " + o.Name}>${o => html`<${Offer} offer=${o} />`}</For></tbody></table>
    <h2>Tools</h2>
    <div class="toolcards"><For each=${() => k().Tools} keyed=${t => t.Name}>${t => html`<${Tool} tool=${t} />`}</For></div>
  </div>`;
}

// View is the toolkit chosen.
function View() {
  const k = () => parts.toolkit.Open;
  return html`<div class="panel view toolkit">
    <Show when=${k} fallback=${() => html`<div class="body"><p class="dim">no toolkit chosen</p></div>`}>
      <div class="head"><${Icon} type="toolkit" /><span>${() => k().Name}</span><span class="facts">${() => k().File}</span>${() => (k().Opening ? html`<span class="approve">opening it needs approval</span>` : "")}</div>
      <Show when=${failed}><p class="said bad failed">${failed}</p></Show>
      <${Body} toolkit=${k} />
    </Show>
  </div>`;
}

// List is the toolkits of each tree.
function List() {
  return html`<div class="filelist list">
    <For each=${() => parts.list.Trees} keyed=${t => t.Prefix} fallback=${() => html`<p class="dim">no toolkits recorded</p>`}>${t => html`
      <h2 class="group">${() => t().Prefix}</h2>
      <For each=${() => t().Toolkits} keyed=${k => k.ID}>${k => html`
        <a href=${() => toolkitHref(k().ID)} aria-current=${() => (parts.toolkit.Open?.ID === k().ID ? "page" : undefined)}><${Icon} type="toolkit" /><span class="name" title=${() => k().File}>${() => k().Name}</span><small>${() => plural(k().Tools.length, "tool")} · ${() => k().Description}</small></a>`}</For>`}</For>
  </div>`;
}

function Toolkits() {
  // The page asks for the toolkit it shows again when the stream gives the
  // trees as read at another mark than the toolkit shown was.
  createEffect(
    () => parts.list.Stamp !== parts.toolkit.Stamp,
    changed => {
      if (changed) show(location.href, false);
    }
  );
  return html`<div class="ide" onClick=${inPlace}>
    <nav class="panel files" aria-label="The toolkits of the machines the controller recorded">
      <${List} />
    </nav>
    <${Handle} size="files" />
    <${View} />
  </div>`;
}

window.addEventListener("popstate", () => show(location.href, false));
mount(Toolkits);
