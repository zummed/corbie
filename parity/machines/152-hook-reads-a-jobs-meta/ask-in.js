// Stands in for a question in a channel: finance says no, every other team yes.
log("asked in " + input.channel + ", of " + input.approvers + ": " + input.tool + " " + JSON.stringify(input.args))
return input.approvers === "finance" ? { ok: false, note: "finance said no" } : { ok: true }
