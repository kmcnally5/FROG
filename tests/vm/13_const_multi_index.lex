// Tier 13 — const, multi-assign (tuple destructure), index-assign.

// const
const PI = 3.14
println(PI)
const NAME = "kLex"
println(NAME)

// multi-assign from a tuple-literal RHS
fn pair() {
    return 1, 2
}
let a, b = pair()
println(a)
println(b)

// multi-assign with discard
let _, only = pair()
println(only)

// multi-assign requires a Tuple. Array literals are NOT accepted —
// the tree-walker errors with "cannot unpack ARRAY into N variables"
// and the VM matches. (Hence no array-RHS test here.)

// index-assign: array
let arr = [1, 2, 3, 4]
arr[1] = 99
println(arr[0])
println(arr[1])
println(arr[2])

// index-assign: hash
let h = {"a": 1, "b": 2}
h["a"] = 100
h["c"] = 300
println(h["a"])
println(h["b"])
println(h["c"])

// Nested mutation: arr[i] inside a loop
for i in [0, 1, 2, 3] {
    arr[i] = i * i
}
println(arr[0])
println(arr[1])
println(arr[2])
println(arr[3])
