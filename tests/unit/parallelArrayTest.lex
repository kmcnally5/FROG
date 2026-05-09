// parallelArrayTest.lex - tests for parallelArrayUpdate, parallelArrayMap, parallelArrayReduce

println("=== parallelArrayUpdate: in-place mutation ===")

arr1 = makeArray(10, 0)
for i in range(0, 10) {
  arr1[i] = i + 1
}

parallelArrayUpdate(arr1, fn(v, i) {
  v * 2
})

expected1 = [2, 4, 6, 8, 10, 12, 14, 16, 18, 20]
for i in range(0, 10) {
  assert(arr1[i] == expected1[i], "parallelArrayUpdate failed at index " + str(i))
}
println("  ✓ parallelArrayUpdate doubled each element correctly")

println("\n=== parallelArrayMap: returns new array ===")

source = makeArray(5, 0)
for i in range(0, 5) {
  source[i] = i
}

mapped = parallelArrayMap(source, fn(v, i) {
  v * v
})

expected2 = [0, 1, 4, 9, 16]
for i in range(0, 5) {
  assert(mapped[i] == expected2[i], "parallelArrayMap failed at index " + str(i))
}

// Source must not be mutated
for i in range(0, 5) {
  assert(source[i] == i, "parallelArrayMap mutated source array (it shouldn't)")
}
println("  ✓ parallelArrayMap squared each element, source unchanged")

println("\n=== parallelArrayReduce: parallel sum ===")

nums = makeArray(100, 0)
for i in range(0, 100) {
  nums[i] = i + 1  // 1..100
}

total = parallelArrayReduce(nums, fn(a, b) {
  a + b
}, 0)

// 1+2+...+100 = 5050
assert(total == 5050, "parallelArrayReduce sum failed: expected 5050, got " + str(total))
println("  ✓ parallelArrayReduce summed 1..100 correctly: " + str(total))

println("\n=== parallelArrayReduce: parallel max ===")

vals = [3, 7, 1, 9, 4, 2, 8, 6, 5]
biggest = parallelArrayReduce(vals, fn(a, b) {
  if b > a { b } else { a }
}, 0)
assert(biggest == 9, "parallelArrayReduce max failed: expected 9, got " + str(biggest))
println("  ✓ parallelArrayReduce found max correctly: " + str(biggest))

println("\n=== Edge case: empty array ===")

empty = makeArray(0, 0)
parallelArrayUpdate(empty, fn(v, i) { v * 2 })
assert(len(empty) == 0, "parallelArrayUpdate broke empty array")

empty_mapped = parallelArrayMap(empty, fn(v, i) { v * 2 })
assert(len(empty_mapped) == 0, "parallelArrayMap broke empty array")

empty_reduce = parallelArrayReduce(empty, fn(a, b) { a + b }, 42)
assert(empty_reduce == 42, "parallelArrayReduce on empty should return initial")
println("  ✓ Empty arrays handled correctly")

println("\n=== Performance test: 100k float multiplication ===")

large = makeArray(100000, 0.0)
for i in range(0, 100000) {
  large[i] = float(i) + 1.0
}

t0 = _timeNanos()
parallelArrayUpdate(large, fn(v, i) {
  v * 0.5
})
t1 = _timeNanos()

elapsed_ms = (t1 - t0) / 1000000
println("  parallelArrayUpdate on 100k floats: " + str(elapsed_ms) + " ms")
assert(large[0] == 0.5, "parallel multiply failed at idx 0")
assert(large[99999] == 50000.0, "parallel multiply failed at last idx")
println("  ✓ 100k element parallel update correct")

println("\n=== All parallel array tests passed ===")
