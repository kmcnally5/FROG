import "stream.lex" as s

println("--- stream.lex tests ---")

// -------------------------------------
// fromArray + collect
// -------------------------------------
arr = [1, 2, 3, 4, 5]

ch = s.fromArray(arr)
res, err = s.collect(ch)

println(err == null)
println(len(res) == 5)
println(res[0] == 1)
println(res[4] == 5)


// -------------------------------------
// map
// -------------------------------------
ch2 = s.fromArray([1, 2, 3, 4, 5])
mapped = s.map(ch2, fn(x) { x * 2 })

res2, err2 = s.collect(mapped)

println(err2 == null)
println(res2[0] == 2)
println(res2[1] == 4)
println(res2[4] == 10)


// -------------------------------------
// filter
// -------------------------------------
ch3 = s.fromArray([1, 2, 3, 4, 5, 6])
filtered = s.filter(ch3, fn(x) { x % 2 == 0 })

res3, err3 = s.collect(filtered)

println(err3 == null)
println(len(res3) == 3)
println(res3[0] == 2)
println(res3[2] == 6)


// -------------------------------------
// take
// -------------------------------------
ch4 = s.rangeStream(0, 100)
taken = s.take(ch4, 5)

res4, err4 = s.collect(taken)

println(err4 == null)
println(len(res4) == 5)
println(res4[0] == 0)
println(res4[4] == 4)


// -------------------------------------
// tap (side effects)
// -------------------------------------
log = []

ch5 = s.fromArray([10, 20, 30])

tapped = s.tap(ch5, fn(x) {
    log = push(log, x)
})

res5, err5 = s.collect(tapped)

println(err5 == null)
println(len(log) == 3)
println(log[0] == 10)
println(log[2] == 30)

println(len(res5) == 3)
println(res5[1] == 20)


// -------------------------------------
// reduce
// -------------------------------------
ch6 = s.fromArray([1, 2, 3, 4])

sum, sumErr = s.reduce(ch6, fn(acc, x) { acc + x }, 0)
println(sumErr == null)
println(sum == 10)

ch7 = s.fromArray([1, 2, 3, 4])
prod, prodErr = s.reduce(ch7, fn(acc, x) { acc * x }, 1)
println(prodErr == null)
println(prod == 24)


// -------------------------------------
// rangeStream
// -------------------------------------
ch8 = s.rangeStream(5, 10)
res8, err8 = s.collect(ch8)

println(err8 == null)
println(len(res8) == 5)
println(res8[0] == 5)
println(res8[4] == 9)


// -------------------------------------
// repeat + take (IMPORTANT COMBO TEST)
// Cancellation: take breaks early, auto-cancels repeat's channel.
// -------------------------------------
ch9 = s.repeat(7)
limited = s.take(ch9, 3)
res9, err9 = s.collect(limited)

println(err9 == null)
println(len(res9) == 3)
println(res9[0] == 7)
println(res9[2] == 7)


// -------------------------------------
// chaining WITHOUT objects (IMPORTANT)
// -------------------------------------
ch10 = s.fromArray([1, 2, 3, 4, 5])

step1 = s.map(ch10, fn(x) { x + 1 })
step2 = s.filter(step1, fn(x) { x % 2 == 0 })
res10, err10 = s.collect(step2)

println(err10 == null)
println(len(res10) > 0)
println(res10[0] == 2)


// -------------------------------------
// empty stream
// -------------------------------------
ch11 = s.fromArray([])
res11, err11 = s.collect(ch11)

println(err11 == null)
println(len(res11) == 0)


// -------------------------------------
// pipe + curried builders
// -------------------------------------

// basic map + filter + collect via pipe
res12, err12 = s.pipe(
    s.fromArray([1, 2, 3, 4, 5]),
    s.Map(fn(x) { x * 2 }),
    s.Filter(fn(x) { x > 4 }),
    s.collect
)
println(err12 == null)
println(len(res12) == 3)     // [6, 8, 10]
println(res12[0] == 6)
println(res12[2] == 10)

// pipe with Take — cancellation path
res13, err13 = s.pipe(
    s.rangeStream(0, 100),
    s.Take(4),
    s.collect
)
println(err13 == null)
println(len(res13) == 4)
println(res13[0] == 0)
println(res13[3] == 3)

// pipe with Tap (side effect)
tlog = []
res14, err14 = s.pipe(
    s.fromArray([10, 20, 30]),
    s.Tap(fn(x) { tlog = push(tlog, x) }),
    s.collect
)
println(err14 == null)
println(len(tlog) == 3)
println(tlog[1] == 20)
println(len(res14) == 3)

// pipe with reduce as terminal op
sum2, sumErr2 = s.pipe(
    s.fromArray([1, 2, 3, 4, 5]),
    s.Map(fn(x) { x * 2 }),
    fn(st) { return s.reduce(st, fn(acc, x) { acc + x }, 0) }
)
println(sumErr2 == null)
println(sum2 == 30)


// -------------------------------------
// flatMap
// -------------------------------------

// each element fans out into two values
res15, err15 = s.pipe(
    s.fromArray([1, 2, 3]),
    s.FlatMap(fn(x) { return s.fromArray([x, x * 10]) }),
    s.collect
)
println(err15 == null)
println(len(res15) == 6)       // [1, 10, 2, 20, 3, 30]
println(res15[0] == 1)
println(res15[1] == 10)
println(res15[4] == 3)
println(res15[5] == 30)

// flatMap with filter in inner stream
res16, err16 = s.pipe(
    s.fromArray([1, 2, 3, 4]),
    s.FlatMap(fn(x) { return s.fromArray([x * 2, x * 3]) }),
    s.Filter(fn(x) { x > 6 }),
    s.collect
)
println(err16 == null)
println(len(res16) == 3)
println(res16[0] == 9)

// direct flatMap call (non-pipe form)
res17, err17 = s.collect(s.flatMap(s.fromArray([10, 20]), fn(x) { return s.fromArray([x, x + 1]) }))
println(err17 == null)
println(len(res17) == 4)       // [10, 11, 20, 21]
println(res17[0] == 10)
println(res17[1] == 11)
println(res17[2] == 20)
println(res17[3] == 21)


// -------------------------------------
// zip
// -------------------------------------

// basic zip of two equal-length streams
zipped, zerr = s.collect(s.zip(s.fromArray([1, 2, 3]), s.fromArray([10, 20, 30])))
println(zerr == null)
println(len(zipped) == 3)
println(zipped[0][0] == 1)
println(zipped[0][1] == 10)
println(zipped[2][0] == 3)
println(zipped[2][1] == 30)

// zip stops at the shorter stream
zipped2, zerr2 = s.collect(s.zip(s.fromArray([1, 2, 3]), s.fromArray([10, 20])))
println(zerr2 == null)
println(len(zipped2) == 2)

// zip three streams
zipped3, zerr3 = s.collect(s.zip(s.fromArray([1, 2]), s.fromArray([10, 20]), s.fromArray([100, 200])))
println(zerr3 == null)
println(len(zipped3) == 2)
println(zipped3[0][0] == 1)
println(zipped3[0][1] == 10)
println(zipped3[0][2] == 100)

// zip feeds into pipe
summed, sumerr = s.pipe(
    s.zip(s.fromArray([1, 2, 3]), s.fromArray([10, 20, 30])),
    s.Map(fn(pair) { pair[0] + pair[1] }),
    s.collect
)
println(sumerr == null)
println(len(summed) == 3)
println(summed[0] == 11)
println(summed[1] == 22)
println(summed[2] == 33)


// -------------------------------------
// merge
// -------------------------------------

// merge two streams
merged, merr = s.collect(s.merge(s.fromArray([1, 2, 3]), s.fromArray([4, 5, 6])))
println(merr == null)
println(len(merged) == 6)

// merge with pipe
merged2, merr2 = s.pipe(
    s.merge(s.fromArray([1, 2]), s.fromArray([3, 4])),
    s.Map(fn(x) { x * 2 }),
    s.collect
)
println(merr2 == null)
println(len(merged2) == 4)

// merge three streams
merged3, merr3 = s.collect(s.merge(s.fromArray([1]), s.fromArray([2]), s.fromArray([3])))
println(merr3 == null)
println(len(merged3) == 3)


// -------------------------------------
// ERROR PROPAGATION — map callback fails
// Error flows through the pipeline to collect.
// -------------------------------------
fn failOnThree(x) {
    if x == 3 {
        return error("TEST_ERROR", "deliberate failure at 3")
    }
    return x * 10
}

eStream = s.map(s.fromArray([1, 2, 3, 4, 5]), failOnThree)
eRes, eErr = s.collect(eStream)

println(eRes == null)       // no result on error
println(eErr != null)       // error is present


// -------------------------------------
// ERROR PROPAGATION — error through pipe chain
// -------------------------------------
eRes2, eErr2 = s.pipe(
    s.fromArray([1, 2, 3, 4, 5]),
    s.Map(failOnThree),
    s.collect
)
println(eRes2 == null)
println(eErr2 != null)


// -------------------------------------
// CANCELLATION — take stops infinite repeat cleanly
// Goroutine leak test: program must not hang after this block.
// -------------------------------------
bigStream = s.take(s.repeat(42), 10)
bigRes, bigErr = s.collect(bigStream)
println(bigErr == null)
println(len(bigRes) == 10)
println(bigRes[0] == 42)


// -------------------------------------
// CANCELLATION — take in multi-stage pipeline cancels upstream
// -------------------------------------
cancelRes, cancelErr = s.pipe(
    s.rangeStream(0, 1000000),
    s.Map(fn(x) { x * 2 }),
    s.Filter(fn(x) { x % 4 == 0 }),
    s.Take(5),
    s.collect
)
println(cancelErr == null)
println(len(cancelRes) == 5)
println(cancelRes[0] == 0)
println(cancelRes[1] == 4)
println(cancelRes[4] == 16)


println("--- stream tests complete ---")
