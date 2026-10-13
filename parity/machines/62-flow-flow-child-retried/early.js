// early.js: fails until the clock passes input.ok_after (milliseconds).
if (Date.now() < input.ok_after) throw new Error("too early")
return { ok: true }
