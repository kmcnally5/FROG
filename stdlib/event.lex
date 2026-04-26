// stdlib/event.lex — EventEmitter struct
//
// Replaces the former global singleton with an instantiable emitter.
// Multiple independent emitters can coexist in the same program.
//
// Usage:
//   import "event.lex" as ev
//   e = ev.newEmitter()
//   e.on("click", fn(data) { println(data) })
//   e.emit("click", 42)

struct EventEmitter {
    handlers

    // on(event, handler) — register a listener for an event.
    fn on(event, handler) {
        if self.handlers[event] == null {
            self.handlers[event] = []
        }
        self.handlers[event] = push(self.handlers[event], handler)
        return null
    }

    // emit(event, data) — call all listeners registered for event.
    fn emit(event, data) {
        if self.handlers[event] == null {
            return null
        }
        hs = self.handlers[event]
        i = 0
        while i < len(hs) {
            hs[i](data)
            i = i + 1
        }
        return null
    }

    // once(event, handler) — register a listener that fires exactly once then removes itself.
    fn once(event, handler) {
        emitter = self
        fn wrapper(data) {
            handler(data)
            emitter.off(event, wrapper)
        }
        self.on(event, wrapper)
        return null
    }

    // off(event, handler) — remove a specific listener from an event.
    fn off(event, handler) {
        if self.handlers[event] == null {
            return null
        }
        old = self.handlers[event]
        newList = []
        i = 0
        while i < len(old) {
            if old[i] != handler {
                newList = push(newList, old[i])
            }
            i = i + 1
        }
        self.handlers[event] = newList
        return null
    }

    // clear(event) — remove all listeners for an event.
    fn clear(event) {
        self.handlers[event] = []
        return null
    }

    // mapEvent(eventIn, eventOut, fn) — forward events through a transform function.
    fn mapEvent(eventIn, eventOut, fnRef) {
        emitter = self
        self.on(eventIn, fn(data) {
            emitter.emit(eventOut, fnRef(data))
        })
        return null
    }

    // filterEvent(eventIn, eventOut, fn) — forward only events where fn returns true.
    fn filterEvent(eventIn, eventOut, fnRef) {
        emitter = self
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
