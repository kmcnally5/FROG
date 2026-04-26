import "math.lex" as math

println("== abs ==")
println(math.abs(-5))    // 5
println(math.abs(5))     // 5
println(math.abs(0))     // 0

println("== max / min ==")
println(math.max(3, 7))    // 7
println(math.max(7, 3))    // 7
println(math.min(3, 7))    // 3
println(math.min(7, 3))    // 3

println("== clamp ==")
println(math.clamp(10, 1, 5))    // 5
println(math.clamp(0, 1, 5))     // 1
println(math.clamp(3, 1, 5))     // 3

println("== pow ==")
println(math.pow(2, 0))    // 1
println(math.pow(2, 3))    // 8
println(math.pow(3, 3))    // 27

println("== sum / product ==")
println(math.sum([1, 2, 3, 4]))       // 10
println(math.product([1, 2, 3, 4]))   // 24

println("== sign ==")
println(math.sign(-99))    // -1
println(math.sign(0))      // 0
println(math.sign(99))     // 1

println("== even / odd ==")
println(math.even(4))    // true
println(math.even(3))    // false
println(math.odd(3))     // true
println(math.odd(4))     // false

println("== gcd ==")
println(math.gcd(12, 8))     // 4
println(math.gcd(100, 75))   // 25

println("== lcm ==")
println(math.lcm(4, 6))      // 12
println(math.lcm(3, 5))      // 15
