// stdlib/observable.lex — reactive value wrapper
//
// An Observable wraps any value. When the value changes via set(), every
// subscribed handler is called synchronously with the new value.
//
// Multiple independent observables can coexist. Multiple handlers can
// subscribe to the same observable.
//
// Usage:
//   import "observable.lex" as obs
//
//   count = obs.newObservable(0)
//
//   count.subscribe(fn(n) { println(format("count is now %d", n)) })
//
//   count.set(1)   // prints: count is now 1
//   count.set(2)   // prints: count is now 2
//   println(count.get())  // 2

import "event.lex" as ev

struct Observable {
    val
    emitter

    // get() — return the current value without triggering any handlers.
    fn get() {
        return self.val
    }

    // set(newVal) — update the value and call every subscribed handler
    // with the new value. Handlers are called synchronously in the order
    // they were registered.
    fn set(newVal) {
        self.val = newVal
        self.emitter.emit("change", newVal)
        return null
    }

    // subscribe(handler) — register handler to be called on every future
    // set(). handler receives the new value as its only argument.
    // Returns null.
    fn subscribe(handler) {
        self.emitter.on("change", handler)
        return null
    }

    // unsubscribe(handler) — remove a previously registered handler.
    // The handler reference must be the same function object passed to
    // subscribe(). No-op if the handler is not registered.
    fn unsubscribe(handler) {
        self.emitter.off("change", handler)
        return null
    }

    // once(handler) — register a handler that fires exactly once on the
    // next set(), then removes itself automatically.
    fn once(handler) {
        self.emitter.once("change", handler)
        return null
    }

    // clear() — remove all subscribers.
    fn clear() {
        self.emitter.clear("change")
        return null
    }

    // map — returns a new Observable whose value is fnRef applied to each
    // emitted value. Initial value is fnRef(current value).
    fn map(fnRef) {
        let src = self
        out = newObservable(fnRef(src.get()))
        src.subscribe(fn(v) { out.set(fnRef(v)) })
        return out
    }

    // filter — returns a new Observable that only emits when fnRef returns true.
    // Initial value is the source's current value (may not satisfy the predicate).
    fn filter(fnRef) {
        let src = self
        out = newObservable(src.get())
        src.subscribe(fn(v) {
            if fnRef(v) { out.set(v) }
        })
        return out
    }

    // distinct — returns a new Observable that suppresses consecutive duplicate
    // values. Uses safe() for the equality check so cross-type comparisons are
    // treated as "different" rather than crashing.
    fn distinct() {
        let src  = self
        let last = src.get()
        out = newObservable(last)
        src.subscribe(fn(v) {
            eq, _ = safe(fn() { return v == last })
            last = v
            if eq != true { out.set(v) }
        })
        return out
    }

    // skip — returns a new Observable that ignores the first n values then
    // forwards all subsequent values unchanged.
    fn skip(n) {
        let src     = self
        let skipped = 0
        out = newObservable(src.get())
        src.subscribe(fn(v) {
            if skipped >= n {
                out.set(v)
            } else {
                skipped = skipped + 1
            }
        })
        return out
    }

    // take — returns a new Observable that forwards only the first n values
    // then silently stops. Source subscription remains active but is ignored.
    fn take(n) {
        let src   = self
        let count = 0
        out = newObservable(src.get())
        src.subscribe(fn(v) {
            if count < n {
                out.set(v)
                count = count + 1
            }
        })
        return out
    }

    // debounce — returns a new Observable that only emits after ms milliseconds
    // of silence. Rapid-fire values reset the timer; only the last one emits.
    // Uses a version counter because kLex has no async task cancellation.
    fn debounce(ms) {
        let src     = self
        let version = 0
        out = newObservable(src.get())
        src.subscribe(fn(v) {
            version = version + 1
            let myVer = version
            let myV   = v
            async(fn() {
                sleep(ms)
                if version == myVer { out.set(myV) }
            })
        })
        return out
    }
}

// newObservable(initial) — returns a new Observable holding initial as its
// starting value. No handlers are called for the initial value.
fn newObservable(initial) {
    return Observable { val: initial, emitter: ev.newEmitter() }
}


// -------------------------------------
// Computed — a read-only derived observable
//
// A Computed holds a value produced by a function over one or more source
// observables (or other Computeds). Whenever any dependency fires a change,
// the compute function re-runs immediately (eager), the cached value is
// updated, and all of this Computed's own subscribers are notified.
//
// Interface mirrors Observable (get, subscribe, unsubscribe, once, clear)
// so Computeds are interchangeable as dependencies of other Computeds.
// There is no set() — the value is always derived, never set externally.
//
// Usage:
//   a = newObservable(2)
//   b = newObservable(3)
//   sum = computed([a, b], fn() { a.get() + b.get() })
//   sum.get()   // 5
//   a.set(10)
//   sum.get()   // 13
// -------------------------------------
struct Computed {
    val
    computeFn
    emitter

    fn get() { return self.val }

    fn subscribe(handler)   { self.emitter.on("change", handler)   return null }
    fn unsubscribe(handler) { self.emitter.off("change", handler)  return null }
    fn once(handler)        { self.emitter.once("change", handler) return null }
    fn clear()              { self.emitter.clear("change")         return null }
}

// computed(deps, computeFn) — returns a new Computed derived from deps.
// deps    — array of Observable or Computed instances to watch.
// computeFn — zero-argument function that reads from deps and returns a value.
// The initial value is computed immediately at construction time.
fn computed(deps, computeFn) {
    c = Computed { val: computeFn(), computeFn: computeFn, emitter: ev.newEmitter() }
    for dep in deps {
        let cRef = c
        dep.subscribe(fn(_) {
            newVal = cRef.computeFn()
            cRef.val = newVal
            cRef.emitter.emit("change", newVal)
        })
    }
    return c
}


// -------------------------------------
// merge — fan-in multiple Observables into one.
// Any source firing causes the output to fire with that source's value.
// Initial value is null (no single meaningful combined starting value).
//
// Example:
//   clicks  = newObservable(null)
//   hovers  = newObservable(null)
//   events  = merge(clicks, hovers)
//   events.subscribe(fn(v) { println(v) })
// -------------------------------------
fn merge(sources...) {
    out = newObservable(null)
    for src in sources {
        src.subscribe(fn(v) { out.set(v) })
    }
    return out
}


// -------------------------------------
// combine — emit an array of the latest values from ALL sources whenever
// any one of them changes (combineLatest semantics).
// Initial value is an array of each source's current get() value.
//
// Example:
//   price    = newObservable(10)
//   quantity = newObservable(3)
//   total    = combine(price, quantity)
//   total.subscribe(fn(vals) { println(vals[0] * vals[1]) })
//   price.set(20)   // fires [20, 3]
// -------------------------------------
fn combine(sources...) {
    fn getAll() {
        vals = []
        for s in sources {
            vals = push(vals, s.get())
        }
        return vals
    }
    out = newObservable(getAll())
    for src in sources {
        src.subscribe(fn(_) { out.set(getAll()) })
    }
    return out
}
