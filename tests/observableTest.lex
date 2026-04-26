import "observable.lex" as obs

println("--- observable: basic get/set ---")

counter = obs.newObservable(0)
println(counter.get() == 0)       // true — initial value accessible immediately

counter.set(42)
println(counter.get() == 42)      // true — get() reflects latest set()

counter.set(99)
println(counter.get() == 99)      // true


println("--- observable: subscribe fires on set ---")

fired = 0
lastVal = null

counter2 = obs.newObservable(0)
counter2.subscribe(fn(v) {
    fired = fired + 1
    lastVal = v
})

counter2.set(10)
println(fired == 1)               // true
println(lastVal == 10)            // true

counter2.set(20)
println(fired == 2)               // true
println(lastVal == 20)            // true


println("--- observable: multiple subscribers all fire ---")

a = 0
b = 0

multi = obs.newObservable(0)
multi.subscribe(fn(v) { a = v })
multi.subscribe(fn(v) { b = v * 2 })

multi.set(5)
println(a == 5)                   // true
println(b == 10)                  // true

multi.set(7)
println(a == 7)                   // true
println(b == 14)                  // true


println("--- observable: unsubscribe stops handler ---")

calls = 0

unsub = obs.newObservable(0)

fn countCalls(v) {
    calls = calls + 1
}

unsub.subscribe(countCalls)
unsub.set(1)
println(calls == 1)               // true

unsub.unsubscribe(countCalls)
unsub.set(2)
println(calls == 1)               // true — handler was removed, count unchanged


println("--- observable: once fires exactly once ---")

onceFired = 0

onceObs = obs.newObservable(0)
onceObs.once(fn(v) {
    onceFired = onceFired + 1
})

onceObs.set(1)
println(onceFired == 1)           // true — fired on first set

onceObs.set(2)
println(onceFired == 1)           // true — not fired again


println("--- observable: clear removes all subscribers ---")

clearCount = 0

clearObs = obs.newObservable(0)
clearObs.subscribe(fn(v) { clearCount = clearCount + 1 })
clearObs.subscribe(fn(v) { clearCount = clearCount + 1 })

clearObs.set(1)
println(clearCount == 2)          // true — both fired

clearObs.clear()
clearObs.set(2)
println(clearCount == 2)          // true — neither fired after clear


println("--- observable: independent instances do not interfere ---")

o1 = obs.newObservable(0)
o2 = obs.newObservable(100)

o1Result = 0
o2Result = 0

o1.subscribe(fn(v) { o1Result = v })
o2.subscribe(fn(v) { o2Result = v })

o1.set(7)
println(o1Result == 7)            // true
println(o2Result == 0)            // true — o2 not affected

o2.set(200)
println(o1Result == 7)            // true — o1 not affected
println(o2Result == 200)          // true


println("--- observable: real-world progress bar ---")

fn renderBar(n, total) {
    filled = int((n * 20) / total)
    bar = ""
    j = 0
    while j < 20 {
        if j < filled {
            bar = bar + "#"
        } else {
            bar = bar + "-"
        }
        j = j + 1
        sleep(15)
    }
    pct = int((n * 100) / total)
    print(format("\r[%s] %3d%%", bar, pct))
}

progress = obs.newObservable(0)
progress.subscribe(fn(n) { renderBar(n, 10) })

i = 0
while i <= 10 {
    progress.set(i)
    i = i + 1
}

println(progress.get() == 10)    // true


println("--- observable: handler receives correct value type ---")

typeObs = obs.newObservable(null)
receivedType = ""

typeObs.subscribe(fn(v) { receivedType = type(v) })

typeObs.set(42)
println(receivedType == "INTEGER")   // true

typeObs.set("hello")
println(receivedType == "STRING")    // true

typeObs.set(true)
println(receivedType == "BOOLEAN")   // true

typeObs.set(null)
println(receivedType == "NULL")      // true


println("--- computed: initial value derived at construction ---")

ca = obs.newObservable(3)
cb = obs.newObservable(4)
cSum = obs.computed([ca, cb], fn() { ca.get() + cb.get() })

println(cSum.get() == 7)            // true


println("--- computed: updates when dependency changes ---")

ca.set(10)
println(cSum.get() == 14)           // true — 10 + 4

cb.set(0)
println(cSum.get() == 10)           // true — 10 + 0


println("--- computed: multiple dependencies, each triggers recompute ---")

cx = obs.newObservable(2)
cy = obs.newObservable(3)
cz = obs.newObservable(4)
cProduct = obs.computed([cx, cy, cz], fn() { cx.get() * cy.get() * cz.get() })

println(cProduct.get() == 24)       // true — 2 * 3 * 4

cx.set(1)
println(cProduct.get() == 12)       // true — 1 * 3 * 4

cy.set(1)
println(cProduct.get() == 4)        // true — 1 * 1 * 4

cz.set(1)
println(cProduct.get() == 1)        // true — 1 * 1 * 1


println("--- computed: subscribers notified on dep change ---")

subFired = 0
subLast  = null

cBase = obs.newObservable(5)
cDoubled = obs.computed([cBase], fn() { cBase.get() * 2 })

cDoubled.subscribe(fn(v) {
    subFired = subFired + 1
    subLast  = v
})

cBase.set(10)
println(subFired == 1)              // true
println(subLast == 20)              // true

cBase.set(3)
println(subFired == 2)              // true
println(subLast == 6)               // true


println("--- computed: chained — computed depending on computed ---")

cA = obs.newObservable(1)
cB = obs.newObservable(2)

cSum2    = obs.computed([cA, cB],    fn() { cA.get() + cB.get() })
cDoubled2 = obs.computed([cSum2],    fn() { cSum2.get() * 2 })

println(cSum2.get() == 3)           // true — 1 + 2
println(cDoubled2.get() == 6)       // true — 3 * 2

cA.set(4)
println(cSum2.get() == 6)           // true — 4 + 2
println(cDoubled2.get() == 12)      // true — 6 * 2


println("--- computed: once fires on first dep change only ---")

onceFiredC = 0
cOnceBase = obs.newObservable(0)
cOnceComp = obs.computed([cOnceBase], fn() { cOnceBase.get() + 1 })

cOnceComp.once(fn(v) { onceFiredC = onceFiredC + 1 })

cOnceBase.set(10)
println(onceFiredC == 1)            // true

cOnceBase.set(20)
println(onceFiredC == 1)            // true — once, not again


println("--- computed: clear removes all subscribers ---")

clearC = 0
cClearBase = obs.newObservable(0)
cClearComp = obs.computed([cClearBase], fn() { cClearBase.get() })

cClearComp.subscribe(fn(v) { clearC = clearC + 1 })
cClearComp.subscribe(fn(v) { clearC = clearC + 1 })

cClearBase.set(1)
println(clearC == 2)                // true — both fired

cClearComp.clear()
cClearBase.set(2)
println(clearC == 2)                // true — neither fired after clear


println("--- computed: independent instances do not interfere ---")

indA = obs.newObservable(1)
indB = obs.newObservable(10)

compA = obs.computed([indA], fn() { indA.get() * 100 })
compB = obs.computed([indB], fn() { indB.get() * 100 })

indA.set(2)
println(compA.get() == 200)         // true
println(compB.get() == 1000)        // true — unchanged

indB.set(5)
println(compA.get() == 200)         // true — unchanged
println(compB.get() == 500)         // true


// =====================================================================
// OPERATORS
// =====================================================================

println("--- operator: map ---")

mapSrc = obs.newObservable(3)
mapOut = mapSrc.map(fn(x) { x * 10 })

println(mapOut.get() == 30)         // true — initial derived value

mapSrc.set(5)
println(mapOut.get() == 50)         // true

mapSrc.set(0)
println(mapOut.get() == 0)          // true


println("--- operator: map subscriber fires ---")

mapFired = 0
mapLast  = null
mapOut.subscribe(fn(v) {
    mapFired = mapFired + 1
    mapLast  = v
})

mapSrc.set(7)
println(mapFired == 1)              // true
println(mapLast == 70)             // true


println("--- operator: filter ---")

fSrc = obs.newObservable(0)
fOut = fSrc.filter(fn(x) { x > 5 })

println(fOut.get() == 0)           // true — initial value (doesn't pass predicate)

fSrc.set(3)
println(fOut.get() == 0)           // true — suppressed (3 <= 5)

fSrc.set(10)
println(fOut.get() == 10)          // true — passed (10 > 5)

fSrc.set(4)
println(fOut.get() == 10)          // true — suppressed, stale value kept

fSrc.set(20)
println(fOut.get() == 20)          // true — passed


println("--- operator: distinct ---")

dSrc = obs.newObservable(1)
dOut = dSrc.distinct()

dFired = 0
dOut.subscribe(fn(v) { dFired = dFired + 1 })

dSrc.set(1)                        // same — suppressed
println(dFired == 0)               // true

dSrc.set(2)                        // different — emitted
println(dFired == 1)               // true
println(dOut.get() == 2)           // true

dSrc.set(2)                        // same — suppressed
println(dFired == 1)               // true

dSrc.set(3)                        // different — emitted
println(dFired == 2)               // true
println(dOut.get() == 3)           // true


println("--- operator: skip ---")

skSrc = obs.newObservable(0)
skOut = skSrc.skip(3)

skVals = []
skOut.subscribe(fn(v) { skVals = push(skVals, v) })

skSrc.set(1)                       // skipped (1 of 3)
skSrc.set(2)                       // skipped (2 of 3)
skSrc.set(3)                       // skipped (3 of 3)
skSrc.set(4)                       // forwarded
skSrc.set(5)                       // forwarded

println(len(skVals) == 2)          // true
println(skVals[0] == 4)            // true
println(skVals[1] == 5)            // true


println("--- operator: take ---")

tkSrc = obs.newObservable(0)
tkOut = tkSrc.take(3)

tkVals = []
tkOut.subscribe(fn(v) { tkVals = push(tkVals, v) })

tkSrc.set(10)                      // forwarded (1 of 3)
tkSrc.set(20)                      // forwarded (2 of 3)
tkSrc.set(30)                      // forwarded (3 of 3)
tkSrc.set(40)                      // silently ignored
tkSrc.set(50)                      // silently ignored

println(len(tkVals) == 3)          // true
println(tkVals[0] == 10)           // true
println(tkVals[2] == 30)           // true


println("--- operator: debounce ---")

dbSrc = obs.newObservable(0)
dbOut = dbSrc.debounce(60)

dbFired = 0
dbLast  = null
dbOut.subscribe(fn(v) {
    dbFired = dbFired + 1
    dbLast  = v
})

dbSrc.set(1)                       // rapid-fire — all but last should be suppressed
dbSrc.set(2)
dbSrc.set(3)
sleep(120)                         // wait for debounce window to settle

println(dbFired == 1)              // true — only one emission
println(dbLast == 3)               // true — latest value won


println("--- operator: chaining map + filter ---")

chainSrc = obs.newObservable(1)
chainOut = chainSrc.map(fn(x) { x * 3 }).filter(fn(x) { x > 10 })

chainVals = []
chainOut.subscribe(fn(v) { chainVals = push(chainVals, v) })

chainSrc.set(2)                    // 2*3=6  — filtered out (6 <= 10)
chainSrc.set(4)                    // 4*3=12 — passes (12 > 10)
chainSrc.set(5)                    // 5*3=15 — passes (15 > 10)
chainSrc.set(3)                    // 3*3=9  — filtered out

println(len(chainVals) == 2)       // true
println(chainVals[0] == 12)        // true
println(chainVals[1] == 15)        // true


println("--- operator: merge ---")

mA = obs.newObservable(0)
mB = obs.newObservable(0)
mOut = obs.merge(mA, mB)

mVals = []
mOut.subscribe(fn(v) { mVals = push(mVals, v) })

mA.set(1)
mB.set(2)
mA.set(3)

println(len(mVals) == 3)           // true — each source fire recorded
println(mVals[0] == 1)             // true
println(mVals[1] == 2)             // true
println(mVals[2] == 3)             // true


println("--- operator: merge three sources ---")

m3A = obs.newObservable(0)
m3B = obs.newObservable(0)
m3C = obs.newObservable(0)
m3Out = obs.merge(m3A, m3B, m3C)

m3Count = 0
m3Out.subscribe(fn(v) { m3Count = m3Count + 1 })

m3A.set(1)
m3B.set(2)
m3C.set(3)

println(m3Count == 3)              // true


println("--- operator: combine ---")

cmbA = obs.newObservable(10)
cmbB = obs.newObservable(20)
cmbOut = obs.combine(cmbA, cmbB)

println(cmbOut.get()[0] == 10)     // true — initial snapshot
println(cmbOut.get()[1] == 20)     // true

cmbVals = []
cmbOut.subscribe(fn(v) { cmbVals = push(cmbVals, v) })

cmbA.set(5)
println(len(cmbVals) == 1)         // true
println(cmbVals[0][0] == 5)        // true — updated A
println(cmbVals[0][1] == 20)       // true — B unchanged

cmbB.set(99)
println(len(cmbVals) == 2)         // true
println(cmbVals[1][0] == 5)        // true — A unchanged
println(cmbVals[1][1] == 99)       // true — updated B


println("--- operator: combine three sources ---")

c3A = obs.newObservable(1)
c3B = obs.newObservable(2)
c3C = obs.newObservable(3)
c3Out = obs.combine(c3A, c3B, c3C)

println(len(c3Out.get()) == 3)     // true — initial array length
println(c3Out.get()[2] == 3)       // true

c3A.set(10)
println(c3Out.get()[0] == 10)      // true
println(c3Out.get()[1] == 2)       // true — B unchanged
println(c3Out.get()[2] == 3)       // true — C unchanged
