// mtl_bridge_darwin.h — C interface between the kLex Go runtime and the
// Apple Metal framework. The full Metal API is Objective-C; this header
// exposes only the plain-C functions that cgo can speak directly, with
// every Objective-C object hidden behind opaque handles.
//
// Why this layer exists:
//
//   cgo cannot import Objective-C types (`id<MTLDevice>` etc.) or call
//   `[obj method]` syntax. So we wrap every interaction we need in a
//   plain C function defined in mtl_bridge_darwin.m. The .m file does
//   the Objective-C dance; the .go file sees only C functions and POD
//   structs.
//
// Handle lifetime:
//
//   Functions returning a handle (`int64_t`) own a retained reference
//   to the underlying ObjC object on the Obj-C side. The caller MUST
//   pair every creator with the matching `mtl_release_*` call when
//   done, OR with `mtl_release_all_handles()` at shutdown. We use
//   integer handles rather than `unsafe.Pointer` for two reasons:
//
//     1. Go GC can't mistake an int for a pointer and free it.
//     2. The Obj-C side maintains its own retain table, so we never
//        hand a raw pointer to Go that could outlive the autorelease
//        pool.
//
// Error reporting:
//
//   Every function that can fail returns 0/NULL on success and a
//   non-zero value on failure, populating *err_buf with a short
//   UTF-8 string (caller-allocated, suggested size 256). The Go side
//   maps these into typed kLex errors.

#ifndef KLEX_MTL_BRIDGE_DARWIN_H
#define KLEX_MTL_BRIDGE_DARWIN_H

#include <stdint.h>
#include <stddef.h>

// ── Async completion contract ──────────────────────────────────────────────
//
// Every bridge function that submits GPU work and currently blocks on
// waitUntilCompleted accepts an `int64_t go_handle` parameter that
// switches behaviour:
//
//   go_handle == 0  — synchronous mode. The bridge function blocks on
//                     waitUntilCompleted and returns the GPU result.
//                     Used by direct C callers (none today; reserved).
//
//   go_handle != 0  — asynchronous mode. The bridge function encodes,
//                     installs an addCompletedHandler that calls back
//                     into Go via goMtlCompletion(go_handle, err_string),
//                     commits, and returns 0 IMMEDIATELY without
//                     waiting. The caller (a Go goroutine) parks on a
//                     cgo.Handle-wrapped channel and the Metal
//                     completion thread wakes it.
//
// Validation errors that happen BEFORE the GPU is invoked (unknown
// handles, buffer size mismatches, etc.) still return -1 with err_buf
// populated, the completion handler is NOT installed, and the Go-side
// channel is not signalled. The caller MUST treat a -1 return as
// "no completion is coming — handle the error now".

// goMtlCompletion is implemented in Go (see builtins_mtl_darwin.go)
// via //export and declared automatically in cgo's generated
// _cgo_export.h.  The Obj-C bridge includes that header rather than
// re-declaring here, to avoid signature conflicts with the cgo-
// generated prototype (cgo uses GoUintptr/char* with no const).

// ── Device info ─────────────────────────────────────────────────────────────

// MtlDeviceInfo carries a snapshot of the default system MTLDevice's
// identity and capabilities. Used by _mtlInfo() to surface diagnostic
// info to kLex scripts (e.g. "are we on Apple Silicon? does this chip
// support hardware ray tracing?").
typedef struct {
    char     name[256];          // MTLDevice.name (UTF-8, null-terminated)
    uint64_t registry_id;        // MTLDevice.registryID — stable per boot
    int      has_unified_memory; // 1 on Apple Silicon, 0 on Intel + discrete
    int      supports_raytracing;// 1 on M3+, 0 elsewhere
    int      supports_family_apple7; // M1+/A14+ baseline
    int      max_threads_per_group; // MTLDevice.maxThreadsPerThreadgroup.width
} MtlDeviceInfo;

// mtl_default_device_info fills *info with the default device's
// capabilities. Returns 0 on success, -1 if no Metal device is
// available (very old hardware, or running under a configuration
// that disabled Metal).
int mtl_default_device_info(MtlDeviceInfo *info, char *err_buf, size_t err_buf_len);

// ── Surfaces (offscreen render targets) ─────────────────────────────────────
//
// A "surface" is an MTLTexture configured for render-target use plus
// CPU read-back, owned by the bridge's handle table. Handles are
// positive int64 values (0 means "invalid"); the bridge retains the
// texture until the caller invokes mtl_surface_release.
//
// Pixel format is MTLPixelFormatRGBA8Unorm — chosen so read-back
// bytes need no channel swap before they hit Go's image/png encoder.
// Storage mode is MTLStorageModeShared, which is zero-copy on
// Apple Silicon (the whole point of unified memory) and a single
// synchronise-on-blit on Intel.
//
// All surface functions are thread-safe — the bridge takes a lock
// around the handle table on every entry so kLex goroutines can
// call into Metal from any goroutine without races.

// mtl_surface_create allocates a new RGBA8 surface of the given
// dimensions and returns its handle. Returns 0 on failure (and
// populates *err_buf with the reason).
int64_t mtl_surface_create(int width, int height,
                            char *err_buf, size_t err_buf_len);

// mtl_surface_create_from_bytes allocates a new RGBA8 surface of
// the given dimensions and uploads `pixels` into it. `pixel_bytes`
// must equal width * height * 4. Returns the handle on success, or
// 0 with *err_buf populated on failure (size mismatch, allocation
// failure, etc).
int64_t mtl_surface_create_from_bytes(const uint8_t *pixels, size_t pixel_bytes,
                                       int width, int height,
                                       char *err_buf, size_t err_buf_len);

// mtl_surface_clear fills the surface with the given RGBA color
// (each channel 0.0..1.0). See the async-completion contract above:
// pass go_handle=0 to block, non-zero for async-with-callback.
// Returns 0 on success, -1 on validation failure (pre-submit).
int mtl_surface_clear(int64_t handle,
                      float r, float g, float b, float a,
                      int64_t go_handle,
                      char *err_buf, size_t err_buf_len);

// mtl_surface_size writes the surface's pixel dimensions into the
// caller-supplied out parameters. Returns 0 on success, -1 if the
// handle is unknown.
int mtl_surface_size(int64_t handle, int *out_width, int *out_height,
                     char *err_buf, size_t err_buf_len);

// mtl_surface_to_rgba copies the surface's pixels into the caller's
// buffer in RGBA8 row-major order. out_bytes must equal
// width * height * 4. Returns 0 on success, -1 on failure (wrong
// buffer size, unknown handle).
int mtl_surface_to_rgba(int64_t handle,
                        uint8_t *out_pixels, size_t out_bytes,
                        char *err_buf, size_t err_buf_len);

// mtl_surface_release drops the bridge's retain on the surface.
// Safe to call with an unknown handle (no-op).
void mtl_surface_release(int64_t handle);

// ── Compute pipelines (kernels) ─────────────────────────────────────────────
//
// A kernel handle wraps a compiled MTLComputePipelineState. Compilation
// happens once via newLibraryWithSource: + newFunctionWithName: +
// newComputePipelineStateWithFunction:. The resulting pipeline state
// is cached under the handle for repeated dispatches.

int64_t mtl_kernel_create(const char *msl_source, const char *fn_name,
                          char *err_buf, size_t err_buf_len);
void mtl_kernel_release(int64_t handle);

// ── Buffers (float storage) ─────────────────────────────────────────────────
//
// MTLBuffer wrappers for kLex float arrays. Storage mode is Shared —
// zero-copy on Apple Silicon. Phase 1 only handles float32 buffers
// because that's what MSL compute shaders consume by default; other
// element types (int32, uint8, struct) can be added when a real
// workload demands them rather than speculatively now.

int64_t mtl_buffer_create_f32(const float *data, int count,
                              char *err_buf, size_t err_buf_len);

// mtl_buffer_alloc creates a zero-initialised shared-storage buffer of
// byte_length bytes WITHOUT taking a CPU-side source array. Per
// Apple's docs, newBufferWithLength: returns an allocation already
// cleared to zero by the OS — no per-element CPU work, no Go-side
// type marshalling, no kLex makeArray cost. This is the right
// primitive for any buffer that the GPU will fully overwrite (output
// buffers for reductions, FFT, matmul, etc.) AND for histogram
// counters that need to start at zero anyway.
int64_t mtl_buffer_alloc(int byte_length,
                         char *err_buf, size_t err_buf_len);
int     mtl_buffer_read_f32(int64_t handle, float *out_data, int count,
                            char *err_buf, size_t err_buf_len);
int     mtl_buffer_count_f32(int64_t handle);

// uint32 buffer creation — needed for triangle index buffers in
// indexed acceleration structures (Metal's MTLIndexTypeUInt32 wants
// real integers, not floats) and for atomic-counter outputs from
// histogram kernels. Same shared-storage semantics as the f32 variant.
int64_t mtl_buffer_create_u32(const uint32_t *data, int count,
                              char *err_buf, size_t err_buf_len);

// uint32 buffer read-back. Same byte-length semantics as f32 (both
// are 4 bytes per element), so mtl_buffer_count_f32 returns the
// correct element count for u32 buffers too.
int mtl_buffer_read_u32(int64_t handle, uint32_t *out_data, int count,
                        char *err_buf, size_t err_buf_len);

void    mtl_buffer_release(int64_t handle);

// ── Dispatch ────────────────────────────────────────────────────────────────
//
// MtlDispatchSpec is the parameter bundle for one compute dispatch.
// Textures and buffers are bound to MSL slots in declaration order:
// the i-th texture goes to [[texture(i)]], the i-th buffer to
// [[buffer(i)]]. Up to 8 of each is more than enough for the
// workloads on the road map (ray tracer, image filters, particles).
//
// The grid is in THREAD count, not threadgroup count — Metal's
// dispatchThreads: divides automatically using the pipeline's
// threadExecutionWidth and maxTotalThreadsPerThreadgroup.

#define MTL_MAX_BIND 8

typedef struct {
    int64_t kernel_handle;
    int64_t texture_handles[MTL_MAX_BIND];
    int     texture_count;
    int64_t buffer_handles[MTL_MAX_BIND];
    int     buffer_count;
    // Acceleration structures share the buffer-slot index space with
    // regular buffers but bind via a different encoder API. They
    // occupy slots [buffer_count .. buffer_count + accel_count - 1],
    // so the MSL author writes `[[buffer(K)]]` where K is past the
    // last regular buffer. Required for ray-tracing kernels.
    int64_t accel_handles[MTL_MAX_BIND];
    int     accel_count;
    int     grid_x, grid_y, grid_z;
} MtlDispatchSpec;

int mtl_dispatch(const MtlDispatchSpec *spec,
                 int64_t go_handle,
                 char *err_buf, size_t err_buf_len);

// ── Command-buffer batching ────────────────────────────────────────────────
//
// For workloads that issue many dispatches in tight succession (e.g.
// a multi-sample renderer doing 64 passes), wrapping them in a single
// command buffer eliminates per-dispatch commit overhead and lets
// Metal pipeline the work end-to-end.
//
// Usage pattern (kLex side):
//
//   batch = mtl.beginBatch()
//   i = 0
//   while i < N {
//       _ = mtl.dispatchInBatch(batch, kernel, bindings, grid)
//       i = i + 1
//   }
//   _, err = mtl.await(mtl.endBatch(batch))   // commits + waits
//
// Inside the batch every dispatch shares one MTLCommandBuffer. There
// is no GPU synchronization between dispatches in the same batch —
// callers must ensure inter-dispatch dependencies are explicit (e.g.
// each dispatch writes to a different buffer slot).

// mtl_batch_begin allocates a fresh MTLCommandBuffer and returns a
// batch handle. Caller MUST eventually call mtl_batch_commit to
// release the underlying objects.
int64_t mtl_batch_begin(char *err_buf, size_t err_buf_len);

// mtl_batch_dispatch encodes one compute dispatch onto an open
// batch's command buffer. Same MtlDispatchSpec shape as
// mtl_dispatch. Returns 0 on success, -1 on validation failure.
int mtl_batch_dispatch(int64_t batch_handle, const MtlDispatchSpec *spec,
                       char *err_buf, size_t err_buf_len);

// mtl_batch_commit closes the batch — commits its command buffer
// and either blocks (go_handle=0) or installs a completion handler
// (go_handle != 0) per the async-completion contract. Releases the
// batch handle either way.
int mtl_batch_commit(int64_t batch_handle, int64_t go_handle,
                     char *err_buf, size_t err_buf_len);

// mtl_batch_release abandons a batch without committing. The encoded
// dispatches are discarded; the underlying MTLCommandBuffer is dropped.
// Use this when a mid-batch dispatch fails and the caller wants to bail
// out instead of committing partial work. Idempotent: releasing an
// unknown handle is a no-op (no error).
void mtl_batch_release(int64_t batch_handle);

// ── MPS-accelerated primitives ──────────────────────────────────────────────
//
// Wrappers around Apple's MetalPerformanceShaders.framework. The MPS
// kernels are tuned for Apple Silicon (using simdgroup matrix
// instructions, tiled access patterns, prefetch, etc.) and hit
// orders of magnitude more of peak GPU throughput than the naive
// MSL kernels we write ourselves.

// mtl_matmul_mps performs C = A · B using MPSMatrixMultiplication.
// All three buffers must already exist (created via mtl_buffer_create_f32);
// the C buffer is overwritten in place.
//
//   A: m × k row-major float32, k*sizeof(float) row stride
//   B: k × n row-major float32, n*sizeof(float) row stride
//   C: m × n row-major float32, n*sizeof(float) row stride
//
// Synchronous on the bridge side. The Go wrapper runs it inside a
// goroutine so the kLex thread doesn't block.
int mtl_matmul_mps(int64_t a_handle, int64_t b_handle, int64_t c_handle,
                   int m, int k, int n,
                   int64_t go_handle,
                   char *err_buf, size_t err_buf_len);

// mtl_fft_complex_1d performs a 1D complex-to-complex Fast Fourier
// Transform of length n via MPSGraph. Both buffers hold 2n floats
// in interleaved (re,im) layout — the same memory format SciPy /
// NumPy use for complex single-precision arrays.
//
//   inverse=0  →  forward FFT: time-domain → frequency-domain
//   inverse=1  →  inverse FFT (without scaling — divide by n on
//                 the caller side if you need a true inverse)
//
// Synchronous on the bridge side; the Go wrapper runs it inside
// a goroutine for the async-channel API.
int mtl_fft_complex_1d(int64_t in_handle, int64_t out_handle, int n,
                       int inverse,
                       int64_t go_handle,
                       char *err_buf, size_t err_buf_len);

// ── Acceleration structures (ray tracing) ───────────────────────────────────
//
// An acceleration structure is the spatial index a ray query
// traverses to find triangle intersections in O(log n) instead of
// O(n). Building one is a GPU-side operation that can take dozens
// of milliseconds for non-trivial meshes — async-by-construction.
//
// Phase 2a accepts triangle-list geometry only: a single vertex
// buffer of float3 positions with vertexCount/3 implicit triangles.
// Indexed geometry (separate vertex+index buffers, vertex reuse)
// can be added once a real workload demands it.
//
// The accel structure retains its source buffers via Metal's
// resource tracking — releasing the vertex buffer's kLex handle
// is safe; the accel structure keeps it alive internally.
//
// Returns 0 (invalid handle) on failure, otherwise the handle to
// the built accel structure.

// Note: accel builds support sync mode only (go_handle is ignored
// for now). Building is one-shot per scene and rarely in a hot
// loop; conversion is mechanical when needed.
int64_t mtl_accel_build_triangles(int64_t vertex_buffer_handle,
                                  int vertex_count,
                                  char *err_buf, size_t err_buf_len);

// mtl_accel_build_indexed_triangles is the indexed-geometry variant.
// The vertex buffer holds N unique float3 positions; the index
// buffer holds 3*M uint32 indices that reference vertices to form M
// triangles. This is the right choice when triangles SHARE
// vertices — unindexed triangle lists with shared exact positions
// silently degenerate the BVH (see feedback-mtl-shared-verts-bvh
// memory). `vertex_count` is the number of float3 vertices in the
// vertex buffer; `index_count` is the number of uint32 indices in
// the index buffer (must be a positive multiple of 3).
int64_t mtl_accel_build_indexed_triangles(int64_t vertex_buffer_handle,
                                          int vertex_count,
                                          int64_t index_buffer_handle,
                                          int index_count,
                                          char *err_buf, size_t err_buf_len);

void mtl_accel_release(int64_t handle);

#endif // KLEX_MTL_BRIDGE_DARWIN_H
