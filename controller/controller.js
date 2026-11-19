// The Controller page: the runners and clients the controller trusts, its
// trees, and the secrets and values set at each location. Each action runs
// one of Corbie's corbie controller commands.
import { html, parts, mount, lit, Act } from "ui";

const join = (list, by = ", ") => (list ?? []).join(by);
const runs = at => "/runs?at=" + encodeURIComponent(at);

// Hidden is a field the page fills in: the name or the location an action
// is for.
const Hidden = props => html`<input type="hidden" name=${() => props.name} value=${() => props.value} />`;

// Change is an action that sets one list of a party's: its tags, its
// prefixes or its ceiling, in a fold of its line.
function Change(props) {
  return html`<details><summary class="btn sm">${() => props.label}</summary>
    <${Act} action=${() => "/controller/" + props.act} cmd=${() => `corbie controller enrol ${props.party.Name} ${props.flag}`} label="Set" button="sm">
      <${Hidden} name="name" value=${() => props.party.Name} />
      <input name=${() => props.act} value=${() => join(props.value, ",")} size=${() => props.size} aria-label=${() => props.aria} required=${() => props.required} />
    <//>
  </details>`;
}

// Acts gives the actions a party's line offers, which the UI gives with it.
function Acts(props) {
  const one = act => {
    switch (act) {
      case "enrol":
        return html`<span class="dim">removed</span>${" "}<a href=${() => "#enrol-" + props.party.Kind}>Enrol</a>`;
      case "tags":
        return html`<${Change} party=${() => props.party} act="tags" label="Tags" flag="--tags <list>" value=${() => props.party.Tags} size="14" aria="Its tags" />`;
      case "prefixes":
        return html`<${Change} party=${() => props.party} act="prefixes" label="Prefixes" flag="--at <prefix>" value=${() => props.party.At} size="14" aria="Its prefixes" required=${true} />`;
      case "ceiling":
        return html`<${Change} party=${() => props.party} act="ceiling" label="Ceiling" flag="--ceiling <list>" value=${() => props.party.Ceiling} size="18" aria="Its ceiling" />`;
      case "remove":
        return html`<${Act} action="/controller/remove" cmd=${() => "corbie controller remove " + props.party.Name} label="Remove" button=${["sm", "stop"]}><${Hidden} name="name" value=${() => props.party.Name} /><//>`;
    }
  };
  return html`<For each=${() => props.party.Acts}>${one}</For>`;
}

// Party is one runner's or client's line.
function Party(props) {
  return html`<tr class=${() => ({ removed: props.party.Removed })} ref=${lit(() => ({ key: "party " + props.party.Name, data: props.party }))}>
    <td>${() => props.party.Name}</td><td class="dim cut" title=${() => props.party.Fingerprint}>${() => props.party.Fingerprint}</td>
    <Show when=${() => props.party.Kind === "runner"} fallback=${() => html`<td class="cut" title=${() => join(props.party.Ceiling) + (props.party.Deny?.length ? "; deny " + join(props.party.Deny) : "")}>${() => (props.party.Ceiling?.length ? join(props.party.Ceiling) : html`<span class="dim">nothing</span>`)}${() => (props.party.Deny?.length ? html`${" "}<span class="dim">deny ${() => join(props.party.Deny)}</span>` : "")}</td>`}>
      <td>${() => join(props.party.At)}</td><td>${() => (props.party.Tags?.length ? join(props.party.Tags) : html`<span class="dim">—</span>`)}</td>
    </Show>
    <td class="row-acts"><${Acts} party=${() => props.party} /></td>
  </tr>`;
}

// Parties is the runners or the clients, with the form that enrols one.
function Parties(props) {
  return html`<div class="panel">
    <div class="head"><h2>${() => props.title}</h2></div>
    <table class="list"><thead><tr>${() => props.heads.map(h => html`<th>${h}</th>`)}</tr></thead>
      <tbody><For each=${() => parts.parties.filter(p => p.Kind === props.kind)} keyed=${p => p.Name}>${p => html`<${Party} party=${p} />`}</For></tbody></table>
    <div id=${() => "enrol-" + props.kind}>${() => props.children}</div>
  </div>`;
}

function Controller() {
  return html`<div class="cols">
    <div class="contents">
      <div class="panel">
        <div class="head"><h2>Check</h2></div>
        <${Act} class="enrol check" action="/controller/check" block=${true} label="Check" button="go" cmd="corbie check">
          <span class="dim">starts one small run, and gives each part of the setup that took it, how long it took, and which did not respond</span>
        <//>
      </div>
      <${Parties} kind="runner" title="Runners" heads=${["Name", "Card", "At", "Tags", ""]}>
        <${Act} class="enrol" action="/controller/enrol" reset=${true} block=${true} label="Enrol" button="go" cmd="corbie controller enrol <card> --tags <list> --at <prefix>">
          <textarea name="card" rows="2" placeholder="a runner's card" aria-label="A runner's card" required></textarea>
          <input name="tags" placeholder="tags, such as gpu,big" aria-label="Its tags" />
          <input name="prefixes" placeholder="prefixes, such as /home,/netops" aria-label="Its prefixes" />
          <label><input type="checkbox" name="replace" value="1" /> Replace</label>
        <//>
      <//>
      <${Parties} kind="client" title="Clients" heads=${["Name", "Card", "Ceiling", ""]}>
        <${Act} class="enrol" action="/controller/enrol" reset=${true} block=${true} label="Enrol" button="go" cmd="corbie controller enrol <card> --ceiling <list>">
          <textarea name="card" rows="2" placeholder="a client's card" aria-label="A client's card" required></textarea>
          <input name="ceiling" placeholder="ceiling, such as network,disk" aria-label="Its ceiling" />
          <label><input type="checkbox" name="replace" value="1" /> Replace</label>
        <//>
      <//>
    </div>
    <div class="panel">
      <div class="head"><h2>Trees</h2></div>
      <table class="list"><thead><tr><th>Location</th><th>Lifetime</th></tr></thead>
        <tbody><For each=${() => parts.trees} keyed=${t => t.Prefix} fallback=${() => html`<tr><td class="dim">no trees</td></tr>`}>${t => html`
          <tr class="tree" ref=${lit(() => ({ key: "tree " + t().Prefix, data: [t().Client, t().Directory] }))}><td><a href=${() => runs(t().Prefix)}>${() => t().Prefix}</a></td><td class="dim cut" title=${() => `the tree of ${t().Client}, last submitted from ${t().Directory}`}>tree of ${() => t().Client}, from ${() => t().Directory}</td></tr>
          <For each=${() => t().Locations} keyed=${l => l.At}>${l => html`<tr ref=${lit(() => ({ key: "location " + l().At, data: l() }))}><td class="in" title=${() => l().At}><a href=${() => runs(l().At)}>${() => l().Name}</a></td><td>${() => l().Lifetime}</td></tr>`}</For>`}</For></tbody></table>
    </div>
    <div class="panel">
      <div class="head"><h2>Secrets and values</h2></div>
      <table class="list"><thead><tr><th>At</th><th>Name</th><th>Kind</th><th>Value</th><th></th></tr></thead>
        <tbody><For each=${() => parts.settings} keyed=${s => s.Kind + " " + s.At + " " + s.Name} fallback=${() => html`<tr><td class="dim" colspan="5">no secrets and no values</td></tr>`}>${s => html`
          <tr ref=${lit(() => ({ key: `${s().Kind} ${s().At} ${s().Name}`, data: s() }))}><td>${() => s().At}</td><td>${() => s().Name}</td><td class="dim">${() => s().Kind}</td><td class="cut" title=${() => s().Value}>${() => s().Value}</td>
            <td class="row-acts"><${Act} action=${() => `/controller/${s().Kind}-remove`} cmd=${() => `corbie controller ${s().Kind} remove ${s().Name} --at ${s().At}`} label="Remove" button=${["sm", "stop"]}><${Hidden} name="name" value=${() => s().Name} /><${Hidden} name="at" value=${() => s().At} /><//></td></tr>`}</For></tbody></table>
      <${Act} class="enrol" action="/controller/secret-set" reset=${true} label="Set" button="go" cmd="corbie controller secret set <NAME> - --at <location>">
        <input name="name" placeholder="secret name" size="14" aria-label="The secret's name" required />
        <input name="at" placeholder="at /" size="14" aria-label="The location it is set at" />
        <input name="value" type="password" placeholder="value" autocomplete="off" aria-label="Its value" required />
      <//>
      <${Act} class="enrol" action="/controller/value-set" reset=${true} label="Set" button="go" cmd="corbie controller value set <NAME> <VALUE> --at <location>">
        <input name="name" placeholder="value name" size="14" aria-label="The value's name" required />
        <input name="at" placeholder="at /" size="14" aria-label="The location it is set at" />
        <input name="value" placeholder="value" autocomplete="off" aria-label="Its text" required />
      <//>
    </div>
  </div>`;
}

mount(Controller);
