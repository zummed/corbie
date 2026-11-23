// odd-args.js: an ask that approves the calls whose argument n is odd. A
// tool's call is shown with what the model wanted under asked.
return { ok: input.job.tool.args.n % 2 == 1, note: "odd numbers only" }
