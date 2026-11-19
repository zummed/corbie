// The tests of the Controller page in Chrome, on a lab whose actions run
// Corbie's commands. The page is read again only when an action changes the
// store, so what appears after an action came at once.
import { test, settle, wait, assert, equal, text } from "/test/page/testdata/test.js";
import { parts } from "ui";

const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];

async function until(ok, what, ms = 4000) {
  const end = performance.now() + ms;
  while (!ok()) {
    if (performance.now() > end) throw new Error("waited for " + what);
    await wait(20);
  }
  await settle();
}

// panel gives the panel headed by a name.
const panel = name => $$("main .panel").find(p => text($(".head h2", p)) === name);

// rowOf gives the row of a table whose first cell is a name.
const rowOf = (p, name) => $$("tbody tr", p).find(tr => text(tr.cells[0]) === name);

test("controller_lists_parties_trees_secrets_and_values", () => {
  for (const [title, kind] of [["Runners", "runner"], ["Clients", "client"]]) {
    const names = $$("tbody tr", panel(title)).map(tr => text(tr.cells[0]));
    equal(names, parts.parties.filter(p => p.Kind === kind).map(p => p.Name), `the ${kind}s`);
  }
  const gpu = rowOf(panel("Runners"), "gpu-box");
  equal(text(gpu.cells[2]), "/home, /hello", "the runner's prefixes");
  equal(text(gpu.cells[3]), "gpu, big", "the runner's tags");
  const trees = $$("tbody tr.tree a", panel("Trees")).map(a => a.getAttribute("href"));
  equal(trees, ["/runs?at=%2Fhello", "/runs?at=%2Fhome"], "the trees, linked to their runs");
  const settings = $$("tbody tr", panel("Secrets and values")).map(tr => [...tr.cells].slice(0, 4).map(text));
  equal(settings, [["/", "ANTHROPIC_KEY", "secret", ""], ["/home", "HUB", "value", "192.0.2.8:8123"]], "the secrets and values");
});

test("controller_removed_party_offers_only_enrol", () => {
  const old = rowOf(panel("Runners"), "old-box");
  assert(old.classList.contains("removed"), "the removed runner is not marked");
  equal($$(".row-acts form, .row-acts details", old).length, 0, "the removed runner's actions");
  equal($(".row-acts a", old).getAttribute("href"), "#enrol-runner", "the removed runner's Enrol");
  const gpu = rowOf(panel("Runners"), "gpu-box");
  equal($$(".row-acts summary", gpu).map(text), ["Tags", "Prefixes"], "a runner's changes");
  equal($$(".row-acts button[data-cmd]", gpu).map(b => b.dataset.cmd), ["corbie controller enrol gpu-box --tags <list>", "corbie controller enrol gpu-box --at <prefix>", "corbie controller remove gpu-box"], "a runner's commands");
});

test("controller_forms_show_their_commands", () => {
  const cmds = $$("main form.enrol button[data-cmd]").map(b => b.dataset.cmd);
  equal(cmds, [
    "corbie controller enrol <card> --tags <list> --at <prefix>",
    "corbie controller enrol <card> --ceiling <list>",
    "corbie controller secret set <NAME> - --at <location>",
    "corbie controller value set <NAME> <VALUE> --at <location>",
  ], "the commands of the forms");
  equal($("main form.enrol input[name=value][type=password]")?.getAttribute("autocomplete"), "off", "the secret's field");
});

test("controller_actions_run_their_commands", async () => {
  const form = $$("main form.enrol").find(f => f.getAttribute("action") === "/controller/value-set");
  form.elements.name.value = "REGION";
  form.elements.value.value = "eu-west-2";
  const started = performance.now();
  form.requestSubmit();
  const settings = panel("Secrets and values");
  await until(() => rowOf(settings, "/") && $$("tbody tr", settings).some(tr => text(tr.cells[1]) === "REGION"), "the value set");
  const took = performance.now() - started;
  assert(took < 2000, `the value took ${Math.round(took)}ms to appear`);
  const row = $$("tbody tr", settings).find(tr => text(tr.cells[1]) === "REGION");
  equal(text(row.cells[3]), "eu-west-2", "the value's text");
  assert(row.classList.contains("new"), "the new row is not lit: " + row.className);
  await until(() => !$(".said", form).classList.contains("waiting"), "the command's answer");
  assert(!$(".said", form).classList.contains("bad"), "the command failed: " + text($(".said", form)));
  equal(form.elements.name.value, "", "the form after it succeeded");
});

test("controller_changed_party_is_lit", async () => {
  const gpu = rowOf(panel("Runners"), "gpu-box");
  const form = $$("form", gpu).find(f => f.getAttribute("action") === "/controller/tags");
  form.elements.tags.value = "gpu,fast";
  form.requestSubmit();
  await until(() => text(rowOf(panel("Runners"), "gpu-box").cells[3]) === "gpu, fast", "the runner's tags as set");
  const row = rowOf(panel("Runners"), "gpu-box");
  assert(row === gpu, "the runner's row was drawn again");
  assert(row.classList.contains("moved"), "the changed row is not lit: " + row.className);
});

test("controller_secret_value_goes_on_standard_input_only", async () => {
  const form = $$("main form.enrol").find(f => f.getAttribute("action") === "/controller/secret-set");
  form.elements.name.value = "GITHUB_TOKEN";
  form.elements.value.value = "hunter2";
  form.requestSubmit();
  const settings = panel("Secrets and values");
  await until(() => $$("tbody tr", settings).some(tr => text(tr.cells[1]) === "GITHUB_TOKEN"), "the secret set");
  equal(form.elements.value.value, "", "the secret's field after it was set");
  assert(!document.body.innerHTML.includes("hunter2") && !JSON.stringify(parts).includes("hunter2"), "the page holds the secret's value");
});

test("controller_panels_fit_their_rows", () => {
  for (const p of $$("main .panel")) {
    const table = $("table", p);
    if (!table) continue;
    const widest = [...table.rows[1]?.cells ?? []].map(c => `${text(c).slice(0, 20)}=${Math.round(c.getBoundingClientRect().width)}`).join(" ");
    assert(table.getBoundingClientRect().right <= p.getBoundingClientRect().right + 1, `the table of ${text($(".head h2", p))} is ${Math.round(table.getBoundingClientRect().width)}px in a panel of ${Math.round(p.getBoundingClientRect().width)}px: ${widest}`);
  }
  for (const cell of $$("main td.cut")) assert(getComputedStyle(cell).textOverflow === "ellipsis", "a cut cell is not cut with …");
});
