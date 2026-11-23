const replies = [
  { text: "Reading.", calls: [
      { id: "c1", name: "fetch", args: { url: "a.example" } },
      { id: "c2", name: "fetch", args: { url: "b.example" } } ] },
  { text: "Finished." },
]
const said = require("agents").conversation(input)
let turns = 0
for (const part of said) if (part.role === "assistant") turns++
if (turns === replies.length - 1) log(JSON.stringify(said.slice(-1)))
const r = replies[turns]
const answer = { role: "assistant", content: r.text || "", calls: r.calls || [] }
return { turn: blobs.make((input.add || []).concat([answer]), { type: "application/json" }), text: answer.content, calls: answer.calls, stop: answer.calls.length ? "tools" : "end", usage: { input: 1, output: 1 } }
