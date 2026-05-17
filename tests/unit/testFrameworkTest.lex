import "stdlib/test.lex" as t

t.describe("Equality", fn() {
    t.it("integers are equal", fn() {
        t.assertEqual(1 + 1, 2)
    })
    t.it("strings are equal", fn() {
        t.assertEqual("hello" + " world", "hello world")
    })
    t.it("floats are equal", fn() {
        t.assertEqual(0.1 + 0.2, 0.30000000000000004)
    })
    t.it("detects inequality", fn() {
        t.assertNotEqual(1, 2)
    })
})

t.describe("Booleans and null", fn() {
    t.it("assertTrue", fn() {
        t.assertTrue(1 < 2)
    })
    t.it("assertFalse", fn() {
        t.assertFalse(1 > 2)
    })
    t.it("assertNull", fn() {
        t.assertNull(null)
    })
    t.it("assertNotNull", fn() {
        t.assertNotNull(42)
    })
})

t.describe("Types", fn() {
    t.it("integer type", fn() {
        t.assertType(42, "INTEGER")
    })
    t.it("string type", fn() {
        t.assertType("hello", "STRING")
    })
    t.it("array type", fn() {
        t.assertType([1, 2, 3], "ARRAY")
    })
    t.it("hash type", fn() {
        t.assertType({"a": 1}, "HASH")
    })
})

t.describe("Contains", fn() {
    t.it("string contains substring", fn() {
        t.assertContains("hello world", "world")
    })
    t.it("string does not contain missing substring", fn() {
        t.assertNotContains("hello world", "xyz")
    })
    t.it("array contains value", fn() {
        t.assertContains([1, 2, 3], 2)
    })
    t.it("array does not contain missing value", fn() {
        t.assertNotContains([1, 2, 3], 99)
    })
})

t.describe("Comparisons", fn() {
    t.it("assertGt", fn() {
        t.assertGt(10, 5)
    })
    t.it("assertLt", fn() {
        t.assertLt(3, 7)
    })
    t.it("assertGte", fn() {
        t.assertGte(5, 5)
    })
    t.it("assertLte", fn() {
        t.assertLte(4, 4)
    })
})

t.describe("Error handling", fn() {
    t.it("runtime error in test = fail not crash (this ✗ is expected)", fn() {
        arr = [1, 2, 3]
        x = arr[99]
    })
    t.it("assertError on two-path errors", fn() {
        _, err = dbOpen("mssql", "bad dsn")
        t.assertError(err)
    })
})

t.summary()
