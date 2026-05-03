// stdlib/strings.lex — string utilities not provided by builtins
//
// Provides trimLeft, trimRight, padLeft, padRight, repeat, count, lines, words.
// For startsWith, endsWith, indexOf, replace, use the builtin functions directly.
//
// Usage:
//   import "strings.lex" as s
//   println(s.padLeft("42", 5, "0"))    // 00042
//   println(s.repeat("x", 3))           // xxx

// repeat returns str concatenated n times.
// repeat("ab", 3) → "ababab". Returns "" if n <= 0.
fn repeat(str, n) {
    result = ""
    i = 0
    while i < n {
        result = result + str
        i = i + 1
    }
    return result
}

// count returns the number of non-overlapping occurrences of sub in str.
// count("banana", "an") → 2
fn count(str, sub) {
    if len(sub) == 0 { return 0 }
    parts = split(str, sub)
    return len(parts) - 1
}

// trimLeft removes leading whitespace (spaces, tabs, newlines). O(n).
fn trimLeft(str) {
    i = 0
    while i < len(str) {
        c = str[i]
        if c != " " && c != "\t" && c != "\n" {
            return substr(str, i)
        }
        i = i + 1
    }
    return ""
}

// trimRight removes trailing whitespace (spaces, tabs, newlines). O(n).
fn trimRight(str) {
    i = len(str) - 1
    while i >= 0 {
        c = str[i]
        if c != " " && c != "\t" && c != "\n" {
            return substr(str, 0, i + 1)
        }
        i = i - 1
    }
    return ""
}

// padLeft pads str on the left with char until it reaches width. O(n).
fn padLeft(str, width, char) {
    if len(str) >= width { return str }
    needed = width - len(str)
    padding = ""
    i = 0
    while i < needed {
        padding = padding + char
        i = i + 1
    }
    return padding + str
}

// padRight pads str on the right with char until it reaches width. O(n).
fn padRight(str, width, char) {
    if len(str) >= width { return str }
    needed = width - len(str)
    padding = ""
    i = 0
    while i < needed {
        padding = padding + char
        i = i + 1
    }
    return str + padding
}

// lines splits str into an array of lines on "\n".
fn lines(str) {
    return split(str, "\n")
}

// words splits str into an array of words on " ".
// Note: multiple spaces produce empty strings between them.
fn words(str) {
    return split(str, " ")
}
