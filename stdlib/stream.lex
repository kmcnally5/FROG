// stdlib/stream.lex — channel-based lazy streams with error propagation
//
// Every Stream carries two channels:
//   ch    — data values
//   errCh — buffered(1); the producer sends exactly once before closing ch:
//             null  → completed without error
//             Error → completed with error
//
// Cancellation: when a consumer breaks out of a for-in loop over ch the
// evaluator automatically closes ch.done, unblocking any blocked sender
// (which then returns false from send()). Explicit cancel(ch) is also
// available for cases where for-in is not used (e.g. zip uses recv()).
//
// Terminal functions (collect, reduce) return (value, null) on success
// and (null, error) on failure.
//
// Usage:
//   import "stream.lex" as s
//   result, err = s.collect(s.map(s.fromArray([1,2,3]), fn(x) { x * 2 }))

struct Stream {
    ch
    errCh
}


// -------------------------------------
// fromArray — produce a stream from an array
// -------------------------------------
fn fromArray(arr) {
    ch    = channel(64)
    errCh = channel(1)
    async(fn() {
        for x in arr {
            if send(ch, x) == false { break }
        }
        send(errCh, null)
        close(ch)
    })
    return Stream { ch: ch, errCh: errCh }
}


// -------------------------------------
// rangeStream — produce a stream of integers [start, stop)
// -------------------------------------
fn rangeStream(start, stop) {
    ch    = channel(64)
    errCh = channel(1)
    async(fn() {
        i = start
        while i < stop {
            if send(ch, i) == false { break }
            i = i + 1
        }
        send(errCh, null)
        close(ch)
    })
    return Stream { ch: ch, errCh: errCh }
}


// -------------------------------------
// repeat — infinite stream of a single value
// -------------------------------------
fn repeat(val) {
    ch    = channel(64)
    errCh = channel(1)
    async(fn() {
        while true {
            if send(ch, val) == false { break }
        }
        send(errCh, null)
        close(ch)
    })
    return Stream { ch: ch, errCh: errCh }
}


// -------------------------------------
// map — transform each element (lazy)
//
// Uses safe() to catch callback errors without letting them auto-propagate
// through the goroutine. Breaking out of the for-in auto-cancels src.
// -------------------------------------
fn map(stream, fnRef) {
    src    = stream.ch
    srcErr = stream.errCh
    out    = channel(64)
    errCh  = channel(1)
    async(fn() {
        let myErr        = null
        let outCancelled = false
        for x in src {
            result, callErr = safe(fnRef, x)
            if callErr != null {
                myErr = callErr
                break
            }
            if isError(result) {
                myErr = result
                break
            }
            if send(out, result) == false {
                outCancelled = true
                break
            }
        }
        if myErr != null || outCancelled {
            send(errCh, myErr)
        } else {
            upstreamErr, _ = recv(srcErr)
            send(errCh, upstreamErr)
        }
        close(out)
    })
    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// filter — keep elements where fnRef returns true (lazy)
// -------------------------------------
fn filter(stream, fnRef) {
    src    = stream.ch
    srcErr = stream.errCh
    out    = channel(64)
    errCh  = channel(1)
    async(fn() {
        let myErr        = null
        let outCancelled = false
        for x in src {
            keep, callErr = safe(fnRef, x)
            if callErr != null {
                myErr = callErr
                break
            }
            if isError(keep) {
                myErr = keep
                break
            }
            if keep {
                if send(out, x) == false {
                    outCancelled = true
                    break
                }
            }
        }
        if myErr != null || outCancelled {
            send(errCh, myErr)
        } else {
            upstreamErr, _ = recv(srcErr)
            send(errCh, upstreamErr)
        }
        close(out)
    })
    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// take — limit stream to first n elements (lazy, terminating)
// -------------------------------------
fn take(stream, n) {
    src    = stream.ch
    srcErr = stream.errCh
    out    = channel(64)
    errCh  = channel(1)
    async(fn() {
        let limitHit     = false
        let outCancelled = false
        count = 0
        for x in src {
            if count >= n {
                limitHit = true
                break
            }
            if send(out, x) == false {
                outCancelled = true
                break
            }
            count = count + 1
        }
        if limitHit || outCancelled {
            send(errCh, null)
        } else {
            upstreamErr, _ = recv(srcErr)
            send(errCh, upstreamErr)
        }
        close(out)
    })
    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// tap — run a side-effect on each element without changing it (lazy)
// -------------------------------------
fn tap(stream, fnRef) {
    src    = stream.ch
    srcErr = stream.errCh
    out    = channel(64)
    errCh  = channel(1)
    async(fn() {
        let myErr        = null
        let outCancelled = false
        for x in src {
            tapResult, callErr = safe(fnRef, x)
            if callErr != null {
                myErr = callErr
                break
            }
            if isError(tapResult) {
                myErr = tapResult
                break
            }
            if send(out, x) == false {
                outCancelled = true
                break
            }
        }
        if myErr != null || outCancelled {
            send(errCh, myErr)
        } else {
            upstreamErr, _ = recv(srcErr)
            send(errCh, upstreamErr)
        }
        close(out)
    })
    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// flatMap — for each element, fn returns a Stream; all inner streams are
// drained into a single output stream in order (sequential, not concurrent).
// -------------------------------------
fn flatMap(stream, fnRef) {
    src    = stream.ch
    srcErr = stream.errCh
    out    = channel(64)
    errCh  = channel(1)
    async(fn() {
        let myErr        = null
        let outCancelled = false
        for x in src {
            inner, callErr = safe(fnRef, x)
            if callErr != null {
                myErr = callErr
                break
            }
            if isError(inner) {
                myErr = inner
                break
            }
            for y in inner.ch {
                if send(out, y) == false {
                    outCancelled = true
                    break
                }
            }
            if outCancelled {
                break
            }
            innerErr, _ = recv(inner.errCh)
            if innerErr != null {
                myErr = innerErr
                break
            }
        }
        if myErr != null || outCancelled {
            send(errCh, myErr)
        } else {
            upstreamErr, _ = recv(srcErr)
            send(errCh, upstreamErr)
        }
        close(out)
    })
    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// merge — fan-in: drain multiple streams into one output stream.
// Values arrive in whichever order the source goroutines produce them.
// Output stream closes only after every source is exhausted.
// -------------------------------------
fn merge(streams...) {
    out     = channel(64)
    errCh   = channel(1)
    results = channel(len(streams))

    for st in streams {
        let src    = st.ch
        let srcErr = st.errCh
        async(fn() {
            let outCancelled = false
            for x in src {
                if send(out, x) == false {
                    outCancelled = true
                    break
                }
            }
            if outCancelled {
                send(results, null)
            } else {
                srcErrVal, _ = recv(srcErr)
                send(results, srcErrVal)
            }
        })
    }

    async(fn() {
        let firstErr = null
        let i = 0
        while i < len(streams) {
            err, _ = recv(results)
            if firstErr == null && err != null {
                firstErr = err
            }
            i = i + 1
        }
        send(errCh, firstErr)
        close(out)
    })

    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// zip — synchronise multiple streams element-by-element.
// Each tick reads one value from every stream and emits them as an array.
// Stops as soon as any stream is exhausted. Cancels remaining streams on exit.
// -------------------------------------
fn zip(streams...) {
    out   = channel(64)
    errCh = channel(1)
    async(fn() {
        let myErr        = null
        let outCancelled = false
        while true {
            let vals = makeArray(len(streams), null)
            let done = false
            let idx = 0
            for st in streams {
                val, ok = recv(st.ch)
                if ok == false {
                    stErr, _ = recv(st.errCh)
                    if stErr != null { myErr = stErr }
                    done = true
                    break
                }
                vals[idx] = val
                idx = idx + 1
            }
            if done {
                for st in streams { cancel(st.ch) }
                break
            }
            if send(out, vals) == false {
                outCancelled = true
                for st in streams { cancel(st.ch) }
                break
            }
        }
        if myErr != null {
            send(errCh, myErr)
        } else {
            send(errCh, null)
        }
        close(out)
    })
    return Stream { ch: out, errCh: errCh }
}


// -------------------------------------
// collect — drain stream into an array (terminal)
// Returns (array, null) on success, (null, error) on failure.
// Uses pre-allocated buffer with doubling to avoid O(n²) push() overhead.
// -------------------------------------
fn collect(stream) {
    let capacity = 1024
    let buffer = makeArray(capacity, null)
    let count = 0

    for x in stream.ch {
        if count >= capacity {
            let newCapacity = capacity * 2
            let newBuffer = makeArray(newCapacity, null)
            let i = 0
            while i < count {
                newBuffer[i] = buffer[i]
                i = i + 1
            }
            buffer = newBuffer
            capacity = newCapacity
        }
        buffer[count] = x
        count = count + 1
    }

    let out = makeArray(count, null)
    let i = 0
    while i < count {
        out[i] = buffer[i]
        i = i + 1
    }

    errVal, _ = recv(stream.errCh)
    if errVal != null { return null, errVal }
    return out, null
}


// -------------------------------------
// reduce — fold stream to a single value (terminal)
// Returns (value, null) on success, (null, error) on failure.
// -------------------------------------
fn reduce(stream, fnRef, init) {
    acc = init
    for x in stream.ch {
        acc = fnRef(acc, x)
    }
    errVal, _ = recv(stream.errCh)
    if errVal != null { return null, errVal }
    return acc, null
}


// -------------------------------------
// pipe — compose a stream through a sequence of operations left-to-right.
// The last op may be a terminal (collect, reduce) returning a non-Stream value.
// -------------------------------------
fn pipe(stream, ops...) {
    let result = stream
    for op in ops {
        result = op(result)
    }
    return result
}


// -------------------------------------
// Curried operator builders for use with pipe()
// Uppercase = returns a fn(stream) suitable for pipe
// Lowercase = direct two-argument form (unchanged)
// -------------------------------------
fn Map(f)     { return fn(st) { return map(st, f) } }
fn Filter(f)  { return fn(st) { return filter(st, f) } }
fn Take(n)    { return fn(st) { return take(st, n) } }
fn Tap(f)     { return fn(st) { return tap(st, f) } }
fn FlatMap(f) { return fn(st) { return flatMap(st, f) } }
