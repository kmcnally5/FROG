// stdlib/strings.lex — kLex standard string library
//
// Provides common string operations built on top of the built-in functions:
// split, join, len, str, upper, lower, trim, and string indexing (str[i]).
//
// Usage:
//   import "strings.lex" as s
//   println(s.indexOf("hello", "ll"))   // 2
//   println(s.padLeft("42", 5, "0"))    // 00042

// contains returns true if sub appears anywhere in str.
fn contains(str, sub) {
    parts = split(str, sub)
    return len(parts) > 1
}

// indexOf returns the index of the first occurrence of sub in str.
// Returns -1 if sub is not found.
// Uses split: everything before the first match is parts[0], so its length is the index.
fn indexOf(str, sub) {
    if len(sub) == 0 { return 0 }
    parts = split(str, sub)
    if len(parts) == 1 { return -1 }
    return len(parts[0])
}

// startsWith returns true if str begins with prefix.
fn startsWith(str, prefix) {
    parts = split(str, prefix)
    return parts[0] == ""
}

// endsWith returns true if str ends with suffix.
fn endsWith(str, suffix) {
    parts = split(str, suffix)
    return parts[len(parts) - 1] == ""
}

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

// replace returns a copy of str with every occurrence of old replaced by new.
// replace("a-b-c", "-", ".") → "a.b.c"
fn replace(str, old, new) {
    parts = split(str, old)
    return join(parts, new)
}

// count returns the number of non-overlapping occurrences of sub in str.
// count("banana", "an") → 2
fn count(str, sub) {
    if len(sub) == 0 { return 0 }
    parts = split(str, sub)
    return len(parts) - 1
}

// trimLeft removes leading whitespace (spaces, tabs, newlines).
// Uses string indexing to scan from the front.
fn trimLeft(str) {
    i = 0
    while i < len(str) {
        c = str[i]
        if c != " " && c != "\t" && c != "\n" {
            result = ""
            while i < len(str) {
                result = result + str[i]
                i = i + 1
            }
            return result
        }
        i = i + 1
    }
    return ""
}

// trimRight removes trailing whitespace (spaces, tabs, newlines).
// Uses string indexing to scan from the back.
fn trimRight(str) {
    i = len(str) - 1
    while i >= 0 {
        c = str[i]
        if c != " " && c != "\t" && c != "\n" {
            result = ""
            j = 0
            while j <= i {
                result = result + str[j]
                j = j + 1
            }
            return result
        }
        i = i - 1
    }
    return ""
}

// trimSpace removes both leading and trailing whitespace. Wraps the built-in trim.
fn trimSpace(str) {
    return trim(str)
}

// padLeft pads str on the left with char until it reaches width.
// padLeft("42", 5, "0") → "00042"
fn padLeft(str, width, char) {
    result = str
    while len(result) < width {
        result = char + result
    }
    return result
}

// padRight pads str on the right with char until it reaches width.
// padRight("hi", 5, ".") → "hi..."
fn padRight(str, width, char) {
    result = str
    while len(result) < width {
        result = result + char
    }
    return result
}

// toUpper returns str with all letters uppercased. Wraps the built-in upper.
fn toUpper(str) {
    return upper(str)
}

// toLower returns str with all letters lowercased. Wraps the built-in lower.
fn toLower(str) {
    return lower(str)
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
