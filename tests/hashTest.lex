import "hash.lex" as h

// 1. map utilities
println("== hasKey (builtin) ==")
m = {"a": 1, "b": 2}
println(hasKey(m, "a"))    // true
println(hasKey(m, "z"))    // false

println("== len (builtin) ==")
println(len(m))        // 2
println(len({}))       // 0

println("== values ==")
single = {"x": 99}
vs = h.values(single)
println(len(vs))          // 1
println(vs[0])            // 99

println("== merge ==")
merged = h.merge({"a": 1}, {"b": 2})
println(merged["a"])      // 1
println(merged["b"])      // 2

// merge: b overwrites a
overwrite = h.merge({"k": 1}, {"k": 9})
println(overwrite["k"])   // 9

println("== invert ==")
inv = h.invert({"x": "p"})
println(inv["p"])         // x

println("== pick ==")
picked = h.pick({"a": 1, "b": 2, "c": 3}, ["a", "c"])
println(picked["a"])      // 1
println(picked["c"])      // 3
println(hasKey(picked, "b"))  // false

println("== omit ==")
omitted = h.omit({"a": 1, "b": 2, "c": 3}, ["b"])
println(omitted["a"])     // 1
println(omitted["c"])     // 3
println(hasKey(omitted, "b"))  // false

// 2. hash functions
println("== hash ==")
v1 = h.hash("hello")
v2 = h.hash("hello")
v3 = h.hash("world")
println(v1 == v2)         // true  (deterministic)
println(v1 == v3)         // false (different input)
println(type(v1))         // INTEGER

println("== hashBytes ==")
b1 = h.hashBytes([104, 101, 108, 108, 111])   // ord values of "hello"
b2 = h.hashBytes([104, 101, 108, 108, 111])
println(b1 == b2)         // true
println(type(b1))         // INTEGER

println("== combineHash ==")
c1 = h.combineHash(100, 200)
c2 = h.combineHash(100, 200)
c3 = h.combineHash(200, 100)
println(c1 == c2)         // true  (deterministic)
println(type(c1))         // INTEGER
// note: combineHash is not commutative by design
println(c1 == c3)         // false
