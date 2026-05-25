import "parallel.lex" as p


let z = makeArray(10000)
let iterations = 1000
let workers = 4
let arrayCount = 50000

fn heavyCompute(acc, x) {
    let result = x + 0.5
    let j = 0
    while j < iterations {
        // Multiple expensive floating-point operations per iteration
        result = result * 1.0001 + 0.001
        result = (result / 1.00001) * 0.99999
        result = result + sqrt(result * 0.0001)

        // More expensive operations instead of modulo
        result = result * (1.0 + sqrt(0.1 / (1.0 + sqrt(result * 0.001))))
        result = (result * result) / (1.0 + result * result)
        result = result + sqrt(sqrt(result * result + 1.0))

        // Keep result in reasonable range to avoid overflow
        if result > arrayCount { result = result / 100 }
        if result < -arrayCount { result = result / 100 }

        j = j + 1
    }
    return acc + result
}

fn merge(a, b) {
    return a + b
}

println("Testing parallel_reduce with 10M elements, " + str(workers) + " workers...")
println("Same work as asyncSafeTest but using parallel_reduce...")
println("")

let arr = makeArray(arrayCount, 0)
let i = 0
while i < arrayCount {
    arr[i] = i
    i = i + 1
}

println("Starting...")
let sum, err = p.parallel_reduce(arr, heavyCompute, merge, workers, 0.0)

println("✓ Done")
println("Sum: " + str(sum))
println("Error: " + str(err))
