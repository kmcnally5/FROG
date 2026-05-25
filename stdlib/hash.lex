// hash.lex — Hash and hashmap utilities (FNV-1a hashing, map merge/pick/omit/invert)
// @module    hash
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   kLex HASH + HASHMAP UTILITIES
//
// Combines:
// - map utilities (merge, pick, omit, invert)
// - hashing functions (FNV-1a: hash, hashBytes, combineHash)
//
// For values(h) use the builtin directly — no wrapper here. The builtin
// is O(n) where this stdlib wrapper used to be O(n²) via push() in a loop.
//
// Usage:
//   import "stdlib/hash.lex" as h
//
//   h.merge(map1, map2)
//   h.hash("hello")
//   h.combineHash(h1, h2)
//   values(myHash)   // builtin — no h. prefix
//
// =====================================

// ord() and chr() are kLex builtins — no import needed here.


// =====================================
// MAP UTILITIES
// =====================================

// merge two hashes (b overwrites a)
fn merge(a, b) {
    let result = {}

    for k in keys(a) {
        result[k] = a[k]
    }

    for k in keys(b) {
        result[k] = b[k]
    }

    return result
}

// invert keys and values
fn invert(h) {
    let result = {}

    for k in keys(h) {
        result[h[k]] = k
    }

    return result
}

// pick only selected keys
fn pick(h, arr) {
    let result = {}

    for k in arr {
        if hasKey(h, k) {
            result[k] = h[k]
        }
    }

    return result
}

// omit selected keys (O(n) with hash-based lookup)
fn omit(h, arr) {
    let excluded = {}
    for k in arr {
        excluded[k] = true
    }

    let result = {}
    for k in keys(h) {
        if !hasKey(excluded, k) {
            result[k] = h[k]
        }
    }

    return result
}


// =====================================
// HASHING (FNV-1a 32-bit)
// =====================================
// -------------------------------------
// bitwise XOR for 32-bit integers
// -------------------------------------
fn xor(a, b) {
    let result = 0
    let bit = 1

    let i = 0
    while i < 32 {
        let abit = a % 2
        let bbit = b % 2

        if abit != bbit {
            result = result + bit
        }

        a = a / 2
        b = b / 2
        bit = bit * 2

        i = i + 1
    }

    return result
}
// -------------------------------------
// hash(string)
// -------------------------------------
let OFFSET = 2166136261
let PRIME  = 16777619
let MOD    = 4294967296   // 2^32

// hash(s) — compute a 32-bit FNV-1a hash of string s and return it as an integer.
fn hash(s) {
    let h = OFFSET
    let i = 0

    while i < len(s) {
        let c = ord(s[i])

        // XOR (safe)
        h = xor(h, c)

        // multiply + constrain immediately
        h = (h * PRIME) % MOD

        i = i + 1
    }

    return h
}

// -------------------------------------
// hashBytes(array)
// -------------------------------------
fn hashBytes(arr) {
    let h = OFFSET
    let i = 0

    while i < len(arr) {
        let c = arr[i]

        if c == 0 {
            c = 63   // fallback for null byte
        }

        h = xor(h, c)
        h = (h * PRIME) % MOD

        i = i + 1
    }

    return h
}


// -------------------------------------
// combineHash(a, b)
// -------------------------------------
fn combineHash(a, b) {
    let h = OFFSET

    h = xor(h, a)
    h = (h * PRIME) % MOD

    h = xor(h, b)
    h = (h * PRIME) % MOD

    return h
}
