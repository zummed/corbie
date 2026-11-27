const replies = [
  { text: "Opening.", calls: [
      { id: "c1", name: "open", args: { list: "toolkits", name: "orders" } },
      { id: "c2", name: "open", args: { list: "toolkits", name: "infra" } } ] },
  { text: "Working.", calls: [
      { id: "c3", name: "orders_refund", args: { order: 7, amount: 20 } },
      { id: "c4", name: "orders_refund", args: { order: 7, amount: 900 } },
      { id: "c5", name: "infra_restart", args: { vm: "web-1" } },
      { id: "c6", name: "infra_wipe", args: { vm: "web-1" } } ] },
  { text: "Finished." },
]
const said = require("agents").conversation(input)
let turns = 0
for (const part of said) if (part.role === "assistant") turns++
if (turns === replies.length - 1) for (const r of said[said.length - 1].results) log("the model is given: " + r.name + " " + JSON.stringify(r.args || "") + " -> " + (r.error || r.result))
const r = replies[turns]
const answer = { role: "assistant", content: r.text || "", calls: r.calls || [] }
return { turn: blobs.make((input.add || []).concat([answer]), { type: "application/json" }), text: answer.content, calls: answer.calls, stop: answer.calls.length ? "tools" : "end", usage: { input: 1, output: 1 } }
