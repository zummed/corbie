const replies = [
  { text: "Opening.", calls: [{ id: "c1", name: "open", args: { list: "toolkits", name: "orders" } }] },
  { text: "Working.", calls: [
      { id: "c2", name: "orders_read_file", args: { path: "a.txt" } },
      { id: "c3", name: "orders_change_file", args: { path: "a.txt", text: "new" } },
      { id: "c4", name: "orders_read_file", args: { path: "a.txt", project: "other/orders" } } ] },
  { text: "Finished." },
]
const said = require("agents").conversation(input)
let turns = 0
for (const part of said) if (part.role === "assistant") turns++
if (turns === 1) log("the model is offered: " + (input.tools || []).map((t) => t.name + "(" + Object.keys(t.args.properties || {}).sort().join(",") + "): " + t.description).sort().join("; "))
// A line of the log is written without quotes, which the record escapes.
const plain = (v) => (typeof v === "string" ? v : JSON.stringify(v)).replace(/"/g, "")
if (turns === replies.length - 1) for (const r of said[said.length - 1].results) log("the model is given: " + r.name + " gave " + plain(r.error || r.result))
const r = replies[turns]
const answer = { role: "assistant", content: r.text || "", calls: r.calls || [] }
return { turn: blobs.make((input.add || []).concat([answer]), { type: "application/json" }), text: answer.content, calls: answer.calls, stop: answer.calls.length ? "tools" : "end", usage: { input: 1, output: 1 } }
