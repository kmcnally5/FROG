// freezeTest.lex — comprehensive test of deep-freeze const semantics
//
// When a const binds an array, hash, or struct, the object and everything
// reachable from it becomes frozen (immutable). Any mutation attempt raises
// a RuntimeError.

// ========================================
// Test 1: const array — index assignment blocked
// ========================================
const arr1 = [1, 2, 3]
result1, err1 = safe(fn() {
    arr1[0] = 99  // should error: cannot mutate frozen array
})
assert(err1 != null)
println("✓ Test 1: const array index assignment blocked")

// ========================================
// Test 2: const hash — key assignment blocked
// ========================================
const hash1 = {"a": 1, "b": 2}
result2, err2 = safe(fn() {
    hash1["a"] = 99  // should error: cannot mutate frozen hash
})
assert(err2 != null)
println("✓ Test 2: const hash key assignment blocked")

// ========================================
// Test 3: const struct — field assignment blocked
// ========================================
struct Person {
    name, age
}

const person1 = Person { name: "alice", age: 30 }
result3, err3 = safe(fn() {
    person1.name = "bob"  // should error: cannot mutate frozen struct
})
assert(err3 != null)
println("✓ Test 3: const struct field assignment blocked")

// ========================================
// Test 4: Nested array freeze — deep mutation blocked
// ========================================
const nested1 = [[1, 2], [3, 4]]
result4, err4 = safe(fn() {
    nested1[0][1] = 99  // should error: cannot mutate frozen array (inner)
})
assert(err4 != null)
println("✓ Test 4: Nested array deep mutation blocked")

// ========================================
// Test 5: delete() on const hash blocked
// ========================================
const hash2 = {"x": 10, "y": 20}
result5, err5 = safe(fn() {
    delete(hash2, "x")  // should error: cannot mutate frozen hash
})
assert(err5 != null)
println("✓ Test 5: delete() on const hash blocked")

// ========================================
// Test 6: const with primitives still works
// ========================================
const PI = 3.14159
const STR = "hello"
const NUM = 42
assert(PI == 3.14159)
assert(STR == "hello")
assert(NUM == 42)
println("✓ Test 6: const with primitives works")

// ========================================
// Test 7: const with functions works
// ========================================
const double = fn(x) { return x * 2 }
assert(double(5) == 10)
println("✓ Test 7: const with functions works")

// ========================================
// Test 8: Attempt to rebind const still fails
// ========================================
result8, err8 = safe(fn() {
    const x = 10
    x = 20  // should error: cannot reassign constant
})
assert(err8 != null)
println("✓ Test 8: const rebind blocked (binding-level protection still works)")

// ========================================
// Test 9: async task with const frozen array
// ========================================
const arr9 = [1, 2, 3]
result9, err9 = safe(fn() {
    task9 = async(fn() {
        arr9[0] = 99  // should error in the task
        return "should not reach here"
    })
    return await(task9)
})
// result should be an error
assert(isError(result9) || err9 != null)
println("✓ Test 9: async task cannot mutate const frozen array")

// ========================================
// Test 10: Non-const array remains mutable
// ========================================
let mutableArr = [1, 2, 3]
mutableArr[0] = 99
assert(mutableArr[0] == 99)
println("✓ Test 10: non-const array remains mutable")

// ========================================
// Test 11: Non-const hash remains mutable
// ========================================
let mutableHash = {"x": 1}
mutableHash["x"] = 2
assert(mutableHash["x"] == 2)
println("✓ Test 11: non-const hash remains mutable")

// ========================================
// Test 12: Non-const struct remains mutable
// ========================================
let p = Person { name: "alice", age: 30 }
p.name = "bob"
assert(p.name == "bob")
println("✓ Test 12: non-const struct remains mutable")

// ========================================
// Summary
// ========================================
println("")
println("All freeze tests passed!")
