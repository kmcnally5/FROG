// ============================================================================
// stream_fusion.lex — Stream operation fusion and chaining
// ============================================================================
//
// Provides a way to chain stream operations (map, filter, reduce) efficiently
// by fusing them into a single pass through the data.
//
// Example:
//   result = fuse(
//       [1, 2, 3, 4, 5],
//       mapStep(fn(x) { x * 2 }),
//       filterStep(fn(x) { x > 4 })
//   )

// mapStep(fn) → step_object
// Creates a map operation step for use with fuse().
fn mapStep(mapFn) {
    return {
        "type": "map",
        "fn": mapFn
    }
}

// filterStep(fn) → step_object
// Creates a filter operation step for use with fuse().
fn filterStep(filterFn) {
    return {
        "type": "filter",
        "fn": filterFn
    }
}

// fuse(array, ...steps) → result_array
// Chains an arbitrary number of stream operations into a single pass.
// Each step is either a mapStep or filterStep object.
// Returns an array of processed elements.
//
// Example:
//   result = fuse([1, 2, 3], mapStep(fn(x) { x * 2 }), filterStep(fn(x) { x > 2 }))
//   // result = [4, 6]
fn fuse(arr, steps...) {
    if type(arr) != "ARRAY" {
        return []
    }

    n      = len(arr)
    nSteps = len(steps)
    result = makeArray(n, null)
    resultIdx = 0

    i = 0
    while i < n {
        item = arr[i]
        skip = false

        // Short-circuit once a filter rejects — subsequent steps would be
        // discarded anyway and might do expensive work.
        j = 0
        while j < nSteps && !skip {
            step     = steps[j]
            stepType = step["type"]
            stepFn   = step["fn"]

            if stepType == "map" {
                item = stepFn(item)
            } else if stepType == "filter" {
                if !stepFn(item) { skip = true }
            }

            j = j + 1
        }

        if !skip {
            result[resultIdx] = item
            resultIdx = resultIdx + 1
        }

        i = i + 1
    }

    return slice(result, 0, resultIdx)
}
