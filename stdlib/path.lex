// stdlib/path.lex — POSIX-style path utilities (pure string logic, no filesystem calls)
// @module    path
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   POSIX-style path utilities (pure string logic, no filesystem calls)
//
// Uses "/" as the separator. No Windows support.
//
// Usage:
//   import "path.lex" as path
//   println(path.join("/usr", "local"))      // /usr/local
//   println(path.basename("/a/b/file.txt"))  // file.txt

// Capture the builtin array-join here, BEFORE the local `fn join(a, b)` below
// shadows the name inside this module. Internal helpers (joinAll, dirname,
// clean) use _strJoin to build paths in O(n) instead of O(n²) string concat.
let _strJoin = join

// normalize converts backslashes to forward slashes.
fn normalize(p) {
    return replace(p, "\\", "/")
}

// parts splits p into its path segments.
fn parts(p) {
    p = normalize(p)
    return split(p, "/")
}

// join joins two path segments, inserting "/" between them if needed.
fn join(a, b) {
    if a == "" { return b }
    if b == "" { return a }
    a = normalize(a)
    b = normalize(b)
    if endsWith(a, "/") {
        return a + b
    }
    return a + "/" + b
}

// joinAll joins any number of path segments left-to-right.
// Leading empty segments are skipped (preserves prior behaviour: joinAll("", "foo") == "foo").
// Once a non-empty segment is found, subsequent segments — including empty ones —
// are joined with "/", so ("a", "", "b") yields "a//b" exactly as before.
fn joinAll(segments...) {
    if len(segments) == 0 {
        return ""
    }
    let start = 0
    while start < len(segments) && segments[start] == "" {
        start = start + 1
    }
    if start >= len(segments) {
        return ""
    }
    return _strJoin(slice(segments, start, len(segments)), "/")
}

// basename returns the final path component.
fn basename(p) {
    p = normalize(p)
    let ps = split(p, "/")
    if len(ps) == 0 { return "" }
    return ps[len(ps) - 1]
}

// dirname returns everything except the final component.
// Returns "." for a bare filename with no directory.
fn dirname(p) {
    p = normalize(p)
    let ps = split(p, "/")
    if len(ps) <= 1 {
        return "."
    }
    return _strJoin(slice(ps, 0, len(ps) - 1), "/")
}

// ext returns the file extension (after the last dot), or "" if none.
fn ext(p) {
    let name = basename(p)
    let i = indexOf(name, ".")
    if i == -1 {
        return ""
    }
    let segs = split(name, ".")
    return segs[len(segs) - 1]
}

// stripExt returns the path with the file extension removed.
fn stripExt(p) {
    let name = basename(p)
    let d = dirname(p)
    let i = indexOf(name, ".")
    if i == -1 {
        return p
    }
    let base = split(name, ".")[0]
    if d == "." {
        return base
    }
    return join(d, base)
}

// isAbsolute returns true if p starts with "/".
fn isAbsolute(p) {
    p = normalize(p)
    return startsWith(p, "/")
}

// isRelative returns true if p does not start with "/".
fn isRelative(p) {
    return !isAbsolute(p)
}

// clean resolves "." and ".." segments and collapses duplicate slashes.
fn clean(p) {
    p = normalize(p)
    let segs = split(p, "/")
    let stack = []
    let i = 0
    while i < len(segs) {
        let seg = segs[i]
        if seg == "" || seg == "." {
            // skip
        } else if seg == ".." {
            if len(stack) > 0 {
                stack = pop(stack)
            }
        } else {
            stack = push(stack, seg)
        }
        i = i + 1
    }
    return _strJoin(stack, "/")
}
