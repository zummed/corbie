// read.js: returns what the emitted blob it is given holds.
return { got: blobs.read(input.item, { as: "json" })[0] }
