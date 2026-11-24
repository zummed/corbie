// Withholds a page that gives the model orders, and masks addresses in any other.
if (/ignore previous instructions/i.test(input.text)) return { ok: false, note: "the page gives instructions" }
return { ok: true, output: input.text.replace(/\S+@\S+/g, "(an address)") }
