import { test, settle, wait } from "/test/page/testdata/test.js";
test("camera", async () => {
  await settle(3); await wait(600);
  const cam = () => document.querySelector("#view g.camera")?.getAttribute("style") ?? "none";
  const rows = [...document.querySelectorAll(".step")];
  const pick = async pred => { const r = rows.find(pred); r.click(); await settle(3); await wait(900); return cam(); };
  const out = { loaded: cam() };
  out.child = await pick(r => r.textContent.includes("review") && r.textContent.includes("entered"));
  out.parent = await pick(r => r === rows[0]);
  out.childAgain = await pick(r => r.textContent.includes("review") && r.textContent.includes("entered"));
  const v = document.querySelector("#view .viewport");
  out.view = [v.clientWidth, v.clientHeight];
  throw new Error("CAMERA " + JSON.stringify(out));
});
