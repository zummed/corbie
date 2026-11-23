// Stands in for a person: the orders team says no to a refund over its limit.
if (input.limit && input.args.amount > input.limit) return { ok: false, note: input.who + " said no: over " + input.limit }
return { ok: true, note: input.who + " said yes" }
