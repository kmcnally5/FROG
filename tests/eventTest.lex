import "event.lex" as ev

e = ev.newEmitter()


// =====================================
// 1. basic emit/on
// =====================================
println("== BASIC EVENT ==")

e.on("ping", fn(data) {
    println("received: " + str(data))
})

e.emit("ping", 10)
e.emit("ping", 20)


// =====================================
// 2. multiple listeners order
// =====================================
println("\n== MULTIPLE LISTENERS ==")

e.on("order", fn(x) {
    println("first: " + str(x))
})

e.on("order", fn(x) {
    println("second: " + str(x))
})

e.emit("order", 1)


// =====================================
// 3. once() behaviour
// =====================================
println("\n== ONCE ==")

e.once("init", fn(x) {
    println("init fired: " + str(x))
})

e.emit("init", 100)
e.emit("init", 200)   // should NOT print again


// =====================================
// 4. off() removal
// =====================================
println("\n== OFF ==")

handler = fn(x) {
    println("should appear only once: " + str(x))
}

e.on("removeTest", handler)
e.emit("removeTest", 1)

e.off("removeTest", handler)
e.emit("removeTest", 2)   // should not print


// =====================================
// 5. event chaining (mapEvent)
// =====================================
println("\n== MAP EVENT CHAIN ==")

e.mapEvent("raw", "double", fn(x) {
    return x * 2
})

e.mapEvent("double", "final", fn(x) {
    return x + 1
})

e.on("final", fn(x) {
    println("final result: " + str(x))
})

e.emit("raw", 10)   // 10 → 20 → 21


// =====================================
// 6. filterEvent
// =====================================
println("\n== FILTER EVENT ==")

e.filterEvent("input", "even", fn(x) {
    return x % 2 == 0
})

e.on("even", fn(x) {
    println("even received: " + str(x))
})

e.emit("input", 1)
e.emit("input", 2)
e.emit("input", 3)
e.emit("input", 4)


// =====================================
// 7. logEvent debugging
// =====================================
println("\n== LOG EVENT ==")

e.logEvent("debug")

e.emit("debug", "hello world")
e.emit("debug", {"a": 1, "b": 2})


// =====================================
// 8. stress test (many emits)
// =====================================
println("\n== STRESS TEST ==")

e.on("tick", fn(x) {
    println("tick: " + str(x))
})

i = 0
while i < 5 {
    e.emit("tick", i)
    i = i + 1
}


// =====================================
// 9. chained system sanity
// =====================================
println("\n== CHAINED SYSTEM ==")

e.mapEvent("a", "b", fn(x) { x + 10 })
e.mapEvent("b", "c", fn(x) { x * 3 })

e.on("c", fn(x) {
    println("chain result: " + str(x))
})

e.emit("a", 5)   // (5+10)*3 = 45


// =====================================
// 10. two independent emitters
// =====================================
println("\n== INDEPENDENT EMITTERS ==")

e1 = ev.newEmitter()
e2 = ev.newEmitter()

e1.on("msg", fn(x) { println("e1: " + str(x)) })
e2.on("msg", fn(x) { println("e2: " + str(x)) })

e1.emit("msg", "hello")   // only e1 fires
e2.emit("msg", "world")   // only e2 fires


// =====================================
// 11. final sanity check
// =====================================
println("\n== FINAL CHECK ==")

e.emit("nonexistent", 123)   // should do nothing, no crash
println("event system stable")
