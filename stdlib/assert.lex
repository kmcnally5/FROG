// stdlib/assert.lex — kLex assertion library for writing tests
//
// Tracks pass/fail counts across assertions. Call summary() at the end
// of a test file to print totals and see whether all assertions passed.
//
// Usage:
//   import "assert.lex" as t
//   t.assertEqual(1 + 1, 2)
//   t.assertTrue(2 > 1)
//   t.summary()
//
// Note: assertEqual uses == which is reference equality for arrays and hashes.
// Two arrays with the same contents are not equal unless they are the same object.

_passed = 0
_failed = 0

fn _pass() {
    _passed = _passed + 1
}

fn _fail(msg) {
    _failed = _failed + 1
    println("FAIL: " + msg)
}

// assertEqual asserts that a == b.
fn assertEqual(a, b) {
    if a == b {
        _pass()
        return null
    }
    _fail("expected " + str(b) + " but got " + str(a))
}

// assertNotEqual asserts that a != b.
fn assertNotEqual(a, b) {
    if a != b {
        _pass()
        return null
    }
    _fail("expected values to differ but both were " + str(a))
}

// assertTrue asserts that val is exactly true.
fn assertTrue(val) {
    if val == true {
        _pass()
        return null
    }
    _fail("expected true but got " + str(val))
}

// assertFalse asserts that val is exactly false.
fn assertFalse(val) {
    if val == false {
        _pass()
        return null
    }
    _fail("expected false but got " + str(val))
}

// assertNull asserts that val is null.
fn assertNull(val) {
    if val == null {
        _pass()
        return null
    }
    _fail("expected null but got " + str(val))
}

// assertNotNull asserts that val is not null.
fn assertNotNull(val) {
    if val != null {
        _pass()
        return null
    }
    _fail("expected a non-null value")
}

// assertType asserts that type(val) == expected.
// expected should be a string: "INTEGER", "STRING", "BOOLEAN", "ARRAY", "HASH", "NULL", "FUNCTION"
fn assertType(val, expected) {
    t = type(val)
    if t == expected {
        _pass()
        return null
    }
    _fail("expected type " + expected + " but got " + t)
}

// summary prints the total number of passed and failed assertions.
// Returns true if all assertions passed, false if any failed.
fn summary() {
    total = _passed + _failed
    println(str(_passed) + "/" + str(total) + " assertions passed")
    if _failed > 0 {
        println(str(_failed) + " FAILED")
        return false
    }
    return true
}
