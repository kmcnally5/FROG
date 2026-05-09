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
// Chains multiple stream operations together in a single pass.
// Each step is either a mapStep or filterStep object.
// Returns an array of processed elements.
//
// Example:
//   result = fuse([1, 2, 3], mapStep(fn(x) { x * 2 }), filterStep(fn(x) { x > 2 }))
//   // result = [4, 6]
fn fuse(arr, step1, step2, step3, step4, step5) {
    if type(arr) != "ARRAY" {
        return []
    }

    steps = makeArray(0, null)
    if step1 != null { steps[len(steps)] = step1 }
    if step2 != null { steps[len(steps)] = step2 }
    if step3 != null { steps[len(steps)] = step3 }
    if step4 != null { steps[len(steps)] = step4 }
    if step5 != null { steps[len(steps)] = step5 }

    result = makeArray(len(arr), null)
    resultIdx = 0

    i = 0
    while i < len(arr) {
        item = arr[i]
        skip = false

        j = 0
        while j < len(steps) {
            step = steps[j]
            stepType = step["type"]
            stepFn = step["fn"]

            if stepType == "map" {
                item = stepFn(item)
            } else if stepType == "filter" {
                if !stepFn(item) {
                    skip = true
                }
            }

            j = j + 1
        }

        if !skip {
            result[resultIdx] = item
            resultIdx = resultIdx + 1
        }

        i = i + 1
    }

    final = makeArray(resultIdx, null)
    k = 0
    while k < resultIdx {
        final[k] = result[k]
        k = k + 1
    }

    return final
}
