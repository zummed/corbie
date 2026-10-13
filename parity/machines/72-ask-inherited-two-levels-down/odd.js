// odd.js: an ask that approves the jobs whose input n is odd.
return { approve: input.jobs.filter(r => r.input.n % 2 == 1).map(r => r.name), note: "odd numbers only" }
