// actor.lex — Actor model for kLex
//
// An actor is a goroutine with a typed mailbox (buffered channel) and
// explicit state threaded between message iterations (Erlang/OTP style).
// Messages are processed one at a time — no shared mutable state needed.
//
// Three spawn variants:
//   actor.spawn(behavior, initialState)            — stateful, mailbox cap 32
//   actor.spawnBuffered(behavior, initialState, n) — stateful, mailbox cap n
//   actor.spawnStateless(behavior)                 — no state, mailbox cap 32
//
// Behavior signatures:
//   stateful:   fn(msg, state) { ... return newState }
//   stateless:  fn(msg)        { ... }
//
// Usage:
//   import "actor.lex" as actor
//
//   enum CountMsg { Add(n) Get(replyTo) }
//
//   a = actor.spawn(fn(msg, count) {
//       switch msg {
//           case CountMsg.Add(n)  { return count + n }
//           case CountMsg.Get(ch) { send(ch, count)  return count }
//       }
//       return count
//   }, 0)
//
//   a.send(CountMsg.Add(5))
//   a.send(CountMsg.Add(3))
//
//   replyCh = channel(1)
//   a.send(CountMsg.Get(replyCh))
//   n, _ = recv(replyCh)       // 8
//
//   final = a.stop()            // close mailbox, await exit, returns final state

struct Actor {
    mailbox
    task

    // send enqueues a message in the actor's mailbox.
    // Returns false if the mailbox has been closed (actor stopped).
    fn send(msg) {
        return send(self.mailbox, msg)
    }

    // stop closes the mailbox and blocks until the actor goroutine exits.
    // Returns the actor's final state, or an error if the behavior crashed.
    fn stop() {
        close(self.mailbox)
        return await(self.task)
    }
}

// _statefulLoop is the shared message loop for spawn and spawnBuffered.
// Errors from the behavior propagate naturally — the actor crashes and stop()
// surfaces the error. Behaviors that need to survive errors should use safe()
// internally.
fn _statefulLoop(behavior, initialState, mailbox) {
    return async(fn() {
        state = initialState
        msg, ok = recv(mailbox)
        while ok {
            state = behavior(msg, state)
            msg, ok = recv(mailbox)
        }
        return state
    })
}

// _statelessLoop is the shared message loop for spawnStateless.
fn _statelessLoop(behavior, mailbox) {
    return async(fn() {
        msg, ok = recv(mailbox)
        while ok {
            behavior(msg)
            msg, ok = recv(mailbox)
        }
    })
}

// spawn creates a stateful actor with a mailbox capacity of 32.
// behavior is called as fn(msg, state) and must return the new state.
fn spawn(behavior, initialState) {
    mailbox = channel(32)
    task = _statefulLoop(behavior, initialState, mailbox)
    return Actor { mailbox: mailbox, task: task }
}

// spawnBuffered creates a stateful actor with a caller-defined mailbox capacity.
// Use when you know your message volume needs a larger or smaller buffer than 32.
fn spawnBuffered(behavior, initialState, capacity) {
    mailbox = channel(capacity)
    task = _statefulLoop(behavior, initialState, mailbox)
    return Actor { mailbox: mailbox, task: task }
}

// spawnStateless creates an actor whose behavior takes only the message.
// Use for side-effectful actors (logging, I/O) that carry no state between messages.
fn spawnStateless(behavior) {
    mailbox = channel(32)
    task = _statelessLoop(behavior, mailbox)
    return Actor { mailbox: mailbox, task: task }
}
