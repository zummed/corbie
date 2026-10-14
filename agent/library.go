package agent

import "github.com/zummed/corbie/plugin"

// The scripts every agent's definition holds: Corbie's own, added when a
// definition is filled out.
const (
	turnScript = "corbie:turn.js"
	turnSource = `return require("agents").turn(input)
`
	// The preview of a blob the model is about to be shown for the first
	// time: how many items it has, and the first few of them. Given a
	// value too large to show in place of a blob, it makes the blob: a text
	// is stored as the text it is, and anything else as JSON.
	previewScript = "corbie:preview.js"
	previewSource = `const blob = input.blob || blobs.make(input.value)
let items = -1, preview = null
if (/^text\/|^application\/jsonl?$/.test(blob.type)) {
  items = 0
  preview = []
  let room = 1024
  for (const item of blobs.items(blob)) {
    if (preview.length < 5 && room > 0) {
      const text = typeof item === "string" ? item : JSON.stringify(item)
      preview.push(text.length > room ? text.slice(0, room) + "…" : item)
      room -= text.length
    }
    items++
  }
}
return { blob, items, preview }
`
)

// large is how a blob tool gives back an answer too large to be a result:
// as a blob, which the model is then shown as a handle.
const large = `
function fits(answer) {
  const text = JSON.stringify(answer)
  return text !== undefined && text.length > 100000 ? blobs.make(text, { type: "application/json" }) : answer
}
`

// blobTools are the tools every agent has for working with blobs without
// reading them whole. They are ordinary scripts.
var blobTools = map[string]struct {
	does   string
	args   map[string]any
	input  map[string]any
	source string
}{
	"blob_read": {
		does:  "Read some of a blob's items: the lines of a text, the rows of a table, the entries of a list. Gives count items starting at from (50 from the start, if not said). The blob's handle says how many items it has. Reaching an item means reading up to it, so reading far into a large blob is slow.",
		args:  map[string]any{"blob": "blob", "from": "number?", "count": "number?"},
		input: map[string]any{"blob": "${args.blob}", "from": "${args.from}", "count": "${args.count}"},
		source: large + `const from = input.from || 0
return fits({ from, items: Array.from(blobs.items(input.blob, { from, count: input.count || 50 })) })
`},
	"blob_search": {
		does:  "Search a blob's items for a text, from the item at from (the start, if not said). Gives up to 50 matching items, each with its place, and where there may be more, the place to search from next.",
		args:  map[string]any{"blob": "blob", "find": "string", "from": "number?"},
		input: map[string]any{"blob": "${args.blob}", "find": "${args.find}", "from": "${args.from}"},
		source: large + `const found = []
const want = input.find.toLowerCase()
let at = input.from || 0, next = null
for (const item of blobs.items(input.blob, { from: at })) {
  const text = typeof item === "string" ? item : JSON.stringify(item)
  if (text.toLowerCase().includes(want)) {
    if (found.length === 50) { next = at; break }
    found.push({ at, item })
  }
  at++
}
return fits(next === null ? { found } : { found, next })
`},
	"blob_code": {
		does:  "Run a few lines of JavaScript over a blob's items and get back only the answer. The code is the body of a function of items and returns the answer. items is walked with for...of and is not a list, so a blob of any size can be gone through, as in: let n = 0; for (const r of items) if (r.total > 100) n++; return n. An answer that is large comes back as a blob.",
		args:  map[string]any{"blob": "blob", "code": "string"},
		input: map[string]any{"blob": "${args.blob}", "code": "${args.code}"},
		// The model's code is given the items and nothing else: not the
		// blob's reference, and not the tool's input.
		source: large + `const items = blobs.items(input.blob)
const code = input.code
return fits((function () { return new Function("items", code)(items) })())
`},
}

func init() {
	plugin.Register(&plugin.Module{Name: "agents", Page: libraryPage, Library: librarySource})
}

// librarySource is the library a turn is made with. A definition that puts
// a script of its own in the turn's place can call it.
const librarySource = `
exports.conversation = function (input) {
  const said = []
  for (const turn of input.turns || []) for (const part of blobs.read(turn, { as: "json" })) said.push(part)
  for (const part of input.add || []) said.push(part)
  return said
}

// attach gives the conversation with each image or document the model has
// been shown beside the place it was shown, for a model whose service
// takes it as content. Anything else stays a handle, which the model works
// on with its tools.
exports.attach = function (said, input) {
  const blobs = input.blobs || {}
  const find = function (value, into) {
    if (Array.isArray(value)) for (const item of value) find(item, into)
    else if (value && typeof value === "object") {
      const blob = typeof value.blob === "string" ? blobs[value.blob] : null
      if (blob && llm.takes(blob, { service: input.model.service })) into[value.blob] = blob
      for (const name of Object.keys(value)) find(value[name], into)
    }
  }
  const carry = function (holder, shown) {
    const files = {}
    find(shown, files)
    return Object.keys(files).length ? Object.assign({}, holder, { files }) : holder
  }
  return said.map(function (part) {
    if (part.role === "tool") return Object.assign({}, part, { results: (part.results || []).map(r => carry(r, r.result)) })
    return part.role === "assistant" ? part : carry(part, part.content)
  })
}

exports.turn = function (input) {
  const said = exports.attach(exports.conversation(input), input)
  const reply = llm.chat(said, {
    service: input.model.service,
    model: input.model.name,
    address: input.model.address,
    key: input.model.secret ? secrets.get(input.model.secret) : null,
    system: input.system || null,
    tools: input.tools || null,
    shape: input.shape || null,
    effort: input.model.effort || null,
    reply_tokens: input.model.reply_tokens || null,
    own: input.model.own && input.model.own.length ? input.model.own : null,
  })
  const answer = { role: "assistant", content: reply.text, calls: reply.calls }
  if (reply.kept) answer.kept = reply.kept
  const turn = blobs.make((input.add || []).concat([answer]), { type: "application/json" })
  return { turn, text: reply.text, calls: reply.calls, stop: reply.stop, usage: reply.usage }
}
`

const libraryPage = `agents: this library is the standard turn of an agent.

An agent makes each turn with a script, which by default calls this library's
turn. A definition replaces that script with turn: ./my-turn.js, and the
replacement may call the library.

  require("agents").turn(input)            makes one turn; gives { turn, text, calls, stop, usage }
  require("agents").conversation(input)    returns the conversation so far, a list of parts
  require("agents").attach(parts, input)   returns the parts, with each image or PDF the model was
                                           shown added as files, when its service accepts it

turn calls conversation, then attach, then llm.chat with the model in input. A
script that changes what the model is sent makes those calls itself.
` + "`corbie manual llm`" + ` gives the parts of a conversation.

A turn's input has these fields.

  model    { name, service, address, key, effort, reply, own }; key is a secret's name
  system   the system prompt
  tools    [{ name, does, args }]
  turns    the blobs that hold the conversation so far
  add      the parts that this turn adds to the conversation
  blobs    the blobs the model has been shown, by the names it knows them by
  shape    the shape of the agent's result, from output:

A turn returns these fields.

  turn    a blob, made with blobs.make, of this turn's parts as JSON:
          input.add, then the reply; without it the agent fails
  text    the model's reply
  calls   the tool calls the model made: [{ id, name, args }]; when there are
          none, text is the agent's reply
  stop    "end", "tools" or "length"; on "length" the agent tells the model
          that its reply was cut short and takes another turn
  usage   { input, output }, in tokens

This turn calls no model and answers at once.

    # quiet.yaml
    agent: quiet
    model: { name: sonnet, service: claude-code }
    prompt: Is there anything to do?
    turn: ./quiet.js

    // quiet.js
    const reply = { role: "assistant", content: "Nothing to do.", calls: [] }
    return {
      turn: blobs.make((input.add || []).concat([reply]), { type: "application/json" }),
      text: reply.content, calls: [], stop: "end", usage: { input: 0, output: 0 },
    }

    corbie test quiet.yaml

This turn calls the library's turn and logs each reply.

    # heard.yaml
    agent: heard
    model: { name: claude-sonnet-5-5, secret: ANTHROPIC_KEY }
    prompt: Say hello.
    turn: ./heard.js

    // heard.js
    const reply = require("agents").turn(input)
    log("the model said", reply.text)
    return reply

    corbie test heard.yaml --secret ANTHROPIC_KEY
`
