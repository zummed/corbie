// Stands in for a person: the team says no to a refund of 300.
log(input.who + " read " + JSON.stringify(input.tool))
if (input.who === "team" && input.tool.args.amount === 300) return { ok: false, note: "the team said no" }
return { ok: true, note: input.who + " said yes" }
