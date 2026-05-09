// atomicTest.lex - tests for lock-free atomic array operations

println("=== AtomicIntArray basics ===")

ai = atomicIntArray(10)
assert(len(ai) == 10, "atomicIntArray length wrong")
assert(atomicLoad(ai, 0) == 0, "AtomicIntArray default value should be 0")

atomicStore(ai, 3, 42)
assert(atomicLoad(ai, 3) == 42, "atomicStore/atomicLoad failed")

new_val = atomicAdd(ai, 3, 8)
assert(new_val == 50, "atomicAdd should return new value: expected 50, got " + str(new_val))
assert(atomicLoad(ai, 3) == 50, "atomicAdd didn't persist")

println("  ✓ AtomicIntArray basic ops work")

println("\n=== AtomicFloatArray basics ===")

af = atomicFloatArray(10, 1.5)
assert(len(af) == 10, "atomicFloatArray length wrong")
assert(atomicLoad(af, 0) == 1.5, "atomicFloatArray initial value wrong")

atomicStore(af, 5, 3.14)
assert(atomicLoad(af, 5) == 3.14, "atomicStore/atomicLoad on float failed")

new_f = atomicAdd(af, 5, 0.86)
// Float arithmetic: 3.14 + 0.86 = 4.0 (exactly)
assert(new_f == 4.0, "atomicAdd float wrong: expected 4.0, got " + str(new_f))

println("  ✓ AtomicFloatArray basic ops work")

println("\n=== atomicCAS (compare-and-swap) ===")

ai2 = atomicIntArray(5, 100)

// Successful CAS: current value matches expected
swapped = atomicCAS(ai2, 0, 100, 999)
assert(swapped == true, "CAS should succeed when value matches")
assert(atomicLoad(ai2, 0) == 999, "CAS didn't update")

// Failed CAS: current value doesn't match
swapped2 = atomicCAS(ai2, 0, 100, 555)
assert(swapped2 == false, "CAS should fail when value doesn't match")
assert(atomicLoad(ai2, 0) == 999, "Failed CAS shouldn't change value")

println("  ✓ atomicCAS works correctly")

println("\n=== Concurrent atomicAdd (the real test) ===")

// 10 workers each increment the same counter 10000 times.
// Without atomic ops, we'd lose increments due to races.
// With atomicAdd, the final count must be exactly 100000.
counter = atomicIntArray(1)

tasks = makeArray(10, null)
for w in range(0, 10) {
  let counter_ref = counter
  tasks[w] = async(fn() {
    for i in range(0, 10000) {
      atomicAdd(counter_ref, 0, 1)
    }
  })
}

for w in range(0, 10) {
  await(tasks[w])
}

final = atomicLoad(counter, 0)
assert(final == 100000, "Concurrent atomicAdd lost updates: expected 100000, got " + str(final))
println("  ✓ 10 workers × 10,000 increments = " + str(final) + " (no lost updates)")

println("\n=== Concurrent atomicAdd on floats ===")

// Same test with floats - the CAS-loop must not lose updates
fcounter = atomicFloatArray(1)

ftasks = makeArray(10, null)
for w in range(0, 10) {
  let fc = fcounter
  ftasks[w] = async(fn() {
    for i in range(0, 10000) {
      atomicAdd(fc, 0, 0.1)
    }
  })
}

for w in range(0, 10) {
  await(ftasks[w])
}

ffinal = atomicLoad(fcounter, 0)
// 10 workers × 10000 × 0.1 = 10000.0 (with floating-point fuzz)
diff = ffinal - 10000.0
if diff < 0.0 { diff = 0.0 - diff }
assert(diff < 1.0, "Concurrent float atomicAdd off by too much: expected ~10000, got " + str(ffinal))
println("  ✓ 10 workers × 10,000 × 0.1 = " + str(ffinal) + " (within float tolerance)")

println("\n=== parallelArrayForEach with atomic side effects ===")

// Real-world pattern: parallel compute + atomic merge
// Sum 1..1000 by having workers atomically add to a single counter
nums = makeArray(1000, 0)
for i in range(0, 1000) {
  nums[i] = i + 1
}

total = atomicIntArray(1)
parallelArrayForEach(nums, fn(v, i) {
  atomicAdd(total, 0, v)
})

result = atomicLoad(total, 0)
assert(result == 500500, "parallelArrayForEach atomic sum wrong: expected 500500, got " + str(result))
println("  ✓ parallelArrayForEach + atomicAdd: sum 1..1000 = " + str(result))

println("\n=== Performance: 1M float atomic adds ===")

big = atomicFloatArray(10000)
items = makeArray(1000000, 0)
for i in range(0, 1000000) {
  items[i] = i % 10000
}

t0 = _timeNanos()
parallelArrayForEach(items, fn(v, i) {
  atomicAdd(big, v, 1.0)
})
t1 = _timeNanos()

elapsed_ms = (t1 - t0) / 1000000
println("  1M parallel atomic adds in " + str(elapsed_ms) + " ms")

// Each cell should have been incremented exactly 100 times
sample = atomicLoad(big, 0)
assert(sample == 100.0, "Sample cell wrong count: expected 100, got " + str(sample))
println("  ✓ All 10,000 cells correctly received 100 increments each")

println("\n=== All atomic tests passed ===")
