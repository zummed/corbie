// The Toolkits page: the toolkits the agents in the directory offer, and the
// toolkit chosen with its tools and what offers and starts it, or its
// file's text. Choosing a toolkit or a tab shows it in place, and the page
// shows the toolkit again when the directory changed on disk.
import { createEffect, createSignal } from "solid-js";
import { html, parts, receive, mount, Icon, Handle, plural } from "ui";
import { Problems, Code, machineHref, base } from "/machines/shared.js";

const toolkitHref = (id, text) => "/toolkits?" + new URLSearchParams(text ? { toolkit: id, view: "text" } : { toolkit: id });

// failed is why the toolkit could not be shown at the address asked for;
// asked counts the addresses asked for, and only the answer to the latest
// is shown.
const [failed, setFailed] = createSignal("");
let asked = 0;

// show asks the UI for the toolkit an address chooses, as its tab shows
// it, and puts it in place as the stream puts a part. A failure is shown in
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

// FileLink is a link to a file on the Machines page, its name in code.
const FileLink = props => html`<a href=${() => machineHref(props.file)} title=${() => props.file}><code>${() => props.text}</code></a>`;

// Job is what a tool's job is: a call with the input it fixes, a send, or
// the machine or script it runs.
function Job(props) {
  const t = () => props.tool;
  const named = text => (t().File ? html`<${FileLink} file=${() => t().File} text=${text} />` : html`<code>${text}</code>`);
  return html`<Show when=${() => t().Call} fallback=${() => html`<Show when=${() => t().Job === "send"} fallback=${() => html`<p class="job">${() => t().Job} ${() => (t().File ? html`<${FileLink} file=${() => t().File} text=${() => base(t().File)} />` : "")}</p>`}>
      <p class="job">send <code>${() => t().Event}</code> to ${() => named(t().To)}</p>
    </Show>`}>
    <p class="job">call ${() => named(t().Call)}</p>
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

function Offer(props) {
  const o = () => props.offer;
  return html`<tr>
    <td><a class="start" href=${() => machineHref(o().File)} title=${() => o().File}><${Icon} type="agent" />${() => o().Name}</a>${" "}<small>${() => o().File}</small>${() => (o().When ? html`${" "}<code class="when">when: ${() => o().When}</code>` : "")}</td>
    <td class="wrap"><For each=${() => o().Starts}>${s => html`<a class="start" href=${() => machineHref(s.File)} title=${() => s.File}><${Icon} type=${() => s.Type} />${() => s.Name}</a>`}</For>${() => (o().Lifetime ? html`<span class="dim">${() => o().Lifetime}: ${() => o().Started}</span>` : "")}</td>
  </tr>`;
}

// Body is the toolkit chosen: its description, its instructions, what
// offers and starts it, and its tools.
function Body(props) {
  const k = () => props.toolkit;
  return html`<div class="body">
    <p class="description">${() => k().Description}</p>
    ${() => (k().InstructionsFile ? html`<p class="instructions">instructions <${FileLink} file=${() => k().InstructionsFile} text=${() => k().InstructionsFile} /></p>` : k().Instructions ? html`<p class="instructions" title="instructions">${() => k().Instructions}</p>` : "")}
    <table class="list offered"><thead><tr><th>Offered by</th><th>Started by</th></tr></thead><tbody><For each=${() => k().Offered} keyed=${o => o.File + " " + o.Name}>${o => html`<${Offer} offer=${o} />`}</For></tbody></table>
    <h2>Tools</h2>
    <div class="toolcards"><For each=${() => k().Tools} keyed=${t => t.Name}>${t => html`<${Tool} tool=${t} />`}</For></div>
  </div>`;
}

// View is the toolkit chosen, with the problems found in its file.
function View() {
  const v = () => parts.toolkit;
  const k = () => v().Open;
  return html`<div class="panel view toolkit">
    <Show when=${k} fallback=${() => html`<div class="body"><p class="dim">no toolkit chosen</p></div>`}>
      <div class="head"><${Icon} type="toolkit" /><span>${() => k().Name}</span><span class="facts">${() => k().File}</span>${() => (k().Opening ? html`<span class="approve">opening it needs approval</span>` : "")}</div>
      <Show when=${failed}><p class="said bad failed">${failed}</p></Show>
      <Show when=${() => k().File}>
        <nav class="tabs"><a href=${() => toolkitHref(k().ID)} aria-current=${() => (v().ShowText ? undefined : "page")}>Drawing</a><a href=${() => toolkitHref(k().ID, true)} aria-current=${() => (v().ShowText ? "page" : undefined)}>Text</a></nav>
      </Show>
      <${Problems} problems=${() => v().Found} />
      <Show when=${() => v().ShowText} fallback=${() => html`<${Body} toolkit=${k} />`}><${Code} lines=${() => v().Text} label=${() => k().File} /></Show>
    </Show>
  </div>`;
}

// List is the toolkits, under the directory's problems.
function List() {
  const v = () => parts.list;
  return html`<div class="filelist list">
    ${() => (v().Unread ? html`<p class="unread">not read to its end: ${() => v().Unread}</p>` : "")}
    <${Problems} problems=${() => v().Problems} />
    <For each=${() => v().Toolkits} keyed=${k => k.ID} fallback=${() => html`<p class="dim">no toolkits here</p>`}>${k => html`
      <a href=${() => toolkitHref(k().ID)} aria-current=${() => (parts.toolkit.Open?.ID === k().ID ? "page" : undefined)}><${Icon} type="toolkit" /><span class="name" title=${() => k().File}>${() => k().Name}</span>${() => (k().Broken ? html`<span class="chip">has problems</span>` : "")}<small>${() => plural(k().Tools.length, "tool")} · ${() => k().Description}</small></a>`}</For>
  </div>`;
}

function Toolkits() {
  // The page asks for the toolkit it shows again when the stream gives the
  // directory as read at another mark than the toolkit shown was.
  createEffect(
    () => parts.list.Stamp !== parts.toolkit.Stamp,
    changed => {
      if (changed) show(location.href, false);
    }
  );
  return html`<div class="ide" onClick=${inPlace}>
    <nav class="panel files" aria-label=${() => "The toolkits in " + parts.list.DirFull}>
      <div class="head"><span class="facts" title=${() => parts.list.DirFull}>${() => parts.list.Dir}</span></div>
      <${List} />
    </nav>
    <${Handle} size="files" />
    <${View} />
  </div>`;
}

window.addEventListener("popstate", () => show(location.href, false));
mount(Toolkits);
