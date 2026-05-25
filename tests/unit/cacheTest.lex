// cacheTest.lex — unit tests for stdlib/cache.lex.
//
// Covers both the generic Cache struct (set/get/has/del/clear/size/memoize)
// and the new keyOf() + Cache.wrap() AI-response-cache helpers.

import "stdlib/cache.lex" as cache


// ── Generic Cache regression smoke ────────────────────────────────────────

println("=== generic Cache ===")

let c = cache.newCache()
assert(c.size() == 0,         "size: fresh cache is empty")
assert(!c.has("k"),           "has: missing key returns false")
assert(c.get("k") == null,    "get: missing key returns null")

c.set("k", 42)
assert(c.size() == 1,         "size: 1 after one set")
assert(c.has("k"),            "has: present key returns true")
assert(c.get("k") == 42,      "get: returns the stored value")

c.set("k", 99)
assert(c.size() == 1,         "size: unchanged on overwrite")
assert(c.get("k") == 99,      "get: overwrite returns latest value")

c.set("other", "hello")
assert(c.size() == 2,         "size: 2 after second key")

c.del("k")
assert(c.size() == 1,         "size: 1 after del")
assert(!c.has("k"),           "has: deleted key gone")

c.del("missing")
assert(c.size() == 1,         "del: missing key is a no-op")

c.clear()
assert(c.size() == 0,         "size: 0 after clear")
println("  generic Cache: PASS")


// ── keyOf — determinism + key-order independence ──────────────────────────

println("=== keyOf ===")

let k1 = cache.keyOf({"a": 1, "b": 2})
let k2 = cache.keyOf({"b": 2, "a": 1})
assert(k1 == k2,              "keyOf: hash key order must not affect output")

let k3 = cache.keyOf({"a": 1, "b": 3})
assert(k1 != k3,              "keyOf: different values must produce different keys")

let k4 = cache.keyOf({"model": "claude-opus-4-7", "messages": [{"role": "user", "content": "hi"}]})
let k5 = cache.keyOf({"messages": [{"role": "user", "content": "hi"}], "model": "claude-opus-4-7"})
assert(k4 == k5,              "keyOf: top-level order independence on nested input")

// Length sanity check — sha256 hex is 64 chars.
assert(len(k1) == 64,         "keyOf: sha256 output is 64 hex chars")

// Arrays are ordered — [1,2] and [2,1] differ.
let ka = cache.keyOf([1, 2, 3])
let kb = cache.keyOf([3, 2, 1])
assert(ka != kb,              "keyOf: array order is significant")

// Primitives.
assert(cache.keyOf("hello") == cache.keyOf("hello"),     "keyOf: same string → same key")
assert(cache.keyOf(42) == cache.keyOf(42),               "keyOf: same int → same key")
assert(cache.keyOf(null) != cache.keyOf("null"),         "keyOf: null vs 'null' string distinct")

println("  keyOf: PASS")


// ── Cache.wrap — hit/miss, error path ─────────────────────────────────────

println("=== Cache.wrap ===")

let c2  = cache.newCache()
let cnt = {"n": 0}

// First call → miss → f runs → result cached.
let v, e = c2.wrap("key1", fn() {
    cnt["n"] = cnt["n"] + 1
    return "computed", null
})
assert(e == null,             "wrap/first: err must be null")
assert(v == "computed",       "wrap/first: returns computed value")
assert(cnt["n"] == 1,         "wrap/first: f called once")
assert(c2.size() == 1,        "wrap/first: cache populated")

// Second call → hit → f NOT called.
v, e = c2.wrap("key1", fn() {
    cnt["n"] = cnt["n"] + 1
    return "should-not-run", null
})
assert(e == null,             "wrap/hit: err must be null")
assert(v == "computed",       "wrap/hit: returns cached value")
assert(cnt["n"] == 1,         "wrap/hit: f must NOT run on cache hit")

// Different key → miss → f runs.
v, e = c2.wrap("key2", fn() {
    cnt["n"] = cnt["n"] + 1
    return "second", null
})
assert(v == "second",         "wrap/newkey: f runs for new key")
assert(cnt["n"] == 2,         "wrap/newkey: f counter advances")
assert(c2.size() == 2,        "wrap/newkey: cache size grows")


// Error path — errors must NOT be cached.
let errCnt = {"n": 0}
v, e = c2.wrap("errkey", fn() {
    errCnt["n"] = errCnt["n"] + 1
    return null, error("BOOM", "first failure")
})
assert(e != null,             "wrap/err: returns the error")
assert(e.code == "BOOM",      "wrap/err: propagates error code")
assert(v == null,             "wrap/err: value is null on error")
assert(errCnt["n"] == 1,      "wrap/err: f ran")
assert(!c2.has("errkey"),     "wrap/err: error result NOT cached")

// Retry after error → succeeds, NOW cached.
v, e = c2.wrap("errkey", fn() {
    errCnt["n"] = errCnt["n"] + 1
    return "recovered", null
})
assert(e == null,             "wrap/recover: err null on success")
assert(v == "recovered",      "wrap/recover: returns new result")
assert(errCnt["n"] == 2,      "wrap/recover: f ran second time (no prior cache)")
assert(c2.has("errkey"),      "wrap/recover: now cached")

// Third call after recovery → hit.
v, e = c2.wrap("errkey", fn() {
    errCnt["n"] = errCnt["n"] + 1
    return "should-not-run", null
})
assert(v == "recovered",      "wrap/postrecover: hit returns cached value")
assert(errCnt["n"] == 2,      "wrap/postrecover: f must NOT re-run")

println("  Cache.wrap: PASS")


// ── memoize regression ───────────────────────────────────────────────────

println("=== memoize regression ===")

let c3 = cache.newCache()
let mcnt = {"n": 0}
let double = c3.memoize(fn(x) {
    mcnt["n"] = mcnt["n"] + 1
    return x * 2
})
assert(double(3) == 6,        "memoize: first call computes")
assert(double(3) == 6,        "memoize: second call cached")
assert(mcnt["n"] == 1,        "memoize: underlying fn called once")
assert(double(5) == 10,       "memoize: new arg triggers compute")
assert(mcnt["n"] == 2,        "memoize: counter advances on new arg")
println("  memoize: PASS")


println("")
println("ALL CACHE TESTS PASSED")
