// make.js: three machines made while running. The first sends to a
// location, the second calls one, and the third sends to its parent.
return {
  sends: machines.define("statechart: sends\ninitial: a\nstates:\n  a: { entry: { send: { to: { at: /case/root }, event: hi } }, on: { never: b } }\n  b: { end: done }\n"),
  calls: machines.define("flow: calls\nstages:\n  c: { job: { call: { at: /case/root } } }\n"),
  talks: machines.define("statechart: talks\ninitial: a\nstates:\n  a: { entry: { send: { to: parent, event: hello } }, on: { never: b } }\n  b: { end: done }\n"),
}
