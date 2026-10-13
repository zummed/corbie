// emit.js: emits each item of input.items as a blob of its own, then returns how many.
for (const x of input.items) {
  emit("item", blobs.make([x], { type: "application/json" }))
}
return { count: input.items.length }
