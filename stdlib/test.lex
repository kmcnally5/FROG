// stdlib/test.lex — kLex test framework
//
// Structured describe/it test runner with runtime error isolation,
// pass/fail tracking, and clean console output.
//
// Usage:
//   import "stdlib/test.lex" as t
//
//   t.describe("My module", fn() {
//       t.it("does the thing", fn() {
//           t.assertEqual(1 + 1, 2)
//       })
//       t.it("handles errors", fn() {
//           result, err = riskyCall()
//           t.assertError(err)
//       })
//   })
//
//   t.summary()
//
// Each it() block is wrapped in safe() — a runtime error fails that test
// cleanly without crashing the suite. All assertions within a test run
// so you see every failure, not just the first.

_passed = 0
_failed = 0
_test_failed = false

// describe groups related tests under a named heading.
// The suite function is called immediately.
fn describe(name, suiteFn) {
    println("\n" + name)
    suiteFn()
}

// it runs a single named test.
// Wraps testFn in safe() so runtime errors fail the test rather than
// crashing the suite. Prints ✓ or ✗ and updates the pass/fail counters.
fn it(name, testFn) {
    _test_failed = false
    result, err = safe(testFn)
    if err != null {
        _failed = _failed + 1
        println("  ✗ " + name)
        println("      → runtime error: " + err.message)
        return
    }
    if _test_failed {
        _failed = _failed + 1
        println("  ✗ " + name)
    } else {
        _passed = _passed + 1
        println("  ✓ " + name)
    }
}

// _fail marks the current test as failed and prints the reason.
fn _fail(msg) {
    _test_failed = true
    println("      → " + msg)
}

// assertEqual asserts a == b.
fn assertEqual(a, b) {
    if a == b { return null }
    _fail("expected " + str(b) + " but got " + str(a))
}

// assertNotEqual asserts a != b.
fn assertNotEqual(a, b) {
    if a != b { return null }
    _fail("expected values to differ but both were " + str(a))
}

// assertTrue asserts val is exactly true.
fn assertTrue(val) {
    if val == true { return null }
    _fail("expected true but got " + str(val))
}

// assertFalse asserts val is exactly false.
fn assertFalse(val) {
    if val == false { return null }
    _fail("expected false but got " + str(val))
}

// assertNull asserts val is null.
fn assertNull(val) {
    if val == null { return null }
    _fail("expected null but got " + str(val))
}

// assertNotNull asserts val is not null.
fn assertNotNull(val) {
    if val != null { return null }
    _fail("expected a non-null value")
}

// assertError asserts val is an error object (as returned by error() or safe()).
fn assertError(val) {
    if isError(val) { return null }
    _fail("expected an error but got " + str(val))
}

// assertNoError asserts val is not an error object.
fn assertNoError(val) {
    if isError(val) == false { return null }
    _fail("expected no error but got: " + val.message)
}

// assertType asserts type(val) == expected.
// expected: "INTEGER", "STRING", "BOOLEAN", "ARRAY", "HASH", "NULL", "FUNCTION"
fn assertType(val, expected) {
    t = type(val)
    if t == expected { return null }
    _fail("expected type " + expected + " but got " + t)
}

// assertContains asserts that a string contains a substring,
// or that an array contains a value.
fn assertContains(haystack, needle) {
    if type(haystack) == "STRING" {
        if indexOf(haystack, needle) != -1 { return null }
        _fail("expected string to contain " + str(needle))
        return
    }
    i = 0
    while i < len(haystack) {
        if haystack[i] == needle { return null }
        i = i + 1
    }
    _fail("expected array to contain " + str(needle))
}

// assertNotContains asserts that a string does not contain a substring,
// or that an array does not contain a value.
fn assertNotContains(haystack, needle) {
    if type(haystack) == "STRING" {
        if indexOf(haystack, needle) == -1 { return null }
        _fail("expected string not to contain " + str(needle))
        return
    }
    i = 0
    while i < len(haystack) {
        if haystack[i] == needle {
            _fail("expected array not to contain " + str(needle))
            return
        }
        i = i + 1
    }
}

// assertGt asserts a > b.
fn assertGt(a, b) {
    if a > b { return null }
    _fail(str(a) + " is not greater than " + str(b))
}

// assertLt asserts a < b.
fn assertLt(a, b) {
    if a < b { return null }
    _fail(str(a) + " is not less than " + str(b))
}

// assertGte asserts a >= b.
fn assertGte(a, b) {
    if a >= b { return null }
    _fail(str(a) + " is not >= " + str(b))
}

// assertLte asserts a <= b.
fn assertLte(a, b) {
    if a <= b { return null }
    _fail(str(a) + " is not <= " + str(b))
}

// summary prints totals and returns true if all tests passed, false if any failed.
fn summary() {
    total = _passed + _failed
    println("\n" + str(_passed) + "/" + str(total) + " tests passed")
    if _failed > 0 {
        println(str(_failed) + " failed")
        return false
    }
    println("All tests passed!")
    return true
}
