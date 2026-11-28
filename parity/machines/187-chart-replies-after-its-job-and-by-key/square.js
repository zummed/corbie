// square.js: gives input.n squared, and fails for anything but a number.
if (typeof input.n !== "number") throw new Error(input.n + " is not a number")
return input.n * input.n
