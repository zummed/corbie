// turn.js: stands in for the model, so no model is called. Reply k below is
// given on turn k: it counts the replies already in the conversation it is
// handed. It logs the last part of the conversation it was shown.
const replies = [
  { text: "Call 1.", calls: [{ id: "call_1", name: "echo", args: { n: 1 } }] },
  { text: "Call 2.", calls: [{ id: "call_2", name: "echo", args: { n: 2 } }] },
  { text: "Call 3.", calls: [{ id: "call_3", name: "echo", args: { n: 3 } }] },
  { text: "Done after three calls." },
]
const said = require("agents").conversation(input)
let turns = 0
for (const part of said) if (part.role === "assistant") turns++
log("turn " + (turns + 1) + " was last shown: " + JSON.stringify(said[said.length - 1]))
if (turns >= replies.length) throw new Error("no scripted reply for turn " + (turns + 1))
const r = replies[turns]
const answer = { role: "assistant", content: r.text || "", calls: r.calls || [] }
return {
  turn: blobs.make((input.add || []).concat([answer]), { type: "application/json" }),
  text: answer.content,
  calls: answer.calls,
  stop: answer.calls.length ? "tools" : "end",
  usage: { input: 1, output: 1 },
}
