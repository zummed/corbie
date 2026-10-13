// maybe.js: fails when told to, else returns its n.
if (input.fail) throw new Error("told to fail")
return { n: input.n }
