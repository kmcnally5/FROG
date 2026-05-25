import "stdlib/async.lex" as a

// Testing how reliable kLex is at running async tasks in parallel and collecting results with await_all() and parallel().
// Hand tested/written in an effort to squeeze every last bit of performance and reliability out of the async system.

println("== await_all ==")
let t1 = async(fn() { return 10 })
let t2 = async(fn() { return 20 })
let t3 = async(fn() { return 30 })

let results = a.await_all([t1, t2, t3])
println(len(results))    // 3
println(results[0])      // 10
println(results[1])      // 20
println(results[2])      // 30

println("== await_all preserves order ==")
let s1 = async(fn() { sleep(50) return "slow" })
let s2 = async(fn() { sleep(10) return "fast" })

let ordered = a.await_all([s1, s2])
println(ordered[0])    // slow  (first task, even though it finished second)
println(ordered[1])    // fast

println("== parallel ==")
let p1 = async(fn() { return 100 })
let p2 = async(fn() { return 200 })

let pr = a.parallel([p1, p2])
println(pr[0])    // 100
println(pr[1])    // 200
