import "async.lex" as a

println("== await_all ==")
t1 = async(fn() { return 10 })
t2 = async(fn() { return 20 })
t3 = async(fn() { return 30 })

results = a.await_all([t1, t2, t3])
println(len(results))    // 3
println(results[0])      // 10
println(results[1])      // 20
println(results[2])      // 30

println("== await_all preserves order ==")
s1 = async(fn() { sleep(50)   return "slow" })
s2 = async(fn() { sleep(10)   return "fast" })

ordered = a.await_all([s1, s2])
println(ordered[0])    // slow  (first task, even though it finished second)
println(ordered[1])    // fast

println("== parallel ==")
p1 = async(fn() { return 100 })
p2 = async(fn() { return 200 })

pr = a.parallel([p1, p2])
println(pr[0])    // 100
println(pr[1])    // 200
