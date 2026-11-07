const table = require("../../people/scopes.yaml") || {}
const row = table[input.email.toLowerCase()]
return row ? { name: row.name, email: input.email, scopes: row.scopes } : { name: "", email: input.email, scopes: [] }
