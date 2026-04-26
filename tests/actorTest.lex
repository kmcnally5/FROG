import "actor.lex" as actor

// --- 1. Stateful actor: counter with Add and request-reply Get ---

enum CountMsg {
    Add(n)
    Get(replyTo)
}

println("=== stateful counter ===")

a = actor.spawn(fn(msg, count) {
    switch msg {
        case CountMsg.Add(n)  { return count + n }
        case CountMsg.Get(ch) { send(ch, count)  return count }
    }
    return count
}, 0)

a.send(CountMsg.Add(5))
a.send(CountMsg.Add(3))

replyCh = channel(1)
a.send(CountMsg.Get(replyCh))
n, _ = recv(replyCh)
println("count after +5+3 = " + str(n))

a.send(CountMsg.Add(10))
replyCh2 = channel(1)
a.send(CountMsg.Get(replyCh2))
n2, _ = recv(replyCh2)
println("count after +10 = " + str(n2))

final = a.stop()
println("final state from stop() = " + str(final))


// --- 2. Stateless actor: side-effectful logger ---

println("")
println("=== stateless logger ===")

log = actor.spawnStateless(fn(msg) {
    println("log: " + msg)
})

log.send("hello")
log.send("world")
log.send("done")
log.stop()


// --- 3. spawnBuffered with custom capacity ---

println("")
println("=== buffered actor (capacity 4) ===")

enum CalcMsg {
    Mul(n)
    Result(replyTo)
}

b = actor.spawnBuffered(fn(msg, acc) {
    switch msg {
        case CalcMsg.Mul(n)       { return acc * n }
        case CalcMsg.Result(ch)   { send(ch, acc)  return acc }
    }
    return acc
}, 2, 4)

b.send(CalcMsg.Mul(3))
b.send(CalcMsg.Mul(4))
b.send(CalcMsg.Mul(5))

resCh = channel(1)
b.send(CalcMsg.Result(resCh))
r, _ = recv(resCh)
println("2 * 3 * 4 * 5 = " + str(r))
b.stop()


// --- 4. Behavior crash surfaces through stop() ---

println("")
println("=== behavior crash ===")

enum BadMsg { Trigger }

bad = actor.spawn(fn(msg, s) {
    switch msg {
        case BadMsg.Trigger { return 1 + "oops" }
    }
    return s
}, 0)

bad.send(BadMsg.Trigger)
result, err = safe(fn() { return bad.stop() })
if err != null {
    println("actor crashed as expected: " + err.message)
} else {
    println("unexpected success")
}
