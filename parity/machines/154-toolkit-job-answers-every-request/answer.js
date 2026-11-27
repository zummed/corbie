// answer.js: answers a toolkit's request as a server of tools would.
const request = input.request
if (request.do == "open") {
  return {
    instructions: "Ask for the shelf " + input.shelf + ".",
    entries: [{ kind: "tool", name: "find", description: "Find a note.", at: "/elsewhere" }],
  }
}
if (request.tool == "export") return { working: { said: "exporting" }, state: { task: "t1" } }
if (request.tool == "bad") return { result: 1, at: "/elsewhere" }
return { result: "found " + request.args.text + " on " + input.shelf }
