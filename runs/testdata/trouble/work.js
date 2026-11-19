if (job.attempt === 1) throw new Error("the first attempt fails")
const start = Date.now()
let n = 0
while (Date.now() - start < 60000) {
  if (Date.now() - start >= n * 1000) {
    log("working " + n)
    n++
  }
}
return n
