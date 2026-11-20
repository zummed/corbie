const start = Date.now()
let n = 0
while (n < 5) {
  if (Date.now() - start >= n * 300) {
    log("counted " + n)
    progress(n, 5)
    n++
  }
}
return n
