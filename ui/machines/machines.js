// The Machines page: the machines the controller recorded, by tree, and the
// machine chosen drawn or written out. Choosing a machine, a tab or a
// card's link shows it in place, and the page shows the machine again when
// a submit changed it. Unfolding a card moves the drawing in to it, and
// folding it moves back out to the card it was in.
import { createEffect, createSignal, flush } from "solid-js";
import { html, parts, receive, mount, Icon, Handle } from "ui";
import { Drawing } from "drawing";
import { Code, machineHref, focusOf, focusAt } from "/machines/shared.js";

// focus is the card the drawing moves in to, "" being the whole machine;
// shownAt is the address the machine chosen is shown as; failed is why the
// machine chosen could not be shown at the address asked for.
const [focus, setFocus] = createSignal(focusAt(location.href));
const [failed, setFailed] = createSignal("");
let shownAt = location.href;

// asked counts the addresses asked for: only the answer to the latest is
// shown.
let asked = 0;

// show asks the UI for the machine chosen as an address shows it, and puts
// it in place as the stream puts a part. A failure is shown in the view,
// and the machine stays as it was.
async function show(href, push) {
  const at = new URL(href, location.href);
  const mine = ++asked;
  try {
    const res = await fetch("/machines/view" + at.search);
    const answer = res.ok ? await res.json() : (await res.text()).trim() || res.statusText;
    if (mine !== asked) return;
    if (!res.ok) {
      setFailed(answer);
      return;
    }
    const moved = focusOf(shownAt, at.href);
    const other = new URL(shownAt).searchParams.get("at") !== at.searchParams.get("at");
    shownAt = at.href;
    const put = () => {
      setFailed("");
      receive({ type: "part", id: "view", data: answer });
      if (moved !== null) setFocus(moved);
      flush();
      if (other) document.querySelector(".ide > .view")?.scrollTo(0, 0);
    };
    // Another machine cross-fades in over the one shown, which stays until the
    // new one is drawn.
    if (other && document.startViewTransition) document.startViewTransition(put);
    else put();
    if (push) history.pushState(null, "", at.href);
  } catch (err) {
    if (mine === asked) setFailed(String(err));
  }
}

// here says an address is of the Machines page, so that it can be shown in
// place.
const here = href => new URL(href, location.href).pathname === "/machines";

// inPlace shows a link to the Machines page in place, and follows any other.
function inPlace(e) {
  const a = e.target.closest("a");
  if (!a || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey) return;
  const href = a.getAttribute("href");
  if (!href || !here(href)) return;
  e.preventDefault();
  show(href, true);
}

function Entry(props) {
  const e = () => props.entry;
  return html`<a href=${() => machineHref(e().Location)} aria-current=${() => (e().Location === parts.view.Open ? "page" : undefined)}>
    <${Icon} type=${() => e().Type} />
    <span class="name" title=${() => e().Location}>${() => e().Name}</span>
    <small title=${() => e().Description || undefined}>${() => e().First}${() => (e().Lifetime ? " · " + e().Lifetime : "")}</small>
  </a>`;
}

// List is the part "list": each tree, and the machine at each of its
// locations.
function List() {
  return html`<div class="filelist list">
    <For each=${() => parts.list.Trees} keyed=${t => t.Prefix} fallback=${() => html`<p class="dim">no machines recorded</p>`}>${t => html`
      <h2 class="group" title=${() => `the tree at ${t().Prefix}, of ${t().Client}, last submitted from ${t().Directory}`}>${() => t().Prefix}<small>${() => t().Client} · from ${() => t().Directory}</small></h2>
      <For each=${() => t().Entries} keyed=${e => e.Location}>${e => html`<${Entry} entry=${e} />`}</For>`}</For>
  </div>`;
}

// counted gives the runs at a location in words: how many run there, and
// how many failed today, or nothing where none do.
function counted(at) {
  const r = parts.runs[at];
  const said = [];
  if (r?.Running) said.push(`${r.Running} running`);
  if (r?.FailedToday) said.push(`${r.FailedToday} failed today`);
  return said.map(s => " · " + s).join("");
}

function Place(props) {
  const p = () => props.place;
  return html`<section class="place">${() => (p().Description ? html`<p class="description">${() => p().Description}</p>` : "")}<div class="named">
    <span>location</span><span><a href=${() => "/runs?at=" + encodeURIComponent(p().Location)} title=${() => "the runs at " + p().Location}>${() => p().Location}<span class="counted">${() => counted(p().Location)}</span></a></span>
    <span>lifetime</span><span>${() => p().Lifetime}: ${() => p().Started}${() => (p().Runs ? `, at most ${p().Runs} running at once` : "")}</span>
    <span>update</span><span>${() => p().Update}: ${() => p().Updated}</span>
    <span>receives</span><span><For each=${() => p().Receives}>${r => html`${r}<br />`}</For>${() => (p().Receives.length ? "and anything from its tree's client" : "from its own tree and its tree's client")}</span>
    <span>sends to</span><span>${() => (p().Sends.length ? p().Sends.join(", ") : "none")}</span>
    ${() => (p().Approve.length ? html`<span>approve</span><span>${() => p().Approve.join(", ")}</span>` : "")}
  </div></section>`;
}

function Uses(props) {
  const v = () => props.view;
  return html`<section class="uses"><h2>Secrets and values</h2>
    <div class="named"><For each=${() => v().Uses}>${u => html`<span>${() => u.Name}</span><span class="dim">${() => u.Kind}${() => (u.State ? " · " + u.State : "")}</span>`}</For></div>
  </section>`;
}

// tab gives the address of one of the tabs of the machine chosen.
const tab = (at, view) => machineHref(at, view === "drawing" ? undefined : view);

// View is the part "view": the machine chosen. Its drawing is drawn anew
// for each machine, so that the camera frames another machine at once, and
// moves only within one.
function View() {
  const v = () => parts.view;
  return html`<Show when=${() => v().Open} fallback=${() => html`<div class="panel view"><p class="dim body">no machines recorded</p></div>`}>
    <div class="panel view">
      <div class="head"><${Icon} type=${() => v().Kind} /><span>${() => v().Open}</span><span class="facts">${() => v().First}</span></div>
      <Show when=${failed}><p class="said bad failed">${failed}</p></Show>
      <nav class="tabs"><For each=${["drawing", "written"]}>${t => html`<a href=${() => tab(v().Open, t)} aria-current=${() => (v().View === t ? "page" : undefined)}>${{ drawing: "Drawing", written: "Written out" }[t]}</a>`}</For></nav>
      <Show when=${() => v().Place}><${Place} place=${() => v().Place} /></Show>
      <Show when=${() => v().Uses.length}><${Uses} view=${v} /></Show>
      <Show when=${() => v().View === "drawing"} fallback=${() => html`<${Code} lines=${() => v().Text} label=${() => v().Open} />`}>
        <section class="drawn" id="drawing"><For each=${() => [v().Open]}>${() => html`<${Drawing} scene=${() => v().Drawing} focus=${focus} counts=${() => parts.runs[v().Open]?.In} />`}</For></section>
      </Show>
    </div>
  </Show>`;
}

function Machines() {
  // The page asks for the machine it shows again when the stream gives the
  // trees as read at another mark than the machine shown was.
  createEffect(
    () => parts.list.Stamp !== parts.view.Stamp,
    changed => {
      if (changed) show(location.href, false);
    }
  );
  return html`<div class="ide" onClick=${inPlace}>
    <nav class="panel files" aria-label="The machines the controller recorded">
      <${List} />
    </nav>
    <${Handle} size="files" />
    <${View} />
  </div>`;
}

window.addEventListener("popstate", () => show(location.href, false));
mount(Machines);
