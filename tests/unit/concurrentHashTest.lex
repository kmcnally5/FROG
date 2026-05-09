// concurrentHashTest.lex - tests for ConcurrentHash and atomic hash operations

println("=== ConcurrentHash basics ===")

ch = concurrentHash()
assert(len(ch) == 0, "new ConcurrentHash should have length 0")

ch["alpha"] = 1
ch["beta"] = 2
ch["gamma"] = 3

assert(len(ch) == 3, "len after 3 inserts should be 3")
assert(ch["alpha"] == 1, "ch[alpha] should be 1")
assert(ch["beta"] == 2, "ch[beta] should be 2")
assert(ch["gamma"] == 3, "ch[gamma] should be 3")
assert(ch["missing"] == null, "missing key should return null")

// Overwrite existing key - count should stay 3
ch["alpha"] = 100
assert(ch["alpha"] == 100, "overwrite failed")
assert(len(ch) == 3, "len should stay 3 after overwrite")

println("  ✓ Basic [] read/write works, len() correct")

println("\n=== Mixed key types ===")

ch2 = concurrentHash()
ch2["str_key"] = "value"
ch2[42] = "int_key_value"
ch2[true] = "bool_key_value"

assert(ch2["str_key"] == "value", "string key failed")
assert(ch2[42] == "int_key_value", "integer key failed")
assert(ch2[true] == "bool_key_value", "boolean key failed")
assert(len(ch2) == 3, "mixed-key hash len wrong")

println("  ✓ Mixed string/int/bool keys work")

println("\n=== delete() ===")

ch3 = concurrentHash()
ch3["x"] = 1
ch3["y"] = 2
delete(ch3, "x")

assert(ch3["x"] == null, "deleted key should be null")
assert(ch3["y"] == 2, "non-deleted key should remain")
assert(len(ch3) == 1, "len after delete should be 1")

// Delete non-existent key is fine
delete(ch3, "nonexistent")
assert(len(ch3) == 1, "deleting missing key shouldn't change len")

println("  ✓ delete() works correctly")

println("\n=== keys() ===")

ch4 = concurrentHash()
ch4["a"] = 1
ch4["b"] = 2
ch4["c"] = 3

ks = keys(ch4)
assert(len(ks) == 3, "keys() should return 3 keys")

// Keys can be in any order; check all three are present
found_a = false
found_b = false
found_c = false
for i in range(0, len(ks)) {
  if ks[i] == "a" { found_a = true }
  if ks[i] == "b" { found_b = true }
  if ks[i] == "c" { found_c = true }
}
assert(found_a == true && found_b == true && found_c == true, "keys() missing entries")

println("  ✓ keys() returns all keys")

println("\n=== atomicHashIncr ===")

ch5 = concurrentHash()

// Increment from missing key (treated as 0)
new_val = atomicHashIncr(ch5, "counter", 5)
assert(new_val == 5, "atomicHashIncr from missing should be delta")
assert(ch5["counter"] == 5, "atomicHashIncr didn't store")

// Increment existing
new_val2 = atomicHashIncr(ch5, "counter", 3)
assert(new_val2 == 8, "atomicHashIncr accumulator wrong: " + str(new_val2))

// Negative delta
atomicHashIncr(ch5, "counter", -10)
assert(ch5["counter"] == -2, "negative atomicHashIncr failed")

println("  ✓ atomicHashIncr works (single-threaded)")

println("\n=== atomicHashAdd (float) ===")

ch6 = concurrentHash()
atomicHashAdd(ch6, "balance", 100.5)
atomicHashAdd(ch6, "balance", 25.25)
val = ch6["balance"]
assert(val == 125.75, "float accumulator wrong: " + str(val))

println("  ✓ atomicHashAdd works")

println("\n=== atomicHashCAS ===")

ch7 = concurrentHash()
ch7["status"] = "pending"

// Successful CAS
swapped = atomicHashCAS(ch7, "status", "pending", "active")
assert(swapped == true, "CAS should succeed when value matches")
assert(ch7["status"] == "active", "CAS didn't update")

// Failed CAS (current value doesn't match)
swapped2 = atomicHashCAS(ch7, "status", "pending", "done")
assert(swapped2 == false, "CAS should fail when value doesn't match")
assert(ch7["status"] == "active", "failed CAS shouldn't change value")

// CAS on missing key
swapped3 = atomicHashCAS(ch7, "missing", null, "new")
assert(swapped3 == false, "CAS on missing key should return false")

println("  ✓ atomicHashCAS works correctly")

println("\n=== Concurrent atomicHashIncr (the real test) ===")

// 10 goroutines × 5000 increments each on the same dynamic key.
// Result MUST be exactly 50000 - any race would lose updates.
counter_hash = concurrentHash()

tasks = makeArray(10, null)
for w in range(0, 10) {
  let h = counter_hash
  tasks[w] = async(fn() {
    for i in range(0, 5000) {
      atomicHashIncr(h, "shared", 1)
    }
  })
}

for w in range(0, 10) {
  await(tasks[w])
}

final = counter_hash["shared"]
assert(final == 50000, "Concurrent atomicHashIncr lost updates: expected 50000, got " + str(final))
println("  ✓ 10 workers × 5,000 hash increments = " + str(final) + " (no lost updates)")

println("\n=== Concurrent inserts of distinct keys ===")

// Each worker inserts to a different key - no contention, just want to verify
// the count tracking and concurrent inserts work correctly.
multi_hash = concurrentHash()
itasks = makeArray(20, null)

for w in range(0, 20) {
  let h = multi_hash
  let worker_id = w
  itasks[w] = async(fn() {
    for i in range(0, 100) {
      key = "worker_" + str(worker_id) + "_item_" + str(i)
      h[key] = worker_id * 1000 + i
    }
  })
}

for w in range(0, 20) {
  await(itasks[w])
}

assert(len(multi_hash) == 2000, "expected 2000 entries, got " + str(len(multi_hash)))
println("  ✓ 20 workers × 100 distinct inserts = " + str(len(multi_hash)) + " entries")

println("\n=== Concurrent dynamic event counter (the realistic use case) ===")

// Simulate event aggregation: workers see random event types,
// atomicHashIncr the counter for each. Final result should be exact.
events = concurrentHash()

event_types = ["login", "logout", "view", "click", "purchase", "error"]

etasks = makeArray(10, null)
for w in range(0, 10) {
  let evs = events
  let types = event_types
  etasks[w] = async(fn() {
    for i in range(0, 1000) {
      // Pick a "random" event type using i % 6
      etype = types[i % 6]
      atomicHashIncr(evs, etype, 1)
    }
  })
}

for w in range(0, 10) {
  await(etasks[w])
}

total = 0
for i in range(0, len(event_types)) {
  c = events[event_types[i]]
  if c != null {
    total = total + c
  }
}
assert(total == 10000, "event counter total wrong: expected 10000, got " + str(total))
println("  ✓ 10 workers logged 10,000 events across 6 dynamic types: total=" + str(total))

// Each event type should have ~1666 occurrences (10 workers * 1000 events / 6 types ≈ 1666)
for i in range(0, len(event_types)) {
  c = events[event_types[i]]
  println("    " + event_types[i] + ": " + str(c))
}

println("\n=== All concurrent hash tests passed ===")
