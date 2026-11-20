// The tests of what a page draws, run in Chrome by `go test` (see
// page/pagetest). A test module calls test() for each test; the page runs
// them in order and gives each one's result to Go.
import { flush } from "solid-js";
import { render } from "@solidjs/web";
import { setParts } from "ui";

const tests = [];

// test adds a test: fn may be async, and fails by throwing.
export function test(name, fn) {
  tests.push({ name, fn });
}

// fixture gives a fixture the Go test made from real views, by its name.
export async function fixture(name) {
  const res = await fetch("/fixture/" + encodeURIComponent(name));
  if (!res.ok) throw new Error(`fixture ${name}: ${res.status} ${await res.text()}`);
  return res.json();
}

// mount draws a component into a box of the given size, with the page's
// parts set to parts, and gives the box. The box is removed after the test.
export function mount(Component, parts = {}, size = { width: 1400, height: 860 }) {
  setParts(s => {
    for (const k of Object.keys(s)) delete s[k];
    Object.assign(s, structuredClone(parts));
  });
  flush();
  const box = document.createElement("main");
  box.style.cssText = `position: fixed; left: 0; top: 0; width: ${size.width}px; height: ${size.height}px; display: flex; flex-direction: column; padding: 20px; box-sizing: border-box;`;
  document.body.append(box);
  const dispose = render(Component, box);
  flush();
  cleanups.push(() => { dispose(); box.remove(); });
  return box;
}

let cleanups = [];

// settle lets Solid apply what is queued, and the browser lay out and run
// what waits for the next frames.
export async function settle(frames = 2) {
  flush();
  for (let i = 0; i < frames; i++) await new Promise(r => requestAnimationFrame(r));
  flush();
}

// wait waits a number of milliseconds, for motion to end.
export const wait = ms => new Promise(r => setTimeout(r, ms));

// until waits until ok says yes, then lets the page settle; after ms it
// fails, naming what it waited for.
export async function until(ok, what, ms = 4000) {
  const end = performance.now() + ms;
  while (!ok()) {
    if (performance.now() > end) throw new Error("waited for " + what);
    await wait(20);
  }
  await settle();
}

// click clicks an element as a person does.
export const click = el => el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));

// copy gives a part's data as plain JSON, to change and send.
export const copy = v => JSON.parse(JSON.stringify(v));

// words gives the words a person reads in an element, each element's apart,
// its icons' names left out.
export const words = el => {
  if (!el) return "";
  const all = [], walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
  for (let n = walk.nextNode(); n; n = walk.nextNode()) if (!n.parentElement.closest("svg.g")) all.push(n.data);
  return all.join(" ").replace(/\s+/g, " ").trim();
};

export function assert(ok, message) {
  if (!ok) throw new Error(message ?? "assertion failed");
}

export function equal(got, want, what = "value") {
  const g = JSON.stringify(got), w = JSON.stringify(want);
  if (g !== w) throw new Error(`${what} is ${g}, not ${w}`);
}

// text gives an element's text with its runs of space made one.
export const text = el => (el?.textContent ?? "").replace(/\s+/g, " ").trim();

// overlap says whether two boxes, as getBoundingClientRect gives them,
// overlap by more than a hair.
export const overlap = (a, b, hair = 0.5) =>
  a.left < b.right - hair && b.left < a.right - hair && a.top < b.bottom - hair && b.top < a.bottom - hair;

// run runs every test in order, and gives each one's name, whether it
// passed, and why not.
export async function run() {
  const results = [];
  for (const t of tests) {
    const started = performance.now();
    try {
      await t.fn();
      results.push({ name: t.name, ok: true, ms: performance.now() - started });
    } catch (err) {
      results.push({ name: t.name, ok: false, ms: performance.now() - started, error: String(err?.stack ?? err) });
    }
    for (const c of cleanups.splice(0)) c();
  }
  return results;
}
