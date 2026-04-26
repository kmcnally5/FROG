import "functional.lex" as f


// =====================================
// 1. identity
// =====================================
println("== identity ==")
println(f.identity(42))
println(f.identity("hello"))
println(f.identity([1,2,3]))


// =====================================
// 2. compose
// f(g(x)) = (x + 1) * 2
// =====================================
println("\n== compose ==")

inc = fn(x) { x + 1 }
double = fn(x) { x * 2 }

comp = f.compose(double, inc)

println(comp(5))   // (5+1)*2 = 12


// =====================================
// 3. pipe
// =====================================
println("\n== pipe ==")

process = f.pipe(
    fn(x) { x + 1 },
    fn(x) { x * 2 },
    fn(x) { x - 3 }
)

println(process(5))   // ((5+1)*2)-3 = 9


// =====================================
// 4. tap (debug side effect)
// =====================================
println("\n== tap ==")

debug = f.tap(fn(x) {
    println("DEBUG VALUE: " + str(x))
})

println(debug(10))   // prints debug + returns 10


// =====================================
// 5. always
// =====================================
println("\n== always ==")

const5 = f.always(5)

println(const5("a"))
println(const5(999))
println(const5([1,2,3]))


// =====================================
// 6. partial application
// =====================================
println("\n== partial ==")

add = fn(a, b) { a + b }

add10 = f.partial(add, 10)

println(add10(5))    // 15
println(add10(20))   // 30


// =====================================
// 7. flip
// =====================================
println("\n== flip ==")

sub = fn(a, b) { a - b }

flipSub = f.flip(sub)

println(sub(10, 3))        // 7
println(flipSub(10, 3))    // 3 - 10 = -7


// =====================================
// 8. pipeline with real data
// =====================================
println("\n== real pipeline ==")

data = [1,2,3,4,5,6]

// double → filter evens → sum
result = f.pipe(
    fn(arr) { map(arr, fn(x) { x * 2 }) },
    fn(arr) { filter(arr, fn(x) { x % 2 == 0 }) },
    fn(arr) { reduce(arr, fn(acc, x) { acc + x }, 0) }
)

println(result(data))   // (2+4+6+8+10+12 filtered evens => all even => sum = 42)


// =====================================
// 9. nested composition test
// =====================================
println("\n== nested compose ==")

add1 = fn(x) { x + 1 }
square = fn(x) { x * x }

f1 = f.compose(square, add1)   // (x+1)^2
f2 = f.compose(add1, square)   // x^2 + 1

println(f1(3))   // 16
println(f2(3))   // 10


// =====================================
// 10. sanity check: function chaining correctness
// =====================================
println("\n== sanity ==")

chain = f.pipe(
    fn(x) { x + 2 },
    fn(x) { x * x },
    fn(x) { x - 1 }
)

println(chain(3))   // ((3+2)^2)-1 = 24