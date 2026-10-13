// turn-helper.js: stands in for the model, so no model is called. Reply k below is
// given on turn k: it counts the replies already in the conversation it is
// handed. It logs the last part of the conversation it was shown.
const replies = [
  { text: "Adding.", calls: [{ id: "call_1", name: "add", args: { n: 5 } }] },
  { text: "It is 6." },
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
