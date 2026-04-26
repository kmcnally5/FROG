// regex.lex
// Regular expression operations for kLex.
//
// All functions accept a Go-syntax regular expression pattern string.
// All fallible functions return (result, err) tuples — an invalid pattern
// is always surfaced as an error rather than a panic.
//
// Capture group functions (groups, groupsAll) return arrays where index 0
// is the full match and indexes 1+ are the individual capture groups.
//
// Usage:
//   import "regex.lex" as regex
//   matched, err = regex.match("[0-9]+", "abc123")
//   if err != null { println(err)  return null }
//   println(matched)   // true

// match returns true if pattern is found anywhere in str. Returns (bool, err).
fn match(pattern, str) {
    return _regexMatch(pattern, str)
}

// find returns the first match, or null if none. Returns (string|null, err).
fn find(pattern, str) {
    return _regexFind(pattern, str)
}

// findAll returns all non-overlapping matches as an array. Returns (array, err).
fn findAll(pattern, str) {
    return _regexFindAll(pattern, str)
}

// replace replaces the first match with repl. Returns (string, err).
fn replace(pattern, str, repl) {
    return _regexReplace(pattern, str, repl)
}

// replaceAll replaces all matches with repl. Returns (string, err).
fn replaceAll(pattern, str, repl) {
    return _regexReplaceAll(pattern, str, repl)
}

// split splits str on every match of pattern. Returns (array, err).
fn split(pattern, str) {
    return _regexSplit(pattern, str)
}

// groups returns the capture groups of the first match as an array, or null
// if there is no match. Index 0 is the full match, 1+ are capture groups.
// Returns (array|null, err).
fn groups(pattern, str) {
    return _regexGroups(pattern, str)
}

// groupsAll returns capture groups for every match. Each element is an array
// where index 0 is the full match and 1+ are capture groups.
// Returns (array, err).
fn groupsAll(pattern, str) {
    return _regexGroupsAll(pattern, str)
}
