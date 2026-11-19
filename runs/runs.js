// The Runs page and a run's page, drawn from their parts. A line stays
// unfolded as the store changes, a line or a row opens its run, the button
// at a line's start unfolds it, and a panel shows one tab at a time.
import { createContext, createDeferred, createEffect, createMemo, createSignal, createStore, isPending, onSettled, useContext, Errored } from "solid-js";
import { html, parts, mount, lit, bump, Icon, Status, Act, Handle, tabs, plural, word } from "ui";
import { Drawing } from "drawing";

// middle cuts a long address in its middle, where keys that differ at
// their end keep it.
function middle(s) {
  const most = 32, r = [...s];
  return r.length > most ? r.slice(0, most / 2 - 1).join("") + "…" + r.slice(r.length - most / 2).join("") : s;
}

// breaks writes a location so that a line may break after each /.
const breaks = location => location.split("/").flatMap((p, i, all) => (i < all.length - 1 ? [p + "/", document.createElement("wbr")] : [p]));

const keyWidth = key => (key ? `--key: ${[...key].length}ch` : undefined);

// now is the time, a second at a time, from which a page says how long ago
// something was.
const [now, setNow] = createSignal(Date.now());

// ago says how long ago a time was: 12s, 4m, 3h 5m.
function ago(time) {
  const s = Math.max(0, Math.round((now() - Date.parse(time)) / 1000));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m";
  return Math.floor(s / 3600) + "h " + (Math.floor(s / 60) % 60) + "m";
}

const go = href => e => {
  if (e.target.closest("a, button, input, form")) return;
  e.preventDefault();
  location = href;
};

// A Machine is a run by its machine, as a link to its page.
function Machine(props) {
  return html`<a href=${() => "/run/" + props.link.ID} title=${() => props.link.Machine}><Show when=${() => props.link.Type}><${Icon} type=${() => props.link.Type} /></Show>${() => props.link.Machine}</a>`;
}

// A Chain is the chain of runs from a root to the deepest: a chain of more
// than three is its first, an ellipsis holding the whole chain, and its last.
function Chain(props) {
  return html`<span class="chain"><For each=${() => props.of.Front}>${(l, i) => html`${() => (i() ? html`<i>›</i>` : "")}<${Machine} link=${l} />`}</For><Show when=${() => props.of.Between}><i>›</i><span class="between" title=${() => props.of.Between}>…</span></Show><For each=${() => props.of.Back}>${l => html`<i>›</i><${Machine} link=${l} />`}</For><Show when=${() => props.of.Shown}><span class="dim">[${() => props.of.Shown}]</span></Show></span>`;
}

// ---- The Runs page

// unfolded holds the lines a person unfolded, by their run's ID.
const [unfolded, setUnfolded] = createStore({});

// A Row is one run a root started, as its line unfolds.
function Row(props) {
  const r = () => props.row;
  return html`<Show when=${() => r().Own} fallback=${() => html`<div class="node"><span><${Status} status=${() => r().Status} failure=${() => r().Failure} /></span><a class="name named" href=${() => "/run/" + r().ID} title=${() => r().Tip} style=${() => keyWidth(r().Key)}><span>${() => r().Name}</span><Show when=${() => r().Key}><span class="key">${() => r().Key}</span></Show></a><${Chain} of=${() => r()} /><span class="says"><Show when=${() => r().Says}><span class="waits" title=${() => r().Says}>${() => r().Says}</span></Show><Show when=${() => r().Under}><Show when=${() => r().Says}><i>·</i></Show><span class="dim">${() => plural(r().Under, "run")}</span></Show></span><span class="dim">${() => r().When}</span><span class="dim">${() => r().Took}</span></div>`}>
    <div class="node own"><span><${Status} status=${() => r().Status} failure=${() => r().Failure} /></span><a class="name" href=${() => "/run/" + r().ID} title=${() => r().Tip}><Show when=${() => r().Type}><${Icon} type=${() => r().Type} /></Show>${() => r().Machine}<Show when=${() => r().Shown}>${" "}<span class="dim">[${() => r().Shown}]</span></Show></a><span class="waits" title=${() => r().Says}>${() => r().Says}</span><span class="dim">${() => r().When}</span><span></span></div>
  </Show>`;
}

// A Line is one root run, which unfolds to the runs it started.
function Line(props) {
  const l = () => props.line;
  const href = () => "/run/" + l().ID;
  return html`<details class="req" open=${() => !!unfolded[l().ID]}>
    <summary class="line" ref=${lit(() => ({ key: "line " + l().ID, data: l(), status: l().Status, ended: l().Ended }))} onClick=${e => go(href())(e)}>
      <span><${Status} status=${() => l().Status} failure=${() => l().Failure} /></span><span class="title named" title=${() => l().Tip} style=${() => keyWidth(l().Key)}><span>${() => (l().Titled ? l().Name : middle(l().Name))}</span><Show when=${() => l().Key}><span class="key">${() => l().Key}</span></Show></span>
      <Show when=${() => l().Ended} fallback=${() => html`<${Chain} of=${() => l()} /><span class="waits" title=${() => l().Waits}>${() => l().Waits}</span><span class="dim">${() => l().When}</span><span></span>`}>
        <span class="chain"><${Machine} link=${() => l()} /></span><span class="dim">${() => (l().Under ? plural(l().Under, "run") + " under it" : "")}</span><span class="dim">${() => l().When}</span><span class="dim">${() => l().Took}</span>
      </Show>
      <button class="unfold" type="button" aria-label=${() => "unfold the runs under " + l().Name} onClick=${e => { e.preventDefault(); setUnfolded(s => { s[l().ID] = !s[l().ID]; }); }}></button>
    </summary>
    <div class="tree"><For each=${() => l().Rows} keyed=${r => r.ID}>${r => html`<${Row} row=${() => r()} />`}</For><Show when=${() => l().More}><a class="node more" href=${href}><span></span><span class="name">${() => l().More} more</span></a></Show></div>
  </details>`;
}

// A Section is one panel of lines, with the windows its head offers.
function Section(props) {
  const s = () => parts[props.id];
  return html`<section class="panel reqs" id=${() => props.id} hidden=${() => !s().Lines.length && !s().Windows.length}>
    <div class="head"><h2>${() => s().Name}</h2><span class="facts" ref=${bump(() => s().Lines.length)}>${() => s().Lines.length}</span>
      <span class="acts"><Show when=${() => s().Windows.length}><span class="pills"><For each=${() => s().Windows}>${w => html`<a href=${() => w.Href} aria-current=${() => w.Current}>${() => w.Name}</a>`}</For></span></Show></span>
    </div>
    <For each=${() => s().Lines} keyed=${l => l.ID} fallback=${() => html`<p class="none dim">none in the window</p>`}>${l => html`<${Line} line=${() => l()} />`}</For>
  </section>`;
}

function Locations() {
  const v = () => parts.locations;
  const place = (p, t) => html`<a class=${() => ["loc", { in: !!t.Prefix }]} href=${() => p.Href} aria-current=${() => p.Current}><span></span><span class="name" title=${() => p.Location}>${() => breaks(p.Name)}</span><span class="dim"><Show when=${() => p.Status} fallback=${() => p.Says}><span class=${() => ["st", p.Status]}>${() => p.Status}</span></Show></span><Show when=${() => p.Lifetime}><small>${() => p.Lifetime}</small></Show></a>`;
  return html`<nav class="panel locs" id="locations" aria-label="Locations">
    <div class="head"><h2>Locations</h2></div>
    <div class="list"><a href=${() => v().Every.Href} aria-current=${() => v().Every.Current}><span></span><span class="name">${() => v().Every.Name}</span><span></span></a>
      <For each=${() => v().Clients} fallback=${() => html`<p class="none dim">no runs at any location</p>`}>${c => html`<h2 class="group" title=${() => "the trees of the client " + c.Name}>${() => c.Name}</h2>
        <For each=${() => c.Trees}>${t => html`<Show when=${() => t.Prefix}><a class="tree" href=${() => t.Href} aria-current=${() => t.Current}><span></span><span class="name" title=${() => t.Prefix}>${() => t.Prefix}</span><span class="dim">tree</span></a></Show><For each=${() => t.Places}>${p => place(p, t)}</For>`}</For>`}</For>
    </div>
  </nav>`;
}

// Here is the head of a page narrowed to one location, and what it offers.
function Here() {
  const h = () => parts.here;
  return html`<section class="panel here" id="here" hidden=${() => !h()}><Show when=${h}>${here => html`
    <div class="head"><span title=${() => here().Location}>${() => here().Location}</span><span class="facts"><Show when=${() => here().Lifetime}><span>${() => here().Lifetime}</span></Show><Show when=${() => here().Tree}><span>tree ${() => here().Tree}</span></Show><span>${() => here().Client}</span></span>
      <span class="acts"><Show when=${() => here().Cancel}><${Act} action="/runs/at/cancel" cmd=${() => here().Cancel} label="Cancel every run here" button="sm stop" saidFirst=${true}><input type="hidden" name="at" value=${() => here().Location} /><//></Show></span></div>
    <Show when=${() => here().Send}><${Act} class="send" action="/runs/at/send" reset=${true} label="Send" cmd=${() => `corbie run send ${here().Location}#<key> <event> <data>`}><input type="hidden" name="at" value=${() => here().Location} /><input name="key" placeholder="key" size="14" required /><input name="event" placeholder="event" size="12" required /><input name="data" placeholder="data, as JSON" size="18" /><//></Show>
    <Show when=${() => here().Start}><${Act} class="send" action="/runs/at/start" reset=${true} label="Start" cmd=${() => `corbie run start ${here().Location} --input <input>`}><input type="hidden" name="at" value=${() => here().Location} /><input name="input" placeholder="input, as JSON" size="30" /><//></Show>
  `}</Show></section>`;
}

function List() {
  const n = () => parts.narrowing;
  return html`<div class="head">
    <span>Runs</span>
    <span class="acts">
      <form class="narrow" method="get" action="/runs">
        <Show when=${() => n().At}><input type="hidden" name="at" value=${() => n().At} /></Show><Show when=${() => n().Window !== "6h"}><input type="hidden" name="window" value=${() => n().Window} /></Show>
        <input name="label" value=${() => n().Label} size="22" placeholder="label k=v" aria-label="Only the runs with these labels" />
      </form>
    </span>
  </div>
  <div class="byloc">
    <${Locations} />
    <div class="sections">
      <${Here} />
      <${Section} id="busy" />
      <${Section} id="idle" />
      <${Section} id="singletons" />
      <${Section} id="ended" />
    </div>
  </div>`;
}

// ---- A run's page

const run = () => parts.page;

// Words are what a row says: a name is bold, and a run a send went to or
// came from links to it.
function Words(props) {
  return html`<For each=${() => props.words}>${w => html`${() => (w.Run ? html`<a href=${() => "/run/" + w.Run.ID} title=${() => w.Run.Tip}><b>${() => w.Text}</b></a>` : w.Bold ? html`<b>${() => w.Text}</b>` : w.Text)}`}</For>`;
}

const said = row => row.What.map(w => w.Text).join("");

// Why is one line under the run's head: why the run is where it is. A
// click selects the row it is at.
function Why() {
  const steps = useContext(StepsContext);
  const w = () => parts.summary.Why;
  return html`<Show when=${w}>${y => html`<p class="why" id="why"><button type="button" class="link" onClick=${() => steps.select(y().Row)} disabled=${() => !y().Row}>${() => y().Lead}${() => (y().Since ? " " + ago(y().Since) : "")}${() => (y().Rest ? " " + y().Rest : "")}</button><Show when=${() => y().Facts?.length}>: <span class="facts">${() => y().Facts.map(f => f.Text).join(", ")}</span></Show></p>`}</Show>
  <Show when=${() => parts.summary.Differs?.length}><details class="differs" id="differs"><summary>the file changed since this run started</summary><pre>${() => parts.summary.Differs.join("\n")}</pre></details></Show>`;
}

function Summary() {
  const s = () => parts.summary;
  return html`<div class="head" id="summary">
    <${Icon} type=${() => s().Type} /><span>${() => s().Machine}</span><span class="dim">${() => s().Type}</span>
    <${Status} status=${() => s().Status} failure=${() => s().Failure} />
    <Show when=${() => s().Title}><span class="title" title=${() => s().Title}>${() => s().Title}</span></Show>
    <Show when=${() => s().Address}><a class="key" href=${() => s().At} title=${() => "the runs at " + s().Location}>${() => s().Address}</a></Show>
    <${Defined} at=${() => s().Defined} />
    <span class="facts"><span title=${() => s().ID}>${() => s().ID}</span><span>started ${() => s().Started}</span><Show when=${() => s().Ended}><span>ended ${() => s().Ended}</span><span>took ${() => s().Took}</span></Show></span>
    <span class="acts"><Show when=${() => s().Cancel}><${Act} action=${() => "/run/" + s().ID + "/cancel"} cmd=${() => s().Cancel} label="Cancel" button="stop" saidFirst=${true} /></Show></span>
  </div>`;
}

// Defined links to the machine a run was started from, as its file is now.
function Defined(props) {
  return html`<Show when=${() => props.at}>${d => html`<a class="def" href=${() => d().Href} title="the file on the Machines page as it is now; the drawing here is the definition this run started with">${() => d().File}${() => (d().Cards ? " › " + d().Cards : "")}</a>`}</Show>`;
}

function Waits() {
  const w = () => parts.waits;
  return html`<div id="waits">
    <div class="waitlist">
      <Show when=${() => w().Ended}><div class="wait dim">nothing: the run has ended</div></Show>
      <For each=${() => w().Waits}>${x => html`<div class="wait"><Show when=${() => x.Run} fallback=${() => html`<span>${() => x.Text}</span><Show when=${() => x.Job}>${j => html`<${JobFacts} job=${() => j()} />`}</Show>`}><span>${() => x.Text}${" "}<a href=${() => "/run/" + x.Run.ID} title=${() => x.Run.Tip}><${Icon} type=${() => x.Run.Type} />${() => x.Run.Machine}</a></span></Show><Show when=${() => x.Value}><code class="value" title="the ticket's value: whoever has it can answer">${() => x.Value}</code><${Act} action=${() => "/run/" + w().ID + "/reply"} reset=${true} button="sm" label="Reply" cmd=${() => `corbie run reply ${w().ID} ${x.Value} <answer>`}><input type="hidden" name="ticket" value=${() => x.Value} /><input name="value" placeholder="answer, as JSON" size="16" /><//></Show></div>`}</For>
      <Show when=${() => !w().Ended && !w().Waits.length}><div class="wait dim">nothing</div></Show>
      <Show when=${() => w().Timeout}><div class="wait dim">the run times out ${() => w().Timeout}</div></Show>
    </div>
    <Show when=${() => w().Reply}><${Act} class="send" action=${() => "/run/" + w().ID + "/reply"} reset=${true} label="Reply" cmd=${() => `corbie run reply ${w().ID} <ticket> <answer>`}><input name="ticket" placeholder="a ticket's value" size="20" required /><input name="value" placeholder="answer, as JSON" size="16" /><//></Show>
    <Show when=${() => w().Send}><${Act} class="send" action=${() => "/run/" + w().ID + "/send"} reset=${true} label="Send" cmd=${() => `corbie run send ${w().ID} <event> <data>`}><input name="event" placeholder="event" size="12" required /><input name="data" placeholder="data, as JSON" size="18" /><//></Show>
    <Show when=${() => w().Labels.length}><dl class="labels"><For each=${() => w().Labels}>${l => html`<dt>${() => l.Name}</dt><dd title=${() => l.Value}>${() => l.Value}</dd>`}</For></dl></Show>
  </div>`;
}

// JobFacts are what is known of a job going, and what it has logged.
function JobFacts(props) {
  const j = () => props.job;
  const facts = () => [
    j().Runner && "on " + j().Runner,
    j().Attempt > 1 && "attempt " + j().Attempt,
    j().Sent && "sent " + j().Sent + (j().Began ? ", " + ago(j().Began) + " ago" : ""),
    j().Due && "due " + j().Due,
    j().Again && "restarts at " + j().Again,
    j().Restarts && "restart " + j().Restarts,
  ].filter(Boolean).join(" · ");
  return html`<span class="jobfacts dim">${facts}</span><${Logs} logs=${() => j().Logs} />`;
}

// Logs are the values a job emitted, with their times.
function Logs(props) {
  return html`<Show when=${() => props.logs?.length}><ol class="logs"><For each=${() => props.logs}>${l => html`<li><span class="t">${() => l.Time}</span><span class="n">${() => (l.Name === "log" ? "" : l.Name)}</span><span class="w">${() => l.Text}</span></li>`}</For></ol></Show>`;
}

function Output() {
  const o = () => parts.output;
  return html`<div class="out" id="output">
    <Show when=${() => o().Refused} fallback=${() => html`<Show when=${() => o().Error}><dl class="refusal"><dt>error</dt><dd>${() => o().Error}</dd></dl></Show>`}><dl class="refusal"><dt>refused</dt><dd>${() => o().Refused}</dd><dt>by</dt><dd>${() => o().Layer}</dd><dt>fix</dt><dd>${() => o().Fix}</dd></dl></Show>
    <Show when=${() => o().Output} fallback=${() => html`<Show when=${() => !o().Error}><p class="dim">${() => (o().Ended ? "no output" : "no output yet")}</p></Show>`}><pre class="json">${() => o().Output}</pre></Show>
    <Show when=${() => o().Used}><p class="dim">used: ${() => o().Used}</p></Show>
    <Show when=${() => o().Files.length}><table class="list files"><thead><tr><th>Files</th><th></th><th></th></tr></thead><tbody><For each=${() => o().Files}>${f => html`<tr><td><a href=${() => f.Href} download>${() => f.Path}</a></td><td class="dim">${() => f.Type}</td><td class="dim num">${() => f.Size}</td></tr>`}</For></tbody></table></Show>
  </div>`;
}

function Children() {
  return html`<table class="list kids" id="children"><tbody><For each=${() => parts.children} keyed=${c => c.ID} fallback=${() => html`<tr><td class="dim">no children</td></tr>`}>${c => html`<tr ref=${lit(() => ({ key: "child " + c().ID, data: c(), status: c().Status, ended: c().Ended }))} onClick=${e => go("/run/" + c().ID)(e)}>
    <td><${Status} status=${() => c().Status} failure=${() => c().Failure} /></td><td title=${() => c().Tip}><Show when=${() => c().Type}><${Icon} type=${() => c().Type} /></Show>${() => c().Name}<Show when=${() => c().Key}><span class="dim">${() => c().Key}</span></Show><Show when=${() => c().Name !== c().Machine}>${" "}<span class="dim">${() => c().Machine}</span></Show></td><td class="dim num">${() => c().When}</td><td class="dim num">${() => c().Took}</td></tr>`}</For></tbody></table>`;
}

// ---- Steps
//
// Each child run is a group under the row that started it: an ended child
// is folded and a running one open, until the person folds or opens it. A
// send or a call to a run the UI may read is a group too, folded, whose
// rows are asked for when it is opened. One row is selected; the drawing
// and the details beside the list show its run as of its step. The
// selection follows the newest row, or the row of the folded group it is
// in, until the person moves it; on a running run, Follow then selects the
// newest row and follows again.

// A node is a row as the page lists it: its key on the page, which is the
// keys of the send and call groups it is in then its own, and the groups
// it is in, outermost first. A group is open as the person left it, or by
// its first state; a send's or a call's group holds its rows once read.
function nodesOf(rows, prefix, groups, out, open, traced) {
  for (const row of rows) {
    const key = prefix ? prefix + " " + row.Key : row.Key;
    const node = { row, key, groups };
    out.push(node);
    if (row.Group) {
      const g = { key: row.Group.Run.ID, head: key, open: open(row.Group.Run.ID, !row.Group.Ended), run: row.Group.Run, trace: false };
      node.group = g;
      nodesOf(row.Group.Rows, prefix, [...groups, g], out, open, traced);
    } else if (row.Trace) {
      const g = { key, head: key, open: open(key, false), run: row.Trace.Run, trace: true };
      node.group = g;
      const got = traced[key];
      if (got?.rows) nodesOf(got.rows, key, [...groups, g], out, open, traced);
    }
  }
  return out;
}

// answers holds what the UI answered for each row asked for: a row's
// drawing and details as of its step never change. A row whose answer
// failed is asked for again.
const answers = new Map();

function answer(href) {
  if (answers.has(href)) return answers.get(href);
  return fetch(href)
    .then(async res => {
      if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
      return res.json();
    })
    .then(v => {
      answers.set(href, v);
      return v;
    });
}

// The state of Steps, which the run's page makes and gives its parts.
const StepsContext = createContext();

// followSteps makes the state of Steps: the rows as the page lists them,
// which are shown, the newest, the selected one, and the drawing and the
// details of the selected row, and what changes them.
function followSteps() {
  // folds holds the groups a person folded or opened, by the group's key.
  const [folds, setFolds] = createStore({});
  // traced holds what was read of each send's or call's group opened, by
  // the group's key: its rows, or why they could not be read.
  const [traced, setTraced] = createStore({});
  // picked is the row the person chose, by its key on the page; none while
  // the selection follows the newest row.
  const [picked, setPicked] = createSignal(null);
  const [details, setDetails] = createSignal(false);
  // chosen counts the person's choices of a row, so choosing the row chosen
  // asks for it again where its answer failed.
  const [chosen, setChosen] = createSignal(0);

  const open = (key, first) => folds[key] ?? first;
  const nodes = createMemo(() => (parts.steps ? nodesOf(parts.steps.Rows, "", [], [], open, traced) : []));
  const byKey = createMemo(() => new Map(nodes().map(n => [n.key, n])));
  const visible = createMemo(() => nodes().filter(n => n.groups.every(g => g.open)));
  // newest is the newest row of the run and the runs under it; the rows of
  // a send's or a call's group are of other runs, and are left out.
  // newest is the row the page selects while it follows the run: the
  // newest, or a failed run's deepest failure, as the UI gives it.
  const newest = createMemo(() => byKey().get(parts.steps?.Follows) ?? nodes().filter(n => !n.groups.some(g => g.trace)).at(-1));
  // shownOf gives the node that shows a node: itself, or the head of the
  // outermost folded group it is in.
  const shownOf = node => {
    const folded = node?.groups.find(g => !g.open);
    return folded ? byKey().get(folded.head) : node;
  };
  const selected = createMemo(() => shownOf(byKey().get(picked()) ?? newest()));
  // address is where the page asks for the selected row's drawing and
  // details; none while it follows the newest row, whose drawing and
  // details the UI sends.
  const address = createMemo(() => {
    const node = selected();
    return picked() && node ? `/run/${node.row.Run.ID}/at?row=${encodeURIComponent(node.row.Key)}&page=${encodeURIComponent(run().ID)}` : "";
  });
  // shown is the drawing and the details of the selected row. While the
  // answer for a row is on its way, the selection is shown at once and the
  // drawing before it stays.
  const shown = createDeferred(() => {
    chosen();
    return address() ? answer(address()) : parts.shown;
  });

  // The rows of each send's or call's group opened are asked for once; a
  // group folded is asked for again when it is opened.
  createEffect(
    () => nodes()
      .filter(n => n.group?.trace && n.group.open && !traced[n.key])
      .map(n => ({ key: n.key, href: `/run/${n.row.Run.ID}/group?row=${encodeURIComponent(n.row.Key)}&depth=${n.row.Depth}&page=${encodeURIComponent(run().ID)}` })),
    wanted => {
      for (const n of wanted) {
        setTraced(s => { s[n.key] = { asked: true }; });
        fetch(n.href)
          .then(async res => {
            if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
            return res.json();
          })
          .then(rows => setTraced(s => { s[n.key] = { rows }; }), err => setTraced(s => { s[n.key] = { error: err.message }; }));
      }
    }
  );

  const fold = (g, to) => {
    setFolds(s => { s[g.key] = to; });
    if (!to && g.trace) setTraced(s => { delete s[g.key]; });
  };
  // hovered is the row under the pointer, whose cards the drawing lights.
  const [hovered, setHovered] = createSignal(null);
  // playing is how fast the selection plays through the rows: none, a row
  // a second, or three.
  const [playing, setPlaying] = createSignal(0);
  const choose = node => {
    if (!node) return;
    setFolds(s => { for (const g of node.groups) s[g.key] = true; });
    setPicked(node.key);
    setChosen(n => n + 1);
  };
  // The selection plays through the rows shown, one at a time, and stops at
  // the last.
  createEffect(
    () => playing(),
    speed => {
      if (!speed) return;
      const timer = setInterval(() => {
        const rows = visible(), next = rows[rows.indexOf(selected()) + 1];
        if (next) choose(next);
        else setPlaying(0);
      }, speed === 1 ? 1000 : 333);
      return () => clearInterval(timer);
    }
  );
  return {
    nodes, visible, selected, shown, picked, details, setDetails, traced, fold, hovered, setHovered, playing,
    // pick selects a row the person chose, opening the groups it is in,
    // and stops the selection playing. Choosing the row chosen asks for its
    // drawing again where it failed.
    pick(node) {
      if (!node) return;
      setPlaying(0);
      choose(node);
    },
    // select selects a row by its key.
    select(key) {
      this.pick(byKey().get(key));
    },
    // touched selects the newest row about a card of the drawing.
    touched(path) {
      const rows = nodes().filter(n => !n.groups.some(g => g.trace) && n.row.Cards?.includes(path));
      this.pick(rows.sort((a, b) => Date.parse(a.row.At) - Date.parse(b.row.At)).at(-1));
    },
    // play plays from the row selected, faster when it plays already, and
    // stops when it plays fast.
    play: () => setPlaying(p => (p + 1) % 3),
    // pause plays or stops.
    pause: () => setPlaying(p => (p ? 0 : 1)),
    // follow selects the newest row, and goes on selecting each new row.
    follow() {
      setPlaying(0);
      setPicked(null);
    },
    // head gives the node at the head of a group.
    head: g => byKey().get(g.head),
    // all opens or folds every group; folding chooses the row at the head
    // of the outermost group the selection is in.
    all(to) {
      const sel = selected();
      for (const n of nodes()) if (n.group) fold(n.group, to);
      if (!to && sel?.groups.length) this.pick(byKey().get(sel.groups[0].head));
    },
  };
}

function StepRow(props) {
  const steps = useContext(StepsContext);
  const n = () => props.node;
  const r = () => n().row;
  return html`<div class=${() => ["step", r().Run.Type, r().Kind, { sel: steps.selected()?.key === n().key }]} style=${() => "--d: " + r().Depth} data-key=${() => n().key} ref=${lit(() => ({ key: "step " + n().key, data: n().key }))} onMouseEnter=${() => steps.setHovered(n().row.Cards ?? null)} onMouseLeave=${() => steps.setHovered(null)} onClick=${e => {
    if (e.target.closest("a, button")) return;
    steps.pick(n());
    steps.setDetails(true);
  }}>
    <span class="t" title=${() => r().Exact}>${() => r().Time}</span><span class="link"></span>
    <span class="who" title=${() => r().Run.Tip}>${() => (n().group ? html`<button class="fold" type="button" aria-label=${() => "fold or open " + n().group.run.Machine} onClick=${() => {
      const g = n().group, sel = steps.selected();
      steps.fold(g, !g.open);
      // Folding the group the selected row is in selects the group's head.
      if (g.open && sel?.groups.some(o => o.key === g.key)) steps.pick(steps.head(g));
    }}></button>` : html`<span class="fold"></span>`)}<Show when=${() => r().Run.Type}><${Icon} type=${() => r().Run.Type} /></Show><span>${() => r().Run.Machine}</span></span>
    <span class="what" title=${() => said(r())}><${Words} words=${() => r().What} /><Show when=${() => r().Took}><span class="took"> · ${() => r().Took}</span></Show>${() => (r().Group ? html`<span class="sum">${r().Group.Run.Title ? html` · <span class="name">${r().Group.Run.Title}</span>` : ""} · <span class=${["st", r().Group.Status]}>${r().Group.Status}${r().Group.Failure ? ` (${r().Group.Failure})` : ""}</span> · ${plural(r().Group.Steps, "step")}${r().Group.Took ? " · " + r().Group.Took : ""}</span>` : "")}${() => (r().Trace ? html`<span class="sum">${r().Trace.Run.Title && r().Trace.Run.Title !== r().Trace.Run.Machine ? html` · <span class="name">${r().Trace.Run.Title}</span>` : ""}${r().Trace.Status ? html` · <span class=${["st", r().Trace.Status]}>${r().Trace.Status}${r().Trace.Failure ? ` (${r().Trace.Failure})` : ""}</span>` : ""}${r().Trace.Took ? " · " + r().Trace.Took : ""}</span>` : "")}</span>
  </div>`;
}

// Rows are the rows of one list, each group's rows in a list under its head.
function Rows(props) {
  return html`<For each=${() => props.nodes} keyed=${n => n.key}>${n => html`<li class=${() => ({ group: !!n().group, trace: !!n().group?.trace, folded: !!n().group && !n().group.open })}><${StepRow} node=${() => n()} /><Show when=${() => n().group}><ol><${Inside} node=${() => n()} /></ol></Show></li>`}</For>`;
}

// Inside is the rows of a group: a child's, or those of the run a send or
// a call reached, once they are read, or why they could not be.
function Inside(props) {
  const steps = useContext(StepsContext);
  const inside = createMemo(() => steps.nodes().filter(m => m.groups.at(-1)?.head === props.node.key));
  const got = () => (props.node.group.trace ? steps.traced[props.node.key] : undefined);
  return html`<Show when=${() => !got()?.error} fallback=${() => html`<li class="dim none">${() => got().error}</li>`}><${Rows} nodes=${inside} /><Show when=${() => got()?.rows && !inside().length}><li class="dim none">nothing yet</li></Show></Show>`;
}

function Steps() {
  const steps = useContext(StepsContext);
  const s = () => parts.steps;
  const top = () => steps.nodes().filter(n => !n.groups.length);
  return html`<div class="steps" id="steps">
    <div class="head"><h2>Steps</h2><span class="facts" ref=${bump(() => s().Count)}>${() => s().Count}</span>
      <span class="legend"><span class="statechart"><i></i>statechart</span><span class="flow"><i></i>flow</span><span class="agent"><i></i>agent</span></span>
      <span class="acts"><button class="btn sm" type="button" data-play title="play through the rows from the one selected; press again to play faster, a third time to stop; Space plays and stops" onClick=${() => steps.play()}>${() => ["Play", "Faster", "Stop"][steps.playing()]}</button><Show when=${() => !s().Ended && steps.picked()}><button class="btn sm" type="button" title="select the newest row, and go on selecting each new row" onClick=${() => steps.follow()}>Follow</button></Show><button class="btn sm" type="button" onClick=${() => steps.all(true)}>Open all</button><button class="btn sm" type="button" onClick=${() => steps.all(false)}>Fold all</button></span>
    </div>
    <ol class=${() => ["steplist", { dated: s().Dated }]} tabindex="0"><${Rows} nodes=${top} /><Show when=${() => !s().Rows.length}><li class="dim none">nothing yet</li></Show></ol>
  </div>`;
}

// keys moves the selection through the rows shown as a person presses the
// arrows, folds and opens groups, and shows the details, while the list
// has the focus or nothing else on the page has it.
function keys(steps) {
  return e => {
    if ((e.target !== document.body && !e.target.closest("#steps .steplist")) || e.altKey || e.ctrlKey || e.metaKey) return;
    if (e.key === "Enter" && e.target.closest("a, button")) return;
    const node = steps.selected();
    const list = document.querySelector("#steps .steplist");
    if (!node || !list || list.offsetParent === null) return;
    const rows = steps.visible();
    const at = rows.indexOf(node);
    const inside = node.groups.at(-1);
    switch (e.key) {
      case "ArrowDown": steps.pick(rows[at + 1]); break;
      case "ArrowUp": steps.pick(rows[at - 1]); break;
      case "ArrowLeft":
        if (node.group?.open) steps.fold(node.group, false);
        else if (inside) { steps.fold(inside, false); steps.pick(steps.head(inside)); }
        break;
      case "ArrowRight": if (node.group) steps.fold(node.group, true); break;
      case "Enter": steps.setDetails(true); break;
      case " ": steps.pause(); break;
      default: return;
    }
    e.preventDefault();
  };
}

// The path from the run's root to the deepest running run lights the
// selected row's run. A run off the path, below this page's run, is shown
// by the groups down to it.
function Path() {
  const steps = useContext(StepsContext);
  const links = createMemo(() => {
    const path = parts.path.map(l => ({ link: l, sent: l.Sent, trace: false }));
    const node = steps.selected();
    const down = !node || path.some(p => p.link.ID === node.row.Run.ID) ? path : [...path.slice(0, path.findIndex(p => p.link.This) + 1), ...node.groups.map(g => ({ link: g.run, trace: g.trace }))];
    return down.map((p, at) => ({ ...p, at }));
  });
  return html`<nav class="path" id="path"><For each=${links} keyed=${p => p.at + " " + p.link.ID}>${q => html`${() => (q().at ? (q().sent || q().trace ? html`<span class="sep send" title=${() => (q().sent ? q().sent + " of the run before started it" : "")}>⇢</span>` : html`<span class="sep">›</span>`) : "")}<a href=${() => "/run/" + q().link.ID} title=${() => q().link.Tip} aria-current=${() => (q().link.This ? "page" : undefined)} class=${() => ({ sel: steps.selected()?.row.Run.ID === q().link.ID })}><Show when=${() => q().link.Type}><${Icon} type=${() => q().link.Type} /></Show>${() => q().link.Machine}</a>`}</For></nav>`;
}

// lineHref gives the address of a line of a file on the Machines page.
function lineHref(at) {
  const i = at.lastIndexOf(":");
  return `/machines?file=${encodeURIComponent(at.slice(0, i))}&view=text#L${at.slice(i + 1)}`;
}

// View is the drawing of the page's run with the cards down to the
// selected row's run unfolded, as of the row's step. The drawing moves its
// own view to what the step marks. While the answer for a row is on its
// way, the drawing before it stays, dimmed.
function View() {
  const steps = useContext(StepsContext);
  const v = () => steps.shown();
  return html`<section class=${() => ["panel", "asof", { pending: isPending(() => steps.shown()) }]} id="view">
    <Show when=${() => v()?.Row}>${row => html`<div class="head"><${Icon} type=${() => v().Type} /><span>${() => row().Run.Machine}</span><span class="dim">${() => v().Type}</span><${Defined} at=${() => v().Defined} /><span class="facts">as of step ${() => row().Step}, ${() => v().Time}</span></div>`}</Show>
    <div class="drawn"><Show when=${() => v()?.Drawing} fallback=${() => html`<p class="dim">${() => (v()?.Row ? "no drawing" : "a run in another client's tree: only its status can be read")}</p>`}><${Drawing} scene=${() => v().Drawing} lit=${() => steps.hovered() ?? []} onCard=${path => steps.touched(path)} lineHref=${() => (parts.summary.Defined ? lineHref : undefined)} /></Show></div>
  </section>`;
}

const [detailTab, detailBar] = tabs(["what", "input", "output", "lines"], "what", n => ({ what: "What happened", input: "Input", output: "Output", lines: "Record" })[n]);

function Details() {
  const steps = useContext(StepsContext);
  const v = () => steps.shown();
  return html`<section class="panel tabbed details" id="details" hidden=${() => !steps.details()}><Show when=${() => v()?.Row}>${row => html`
    <div class="head"><Show when=${() => row().Run.Type}><${Icon} type=${() => row().Run.Type} /></Show><span>${() => row().Run.Machine}</span><span class="dim">step ${() => row().Step} · ${() => row().Time}</span>
      <span class="facts">${() => said(row())}</span>
      <span class="acts"><Show when=${() => row().Run.ID !== v().Page}><a class="btn sm" href=${() => "/run/" + row().Run.ID}>Open ${() => row().Run.Machine} ›</a></Show></span></div>
    ${detailBar}
    <div class="pane" hidden=${() => detailTab() !== "what"}><dl class="kv"><For each=${() => row().Facts} fallback=${() => html`<dt></dt><dd class="dim">${() => said(row())}</dd>`}>${f => html`<dt>${() => f.Name}</dt><dd><Show when=${() => f.Run} fallback=${() => f.Text}><a href=${() => "/run/" + f.Run.ID} title=${() => f.Run.Tip}><Show when=${() => f.Run.Type}><${Icon} type=${() => f.Run.Type} /></Show>${() => f.Run.Machine}</a>${" "}<span class="dim">${() => f.Text}</span></Show></dd>`}</For></dl></div>
    <div class="pane" hidden=${() => detailTab() !== "input"}><Show when=${() => v().Input} fallback=${() => html`<p class="dim">nothing went in</p>`}><pre class="json">${() => v().Input}</pre></Show></div>
    <div class="pane" hidden=${() => detailTab() !== "output"}><Show when=${() => v().Output} fallback=${() => html`<p class="dim">nothing came out</p>`}><pre class="json">${() => v().Output}</pre></Show><Show when=${() => v().Logs?.length}><h3 class="label">logged by ${() => row().Job}</h3><${Logs} logs=${() => v().Logs} /></Show></div>
    <div class="pane" hidden=${() => detailTab() !== "lines"}><ol class="record"><For each=${() => v().Lines}>${l => html`<li><span class="t">${() => l.Time}</span><span class="i" title=${() => l.Machine}>${() => l.Machine}</span><span class="n">${() => l.Step || ""}</span><span class=${() => ["w", { end: !!l.End }, l.End]}>${() => (l.Way === "in" ? html`<b>&lt;-</b>${" "}` : l.Way === "out" ? html`<b>-&gt;</b>${" "}` : "")}${() => l.Text}</span></li>`}</For></ol></div>
  `}</Show></section>`;
}

function RunPage() {
  const steps = followSteps();
  // The selected row is kept in view.
  createEffect(
    () => steps.selected()?.key,
    key => {
      if (key) document.querySelector(`.step[data-key="${CSS.escape(key)}"]`)?.scrollIntoView({ block: "nearest" });
    }
  );
  onSettled(() => {
    const press = keys(steps);
    document.addEventListener("keydown", press);
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      document.removeEventListener("keydown", press);
      clearInterval(tick);
    };
  });
  const names = run().Mine ? ["steps", "waits", "output", "children"] : ["waits", "output", "children"];
  const [tab, bar] = tabs(names, names[0], n => ({ steps: "Steps", waits: "Waiting on", output: "Output", children: "Children" })[n]);
  // The drawing and the details of a row whose answer failed say why; the
  // row can be selected again to ask again.
  return html`<${StepsContext} value=${steps}>
  <${Summary} />
  <${Why} />
  <${Path} />
  <div class="split">
    <div class="panel tabbed">
      ${bar}
      <Show when=${() => run().Mine}><div class="pane" hidden=${() => tab() !== "steps"}><${Steps} /></div></Show>
      <div class="scroll pane" hidden=${() => tab() !== "waits"}><${Waits} /></div>
      <div class="scroll pane" hidden=${() => tab() !== "output"}><${Output} /></div>
      <div class="scroll pane" hidden=${() => tab() !== "children"}><${Children} /></div>
    </div>
    <${Handle} size="steps" />
    <div class="side">
      <${Errored} fallback=${err => html`<section class="panel asof" id="view"><p class="dim failed">${() => String(err()?.message ?? err())}</p></section>`}>
        <${View} />
        <${Handle} size="details" row=${true} />
        <${Details} />
      <//>
    </div>
  </div>
  <//>`;
}

mount(parts.page ? RunPage : List);
