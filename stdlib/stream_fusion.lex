// stdlib/stream_fusion.lex — single-pass eager stream fusion
//
// fuse runs an array through a pipeline of step functions in one pass.
// Each step receives (value, emit) — call emit(v) to pass a value forward,
// or return without calling emit to drop it.
//
// Usage:
//   import "stream_fusion.lex" as sf
//   result = sf.fuse(
//       [1, 2, 3, 4, 5, 6],
//       sf.mapStep(fn(x) { x * 2 }),
//       sf.filterStep(fn(x) { x > 4 })
//   )

struct _Slot {
    v, ok

    fn emit(val) {
        self.v  = val
        self.ok = true
        return null
    }
}

struct _Counter {
    i
}

// fuse(arr, steps...) — run arr through the given step pipeline in a single pass.
// Returns a new array containing only the values that survived all steps.
fn fuse(arr, steps...) {
    out = []
    i = 0
    while i < len(arr) {
        value = arr[i]
        ok = true

        j = 0
        while j < len(steps) {
            slot = _Slot { v: null, ok: false }
            steps[j](value, fn(v) { slot.emit(v) })

            if slot.ok == false {
                ok = false
                break
            }

            value = slot.v
            j = j + 1
        }

        if ok {
            out = push(out, value)
        }

        i = i + 1
    }

    return out
}

// mapStep(fnRef) — step that transforms each value via fnRef.
fn mapStep(fnRef) {
    return fn(x, emit) {
        emit(fnRef(x))
    }
}

// filterStep(fnRef) — step that forwards only values where fnRef returns true.
fn filterStep(fnRef) {
    return fn(x, emit) {
        if fnRef(x) {
            emit(x)
        }
    }
}

// tapStep(fnRef) — step that calls fnRef for its side effect, then forwards unchanged.
fn tapStep(fnRef) {
    return fn(x, emit) {
        fnRef(x)
        emit(x)
    }
}

// takeStep(n) — step that forwards only the first n values, dropping the rest.
fn takeStep(n) {
    counter = _Counter { i: 0 }
    return fn(x, emit) {
        if counter.i < n {
            emit(x)
            counter.i = counter.i + 1
        }
    }
}
