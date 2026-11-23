// Runs the command built for js/wasm as the code of a page does, under Node:
// corbie up on a directory, then the gateway's
// requests, made by calling it. Prints what it was answered, as JSON.
//
//   node js_wasm.cjs <wasm_exec.js> <corbie.wasm> <directory> <config> <token file>
"use strict"
const [execFile, program, directory, config, tokenFile] = process.argv.slice(2)
globalThis.require = require
globalThis.fs = require("fs")
globalThis.path = require("path")
globalThis.performance ??= require("perf_hooks").performance
globalThis.crypto ??= require("crypto")
require(execFile)

// One request of the gateway's: its status and its body as text.
const ask = (method, path, body) => new Promise((resolve) => {
  const parts = []
  let status
  const headers = { Authorization: "Bearer token-of-the-test", "Content-Type": "application/json" }
  globalThis.corbieGateway.fetch(method, path, headers, body ? new TextEncoder().encode(JSON.stringify(body)) : null,
    (s) => { status = s }, (part) => parts.push(Buffer.from(part)), () => resolve({ status, body: Buffer.concat(parts).toString() }))
})

;(async () => {
  const go = new Go()
  go.argv = ["corbie", "up", "--in", directory, "--at", "/work", "--config", config, "gateway", "--token-file", tokenFile]
  go.env = { TMPDIR: require("os").tmpdir(), HOME: config }
  go.exit = (code) => { console.error(`corbie up ended with ${code}`); process.exit(1) }
  // What the command prints is kept, and not printed.
  const said = []
  const write = fs.write, writeSync = fs.writeSync
  fs.writeSync = (fd, buf, ...rest) => (fd === 1 || fd === 2 ? (said.push(Buffer.from(buf).toString()), buf.length) : writeSync(fd, buf, ...rest))
  fs.write = (fd, buf, offset, length, position, done) => {
    if (fd !== 1 && fd !== 2) return write(fd, buf, offset, length, position, done)
    said.push(Buffer.from(buf.subarray(offset, offset + length)).toString())
    done(null, length)
  }
  go.run((await WebAssembly.instantiate(fs.readFileSync(program), go.importObject)).instance)
  // corbie up starts the gateway, then submits the directory and prints each location.
  for (let i = 0; !globalThis.corbieGateway || !said.join("").includes("/work/made"); i++) {
    if (i > 3000) { console.error("corbie up did not start the gateway and submit the directory:\n" + said.join("")); process.exit(1) }
    await new Promise((r) => setTimeout(r, 10))
  }
  // A call the gateway cannot read asks nothing, and the program stays up.
  const short = String(globalThis.corbieGateway.fetch("GET", "/client", {}))
  let stopFirst
  await new Promise((resolve) => { stopFirst = globalThis.corbieGateway.fetch("GET", "/client", {}, null, () => {}, () => {}, resolve) })
  const stayed = stopFirst() === undefined
  const who = await ask("GET", "/client")
  const started = await ask("POST", "/runs", { at: "/work/double", input: { n: 21 } })
  const id = JSON.parse(started.body).id
  const run = id ? await ask("GET", `/runs/${id}?wait`) : { status: 0, body: "{}" }
  const madeID = JSON.parse((await ask("POST", "/runs", { at: "/work/made" })).body).id
  const made = madeID ? await ask("GET", `/runs/${madeID}?wait`) : { status: 0, body: "{}" }
  process.stdout.write(JSON.stringify({ said: said.join(""), short, stayed, who, started, run: { status: run.status, body: JSON.parse(run.body) }, made: { status: made.status, body: JSON.parse(made.body) } }))
  process.exit(0)
})()
