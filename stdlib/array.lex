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
    result = []
    i = len(arr) - 1
    while i >= 0 {
        result = push(result, arr[i])
        i = i - 1
    }
    return result
}

// unique returns a new array with duplicate values removed.
// First occurrence of each value is kept; order is preserved.
fn unique(arr) {
    result = []
    for x in arr {
        if !contains(result, x) {
            result = push(result, x)
        }
    }
    return result
}

// flatten returns a new array with one level of nesting removed.
// Non-array elements are included as-is.
// flatten([[1, 2], [3, 4], 5]) => [1, 2, 3, 4, 5]
fn flatten(arr) {
    result = []
    for x in arr {
        if type(x) == "ARRAY" {
            for item in x {
                result = push(result, item)
            }
        } else {
            result = push(result, x)
        }
    }
    return result
}

// zip pairs elements from two arrays by index into an array of two-element arrays.
// Stops at the shorter array's length.
// zip([1, 2, 3], ["a", "b", "c"]) => [[1, "a"], [2, "b"], [3, "c"]]
fn zip(a, b) {
    result = []
    n = len(a)
    if len(b) < n { n = len(b) }
    i = 0
    while i < n {
        result = push(result, [a[i], b[i]])
        i = i + 1
    }
    return result
}

// sortBy returns a new array sorted by a caller-supplied comparator.
// compareFn(a, b) must return true if a should come before b.
// Uses bubble sort — suitable for small arrays.
//
//   sortBy(nums,  fn(a, b) { return a < b })          // ascending
//   sortBy(nums,  fn(a, b) { return a > b })          // descending
//   sortBy(users, fn(a, b) { return a.age < b.age })  // by struct field
//   sortBy(words, fn(a, b) { return len(a) < len(b) }) // by derived key
fn sortBy(arr, compareFn) {
    result = []
    for x in arr {
        result = push(result, x)
    }
    n = len(result)
    i = 0
    while i < n {
        j = 0
        while j < n - i - 1 {
            if compareFn(result[j + 1], result[j]) {
                tmp = result[j]
                result[j] = result[j + 1]
                result[j + 1] = tmp
            }
            j = j + 1
        }
        i = i + 1
    }
    return result
}

// sort returns a new array sorted in ascending order.
// Works for integers and strings. For structs, custom ordering,
// or descending sorts use sortBy.
fn sort(arr) {
    return sortBy(arr, fn(a, b) { return a < b })
}

// splitArray splits an array on a separator value.
// Example:
//   splitArray([1, 2, ":", 3, 4], ":")
//   => [[1, 2], [3, 4]]

fn split(arr, sep) {
    result = []
    current = []

    i = 0
    while i < len(arr) {
        x = arr[i]

        if type(x) == type(sep) && x == sep {
            result = push(result, current)
            current = []
        } else {
            current = push(current, x)
        }

        i = i + 1
    }

    // push final chunk
    result = push(result, current)

    return result
}
