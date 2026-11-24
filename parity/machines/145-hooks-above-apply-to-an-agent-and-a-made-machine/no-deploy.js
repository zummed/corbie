const deploys = (input.job.allow || []).includes("exec:deploy")
return { ok: !deploys, note: "no deploy from a machine made while running" }
