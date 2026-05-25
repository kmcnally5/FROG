// mtl_pool_bench.lex — Phase F buffer-pool effect.
//
// Allocates + releases buffers of a fixed size in a tight loop.
// First pass: every alloc is a fresh `newBufferWithLength:` (cold pool).
// Subsequent passes: every alloc comes from the recycle pool.

import "stdlib/mtl.lex" as mtl

let info, infoErr = mtl.device()
if infoErr != null {
    println(format("Metal unavailable: %s", infoErr))
    return
}
println(format("Device: %s, max_bind_slots=%d",
    info["name"], info["max_bind_slots"]))

let count = 64         // 64 float32 = 256 B per buffer — small, alloc-bound
let iters = 20000

// Pre-build the source array once — its construction is not part
// of the benchmark.
let src = makeArray(count, 1.5)

// Pass 1 — cold pool. Every iteration hits newBufferWithBytes.
let t0 = _timeNanos()
let i = 0
while i < iters {
    let handle, err = mtl.buffer(src)
    if err != null {
        println(format("alloc failed: %s", err))
        return
    }
    mtl.releaseBuffer(handle)
    i = i + 1
}
let coldMs = (_timeNanos() - t0) / 1000000.0

// Pool is now warm (pool cap = 8 same-size buffers).

// Pass 2 — warm pool. Same loop, but every alloc comes from pool.
let t1 = _timeNanos()
i = 0
while i < iters {
    let handle, err = mtl.buffer(src)
    if err != null {
        println(format("alloc failed: %s", err))
        return
    }
    mtl.releaseBuffer(handle)
    i = i + 1
}
let warmMs = (_timeNanos() - t1) / 1000000.0

println("")
println(format("Phase F buffer-pool effect (%d alloc/release cycles, %d f32 each):", iters, count))
println(format("  cold pool: %.3f ms", coldMs))
println(format("  warm pool: %.3f ms", warmMs))
println(format("  speedup:   %.2fx", coldMs / warmMs))
