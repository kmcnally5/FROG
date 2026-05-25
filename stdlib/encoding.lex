// stdlib/encoding.lex — string ↔ integer-array conversion utilities
// @module    encoding
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   string ↔ integer-array conversion utilities
//
// kLex exposes ord() and chr() as native builtins (see eval/builtins_strings.go).
// This file provides the array-shaped helpers bytes()/stringFromBytes() that
// the builtins do not — convenient when you want a kLex Array of ints rather
// than the *Bytes type returned by strToBytes/bytesToStr.
//
// HISTORY: the prior version of this file re-implemented ord/chr in FROG using
// indexOf on a 95-char ASCII table. That was O(95) per character AND restricted
// the API to printable ASCII — every non-ASCII rune became "?". The native
// builtins are O(1), Unicode-aware, and treat the full code-point range as
// first-class.
//
// Usage:
//   import "stdlib/encoding.lex" as enc
//   println(enc.bytes("AB"))            // [65, 66]
//   println(enc.stringFromBytes([72, 73]))  // "HI"
//
// For single-character conversion call the builtins directly:
//   println(ord("A"))    // 65
//   println(chr(65))     // "A"

// bytes(s) — return an array of Unicode code points, one per character.
// Pre-allocates the result so there's no O(n²) push-in-loop.
fn bytes(s) {
    let n = len(s)
    let out = makeArray(n, 0)
    let i = 0
    while i < n {
        out[i] = ord(s[i])
        i = i + 1
    }
    return out
}

// stringFromBytes(arr) — inverse of bytes(): join an array of code points into
// a string. Pre-allocates the parts buffer and joins once — O(n) instead of the
// prior O(n²) "out = out + c" loop.
fn stringFromBytes(arr) {
    let n = len(arr)
    let parts = makeArray(n, "")
    let i = 0
    while i < n {
        parts[i] = chr(arr[i])
        i = i + 1
    }
    return join(parts, "")
}
