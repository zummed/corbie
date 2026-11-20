// The tests of the page of a desk that sent a knock to a room: the send's
// group is folded, its rows are read when it is opened, and a group that
// cannot be read says why and is read again when it is next opened.
import { test, until, click, words, assert } from "/test/page/testdata/test.js";

const rowOf = said => [...document.querySelectorAll("#steps .step")].find(r => words(r.querySelector(".what")).startsWith(said));
const group = () => rowOf("⇢ sent knock to /house/room#1").closest("li");

function failing(part) {
  const real = window.fetch;
  window.fetch = (url, ...rest) => (String(url).includes(part) ? Promise.resolve(new Response("the store cannot be read", { status: 500 })) : real(url, ...rest));
  return () => { window.fetch = real; };
}

test("a_send_is_a_folded_group_read_when_opened", async () => {
  await until(() => rowOf("⇢ sent knock to /house/room#1"), "the send's row");
  assert(group().matches(".group.trace.folded"), "the send is not a folded group");
  click(rowOf("⇢ sent knock to /house/room#1").querySelector("button.fold"));
  await until(() => group().querySelector("ol .step") && words(group().querySelector("ol")).includes("started by"), "the room's rows");
  click(rowOf("⇢ sent knock to /house/room#1").querySelector("button.fold"));
  await until(() => group().classList.contains("folded"), "the group to fold");
});

test("a_failed_group_says_why_and_is_read_again", async () => {
  const mend = failing("/group?");
  click(rowOf("⇢ sent knock to /house/room#1").querySelector("button.fold"));
  await until(() => words(group().querySelector("ol")).includes("the store cannot be read"), "the group's error");
  mend();
  click(rowOf("⇢ sent knock to /house/room#1").querySelector("button.fold"));
  await until(() => group().classList.contains("folded"), "the group to fold");
  click(rowOf("⇢ sent knock to /house/room#1").querySelector("button.fold"));
  await until(() => words(group().querySelector("ol")).includes("started by"), "the room's rows, read again");
});
