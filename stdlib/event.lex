// stdlib/event.lex — EventEmitter struct
// @module    event
// @version   1.0.0
// @since     klex 0.3.35
// @author    karl
// @summary   EventEmitter struct
//
// Replaces the former global singleton with an instantiable emitter.
// Multiple independent emitters can coexist in the same program.
//
// Usage:
//   import "stdlib/event.lex" as ev
//   e = ev.newEmitter()
//   e.on("click", fn(data) { println(data) })
//   e.emit("click", 42)

// Each event's listener list is stored as a doubling buffer:
//   { arr: <array>, count: <live entries>, cap: <allocated slots> }
//
// `arr` always has length `cap`; only the first `count` slots are live.
// When count == cap, we allocate a new array at 2× capacity and copy.
// This keeps on()/off() amortised O(1) and avoids the O(n²) push() growth
// that the previous implementation incurred per subscription.

struct EventEmitter {
    handlers

    // on(event, handler) — register a listener for an event.
    fn on(event, handler) {
        let slot = self.handlers[event]
        if slot == null {
            slot = { "arr": makeArray(4, null), "count": 0, "cap": 4 }
        }
        if slot["count"] >= slot["cap"] {
            let newCap = slot["cap"] * 2
            let newArr = makeArray(newCap, null)
            let i = 0
            while i < slot["count"] {
                newArr[i] = slot["arr"][i]
                i = i + 1
            }
            slot["arr"] = newArr
            slot["cap"] = newCap
        }
        slot["arr"][slot["count"]] = handler
        slot["count"] = slot["count"] + 1
        self.handlers[event] = slot
        return null
    }

    // emit(event, data) — call all listeners registered for event.
    fn emit(event, data) {
        let slot = self.handlers[event]
        if slot == null { return null }
        let hs = slot["arr"]
        let n  = slot["count"]
        let i  = 0
        while i < n {
            hs[i](data)
            i = i + 1
        }
        return null
    }

    // once(event, handler) — register a listener that fires exactly once then removes itself.
    fn once(event, handler) {
        let emitter = self
        fn wrapper(data) {
            handler(data)
            emitter.off(event, wrapper)
        }
        self.on(event, wrapper)
        return null
    }

    // off(event, handler) — remove a specific listener from an event.
    // Compacts the buffer in place; capacity is not shrunk (idiomatic — most
    // emitters add/remove around a steady-state size).
    fn off(event, handler) {
        let slot = self.handlers[event]
        if slot == null { return null }
        let arr   = slot["arr"]
        let count = slot["count"]
        let w = 0
        let i = 0
        while i < count {
            if arr[i] != handler {
                arr[w] = arr[i]
                w = w + 1
            }
            i = i + 1
        }
        // Null out the freed tail so we don't pin handlers garbage-collector-style.
        let j = w
        while j < count {
            arr[j] = null
            j = j + 1
        }
        slot["count"] = w
        self.handlers[event] = slot
        return null
    }

    // clear(event) — remove all listeners for an event.
    fn clear(event) {
        self.handlers[event] = { "arr": makeArray(4, null), "count": 0, "cap": 4 }
        return null
    }

    // mapEvent(eventIn, eventOut, fn) — forward events through a transform function.
    fn mapEvent(eventIn, eventOut, fnRef) {
        let emitter = self
        self.on(eventIn, fn(data) {
            emitter.emit(eventOut, fnRef(data))
        })
        return null
    }

    // filterEvent(eventIn, eventOut, fn) — forward only events where fn returns true.
    fn filterEvent(eventIn, eventOut, fnRef) {
        let emitter = self
        self.on(eventIn, fn(data) {
            if fnRef(data) {
                emitter.emit(eventOut, data)
            }
        })
        return null
    }

    // logEvent(event) — attach a debug listener that prints every emission.
    fn logEvent(event) {
        self.on(event, fn(data) {
            println("[event: " + event + "] " + str(data))
        })
        return null
    }
}

// newEmitter() returns a fresh EventEmitter with no listeners registered.
fn newEmitter() {
    return EventEmitter { handlers: {} }
}
