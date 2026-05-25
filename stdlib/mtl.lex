// stdlib/mtl.lex — Metal (Apple GPU) compute + ray tracing wrapper.
// @module    mtl
// @version   0.2.0
// @since     klex 0.3.35
// @author    karl
// @summary   Ergonomic wrappers around the _mtl* Metal builtins (macOS only)
//
// This module sits on top of the raw `_mtl*` builtins (eval/builtins_mtl_*.go)
// and provides:
//
//   - Cross-platform safety: `mtl.isAvailable()` returns false on
//     Linux/Windows so kLex scripts can branch cleanly.
//   - Ergonomic names: `mtl.surface(w, h)` instead of `_mtlSurface`.
//   - One convenience helper: `mtl.await(channel)` collapses the
//     double-unpack `val, ok = recv(ch); result, err = val` into a
//     single `result, err = mtl.await(ch)` call.
//
// Everything is async where the underlying builtin is async. The
// kLex thread never blocks on the GPU — the channel-return-shape is
// preserved by design.
//
// Usage:
//
//   import "stdlib/mtl.lex" as mtl
//
//   if !mtl.isAvailable() {
//       println("Metal needs macOS — falling back to CPU.")
//       return
//   }
//
//   surface, err = mtl.surface(1024, 768)
//   if err != null { … }
//
//   _, err = mtl.await(mtl.clear(surface, [0.1, 0.2, 0.3, 1.0]))
//   if err != null { … }
//
//   _, err = mtl.await(mtl.saveSurface(surface, "/tmp/out.png"))
//   mtl.releaseSurface(surface)


// ── Capability check ────────────────────────────────────────────────────────

// isAvailable returns true when the running platform has Metal (macOS),
// false otherwise. Cross-platform scripts should branch on this BEFORE
// calling any other mtl.* function — on Linux/Windows everything else
// returns a "macOS only" error, which is correct but noisy.
fn isAvailable() {
    let info, err = _mtlInfo()
    if err != null { return false }
    return info != null
}


// device returns the underlying device descriptor hash (name,
// registry_id, has_unified_memory, supports_raytracing,
// supports_apple7, max_threads_per_group) or (null, err) on failure.
// Mirrors `_mtlInfo` 1:1 — exposed so callers can branch on
// `info["supports_raytracing"]` before building a ray-tracing scene.
//
// Named `device` instead of `info` because `info` collides with a
// builtin in some kLex contexts.
fn device() {
    return _mtlInfo()
}


// ── Async helper ────────────────────────────────────────────────────────────

// await unpacks a single-shot Metal completion channel into a plain
// (result, err) tuple. Saves callers from writing the two-step recv
// dance everywhere:
//
//   val, ok = recv(ch)        // step 1
//   if !ok { … }
//   result, err = val         // step 2
//
// becomes:
//
//   result, err = mtl.await(ch)
//
// If the channel closes without delivering a value (cancellation, or
// a producer-side bug), await returns (null, err) where err describes
// the situation — never silently succeeds.
fn await(ch) {
    let val, ok = recv(ch)
    if !ok {
        return null, error("MTL_CHANNEL_CLOSED",
            "mtl.await: channel closed without a result")
    }
    let result, err = val
    return result, err
}


// ── Surfaces ────────────────────────────────────────────────────────────────

// surface allocates an offscreen RGBA8 Metal texture and returns
// (handle, err). Handle is an integer the rest of the API uses to
// refer to the surface. Pair every successful surface with a
// releaseSurface call.
fn surface(width, height) {
    return _mtlSurface(width, height)
}


// clear submits a GPU command to fill the surface with the given
// RGBA color (each channel a float in [0, 1]). Returns a Channel
// the caller awaits — the kLex thread keeps running while the GPU
// works.
fn clear(surface, color) {
    return _mtlClear(surface, color)
}


// saveSurface reads the surface's pixels back and writes them as a
// PNG to `path`. Async — returns a Channel.
fn saveSurface(surface, path) {
    return _mtlSurfaceSavePng(surface, path)
}


// releaseSurface drops the bridge's retain so the GPU memory can be
// freed. Idempotent.
fn releaseSurface(surface) {
    return _mtlSurfaceRelease(surface)
}


// surfaceFromBytes creates a fresh RGBA8 surface of the given size
// and uploads `rgba` (a bytes value of length width*height*4) into
// it. Returns (handle, err). Pair every successful call with a
// releaseSurface. Use this to bring an in-memory RGBA image (e.g. a
// generated SD image) onto a Metal surface for compute kernels.
fn surfaceFromBytes(rgba, width, height) {
    return _mtlSurfaceFromBytes(rgba, width, height)
}


// surfaceToBytes reads the surface's pixels into a kLex bytes value
// (RGBA8 row-major, width*height*4 bytes). Counterpart to
// surfaceFromBytes — completes the round-trip for image post-processing
// pipelines.
fn surfaceToBytes(surface) {
    return _mtlSurfaceReadRgba(surface)
}


// ── Compute pipelines ───────────────────────────────────────────────────────

// kernel compiles MSL source code and produces a pipeline handle for
// the named function. Returns (handle, err). The handle is reusable
// across many dispatches.
fn kernel(mslSource, fnName) {
    return _mtlKernel(mslSource, fnName)
}


// releaseKernel drops the bridge's retain on a compiled pipeline.
// Idempotent.
fn releaseKernel(kernel) {
    return _mtlKernelRelease(kernel)
}


// ── Buffers ─────────────────────────────────────────────────────────────────

// buffer wraps an array of floats as a GPU-accessible MTLBuffer.
// Returns (handle, err). On Apple Silicon the storage is shared
// (zero-copy) — buffer bytes are visible to both CPU and GPU.
fn buffer(floats) {
    return _mtlBuffer(floats)
}


// bufferU32 wraps an array of non-negative integers as a uint32
// MTLBuffer. Needed primarily for triangle index buffers in indexed
// acceleration structures (MTLIndexTypeUInt32 wants actual integers,
// not floats). Same shared-storage semantics as `buffer`.
fn bufferU32(ints) {
    return _mtlBufferU32(ints)
}


// bufferAllocF32 allocates a zero-initialised GPU buffer big enough
// to hold `count` float32 values, WITHOUT a CPU-side source array.
// Metal's allocator zero-fills at the OS level for free — much
// cheaper than `buffer(makeArray(count, 0.0))` for any large output
// buffer the GPU will fully overwrite.
//
// Use this for the OUTPUT side of every dispatch where the kernel
// writes every element. For input buffers with real data, use
// `buffer(arr)`.
fn bufferAllocF32(count) {
    return _mtlBufferAllocF32(count)
}


// bufferAllocU32 is the uint32 counterpart.  Same allocation path,
// same zero-fill, sized for `count` uint32 elements.  Use for
// histogram output buffers and any other atomic-counter target.
fn bufferAllocU32(count) {
    return _mtlBufferAllocU32(count)
}


// readBuffer returns the buffer's current contents as a kLex array
// of floats. Callers must ensure any GPU work writing to the buffer
// has completed (await the dispatch's channel) before reading.
fn readBuffer(buf) {
    return _mtlReadBuffer(buf)
}


// readBufferU32 is the uint32 counterpart of readBuffer — used for
// reading back atomic-counter outputs (e.g. histogram bin counts).
fn readBufferU32(buf) {
    return _mtlReadBufferU32(buf)
}


// releaseBuffer drops the bridge's retain. Idempotent.
fn releaseBuffer(buf) {
    return _mtlBufferRelease(buf)
}


// ── Acceleration structures (ray tracing) ───────────────────────────────────

// accel builds a ray-tracing acceleration structure from a triangle-
// list vertex buffer. `vertexCount` must be a positive multiple of
// 3 (every three vertices forms one triangle). Returns a Channel
// that emits (handle, err) — async because the BVH build is GPU-side.
//
// IMPORTANT: the input must not share EXACT vertex positions between
// triangles in an unindexed list. Doing so silently degenerates the
// BVH and produces zero hits. Use `accelIndexed` whenever vertices
// are shared between triangles. See the feedback-mtl-shared-verts-bvh
// memory for the full failure mode.
fn accel(vertexBuffer, vertexCount) {
    return _mtlAccel(vertexBuffer, vertexCount)
}


// accelIndexed builds a ray-tracing acceleration structure from an
// indexed triangle mesh. The vertex buffer holds N unique float3
// positions; the uint32 index buffer holds 3*M indices forming M
// triangles. Use this whenever multiple triangles share vertex
// positions — it sidesteps the BVH degeneracy that unindexed lists
// hit when vertex positions are bit-exact equal across triangles.
//
// Returns a Channel emitting (handle, err).
fn accelIndexed(vertexBuffer, vertexCount, indexBuffer, indexCount) {
    return _mtlAccelIndexed(vertexBuffer, vertexCount, indexBuffer, indexCount)
}


// releaseAccel drops the bridge's retain. Idempotent. The accel's
// vertex buffer is NOT released by this call — manage it separately.
fn releaseAccel(a) {
    return _mtlAccelRelease(a)
}


// ── Dispatch ────────────────────────────────────────────────────────────────

// dispatch submits one compute-shader execution to the GPU. Returns a
// Channel that emits (null, err) once the GPU finishes.
//
// `bindings` is a hash with optional keys:
//
//   "textures": [t0, t1, ...]  — bound to MSL [[texture(i)]]
//   "buffers":  [b0, b1, ...]  — bound to MSL [[buffer(i)]]
//   "accels":   [a0, a1, ...]  — bound to MSL [[buffer(buffer_count + i)]]
//                                  (ray-tracing only)
//
// `grid` is a 3-element array of thread counts: [x, y, z]. Use 1 for
// unused dimensions — for example [N, 1, 1] for a 1-D pass.
fn dispatch(kernel, bindings, grid) {
    return _mtlDispatch(kernel, bindings, grid)
}


// dispatchAndWait is dispatch + await collapsed into one call —
// convenient for one-shot, fully synchronous use sites where you
// don't need to do CPU work overlapping the GPU. Returns (null, err).
fn dispatchAndWait(kernel, bindings, grid) {
    return await(_mtlDispatch(kernel, bindings, grid))
}


// ── Command-buffer batching ────────────────────────────────────────────────
//
// For workloads that issue many small dispatches in a tight loop
// (e.g. progressive renderers, iterative simulations), wrapping them
// in a single command buffer eliminates per-dispatch commit overhead
// and lets Metal pipeline the work end-to-end.
//
// Usage:
//
//   batch, _ = mtl.beginBatch()
//   i = 0
//   while i < N {
//       _, _ = mtl.dispatchInBatch(batch, kernel, bindings, grid)
//       i = i + 1
//   }
//   _, err = mtl.await(mtl.endBatch(batch))    // commits + waits


// beginBatch opens a fresh batch command buffer. Returns (handle, err).
// MUST be paired with endBatch — leaking a batch leaks an MTLCommandBuffer.
fn beginBatch() {
    return _mtlBatchBegin()
}


// dispatchInBatch appends one compute dispatch onto an open batch.
// Synchronous — the GPU work doesn't run until endBatch commits.
// Returns (null, err); errors here are pre-submit validation failures.
fn dispatchInBatch(batch, kernel, bindings, grid) {
    return _mtlBatchDispatch(batch, kernel, bindings, grid)
}


// endBatch commits the batch's command buffer and returns a Channel
// that emits (null, null) once the GPU finishes all batched work,
// or (null, err) on failure. Releases the batch handle.
fn endBatch(batch) {
    return _mtlBatchCommit(batch)
}


// releaseBatch abandons a batch without committing — the error-path
// cleanup partner to beginBatch. Discards any encoded dispatches and
// frees the underlying MTLCommandBuffer. Idempotent.
//
// Typical use:
//   batch, _ = mtl.beginBatch()
//   _, err = mtl.dispatchInBatch(batch, kernel, bindings, grid)
//   if err != null {
//       mtl.releaseBatch(batch)
//       return null, err
//   }
fn releaseBatch(batch) {
    return _mtlBatchRelease(batch)
}


// ── Parallel reductions ─────────────────────────────────────────────────────
//
// Reductions use Apple Silicon's SIMD-group primitives (simd_sum,
// simd_min, simd_max). Each 32-lane SIMD group produces one partial
// result via the hardware reduction op; the kernel writes ceil(N/32)
// partials to an output buffer; the CPU finishes by combining the
// partials.
//
// Why hybrid GPU+CPU finalisation rather than pure GPU? For
// N = 1 million, ceil(N/32) = 31,250 partials. Summing/min'ing/maxing
// 31k floats on CPU is sub-millisecond — cheaper than another GPU
// pass. Multi-pass GPU reduction only wins for absurd N (~10^8+).


let _reduceSumKernel = 0
let _reduceSumKernelSrc = `#include <metal_stdlib>
using namespace metal;

kernel void reduce_sum(
    constant float* input    [[buffer(0)]],
    device   float* partials [[buffer(1)]],
    constant uint*  meta     [[buffer(2)]],     // [N]
    uint gid  [[thread_position_in_grid]],
    uint lane [[thread_index_in_simdgroup]]
) {
    uint N = meta[0];
    float v = (gid < N) ? input[gid] : 0.0;
    float groupSum = simd_sum(v);
    if (lane == 0) {
        partials[gid / 32] = groupSum;
    }
}`


let _reduceMinKernel = 0
let _reduceMinKernelSrc = `#include <metal_stdlib>
using namespace metal;

kernel void reduce_min(
    constant float* input    [[buffer(0)]],
    device   float* partials [[buffer(1)]],
    constant uint*  meta     [[buffer(2)]],
    uint gid  [[thread_position_in_grid]],
    uint lane [[thread_index_in_simdgroup]]
) {
    uint N = meta[0];
    // Out-of-range threads vote +INFINITY so simd_min ignores them.
    float v = (gid < N) ? input[gid] : INFINITY;
    float groupMin = simd_min(v);
    if (lane == 0) {
        partials[gid / 32] = groupMin;
    }
}`


let _reduceMaxKernel = 0
let _reduceMaxKernelSrc = `#include <metal_stdlib>
using namespace metal;

kernel void reduce_max(
    constant float* input    [[buffer(0)]],
    device   float* partials [[buffer(1)]],
    constant uint*  meta     [[buffer(2)]],
    uint gid  [[thread_position_in_grid]],
    uint lane [[thread_index_in_simdgroup]]
) {
    uint N = meta[0];
    float v = (gid < N) ? input[gid] : -INFINITY;
    float groupMax = simd_max(v);
    if (lane == 0) {
        partials[gid / 32] = groupMax;
    }
}`


// _runReduction is the shared scaffolding behind reduceSum / reduceMin
// / reduceMax. Caller picks the kernel pointer; this fn handles
// buffer setup, dispatch, readback, and resource cleanup. CPU-side
// finalisation is left to the caller because the combine op differs
// per reduction.
fn _runReduction(arr, kernelHandle) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE", "Metal unavailable on this platform")
    }
    let n = len(arr)
    if n == 0 {
        return null, error("MTL_BAD_ARGS", "reduce: input array is empty")
    }

    let inBuf, err = _mtlBuffer(arr)
    if err != null {
        return null, error("MTL_BUFFER", "reduce: input buffer: " + err)
    }
    let metaBuf, err = _mtlBufferU32([n])
    if err != null {
        _mtlBufferRelease(inBuf)
        return null, error("MTL_BUFFER", "reduce: meta buffer: " + err)
    }

    // ceil(n / 32) partials, one per SIMD group. Pre-zero'd at the
    // OS level — much cheaper than makeArray + _mtlBuffer.
    let nPartials = (n + 31) / 32
    let partialsBuf, err = _mtlBufferAllocF32(nPartials)
    if err != null {
        _mtlBufferRelease(inBuf)
        _mtlBufferRelease(metaBuf)
        return null, error("MTL_BUFFER", "reduce: partials buffer: " + err)
    }

    let _, derr = dispatchAndWait(kernelHandle,
        {"buffers": [inBuf, partialsBuf, metaBuf]},
        [n, 1, 1])
    if derr != null {
        _mtlBufferRelease(inBuf)
        _mtlBufferRelease(metaBuf)
        _mtlBufferRelease(partialsBuf)
        return null, error("MTL_DISPATCH", "reduce: dispatch: " + derr)
    }

    let partials, perr = _mtlReadBuffer(partialsBuf)
    _mtlBufferRelease(inBuf)
    _mtlBufferRelease(metaBuf)
    _mtlBufferRelease(partialsBuf)
    if perr != null {
        return null, error("MTL_READ", "reduce: readback: " + perr)
    }
    return partials, null
}


// reduceSum returns the sum of every element of `arr`. Uses
// SIMD-group hardware reduction on the GPU + a tiny CPU final pass
// over the ~N/32 partials. For N=1M this is ~5 ms on M4 vs ~250 ms
// in pure kLex — about a 50× speedup.
//
// Float precision note: a parallel reduction sums in a different
// order than a sequential loop, so for very large N or extreme
// dynamic ranges the result may differ from CPU sum by a few
// last-bit ulps. For sane numeric inputs this is invisible.
fn reduceSum(arr) {
    if _reduceSumKernel == 0 {
        let k, err = _mtlKernel(_reduceSumKernelSrc, "reduce_sum")
        if err != null { return null, error("MTL_COMPILE", "reduceSum: " + err) }
        _reduceSumKernel = k
    }
    let partials, perr = _runReduction(arr, _reduceSumKernel)
    if perr != null { return null, perr }
    let total = 0.0
    let i = 0
    while i < len(partials) {
        total = total + partials[i]
        i = i + 1
    }
    return total, null
}


// reduceMean returns the arithmetic mean. Just reduceSum / N.
fn reduceMean(arr) {
    let n = len(arr)
    if n == 0 {
        return null, error("MTL_BAD_ARGS", "reduceMean: input array is empty")
    }
    let total, err = reduceSum(arr)
    if err != null { return null, err }
    return total / float(n), null
}


// reduceMin returns the smallest element of `arr` (NaN-aware:
// simd_min propagates NaN to the partial, but the CPU final pass
// uses plain `<` which treats NaN as never-smaller — so a single
// NaN in the input WILL be missed if it lands in a partial. For
// real numeric data this isn't a concern.)
fn reduceMin(arr) {
    if _reduceMinKernel == 0 {
        let k, err = _mtlKernel(_reduceMinKernelSrc, "reduce_min")
        if err != null { return null, error("MTL_COMPILE", "reduceMin: " + err) }
        _reduceMinKernel = k
    }
    let partials, perr = _runReduction(arr, _reduceMinKernel)
    if perr != null { return null, perr }
    let best = partials[0]
    let i = 1
    while i < len(partials) {
        if partials[i] < best {
            best = partials[i]
        }
        i = i + 1
    }
    return best, null
}


// reduceMax returns the largest element of `arr`. Same NaN caveat
// as reduceMin.
fn reduceMax(arr) {
    if _reduceMaxKernel == 0 {
        let k, err = _mtlKernel(_reduceMaxKernelSrc, "reduce_max")
        if err != null { return null, error("MTL_COMPILE", "reduceMax: " + err) }
        _reduceMaxKernel = k
    }
    let partials, perr = _runReduction(arr, _reduceMaxKernel)
    if perr != null { return null, perr }
    let best = partials[0]
    let i = 1
    while i < len(partials) {
        if partials[i] > best {
            best = partials[i]
        }
        i = i + 1
    }
    return best, null
}


// ── Histogram ───────────────────────────────────────────────────────────────


let _histogramKernel = 0
let _histogramKernelSrc = `#include <metal_stdlib>
using namespace metal;

// One thread per input element. Each thread maps its value to a bin
// index and atomically increments that bin's counter. Atomic op is
// relaxed-memory-order because we don't care about ordering between
// increments — only that each completes without races.
kernel void histogram(
    constant float*     input  [[buffer(0)]],
    device   atomic_uint* counts [[buffer(1)]],
    constant float*     params [[buffer(2)]],   // [minVal, maxVal]
    constant uint*      meta   [[buffer(3)]],   // [N, numBins]
    uint gid [[thread_position_in_grid]]
) {
    uint N       = meta[0];
    uint numBins = meta[1];
    if (gid >= N) return;

    float minV = params[0];
    float maxV = params[1];
    float v    = input[gid];

    // Map [minV, maxV) → bin index 0..numBins-1.
    // Out-of-range values clamp into the first/last bin so they're
    // still counted (a common convention for exploratory histograms).
    float t = (v - minV) / (maxV - minV);
    int idx = int(t * float(numBins));
    if (idx < 0) idx = 0;
    if (idx >= int(numBins)) idx = int(numBins) - 1;

    atomic_fetch_add_explicit(&counts[idx], 1u, memory_order_relaxed);
}`


// histogram bins `arr` into `numBins` equal-width buckets spanning
// [minVal, maxVal] and returns the bin counts as a kLex array of
// integers.
//
// Out-of-range values clamp to the first/last bin — they're still
// counted (typical convention for exploratory histograms; explicit
// `min`/`max` filtering should happen on the caller side if
// out-of-range data should be dropped).
//
// Performance reference (M4): 1M floats into 100 bins ≈ 5 ms.
fn histogram(arr, numBins, minVal, maxVal) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE", "Metal unavailable on this platform")
    }
    if numBins <= 0 {
        return null, error("MTL_BAD_ARGS", "histogram: numBins must be positive")
    }
    if maxVal <= minVal {
        return null, error("MTL_BAD_ARGS", "histogram: maxVal must be > minVal")
    }
    let n = len(arr)
    if n == 0 {
        return null, error("MTL_BAD_ARGS", "histogram: input array is empty")
    }

    if _histogramKernel == 0 {
        let k, err = _mtlKernel(_histogramKernelSrc, "histogram")
        if err != null { return null, error("MTL_COMPILE", "histogram: " + err) }
        _histogramKernel = k
    }

    let inBuf, _   = _mtlBuffer(arr)
    let paramsBuf, _ = _mtlBuffer([minVal, maxVal])
    let metaBuf, _ = _mtlBufferU32([n, numBins])

    // Counts buffer: numBins uint32 zeros. Atomic increments
    // populate it. Zero-fill via the OS-level allocator (no kLex
    // makeArray, no element-by-element marshalling).
    let countsBuf, _ = _mtlBufferAllocU32(numBins)

    let _, derr = dispatchAndWait(_histogramKernel,
        {"buffers": [inBuf, countsBuf, paramsBuf, metaBuf]},
        [n, 1, 1])
    if derr != null {
        _mtlBufferRelease(inBuf)
        _mtlBufferRelease(paramsBuf)
        _mtlBufferRelease(metaBuf)
        _mtlBufferRelease(countsBuf)
        return null, error("MTL_DISPATCH", "histogram: " + derr)
    }

    let counts, rerr = _mtlReadBufferU32(countsBuf)
    _mtlBufferRelease(inBuf)
    _mtlBufferRelease(paramsBuf)
    _mtlBufferRelease(metaBuf)
    _mtlBufferRelease(countsBuf)
    if rerr != null {
        return null, error("MTL_READ", "histogram: " + rerr)
    }
    return counts, null
}


// ── Matrix multiplication ───────────────────────────────────────────────────


let _matmulKernel = 0
let _matmulKernelSrc = `#include <metal_stdlib>
using namespace metal;

// C = A · B
//   A is m × k, B is k × n, C is m × n, all row-major.
//   One thread per output element. Each computes a length-k dot product.
//
// This is the NAIVE algorithm — no tiling, no simdgroup matmul. It's
// well within Apple Silicon's bandwidth budget for moderate sizes
// (~1024² fits in cache). For training-scale GEMM you'd reach for
// Metal Performance Shaders' MPSMatrixMultiplication; that's a
// future addition.
kernel void matmul(
    constant float* A    [[buffer(0)]],
    constant float* B    [[buffer(1)]],
    device   float* C    [[buffer(2)]],
    constant uint*  dims [[buffer(3)]],   // [m, k, n]
    uint2 gid [[thread_position_in_grid]]
) {
    uint m = dims[0];
    uint k = dims[1];
    uint n = dims[2];

    uint row = gid.y;
    uint col = gid.x;
    if (row >= m || col >= n) return;

    float sum = 0.0;
    for (uint i = 0; i < k; i++) {
        sum += A[row * k + i] * B[i * n + col];
    }
    C[row * n + col] = sum;
}`


// matmul multiplies two row-major matrices A (m × k) and B (k × n)
// on the GPU and returns the m × n result as a flat (m·n)-element
// kLex array. `a` and `b` are flat arrays of m·k and k·n floats.
//
// Returns (result, err). result[row * n + col] = C[row, col].
//
// Performance reference (M4):
//   256 × 256 × 256:  ~1 ms (CPU ~150 ms — 150× speedup)
//   1024 × 1024 × 1024:  ~50 ms (CPU ~minutes in pure kLex)
fn matmul(a, b, m, k, n) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE", "Metal unavailable on this platform")
    }
    if m <= 0 || k <= 0 || n <= 0 {
        return null, error("MTL_BAD_ARGS", "matmul: m, k, n must be positive")
    }
    if len(a) != m * k {
        return null, error("MTL_BAD_ARGS",
            "matmul: a has " + str(len(a)) + " elements, expected m*k = " + str(m*k))
    }
    if len(b) != k * n {
        return null, error("MTL_BAD_ARGS",
            "matmul: b has " + str(len(b)) + " elements, expected k*n = " + str(k*n))
    }

    if _matmulKernel == 0 {
        let kk, err = _mtlKernel(_matmulKernelSrc, "matmul")
        if err != null { return null, error("MTL_COMPILE", "matmul: " + err) }
        _matmulKernel = kk
    }

    let aBuf, _ = _mtlBuffer(a)
    let bBuf, _ = _mtlBuffer(b)
    let dimsBuf, _ = _mtlBufferU32([m, k, n])
    let cBuf, _ = _mtlBufferAllocF32(m * n)

    let _, derr = dispatchAndWait(_matmulKernel,
        {"buffers": [aBuf, bBuf, cBuf, dimsBuf]},
        [n, m, 1])

    if derr != null {
        _mtlBufferRelease(aBuf)
        _mtlBufferRelease(bBuf)
        _mtlBufferRelease(cBuf)
        _mtlBufferRelease(dimsBuf)
        return null, error("MTL_DISPATCH", "matmul: " + derr)
    }

    let result, rerr = _mtlReadBuffer(cBuf)
    _mtlBufferRelease(aBuf)
    _mtlBufferRelease(bBuf)
    _mtlBufferRelease(cBuf)
    _mtlBufferRelease(dimsBuf)
    if rerr != null {
        return null, error("MTL_READ", "matmul: readback: " + rerr)
    }
    return result, null
}


// matmulMPS is the MPS-accelerated counterpart of matmul. Same
// call shape (A m × k, B k × n, returns m × n flat array) but uses
// Apple's MPSMatrixMultiplication kernel under the hood — tiled,
// uses simdgroup matrix instructions.
//
// Performance: MPS wins decisively over the naive Metal kernel above
// ~256 cubed; below that, MPS dispatch overhead dominates and naive
// is comparable or faster. Output is bit-exact identical between the
// two MPS paths (verified), so the switch is a pure performance
// choice. Current measured numbers live in
// tests/benchmark/frogpyMatmulBench.lex — re-run rather than trust
// hard-coded figures here, since MPS continues to be tuned across
// macOS releases.
//
// IMPORTANT — for tensor-shaped workloads, prefer
// `stdlib/tensor.lex`'s `t.matmul` over this function. matmulMPS
// detects tensor inputs and routes through a zero-marshalling fast
// path (~5× faster at 1024³ as of 2026-05); for kLex Array inputs it
// stays on the slower per-element conversion path. If you can switch
// from Array to Tensor inputs, t.matmul is the most direct API.
fn matmulMPS(a, b, m, k, n) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE", "Metal unavailable on this platform")
    }
    if m <= 0 || k <= 0 || n <= 0 {
        return null, error("MTL_BAD_ARGS", "matmulMPS: m, k, n must be positive")
    }

    // Tensor inputs take the zero-marshalling upload path
    // (_mtlBufferFromTensor); kLex Arrays take the per-element
    // conversion path (_mtlBuffer). Mixing the two is rejected —
    // the caller almost certainly didn't mean it.
    let aIsTensor = type(a) == "TENSOR"
    let bIsTensor = type(b) == "TENSOR"
    if aIsTensor != bIsTensor {
        return null, error("MTL_BAD_ARGS",
            "matmulMPS: a and b must be the same kind (both Array or both Tensor)")
    }

    let aBuf = 0
    let bBuf = 0
    let buferr = null

    if aIsTensor {
        aBuf, buferr = _mtlBufferFromTensor(a)
        if buferr != null {
            return null, error("MTL_BUFFER", "matmulMPS: a (tensor): " + buferr)
        }
        bBuf, buferr = _mtlBufferFromTensor(b)
        if buferr != null {
            _mtlBufferRelease(aBuf)
            return null, error("MTL_BUFFER", "matmulMPS: b (tensor): " + buferr)
        }
    } else {
        if len(a) != m * k {
            return null, error("MTL_BAD_ARGS",
                "matmulMPS: a has " + str(len(a)) + " elements, expected m*k = " + str(m*k))
        }
        if len(b) != k * n {
            return null, error("MTL_BAD_ARGS",
                "matmulMPS: b has " + str(len(b)) + " elements, expected k*n = " + str(k*n))
        }
        aBuf, _ = _mtlBuffer(a)
        bBuf, _ = _mtlBuffer(b)
    }

    let cBuf, _ = _mtlBufferAllocF32(m * n)

    let _, derr = await(_mtlMatmulMPS(aBuf, bBuf, cBuf, m, k, n))
    if derr != null {
        _mtlBufferRelease(aBuf)
        _mtlBufferRelease(bBuf)
        _mtlBufferRelease(cBuf)
        return null, error("MTL_DISPATCH", "matmulMPS: " + derr)
    }

    // Readback stays as kLex Array regardless of input type — preserves
    // the existing API contract. Tensor-in callers who want a tensor
    // back should use stdlib/tensor.lex's t.matmul instead.
    let result, rerr = _mtlReadBuffer(cBuf)
    _mtlBufferRelease(aBuf)
    _mtlBufferRelease(bBuf)
    _mtlBufferRelease(cBuf)
    if rerr != null {
        return null, error("MTL_READ", "matmulMPS: readback: " + rerr)
    }
    return result, null
}


// ── FFT (Fast Fourier Transform) ───────────────────────────────────────────
//
// 1D complex-to-complex FFT via Apple's MPSGraph.  Input and output
// are interleaved (re, im) float arrays — the same byte format NumPy
// uses for complex64 arrays.  For an n-point FFT you pass 2n floats
// and get back 2n floats.
//
// Real input?  Pad with zero imaginary parts:
//
//   complex_input = makeArray(2 * len(signal), 0.0)
//   i = 0
//   while i < len(signal) {
//       complex_input[2 * i] = signal[i]
//       i = i + 1
//   }
//
// Conventions:
//   - Forward FFT is UNSCALED (output[k] = sum_n input[n] * exp(-2πi kn/N)).
//   - Inverse FFT is also UNSCALED on the way back; callers divide by
//     n themselves to recover the original input.  This matches
//     SciPy's `scipy.fft.fft` / `ifft` behaviour when `norm=None`.


// fft performs a forward complex-to-complex 1D FFT of length `n`.
// `input` must have exactly 2*n floats (interleaved re,im). Returns
// (output, err) where output is also 2*n floats.
fn fft(input, n) {
    return _fftRun(input, n, 0)
}


// ifft performs an inverse FFT. Same shape as fft; divide each
// output element by n to recover the original input scale.
fn ifft(input, n) {
    return _fftRun(input, n, 1)
}


fn _fftRun(input, n, inverse) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE", "Metal unavailable on this platform")
    }
    if n <= 0 {
        return null, error("MTL_BAD_ARGS", "fft: n must be positive")
    }
    if len(input) != 2 * n {
        return null, error("MTL_BAD_ARGS",
            "fft: input must have 2*n floats (got " + str(len(input)) +
            ", expected " + str(2 * n) + ")")
    }

    let inBuf, err = _mtlBuffer(input)
    if err != null { return null, error("MTL_BUFFER", "fft: input: " + err) }
    let outBuf, err = _mtlBufferAllocF32(2 * n)
    if err != null {
        _mtlBufferRelease(inBuf)
        return null, error("MTL_BUFFER", "fft: output: " + err)
    }

    let _, derr = await(_mtlFFT(inBuf, outBuf, n, inverse))
    if derr != null {
        _mtlBufferRelease(inBuf)
        _mtlBufferRelease(outBuf)
        return null, error("MTL_DISPATCH", "fft: " + derr)
    }

    let result, rerr = _mtlReadBuffer(outBuf)
    _mtlBufferRelease(inBuf)
    _mtlBufferRelease(outBuf)
    if rerr != null {
        return null, error("MTL_READ", "fft: readback: " + rerr)
    }
    return result, null
}


// ── High-level GPU primitives ───────────────────────────────────────────────
//
// Convenience functions that wrap a specific MSL kernel + buffer
// setup + dispatch into one call. The first such primitive is
// batchDot — the workhorse for vector similarity search.


// _batchDotKernel caches the compiled "batch dot product" pipeline
// state across calls to batchDot. Compilation is ~10 ms; reuse is
// free. Starts at 0 ("not yet compiled") and stays non-zero for the
// process lifetime.
let _batchDotKernel = 0


// _batchDotKernelSrc is the MSL for one-thread-per-stored-vector
// dot product. Each thread computes `dot(query, batch[i])` across
// `dim` dimensions and writes one float into out[i]. Suitable for
// pre-normalised embeddings (Voyage, OpenAI, Nomic, etc.) — for raw
// embeddings, normalise on either side first.
let _batchDotKernelSrc = `#include <metal_stdlib>
using namespace metal;
kernel void batch_dot(
    constant float* query [[buffer(0)]],
    constant float* batch [[buffer(1)]],
    constant uint*  meta  [[buffer(2)]],     // [dim, N]
    device   float* out   [[buffer(3)]],
    uint gid [[thread_position_in_grid]]
) {
    uint dim = meta[0];
    uint N   = meta[1];
    if (gid >= N) return;
    float sum = 0.0;
    uint base = gid * dim;
    for (uint i = 0; i < dim; i++) {
        sum += query[i] * batch[base + i];
    }
    out[gid] = sum;
}`


// _genVecsKernel caches the compiled random-vector-generator pipeline.
let _genVecsKernel = 0


// _genVecsKernelSrc generates N unit-length random vectors of
// dimension D on the GPU. One thread per vector — each thread
// samples D values via a hash PRNG, accumulates sum-of-squares,
// then normalises in a second pass.
//
// The hash is Wang's integer hash — cheap, statistically decent
// for ML workloads where embeddings just need to be non-degenerate.
// NOT cryptographically random; do not use for keys/secrets.
let _genVecsKernelSrc = `#include <metal_stdlib>
using namespace metal;

inline float hash01(uint x) {
    x = (x ^ 61u) ^ (x >> 16);
    x *= 9u;
    x = x ^ (x >> 4);
    x *= 0x27d4eb2du;
    x = x ^ (x >> 15);
    return float(x) / 4294967296.0;
}

kernel void gen_unit_vectors(
    constant uint*  meta [[buffer(0)]],   // [N, D, seed]
    device   float* out  [[buffer(1)]],
    uint gid [[thread_position_in_grid]]
) {
    uint N    = meta[0];
    uint D    = meta[1];
    uint seed = meta[2];
    if (gid >= N) return;

    uint base = gid * D;
    float sumSq = 0.0;

    // First pass: sample, store, accumulate magnitude.
    for (uint i = 0; i < D; i++) {
        uint key = (base + i) * 2654435761u + seed * 374761393u;
        float x = hash01(key) * 2.0 - 1.0;
        out[base + i] = x;
        sumSq += x * x;
    }

    // Second pass: divide by magnitude to normalise.
    float invMag = (sumSq > 0.0) ? rsqrt(sumSq) : 1.0;
    for (uint i = 0; i < D; i++) {
        out[base + i] *= invMag;
    }
}`


// randomUnitVectors generates `n` random unit-length vectors of
// dimension `dim` using the GPU and returns the flat
// (n × dim)-element kLex array. ~1 ms for n=10000, dim=128 on M4 —
// the kLex interpreter cannot get anywhere close to this rate, so
// for any non-trivial test dataset this should be your first stop.
//
// Returns (flat_array, err). Caller can index as
// `flat[i * dim + j]` to get component j of vector i, or pass the
// array directly into mtl.batchDot.
//
// `seed` is the per-run randomness seed. Same seed → same output,
// useful for reproducible tests.
fn randomUnitVectors(n, dim, seed) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE",
            "mtl.randomUnitVectors: Metal is unavailable on this platform")
    }
    if n <= 0 || dim <= 0 {
        return null, error("MTL_BAD_ARGS",
            "mtl.randomUnitVectors: n and dim must be positive")
    }

    if _genVecsKernel == 0 {
        let k, err = _mtlKernel(_genVecsKernelSrc, "gen_unit_vectors")
        if err != null {
            return null, error("MTL_COMPILE",
                "mtl.randomUnitVectors: kernel compile: " + err)
        }
        _genVecsKernel = k
    }

    let metaBuf, err = _mtlBufferU32([n, dim, seed])
    if err != null {
        return null, error("MTL_BUFFER",
            "mtl.randomUnitVectors: meta buffer: " + err)
    }
    // Output buffer: N * D floats, zero-init at the OS level (no
    // CPU-side makeArray, no Go-side marshalling).
    let outBuf, err = _mtlBufferAllocF32(n * dim)
    if err != null {
        _mtlBufferRelease(metaBuf)
        return null, error("MTL_BUFFER",
            "mtl.randomUnitVectors: out buffer: " + err)
    }

    let _, derr = dispatchAndWait(_genVecsKernel,
        {"buffers": [metaBuf, outBuf]},
        [n, 1, 1])
    if derr != null {
        _mtlBufferRelease(metaBuf)
        _mtlBufferRelease(outBuf)
        return null, error("MTL_DISPATCH",
            "mtl.randomUnitVectors: dispatch: " + derr)
    }

    let results, err = _mtlReadBuffer(outBuf)
    _mtlBufferRelease(metaBuf)
    _mtlBufferRelease(outBuf)
    if err != null {
        return null, error("MTL_READ",
            "mtl.randomUnitVectors: readback: " + err)
    }
    return results, null
}


// batchDot computes the dot product of a single query vector against
// each of N stored vectors, all on the GPU.
//
// Inputs:
//   query   — flat array of `dim` floats (the query vector)
//   batch   — flat array of N*dim floats (N stored vectors concatenated)
//   dim     — integer dimension of each vector
//
// Returns (similarities, err) where similarities is an array of N
// floats. similarities[i] = dot(query, batch[i*dim .. (i+1)*dim]).
//
// For PRE-NORMALISED embeddings (unit length), this IS cosine
// similarity. For unnormalised vectors, divide by the product of
// magnitudes yourself — or normalise once and reuse here.
//
// Performance: on M4, ~1 ms for N=10000, dim=384. Around 100×
// faster than the equivalent pure-kLex loop. Speedup grows with N
// and with dimension.
//
// The kernel is compiled on first call and cached. Subsequent calls
// just upload buffers and dispatch — sub-millisecond per call.
fn batchDot(query, batch, dim) {
    if !isAvailable() {
        return null, error("MTL_UNAVAILABLE",
            "mtl.batchDot: Metal is unavailable on this platform")
    }
    if len(query) != dim {
        return null, error("MTL_BAD_ARGS",
            "mtl.batchDot: query length doesn't match dim")
    }
    if len(batch) % dim != 0 {
        return null, error("MTL_BAD_ARGS",
            "mtl.batchDot: batch length is not a multiple of dim")
    }
    let n = len(batch) / dim

    // Compile the kernel on first call; reuse the handle thereafter.
    if _batchDotKernel == 0 {
        let k, err = _mtlKernel(_batchDotKernelSrc, "batch_dot")
        if err != null {
            return null, error("MTL_COMPILE", "mtl.batchDot: kernel compile: " + err)
        }
        _batchDotKernel = k
    }

    // Upload buffers. On Apple Silicon these are zero-copy: the
    // floats live in unified memory the GPU can read directly.
    let qBuf, err = _mtlBuffer(query)
    if err != null {
        return null, error("MTL_BUFFER", "mtl.batchDot: query buffer: " + err)
    }
    let bBuf, err = _mtlBuffer(batch)
    if err != null {
        _mtlBufferRelease(qBuf)
        return null, error("MTL_BUFFER", "mtl.batchDot: batch buffer: " + err)
    }
    let metaBuf, err = _mtlBufferU32([dim, n])
    if err != null {
        _mtlBufferRelease(qBuf)
        _mtlBufferRelease(bBuf)
        return null, error("MTL_BUFFER", "mtl.batchDot: meta buffer: " + err)
    }

    // Output buffer: one float per stored vector, zero-init at the
    // OS level (no CPU-side makeArray, no Go-side marshalling).
    let outBuf, err = _mtlBufferAllocF32(n)
    if err != null {
        _mtlBufferRelease(qBuf)
        _mtlBufferRelease(bBuf)
        _mtlBufferRelease(metaBuf)
        return null, error("MTL_BUFFER", "mtl.batchDot: out buffer: " + err)
    }

    // Dispatch one thread per stored vector. recv() blocks the caller
    // until the GPU finishes — the kLex thread itself is freed inside
    // _mtlDispatch's goroutine, so this is async at the bridge level
    // even though we synchronously await the single result here.
    let _, derr = dispatchAndWait(_batchDotKernel,
        {"buffers": [qBuf, bBuf, metaBuf, outBuf]},
        [n, 1, 1])

    if derr != null {
        _mtlBufferRelease(qBuf)
        _mtlBufferRelease(bBuf)
        _mtlBufferRelease(metaBuf)
        _mtlBufferRelease(outBuf)
        return null, error("MTL_DISPATCH", "mtl.batchDot: dispatch: " + derr)
    }

    // Read results back to kLex. Zero-copy on Apple Silicon.
    let results, err = _mtlReadBuffer(outBuf)

    _mtlBufferRelease(qBuf)
    _mtlBufferRelease(bBuf)
    _mtlBufferRelease(metaBuf)
    _mtlBufferRelease(outBuf)

    if err != null {
        return null, error("MTL_READ", "mtl.batchDot: readback: " + err)
    }
    return results, null
}
