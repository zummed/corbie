const replies = [
  { text: "Opening.", calls: [
      { id: "c1", name: "open", args: { list: "toolkits", name: "orders" } },
      { id: "c2", name: "open", args: { list: "toolkits", name: "messages" } } ] },
  { text: "Refunding twice and telling the team.", calls: [
      { id: "c3", name: "orders_read", args: { order: 7 } },
      { id: "c4", name: "orders_refund", args: { order: 7, amount: 20 } },
      { id: "c5", name: "orders_refund", args: { order: 7, amount: 900 } },
      { id: "c6", name: "messages_tell", args: { text: "Order 7 refunded." } } ] },
  { text: "Finished." },
]
const said = require("agents").conversation(input)
let turns = 0
for (const part of said) if (part.role === "assistant") turns++
if (turns === replies.length - 1) log(JSON.stringify(said.slice(-4)))
const r = replies[turns]
const answer = { role: "assistant", content: r.text || "", calls: r.calls || [] }
return { turn: blobs.make((input.add || []).concat([answer]), { type: "application/json" }), text: answer.content, calls: answer.calls, stop: answer.calls.length ? "tools" : "end", usage: { input: 1, output: 1 } }
