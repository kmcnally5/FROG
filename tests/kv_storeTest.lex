import "kv_store.lex" as kv

store = kv.newKVStore()

// =====================================
// 1. basic set/get
// =====================================
println("== BASIC SET/GET ==")

store.set("a", 10)
store.set("b", 20)
store.set("c", 30)

println(store.get("a"))   // 10
println(store.get("b"))   // 20
println(store.get("c"))   // 30
println(store.get("x"))   // null


// =====================================
// 2. cache consistency
// =====================================
println("\n== CACHE TEST ==")

println(store.get("a"))   // should hit cache internally (no visible difference)
println(store.get("a"))   // repeated access


// =====================================
// 3. delete
// =====================================
println("\n== DELETE ==")

store.del("b")
println(store.get("b"))   // null


// =====================================
// 4. persistence (save/load round trip)
// =====================================
println("\n== PERSISTENCE ==")

store.set("x", 100)
store.set("y", 200)

store.save("tests/kv_test.txt")

store.del("x")
store.del("y")

println(store.get("x"))   // null
println(store.get("y"))   // null

store.load("tests/kv_test.txt")

println(store.get("x"))   // 100 (stored as string after round-trip)
println(store.get("y"))   // 200


// =====================================
// 5. allKeys / allValues
// =====================================
println("\n== KEYS / VALUES ==")

ks = store.allKeys()
vs = store.allValues()

println(len(ks))
println(len(vs))


// =====================================
// 6. functional map over values
// =====================================
println("\n== MAP VALUES ==")

mapped = store.mapValues(fn(v) {
    return int(v) * 2
})

i = 0
while i < len(mapped) {
    println(mapped[i])
    i = i + 1
}


// =====================================
// 7. filter keys
// =====================================
println("\n== FILTER KEYS ==")

filtered = store.filterKeys(fn(k, v) {
    return int(v) >= 20
})

i = 0
while i < len(filtered) {
    println(filtered[i])
    i = i + 1
}


// =====================================
// 8. reduce store (sum test)
// =====================================
println("\n== REDUCE ==")

sum = store.reduceStore(fn(acc, k, v) {
    return acc + int(v)
}, 0)

println(sum)


// =====================================
// 9. overwrite + consistency
// =====================================
println("\n== OVERWRITE ==")

store.set("a", 999)
println(store.get("a"))


// =====================================
// 10. stress test (repeated writes)
// =====================================
println("\n== STRESS ==")

i = 0
while i < 10 {
    store.set("k" + str(i), i)
    i = i + 1
}

println(store.get("k5"))
println(store.get("k9"))


// =====================================
// 11. final integrity check
// =====================================
println("\n== FINAL CHECK ==")

println(len(store.allKeys()))
println(len(store.allValues()))
