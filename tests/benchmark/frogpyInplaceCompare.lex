// frogpyInplaceCompare.lex — verify in-place op delivers the predicted
// throughput gain over the allocating variant.
//
// Methodology mirrors frogpyDiagnostic.lex (probe #1) but at a single
// representative size and with both code paths side-by-side. Each path
// runs enough iterations to amass ~200 ms total wall time. Reports
// ns/call and effective GB/s (kernel-work bytes = 3*N*8 for binary
// f64; for in-place add the "out" is one of the inputs, so the actual
// memory bytes are 2*N*8 read + 1*N*8 write = same 3*N*8 total even
// though only two distinct buffers exist).
//
// Expected (from earlier Go bench on M4):
//   allocating add  ~79 GB/s, 305 us/call at N=1M
//   inplace add    ~119 GB/s, 200 us/call at N=1M
// Gain target: ~50% speedup.

import "stdlib/datetime.lex" as dt
import "stdlib/tensor.lex" as t

fn benchAllocate(aT, bT, nElems) {
    // Use let to force locals — bare assignment in kLex walks the
    // scope chain and would clobber the caller's `i`.
    let _ = t.add(aT, bT)
    let _ = t.add(aT, bT)
    let _ = t.add(aT, bT)

    let t0 = dt.nowNanos()
    let _ = t.add(aT, bT)
    let t1 = dt.nowNanos()
    let oneCallNs = t1 - t0
    if oneCallNs < 1 {
        oneCallNs = 1
    }
    let target = 200000000
    let iters = target / oneCallNs
    if iters < 5 {
        iters = 5
    }

    let start = dt.nowNanos()
    let j = 0
    while j < iters {
        let _ = t.add(aT, bT)
        j = j + 1
    }
    let endT = dt.nowNanos()

    let totalNs = endT - start
    let nsPerCall = totalNs / iters
    let gbPerSec = float(iters * nElems * 24) / float(totalNs)
    println("  allocating   iters=" + str(iters) + "  ns/call=" + str(nsPerCall) + "  GB/s=" + str(gbPerSec))
    return gbPerSec
}

fn benchInplace(aT, bT, nElems) {
    let _ = t.add_inplace(aT, bT)
    let _ = t.add_inplace(aT, bT)
    let _ = t.add_inplace(aT, bT)

    let t0 = dt.nowNanos()
    let _ = t.add_inplace(aT, bT)
    let t1 = dt.nowNanos()
    let oneCallNs = t1 - t0
    if oneCallNs < 1 {
        oneCallNs = 1
    }
    let target = 200000000
    let iters = target / oneCallNs
    if iters < 5 {
        iters = 5
    }

    let start = dt.nowNanos()
    let j = 0
    while j < iters {
        let _ = t.add_inplace(aT, bT)
        j = j + 1
    }
    let endT = dt.nowNanos()

    let totalNs = endT - start
    let nsPerCall = totalNs / iters
    let gbPerSec = float(iters * nElems * 24) / float(totalNs)
    println("  in-place     iters=" + str(iters) + "  ns/call=" + str(nsPerCall) + "  GB/s=" + str(gbPerSec))
    return gbPerSec
}

println("=== FrogPy in-place vs allocating: throughput comparison ===")
println("Theoretical M4 sustained BW ~100 GB/s; pre-fix peak ~55 GB/s at 1M f64.")
println("")

let sizes = [1000, 10000, 100000, 1000000, 10000000]
let i = 0
while i < len(sizes) {
    let n = sizes[i]
    println("N=" + str(n))

    let arrA = makeArray(n, 1.5)
    let arrB = makeArray(n, 2.5)
    let aAlloc = t.from_array(arrA, "f64")
    let bAlloc = t.from_array(arrB, "f64")
    let aInplace = t.from_array(arrA, "f64")
    let bInplace = t.from_array(arrB, "f64")

    let gbAlloc = benchAllocate(aAlloc, bAlloc, n)
    let gbInplace = benchInplace(aInplace, bInplace, n)
    let speedup = gbInplace / gbAlloc
    println("  speedup      " + str(speedup) + "x")
    println("")

    i = i + 1
}
