// odd-args.js: a hook that says yes to a call whose argument n is odd. It
// reads what the model wanted as job.tool.
return { ok: input.job.tool.args.n % 2 == 1, note: "odd numbers only" }
