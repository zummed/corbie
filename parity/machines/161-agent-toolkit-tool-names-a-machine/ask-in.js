// Stands in for a question in a channel, which is answered yes.
log("asked in " + input.channel + ": " + input.tool + " " + JSON.stringify(input.args).replace(/"/g, ""))
return { ok: true }
