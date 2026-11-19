// streamed waits until the page has had its WebSocket's first message, so a
// test that sends the page a part of its own is not undone by it. A line is
// lit only once the page has had a message, so a line drawn here, whose
// data changes at each look, is lit from then on.
import { createRoot, createSignal, flush } from "solid-js";
import { lit } from "ui";
import { until } from "/test/page/testdata/test.js";

const probe = document.createElement("i");
const [count, setCount] = createSignal(0);
createRoot(() => lit(() => ({ key: "the stream's first message", data: count() }))(probe));

export async function streamed() {
  await until(() => {
    setCount(n => n + 1);
    flush();
    return probe.classList.contains("moved");
  }, "the stream's first message");
}
