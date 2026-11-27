// Stands in for work that ends at the third ask.
const asked = input.request.do === "call" ? 0 : input.request.state.asked
if (asked < 2) return { working: { said: "asked " + (asked + 1), wait: 2 }, state: { asked: asked + 1 } }
return "exported " + input.what + " at ask " + (asked + 1)
