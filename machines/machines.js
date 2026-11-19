// The Machines page: the machines in one directory, the file chosen drawn
// or shown as text, and the problems Corbie finds in them. A card's link in
// the drawing, and a tab, show the file chosen again where it is, and the
// page shows it again when it, or what Corbie found in it, changed on disk.
import { createEffect, reconcile } from "solid-js";
import { html, parts, setParts, mount, Icon, Handle, plural } from "ui";
import { Drawing } from "drawing";
import { Problems, Code, machineHref, base } from "/machines/shared.js";

// show reads the page at an address, and puts the file chosen in place.
async function show(href, push) {
  const res = await fetch(href);
  if (!res.ok) {
    location = href;
    return;
  }
  const page = new DOMParser().parseFromString(await res.text(), "text/html");
  const next = JSON.parse(page.getElementById("parts").textContent);
  // Merged in place, so only what changed is drawn again.
  setParts(s => { reconcile(next.view, null)(s.view); });
  if (push) history.pushState(null, "", href);
}

// here says an address shows the file chosen, so that it can be shown in
// place.
const here = href => {
  const at = new URL(href, location.href);
  return at.pathname === "/machines" && at.searchParams.get("file") === parts.view.Open;
};

// inPlace shows a link to the file chosen in place, and follows any other.
function inPlace(e) {
  const a = e.target.closest("a");
  if (!a || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey) return;
  const href = a.getAttribute("href");
  if (!href || !here(href)) return;
  e.preventDefault();
  show(href, true);
}

function Entry(props) {
  return html`<a href=${() => machineHref(props.entry.Path)} aria-current=${() => (props.entry.Path === parts.files.Open ? "page" : undefined)}>
    <${Icon} type=${() => props.entry.Type || "file"} />
    <span class="name" title=${() => props.entry.Location}>${() => base(props.entry.Path)}</span>
    ${() => (props.entry.Problems ? html`<span class="bad" title=${() => plural(props.entry.Problems, "problem")}>${() => props.entry.Problems}</span>` : "")}
    <small title=${() => props.entry.Description || undefined}>${() => props.entry.First}${() => (props.entry.Lifetime ? " · " + props.entry.Lifetime : "")}</small>
  </a>`;
}

function File(props) {
  const listed = () => (props.file.Listed ? "listed in .corbieignore · " : "");
  const included = () => (props.file.Included ? "included by " + plural(props.file.Included, "machine") : "included by none");
  return html`<Show when=${() => !props.file.Path.endsWith("/")} fallback=${() => html`<div class="row"><span></span><span class="name" title=${() => props.file.Path}>${() => props.file.Path}</span><small>listed in .corbieignore</small></div>`}>
    <a href=${() => machineHref(props.file.Path)} aria-current=${() => (props.file.Path === parts.files.Open ? "page" : undefined)}><${Icon} type="file" /><span class="name" title=${() => props.file.Path}>${() => props.file.Path}</span><small>${listed}${included}</small></a>
  </Show>`;
}

// List is the part "files": the machines by directory, then the files that
// are no machine.
function List() {
  const f = () => parts.files;
  return html`<div class="filelist list">
    ${() => (f().Unread ? html`<p class="unread">not read to its end: ${() => f().Unread}</p>` : "")}
    <${Problems} problems=${() => f().Problems} />
    <For each=${() => f().Groups} keyed=${g => g.Dir} fallback=${() => html`<p class="dim">no machines here</p>`}>${g => html`
      ${() => (g().Dir !== "." ? html`<h2 class="group" title=${() => g().Dir}>${() => g().Dir}/</h2>` : "")}
      <For each=${() => g().Entries} keyed=${e => e.Path}>${e => html`<${Entry} entry=${e} />`}</For>`}</For>
    <Show when=${() => f().Files.length}>
      <h2 class="kind">Files${() => (f().Ignore ? html`<a href=${machineHref(".corbieignore")} aria-current=${() => (f().Open === ".corbieignore" ? "page" : undefined)}>.corbieignore</a>` : "")}</h2>
      <For each=${() => f().Files} keyed=${e => e.Path}>${e => html`<${File} file=${e} />`}</For>
    </Show>
  </div>`;
}

function Place(props) {
  const p = () => props.place;
  return html`<section class="place">${() => (p().Description ? html`<p class="description">${() => p().Description}</p>` : "")}<div class="named">
    <span>location</span><span>${() => (parts.view.Runs ? html`<a href=${() => "/runs?at=" + encodeURIComponent(p().Location)} title=${() => "the runs at " + p().Location}>${() => p().Location}</a>` : p().Location)}</span>
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
    ${() => (v().Controller ? "" : html`<p class="dim">set by the controller's owner: <code>corbie controller secret set NAME --at ${() => v().Place.Location}</code>, <code>corbie controller value set NAME VALUE --at ${() => v().Place.Location}</code></p>`)}
  </section>`;
}

function Included(props) {
  return html`<section class="included"><h2>Included by</h2>
    <For each=${() => props.files} fallback=${() => html`<p class="dim">no machine takes a value from this file</p>`}>${f => html`<a href=${machineHref(f)}>${f}</a>`}</For>
  </section>`;
}

// tab gives the address of one of the tabs of the file chosen.
const tab = (file, view) => machineHref(file, view === "drawing" ? undefined : view);

// View is the part "view": the file chosen.
function View() {
  const v = () => parts.view;
  return html`<Show when=${() => v().Open} fallback=${() => html`<div class="panel view"><p class="dim body">no machines in this directory</p></div>`}>
    <div class="panel view" onClick=${inPlace}>
      <div class="head"><${Icon} type=${() => v().Kind || "file"} /><span>${() => v().Open}</span><span class="facts">${() => v().Facts}</span></div>
      <Show when=${() => v().Drawable}>
        <nav class="tabs"><For each=${["drawing", "text", "written"]}>${t => html`<a href=${() => tab(v().Open, t)} aria-current=${() => (v().View === t ? "page" : undefined)}>${{ drawing: "Drawing", text: "Text", written: "Written out" }[t]}</a>`}</For></nav>
      </Show>
      <${Problems} problems=${() => v().Found} />
      <Show when=${() => v().Place}><${Place} place=${() => v().Place} /></Show>
      <Show when=${() => v().Uses.length}><${Uses} view=${v} /></Show>
      <Show when=${() => v().File}><${Included} files=${() => v().Included} /></Show>
      <Show when=${() => v().View === "drawing"} fallback=${() => html`<${Code} lines=${() => v().Text} written=${() => v().View === "written"} label=${() => v().Open} />`}>
        <section class="drawn" id="drawing"><${Drawing} scene=${() => v().Drawing} /></section>
      </Show>
    </div>
  </Show>`;
}

function Machines() {
  // The page reads the file chosen again when the mark of the file on disk
  // differs from the mark of the file it shows, once for each mark.
  let asked = "";
  createEffect(
    () => [parts.disk.Base, parts.view.Base],
    ([disk, shown]) => {
      if (disk === shown || disk === asked) return;
      asked = disk;
      show(location.href, false);
    }
  );
  return html`<div class="ide">
    <nav class="panel files" aria-label=${() => "The machines in " + parts.files.DirFull}>
      <div class="head"><span class="facts" title=${() => parts.files.DirFull}>${() => parts.files.Dir}</span>${() => (parts.files.Prefix ? html`<span class="facts at" title=${() => "the locations of the directory's machines are under " + parts.files.Prefix}>at ${() => parts.files.Prefix}</span>` : "")}</div>
      <${List} />
    </nav>
    <${Handle} size="files" />
    <${View} />
  </div>`;
}

window.addEventListener("popstate", () => show(location.href, false));
mount(Machines);
document.getElementById(location.hash.slice(1))?.scrollIntoView({ block: "center" });
