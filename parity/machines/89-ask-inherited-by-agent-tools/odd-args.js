// odd-args.js: an ask that approves the calls whose argument n is odd. A
// tool's call is shown with what the model wanted under asked.
return { approve: input.jobs.filter(r => r.asked.args.n % 2 == 1).map(r => r.name), note: "odd numbers only" }
