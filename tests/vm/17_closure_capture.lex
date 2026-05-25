// Tier 17 — closure capture with MUTATION through the captured cell.
//
// This is the harder closure test: an inner function mutates a
// variable from its enclosing scope, and the outer scope must see
// the mutation. Requires reference-semantic upvalues — value-capture
// would silently fail here.

// Counter-closure pattern. counter is top-level; inc mutates it.
let counter = 0
fn inc() {
    counter = counter + 1
}
inc()
inc()
inc()
println(counter)

// Read-after-mutate from inside another closure
fn snapshot() {
    return counter
}
inc()
println(snapshot())

// Closure capturing a local of another function (not top-level)
// Note: returning a closure from a function — the closure has to
// outlive its parent frame. The captured cell must survive on the
// heap.
fn makeAdder(n) {
    fn add(x) {
        return x + n
    }
    return add
}
let add5 = makeAdder(5)
let add10 = makeAdder(10)
println(add5(3))
println(add5(100))
println(add10(7))
