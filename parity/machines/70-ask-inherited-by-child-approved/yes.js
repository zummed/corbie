// yes.js: an ask that approves every job it is asked about.
return { approve: input.jobs.map(r => r.name), note: "yes to all" }
