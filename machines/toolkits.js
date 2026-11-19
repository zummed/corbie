// The Toolkits page: the toolkits the agents in the directory offer, and the
// toolkit chosen with its tools and what offers and starts it, or its
// file's text.
import { html, parts, mount, Icon, Handle, plural } from "ui";
import { Problems, Code, machineHref, base } from "/machines/shared.js";

const toolkitHref = (id, text) => "/toolkits?" + new URLSearchParams(text ? { toolkit: id, view: "text" } : { toolkit: id });

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
  const v = () => parts.toolkits;
  const k = () => v().Open;
  return html`<div class="panel view toolkit">
    <Show when=${k} fallback=${() => html`<div class="body"><p class="dim">no toolkit chosen</p></div>`}>
      <div class="head"><${Icon} type="toolkit" /><span>${() => k().Name}</span><span class="facts">${() => k().File}</span>${() => (k().Opening ? html`<span class="approve">opening it needs approval</span>` : "")}</div>
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
  const v = () => parts.toolkits;
  return html`<div class="filelist list">
    ${() => (v().Unread ? html`<p class="unread">not read to its end: ${() => v().Unread}</p>` : "")}
    <${Problems} problems=${() => v().Problems} />
    <For each=${() => v().Toolkits} keyed=${k => k.ID} fallback=${() => html`<p class="dim">no toolkits here</p>`}>${k => html`
      <a href=${() => toolkitHref(k().ID)} aria-current=${() => (v().Open?.ID === k().ID ? "page" : undefined)}><${Icon} type="toolkit" /><span class="name" title=${() => k().File}>${() => k().Name}</span>${() => (k().Broken ? html`<span class="chip">has problems</span>` : "")}<small>${() => plural(k().Tools.length, "tool")} · ${() => k().Description}</small></a>`}</For>
  </div>`;
}

function Toolkits() {
  return html`<div class="ide">
    <nav class="panel files" aria-label=${() => "The toolkits in " + parts.toolkits.DirFull}>
      <div class="head"><span class="facts" title=${() => parts.toolkits.DirFull}>${() => parts.toolkits.Dir}</span></div>
      <${List} />
    </nav>
    <${Handle} size="files" />
    <${View} />
  </div>`;
}

mount(Toolkits);
