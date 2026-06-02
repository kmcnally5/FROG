// annotationTest.lex — coverage for optional function type annotations.
//
// Annotations are opt-in per function (type-first params, `: type` return) and
// ENFORCED at runtime. This suite is run through vmdiff, so it also proves the
// tree-walker and the bytecode VM enforce identically (same accept/reject, same
// messages). Error cases are wrapped in safe() so one file can exercise many
// violations without aborting.
//
// Sections:
//   1. Valid: params, return, variadic-per-element, any
//   2. Valid: closures, recursion, struct-typed params
//   3. Valid: defaults (provided + omitted), methods, map/filter/reduce
//   4. Violations (caught via safe): every rejection path
//
// Run: ./klex tests/unit/annotationTest.lex

println("=== 1. valid: params / return / variadic / any ===")
fn add(int a, int b) : int { return a + b }
println(add(2, 3))                       // 5

fn sumAll(int nums...) : int {
    let t = 0
    for n in nums { t = t + n }
    return t
}
println(sumAll(1, 2, 3, 4))              // 10

fn tag(any v) : string { return "<" + str(v) + ">" }
println(tag(true))                        // <true>
println(tag(42))                          // <42>

println("")
println("=== 2. valid: closures / recursion / struct args ===")
fn makeAdder(int base) { return fn(int x) : int { return x + base } }
let add5 = makeAdder(5)
println(add5(10))                         // 15

fn fact(int n) : int {
    if n <= 1 { return 1 }
    return n * fact(n - 1)
}
println(fact(5))                          // 120

struct Point { x, y }
fn mag(Point p) : int { return p.x + p.y }
println(mag(Point { x: 3, y: 4 }))        // 7

println("")
println("=== 3. valid: defaults / methods / map-filter-reduce ===")
fn greet(string name = "world") : string { return "hi " + name }
println(greet())                          // hi world
println(greet("kLex"))                    // hi kLex

struct Box {
    n
    fn scale(int factor) : int { return self.n * factor }
}
let b = Box { n: 10 }
println(b.scale(3))                       // 30

println(map([1, 2, 3], fn(int x) : int { return x * 2 }))      // [2, 4, 6]
println(filter([1, 2, 3, 4], fn(int x) { return x % 2 == 0 })) // [2, 4]
println(reduce([1, 2, 3, 4], fn(int acc, int x) : int { return acc + x }, 0)) // 10

println("")
println("=== 4. violations (caught via safe) ===")

let _, e1 = safe(add, 2, "x")
println(e1 != null)                       // true  — wrong arg type
println(e1.message)

let _, e2 = safe(fn() : int { return "nope" })
println(e2 != null)                       // true  — wrong return type
println(e2.message)

fn needInt(int a) : int { return a }
let _, e3 = safe(needInt, null)
println(e3 != null)                       // true  — null to int (strict)
println(e3.message)

let _, e4 = safe(sumAll, 1, "two", 3)
println(e4 != null)                       // true  — variadic bad element
println(e4.message)

let _, e5 = safe(mag, 42)
println(e5 != null)                       // true  — struct type mismatch
println(e5.message)

fn badDefault(string name = 42) { return name }
let _, e6 = safe(badDefault)
println(e6 != null)                       // true  — default violates annotation
println(e6.message)

let _, e7 = safe(fn() { return b.scale("x") })
println(e7 != null)                       // true  — method bad param
println(e7.message)

let _, e8 = safe(fn() { return map([1, "two"], fn(int x) : int { return x * 2 }) })
println(e8 != null)                       // true  — map callback bad element
println(e8.message)

println("")
println("annotation test suite complete")
