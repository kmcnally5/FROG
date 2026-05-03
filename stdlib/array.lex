// stdlib/array.lex — kLex standard array library
//
// Provides common array operations not built into the language.
// All functions return new arrays — none mutate the input.
//
// Usage:
//   import "array.lex" as arr
//   println(arr.first([10, 20, 30]))    // 10
//   println(arr.reverse([1, 2, 3]))     // [3, 2, 1]

// first returns the first element of an array.
fn first(arr) {
    return arr[0]
}

// last returns the last element of an array.
fn last(arr) {
    return arr[len(arr) - 1]
}

// contains returns true if val is present in arr.
// Uses == so it works for integers, strings, booleans, and null.
fn contains(arr, val) {
    for x in arr {
        if x == val { return true }
    }
    return false
}

// reverse returns a new array with elements in reverse order.
fn reverse(arr) {
    result = makeArray(len(arr), null)
    i = len(arr) - 1
    j = 0
    while i >= 0 {
        result[j] = arr[i]
        i = i - 1
        j = j + 1
    }
    return result
}

// unique returns a new array with duplicate values removed.
// First occurrence of each value is kept; order is preserved.
fn unique(arr) {
    result = makeArray(len(arr), null)
    idx = 0
    for x in arr {
        if !contains(slice(result, 0, idx), x) {
            result[idx] = x
            idx = idx + 1
        }
    }
    return slice(result, 0, idx)
}

// flatten returns a new array with one level of nesting removed.
// Non-array elements are included as-is.
// flatten([[1, 2], [3, 4], 5]) => [1, 2, 3, 4, 5]
fn flatten(arr) {
    result = makeArray(len(arr) * 2, null)
    idx = 0
    for x in arr {
        if type(x) == "ARRAY" {
            for item in x {
                result[idx] = item
                idx = idx + 1
            }
        } else {
            result[idx] = x
            idx = idx + 1
        }
    }
    return slice(result, 0, idx)
}

// zip pairs elements from two arrays by index into an array of two-element arrays.
// Stops at the shorter array's length.
// zip([1, 2, 3], ["a", "b", "c"]) => [[1, "a"], [2, "b"], [3, "c"]]
fn zip(a, b) {
    n = len(a)
    if len(b) < n { n = len(b) }
    result = makeArray(n, null)
    i = 0
    while i < n {
        result[i] = [a[i], b[i]]
        i = i + 1
    }
    return result
}

// sort and sortBy are built into the interpreter — no import needed.
// sort(arr)              → ascending order (integers, floats, strings)
// sortBy(arr, compareFn) → custom order; compareFn(a, b) returns true if a < b
// Both use a stable O(n log n) sort.

// splitArray splits an array on a separator value.
// Example:
//   splitArray([1, 2, ":", 3, 4], ":")
//   => [[1, 2], [3, 4]]

fn split(arr, sep) {
    result = makeArray(len(arr), null)
    current = makeArray(len(arr), null)
    resultIdx = 0
    currentIdx = 0

    i = 0
    while i < len(arr) {
        x = arr[i]

        if type(x) == type(sep) && x == sep {
            result[resultIdx] = slice(current, 0, currentIdx)
            resultIdx = resultIdx + 1
            currentIdx = 0
        } else {
            current[currentIdx] = x
            currentIdx = currentIdx + 1
        }

        i = i + 1
    }

    // add final chunk
    result[resultIdx] = slice(current, 0, currentIdx)
    resultIdx = resultIdx + 1

    return slice(result, 0, resultIdx)
}
