// What every page's script shares. A page is drawn by Solid from its parts:
// the page carries every part as it is when it loads, and its WebSocket
// sends each part again whenever its data changes. A part is drawn by one
// component, on the first load and on every change.
import { createEffect, createSignal, createStore, reconcile, For, Show } from "solid-js";
import { render } from "@solidjs/web";
import h from "@solidjs/html";

// html is the tag every page writes its elements with. For and Show are
// written by name.
export const html = h.define({ For, Show });

// parts holds every part of the page by its name, as last sent.
export const [parts, setParts] = createStore(JSON.parse(document.getElementById("parts").textContent));

// following says the page has been sent a message over its WebSocket: a
// line is lit only from then on.
let following = false;

// receive takes a message the UI sent: a part, which it puts in its place,
// or whether the store can be read, which the header's dot shows.
/** @param {import("../../types").Message} m */
export function receive(m) {
  following = true;
  switch (m.type) {
    case "part":
      setParts(s => { merge(s, m.id, m.data); });
      break;
    case "store":
      document.getElementById("live")?.classList.toggle("lost", m.store === "lost");
      break;
  }
}

// follow opens the page's WebSocket, and opens it again when it is cut.
function follow(stream) {
  const live = document.getElementById("live");
  const ws = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}${stream}`);
  ws.onmessage = e => receive(JSON.parse(e.data));
  ws.onclose = () => {
    live.classList.add("lost");
    setTimeout(() => follow(stream), 2000);
  };
}

// merge puts a part's data in its place. A record the part held before, by
// its ID or its key, keeps its identity, so what is drawn of it is kept.
function merge(s, id, data) {
  const was = s[id];
  if (was && data && typeof was === "object" && typeof data === "object" && Array.isArray(was) === Array.isArray(data)) reconcile(data, keyOf)(was);
  else s[id] = data;
}

const keyOf = record => record?.ID ?? record?.Key;

// mount draws a page into its main element, follows its stream, and gives
// each panel the size the browser kept for it.
export function mount(Page) {
  const main = document.querySelector("main");
  render(Page, main);
  for (const handle of main.querySelectorAll(".handle[data-size]")) {
    const kept = stored("size:" + handle.dataset.size);
    if (kept) handle.parentElement.style.setProperty("--" + handle.dataset.size, kept);
  }
  if (main.dataset.stream) follow(main.dataset.stream);
}

// stored reads what the browser keeps for every page, where it keeps it.
export function stored(name) {
  try {
    return localStorage.getItem(name);
  } catch {
    return null;
  }
}

function store(name, value) {
  try {
    localStorage.setItem(name, value);
  } catch {}
}

// drawn is each thing a page lists, by its key, as it was last drawn: its
// data, its status, and whether it ended.
const drawn = new Map();

// lit lights a line as the look says, from its key, its data, its status
// and whether it ended: one that is new fades in, one that changed is lit,
// and a run that ended is lit in its status's colour. A line is found by
// its key anywhere on the page, since a run that ends moves to another part.
export function lit(of) {
  let el;
  createEffect(
    () => {
      const v = of();
      return { key: v.key, data: JSON.stringify(v.data), status: v.status, ended: v.ended };
    },
    v => {
      const was = drawn.get(v.key);
      // The newest are kept: a line drawn long ago is forgotten.
      drawn.delete(v.key);
      drawn.set(v.key, v);
      if (drawn.size > 5000) drawn.delete(drawn.keys().next().value);
      if (!el || !following) return;
      const light = !was ? "new" : was.status !== v.status && v.ended ? "ended" : was.data !== v.data ? "moved" : "";
      if (light) relight(el, light);
    }
  );
  return e => { el = e; };
}

// bump lifts a count that changed, and lets it settle.
export function bump(of) {
  let el, was;
  createEffect(of, v => {
    if (el && was !== undefined && was !== v) relight(el, "bump");
    was = v;
  });
  return e => { el = e; };
}

function relight(el, light) {
  el.classList.remove("new", "moved", "ended", "bump");
  void el.offsetWidth;
  el.classList.add(light);
}

// plural gives a count of a thing: 1 run, 2 runs.
export const plural = (n, one) => `${n} ${word(n, one)}`;

// word gives a thing's name for a count of it: run, runs.
export const word = (n, one) => (n === 1 ? one : one + "s");

// Icon is a machine's type, or what a job is, as an icon beside its name.
export function Icon(props) {
  return html`<svg class="g m" role="img" aria-label=${() => props.type}><title>${() => props.type}</title><use href=${() => "#g-" + props.type} /></svg>`;
}

// Status is a status as a dot and a word, with a failure's kind in brackets.
export function Status(props) {
  return html`<span class=${() => ["st", props.status]}>${() => props.status}${() => (props.failure ? ` (${props.failure})` : "")}</span>`;
}

// tabs gives the name of the tab chosen among a panel's tabs, the bar that
// chooses it, and what chooses one.
export function tabs(names, first, named = name => name) {
  const [chosen, choose] = createSignal(first ?? names[0]);
  const bar = () => html`<nav class="tabs">
    <For each=${names}>${name => html`<a href=${"#" + name} aria-current=${() => chosen() === name} onClick=${e => { e.preventDefault(); choose(name); }}>${named(name)}</a>`}</For>
  </nav>`;
  return [chosen, bar, choose];
}


// Act is a form whose button runs one of Corbie's commands, and shows the
// command's answer beside the button, or its error, untouched. While the
// command runs, it says so. The ID of a run the command started links to
// the run's page. A form that resets is emptied when its command succeeds.
export function Act(props) {
  const [said, setSaid] = createSignal({ text: "", kind: "" });
  const [busy, setBusy] = createSignal(false);
  async function submit(e) {
    e.preventDefault();
    const form = e.currentTarget;
    setBusy(true);
    setSaid({ text: "waiting for the answer", kind: "waiting" });
    try {
      const res = await fetch(props.action, { method: "POST", body: new URLSearchParams(new FormData(form)) });
      setSaid({ text: (await res.text()).trim(), kind: res.ok ? "" : "bad" });
      if (res.ok && props.reset) form.reset();
    } catch (err) {
      setSaid({ text: String(err), kind: "bad" });
    } finally {
      setBusy(false);
    }
  }
  const started = () => (said().kind === "" ? /^started (\S+\/\S+)$/.exec(said().text)?.[1] : undefined);
  const answer = () => html`<Show when=${started} fallback=${() => said().text}>${id => html`started <a href=${() => encodeURI("/run/" + id())}>${id}</a>`}</Show>`;
  const Said = () => (props.block ? html`<pre class=${() => ["said", said().kind]}>${answer}</pre>` : html`<span class=${() => ["said", said().kind]}>${answer}</span>`);
  return html`<form class=${() => props.class} method="post" action=${() => props.action} data-act="" onSubmit=${submit}>
    ${() => props.children}${() => props.saidFirst && Said()}<button class=${() => ["btn", props.button]} data-cmd=${() => props.cmd} disabled=${busy}>${() => props.label}</button>${() => !props.saidFirst && Said()}
  </form>`;
}

// A handle between two panels resizes them as a person drags it. It sets
// the size of the panel before it, or for a row the panel after it, on the
// element that holds both, and the browser keeps the size for every page.
export function Handle(props) {
  function drag(e) {
    if (e.button !== 0) return;
    e.preventDefault();
    const h = e.currentTarget;
    const holder = h.parentElement;
    h.setPointerCapture(e.pointerId);
    h.classList.add("dragging");
    const move = m => {
      const r = holder.getBoundingClientRect();
      const whole = props.row ? r.height : r.width;
      const at = props.row ? r.bottom - m.clientY : m.clientX - r.left;
      const size = Math.round(Math.min(Math.max(at, 160), whole - 160)) + "px";
      holder.style.setProperty("--" + props.size, size);
      store("size:" + props.size, size);
    };
    h.addEventListener("pointermove", move);
    h.addEventListener("lostpointercapture", () => { h.removeEventListener("pointermove", move); h.classList.remove("dragging"); }, { once: true });
  }
  return html`<div class=${() => ["handle", { row: props.row }]} data-size=${() => props.size} aria-hidden="true" onPointerDown=${drag}></div>`;
}
