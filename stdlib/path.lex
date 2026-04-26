// stdlib/path.lex — POSIX-style path utilities (pure string logic, no filesystem calls)
//
// Uses "/" as the separator. No Windows support.
//
// Usage:
//   import "path.lex" as path
//   println(path.join("/usr", "local"))      // /usr/local
//   println(path.basename("/a/b/file.txt"))  // file.txt

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
fn joinAll(segments...) {
    if len(segments) == 0 {
        return ""
    }
    out = segments[0]
    i = 1
    while i < len(segments) {
        if out == "" {
            out = segments[i]
        } else {
            out = out + "/" + segments[i]
        }
        i = i + 1
    }
    return out
}

// basename returns the final path component.
fn basename(p) {
    p = normalize(p)
    ps = split(p, "/")
    if len(ps) == 0 { return "" }
    return ps[len(ps) - 1]
}

// dirname returns everything except the final component.
// Returns "." for a bare filename with no directory.
fn dirname(p) {
    p = normalize(p)
    ps = split(p, "/")
    if len(ps) <= 1 {
        return "."
    }
    out = ps[0]
    i = 1
    while i < len(ps) - 1 {
        out = out + "/" + ps[i]
        i = i + 1
    }
    return out
}

// ext returns the file extension (after the last dot), or "" if none.
fn ext(p) {
    name = basename(p)
    i = indexOf(name, ".")
    if i == -1 {
        return ""
    }
    segs = split(name, ".")
    return segs[len(segs) - 1]
}

// stripExt returns the path with the file extension removed.
fn stripExt(p) {
    name = basename(p)
    d = dirname(p)
    i = indexOf(name, ".")
    if i == -1 {
        return p
    }
    base = split(name, ".")[0]
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
    segs = split(p, "/")
    stack = []
    i = 0
    while i < len(segs) {
        seg = segs[i]
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
    result = ""
    i = 0
    while i < len(stack) {
        if i == 0 {
            result = stack[i]
        } else {
            result = result + "/" + stack[i]
        }
        i = i + 1
    }
    return result
}
