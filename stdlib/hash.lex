// =====================================
// kLex HASH + HASHMAP UTILITIES
// =====================================
//
// Combines:
// - map utilities (merge, pick, omit, invert, values)
// - hashing functions (FNV-1a: hash, hashBytes, combineHash)
//
// Usage:
//   import "hash.lex" as h
//
//   h.merge(map1, map2)
//   h.hash("hello")
//   h.combineHash(h1, h2)
//
// =====================================

import "encoding.lex" as enc


// =====================================
// MAP UTILITIES
// =====================================

// values returns all values (order not guaranteed)
fn values(h) {
    result = []
    for k in keys(h) {
        result = push(result, h[k])
    }
    return result
}

// merge two hashes (b overwrites a)
fn merge(a, b) {
    result = {}

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
    result = {}

    for k in keys(h) {
        result[h[k]] = k
    }

    return result
}

// pick only selected keys
fn pick(h, arr) {
    result = {}

    for k in arr {
        if hasKey(h, k) {
            result[k] = h[k]
        }
    }

    return result
}

// omit selected keys (O(n) with hash-based lookup)
fn omit(h, arr) {
    excluded = {}
    for k in arr {
        excluded[k] = true
    }

    result = {}
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
    result = 0
    bit = 1

    i = 0
    while i < 32 {
        abit = a % 2
        bbit = b % 2

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
OFFSET = 2166136261
PRIME  = 16777619
MOD    = 4294967296   // 2^32

fn hash(s) {
    h = OFFSET
    i = 0

    while i < len(s) {
        c = enc.ord(s[i])

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
    h = OFFSET
    i = 0

    while i < len(arr) {
        c = arr[i]

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
    h = OFFSET

    h = xor(h, a)
    h = (h * PRIME) % MOD

    h = xor(h, b)
    h = (h * PRIME) % MOD

    return h
}