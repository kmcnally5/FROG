// mtl_bridge_darwin.m — Objective-C side of the kLex ↔ Metal bridge.
//
// Each function here corresponds 1:1 with a declaration in
// mtl_bridge_darwin.h. The .h file is what cgo includes; this .m
// file does the actual Metal work and never appears on the Go side.
//
// Building:
//
//   cgo on darwin auto-compiles any .m sibling of a .go file that
//   imports the same package. The `#cgo LDFLAGS` in builtins_mtl_darwin.go
//   pulls in the Metal/Foundation/QuartzCore frameworks.
//
// Error convention:
//
//   Every function that can fail takes a (err_buf, err_buf_len) pair
//   and writes a UTF-8 error string into err_buf on failure. The
//   string is short (< 200 chars) and never allocates — we use
//   snprintf with format strings against statically known message
//   forms.

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#import <MetalPerformanceShaders/MetalPerformanceShaders.h>
#import <MetalPerformanceShadersGraph/MetalPerformanceShadersGraph.h>
#include <string.h>
#include <stdio.h>

#include "mtl_bridge_darwin.h"

// cgo auto-generates this header during the build; it declares
// goMtlCompletion with cgo's preferred prototype shape.
#include "_cgo_export.h"

// write_err copies a static C string into the caller's err_buf with
// bounds checking. Used to avoid raw strncpy at every error site.
static void write_err(char *err_buf, size_t err_buf_len, const char *msg) {
    if (err_buf == NULL || err_buf_len == 0) return;
    snprintf(err_buf, err_buf_len, "%s", msg);
}

// ── Shared bridge state ─────────────────────────────────────────────────────
//
// One device + one command queue per process. Apple's docs explicitly
// recommend reusing both rather than recreating per operation —
// MTLDevice/MTLCommandQueue creation is expensive (hundreds of µs) and
// they're safe to share across threads. The handle table maps int64
// handles (the values kLex sees as integers) to retained MTLTexture
// references. With ARC, putting a texture into the dictionary takes
// ownership; removing it drops the retain.
//
// gNextHandle starts at 1 because 0 is the "invalid handle" sentinel.

static id<MTLDevice>         gDevice  = nil;
static id<MTLCommandQueue>   gQueue   = nil;
static NSMutableDictionary<NSNumber *, id<MTLTexture>>              *gSurfaces = nil;
static NSMutableDictionary<NSNumber *, id<MTLComputePipelineState>> *gKernels  = nil;
static NSMutableDictionary<NSNumber *, id<MTLBuffer>>               *gBuffers  = nil;
static NSMutableDictionary<NSNumber *, id<MTLAccelerationStructure>> *gAccels  = nil;
// For each AS handle, remember its source vertex buffer so the
// compute command encoder can `useResource:` it during ray-tracing
// dispatches. Apple's docs require this for any resource the kernel
// touches indirectly (via intersection_query) — without it the GPU
// reads stale or zeroed memory.
static NSMutableDictionary<NSNumber *, id<MTLBuffer>> *gAccelVertBufs = nil;

// FFT graph executable cache.  Keyed by "n=<n>,inv=<0|1>", values
// are MtlFFTEntry objects holding the compiled MPSGraphExecutable
// plus the input/output tensor references the executable expects
// (preserved in feedTensors / targetTensors order).
//
// Compiling an MPSGraph is the slow step (~1-2 ms for a 1-op FFT
// graph, dominated by kernel selection). Running an executable is
// essentially just a setBuffer + dispatch — much cheaper. Apple's
// canonical pattern is compile-once, run-many. See
// MPSGraphCompilationDescriptor docs.
@class MtlFFTEntry;
static NSMutableDictionary *gFFTCache = nil;

// MPSMatrixMultiplication object cache. Keyed by "m=<m>,k=<k>,n=<n>".
// The expensive step inside [[MPSMatrixMultiplication alloc] init…] is
// kernel selection — for a given (m, k, n) Metal picks an optimal
// tile size and simdgroup matrix instruction pattern. Reusing the
// kernel across calls saves that work entirely.
static NSMutableDictionary *gMatmulCache = nil;

// Open batches.  Maps batch handle → MTLCommandBuffer that hasn't
// yet been committed.  Subsequent mtl_batch_dispatch calls add more
// compute encoders onto the same command buffer; mtl_batch_commit
// closes it.
static NSMutableDictionary<NSNumber *, id<MTLCommandBuffer>> *gBatches = nil;

// Buffer recycle pool. Keyed by NSNumber(byte_length); value is an
// NSMutableArray of recently-released MTLBuffers of that exact size.
// On mtl_buffer_release we push the buffer into the bucket (up to
// MTL_BUFFER_POOL_BUCKET_CAP entries; beyond that we let the buffer
// deallocate). On the next mtl_buffer_alloc / create_f32 /
// create_u32 of matching size we pop instead of asking Metal.
//
// Why bother: Metal's allocator is fast but not free — repeated
// alloc/dealloc cycles of buffers of the same size show up under
// Instruments. For workloads that repeatedly create-then-release
// (e.g. per-sample renderers, per-frame uniform updates) pooling
// eliminates the alloc churn entirely.
#define MTL_BUFFER_POOL_BUCKET_CAP 8
static NSMutableDictionary<NSNumber *, NSMutableArray<id<MTLBuffer>> *> *gBufferPool = nil;

static NSLock               *gLock    = nil;
// Single counter shared across all resource types. Handles are
// unique across the whole bridge, which simplifies debugging — if
// kLex passes handle 17, exactly one table holds it.
static int64_t               gNextHandle = 1;

// bridge_init is called once on first use. Lazy because constructing a
// MTLDevice on a process that never calls into Metal is wasteful and
// can trigger spurious GPU-driver loading.
static int bridge_init(char *err_buf, size_t err_buf_len) {
    static dispatch_once_t once;
    static int last_rc = 0;
    dispatch_once(&once, ^{
        gDevice = MTLCreateSystemDefaultDevice();
        if (gDevice == nil) {
            last_rc = -1;
            return;
        }
        gQueue = [gDevice newCommandQueue];
        if (gQueue == nil) {
            last_rc = -1;
            return;
        }
        gSurfaces      = [NSMutableDictionary new];
        gKernels       = [NSMutableDictionary new];
        gBuffers       = [NSMutableDictionary new];
        gAccels        = [NSMutableDictionary new];
        gAccelVertBufs = [NSMutableDictionary new];
        gFFTCache      = [NSMutableDictionary new];
        gMatmulCache   = [NSMutableDictionary new];
        gBatches       = [NSMutableDictionary new];
        gBufferPool    = [NSMutableDictionary new];
        gLock = [NSLock new];
    });
    if (last_rc != 0) {
        write_err(err_buf, err_buf_len,
            "Metal bridge init failed (no device or no command queue)");
    }
    return last_rc;
}

// pool_try_acquire returns a recycled buffer of exactly the requested
// byte length, or nil if none is available. Must be called WITHOUT
// holding gLock — acquires it internally.
static id<MTLBuffer> pool_try_acquire(size_t byte_length) {
    if (gBufferPool == nil) return nil;
    NSNumber *key = @(byte_length);
    [gLock lock];
    NSMutableArray<id<MTLBuffer>> *bucket = gBufferPool[key];
    id<MTLBuffer> buf = nil;
    if (bucket != nil && bucket.count > 0) {
        buf = bucket.lastObject;
        [bucket removeLastObject];
    }
    [gLock unlock];
    return buf;
}

// pool_release returns a buffer to the recycle pool. If the bucket
// is full or pooling is disabled, the buffer is dropped (Metal frees
// it when ARC's retain count reaches zero). Must be called WITHOUT
// holding gLock — acquires it internally.
static void pool_release(id<MTLBuffer> buf) {
    if (buf == nil || gBufferPool == nil) return;
    NSNumber *key = @([buf length]);
    [gLock lock];
    NSMutableArray<id<MTLBuffer>> *bucket = gBufferPool[key];
    if (bucket == nil) {
        bucket = [NSMutableArray new];
        gBufferPool[key] = bucket;
    }
    if (bucket.count < MTL_BUFFER_POOL_BUCKET_CAP) {
        [bucket addObject:buf];
    }
    // else: drop. Buffer's strong ref goes away when caller's
    // dictionary entry is removed — Metal recycles the memory.
    [gLock unlock];
}

// commit_or_install_completion implements the async-completion
// contract for any command buffer ready to be committed:
//
//   go_handle == 0  →  commit + waitUntilCompleted (sync). Returns
//                       0 on success or -1 with err_buf populated.
//   go_handle != 0  →  install addCompletedHandler that calls back
//                       into Go via goMtlCompletion, then commit
//                       and return 0 immediately. The Go side
//                       waits for the callback to deliver result.
//
// In async mode, errors that happen on the GPU are delivered via
// the callback (err_or_null != NULL). The function itself always
// returns 0 in async mode — failure surfaces through the callback.
static int commit_or_install_completion(id<MTLCommandBuffer> cb,
                                        int64_t go_handle,
                                        const char *sync_err_prefix,
                                        char *err_buf, size_t err_buf_len) {
    if (go_handle != 0) {
        [cb addCompletedHandler:^(id<MTLCommandBuffer> done) {
            if (done.error != nil) {
                NSString *msg = [done.error localizedDescription];
                // const char* → char* cast because cgo's exported
                // goMtlCompletion prototype doesn't include const.
                // The Go side treats it as read-only.
                char *cstr = (char *)(msg ? [msg UTF8String] : NULL);
                goMtlCompletion((uintptr_t)go_handle,
                    cstr ? cstr : (char *)"GPU command buffer reported an error");
            } else {
                goMtlCompletion((uintptr_t)go_handle, NULL);
            }
        }];
        [cb commit];
        return 0;
    }
    // Sync path.
    [cb commit];
    [cb waitUntilCompleted];
    if (cb.error != nil) {
        const char *desc = [[cb.error localizedDescription] UTF8String];
        snprintf(err_buf, err_buf_len, "%s%s",
            sync_err_prefix ? sync_err_prefix : "",
            desc ? desc : "(no description)");
        return -1;
    }
    return 0;
}

// lookup_surface returns the texture for a handle, or nil if absent.
// Acquires gLock internally; callers must NOT hold the lock.
// The returned reference is retained by ARC for the duration of the
// caller's local — the texture survives even if another thread
// releases the handle between the lookup and the use.
static id<MTLTexture> lookup_surface(int64_t handle) {
    if (gSurfaces == nil) return nil;
    NSNumber *key = @(handle);
    [gLock lock];
    id<MTLTexture> tex = gSurfaces[key];
    [gLock unlock];
    return tex;
}

int mtl_default_device_info(MtlDeviceInfo *info, char *err_buf, size_t err_buf_len) {
    if (info == NULL) {
        write_err(err_buf, err_buf_len, "mtl_default_device_info: info pointer is NULL");
        return -1;
    }

    // Defensive zero — every field has a documented "absent" meaning
    // (0/false), so partial fills can't ship surprise data.
    memset(info, 0, sizeof(MtlDeviceInfo));

    @autoreleasepool {
        id<MTLDevice> device = MTLCreateSystemDefaultDevice();
        if (device == nil) {
            write_err(err_buf, err_buf_len,
                "no Metal-capable device available (very old hardware, or Metal disabled)");
            return -1;
        }

        NSString *deviceName = [device name];
        if (deviceName != nil) {
            const char *cname = [deviceName UTF8String];
            if (cname != NULL) {
                strncpy(info->name, cname, sizeof(info->name) - 1);
                info->name[sizeof(info->name) - 1] = '\0';
            }
        }

        info->registry_id        = (uint64_t)[device registryID];
        info->has_unified_memory = [device hasUnifiedMemory] ? 1 : 0;

        // supportsFamily: appears in macOS 10.15+. The kLex deployment
        // target is macOS 11+ (per the Go module / Tadpole etc.) so this
        // is always safe to call.
        info->supports_family_apple7 =
            [device supportsFamily:MTLGPUFamilyApple7] ? 1 : 0;

        // Hardware ray-tracing API: MTLGPUFamilyApple9 (M3+). Older
        // Apple Silicon still does software ray tracing via compute
        // kernels — see Phase 2.
        info->supports_raytracing =
            [device supportsFamily:MTLGPUFamilyApple9] ? 1 : 0;

        // Maximum threads per threadgroup .width. The full triple
        // (width/height/depth) is more nuanced but for our 1D and 2D
        // compute dispatches the width is the binding constraint.
        info->max_threads_per_group =
            (int)[device maxThreadsPerThreadgroup].width;
    }

    return 0;
}

// ── Surface lifecycle + clear + read-back ───────────────────────────────────

int64_t mtl_surface_create(int width, int height,
                            char *err_buf, size_t err_buf_len) {
    if (width <= 0 || height <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_create: width and height must be positive");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        MTLTextureDescriptor *desc =
            [MTLTextureDescriptor texture2DDescriptorWithPixelFormat:MTLPixelFormatRGBA8Unorm
                                                              width:(NSUInteger)width
                                                             height:(NSUInteger)height
                                                          mipmapped:NO];
        // Render-target + shader-read + shader-write so the texture
        // can serve as input or output of any pipeline stage —
        // necessary for compute kernels (image FX) that write into a
        // surface via texture<float, access::write>. Shared storage
        // = zero-copy CPU read-back on Apple Silicon.
        desc.usage = MTLTextureUsageRenderTarget |
                     MTLTextureUsageShaderRead |
                     MTLTextureUsageShaderWrite;
        desc.storageMode = MTLStorageModeShared;

        id<MTLTexture> tex = [gDevice newTextureWithDescriptor:desc];
        if (tex == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_surface_create: newTextureWithDescriptor returned nil");
            return 0;
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gSurfaces[@(handle)] = tex;
        [gLock unlock];

        return handle;
    }
}

int64_t mtl_surface_create_from_bytes(const uint8_t *pixels, size_t pixel_bytes,
                                       int width, int height,
                                       char *err_buf, size_t err_buf_len) {
    if (width <= 0 || height <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_create_from_bytes: width and height must be positive");
        return 0;
    }
    if (pixels == NULL) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_create_from_bytes: pixels is NULL");
        return 0;
    }
    size_t expected = (size_t)width * (size_t)height * 4;
    if (pixel_bytes != expected) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_create_from_bytes: pixel_bytes mismatch "
            "(caller must pass exactly width*height*4)");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        MTLTextureDescriptor *desc =
            [MTLTextureDescriptor texture2DDescriptorWithPixelFormat:MTLPixelFormatRGBA8Unorm
                                                              width:(NSUInteger)width
                                                             height:(NSUInteger)height
                                                          mipmapped:NO];
        desc.usage = MTLTextureUsageRenderTarget |
                     MTLTextureUsageShaderRead |
                     MTLTextureUsageShaderWrite;
        desc.storageMode = MTLStorageModeShared;

        id<MTLTexture> tex = [gDevice newTextureWithDescriptor:desc];
        if (tex == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_surface_create_from_bytes: newTextureWithDescriptor returned nil");
            return 0;
        }

        // Upload the caller's bytes into the texture. RGBA8 row stride
        // is width*4. replaceRegion: copies into the texture's shared
        // storage; for unified memory this is a real memcpy of the
        // pixels, no roundtrip to GPU memory.
        MTLRegion region = MTLRegionMake2D(0, 0, (NSUInteger)width, (NSUInteger)height);
        [tex replaceRegion:region
               mipmapLevel:0
                 withBytes:pixels
               bytesPerRow:(NSUInteger)width * 4];

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gSurfaces[@(handle)] = tex;
        [gLock unlock];

        return handle;
    }
}

int mtl_surface_clear(int64_t handle,
                      float r, float g, float b, float a,
                      int64_t go_handle,
                      char *err_buf, size_t err_buf_len) {
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return -1;
    }

    @autoreleasepool {
        id<MTLTexture> tex = lookup_surface(handle);
        if (tex == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_surface_clear: unknown surface handle");
            return -1;
        }

        MTLRenderPassDescriptor *rp = [MTLRenderPassDescriptor new];
        rp.colorAttachments[0].texture     = tex;
        rp.colorAttachments[0].loadAction  = MTLLoadActionClear;
        rp.colorAttachments[0].storeAction = MTLStoreActionStore;
        rp.colorAttachments[0].clearColor  = MTLClearColorMake(r, g, b, a);

        id<MTLCommandBuffer> cb = [gQueue commandBuffer];
        id<MTLRenderCommandEncoder> enc =
            [cb renderCommandEncoderWithDescriptor:rp];
        [enc endEncoding];
        return commit_or_install_completion(cb, go_handle,
            "mtl_surface_clear: ", err_buf, err_buf_len);
    }
}

int mtl_surface_size(int64_t handle, int *out_width, int *out_height,
                     char *err_buf, size_t err_buf_len) {
    if (out_width == NULL || out_height == NULL) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_size: output pointers are NULL");
        return -1;
    }

    id<MTLTexture> tex = lookup_surface(handle);
    if (tex == nil) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_size: unknown surface handle");
        return -1;
    }
    *out_width  = (int)[tex width];
    *out_height = (int)[tex height];
    return 0;
}

int mtl_surface_to_rgba(int64_t handle,
                        uint8_t *out_pixels, size_t out_bytes,
                        char *err_buf, size_t err_buf_len) {
    if (out_pixels == NULL) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_to_rgba: out_pixels is NULL");
        return -1;
    }

    id<MTLTexture> tex = lookup_surface(handle);
    if (tex == nil) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_to_rgba: unknown surface handle");
        return -1;
    }

    NSUInteger w = [tex width];
    NSUInteger h = [tex height];
    size_t expected = (size_t)w * (size_t)h * 4;
    if (out_bytes != expected) {
        write_err(err_buf, err_buf_len,
            "mtl_surface_to_rgba: buffer size mismatch (caller must pass w*h*4)");
        return -1;
    }

    MTLRegion region = MTLRegionMake2D(0, 0, w, h);
    [tex getBytes:out_pixels
       bytesPerRow:w * 4
        fromRegion:region
       mipmapLevel:0];
    return 0;
}

void mtl_surface_release(int64_t handle) {
    [gLock lock];
    if (gSurfaces != nil) {
        [gSurfaces removeObjectForKey:@(handle)];
    }
    [gLock unlock];
}

// ── Compute kernels ─────────────────────────────────────────────────────────
//
// mtl_kernel_create compiles MSL source, looks up the named function,
// and produces an MTLComputePipelineState. The pipeline is what
// dispatch encodes against; once created it can be reused for many
// dispatches without recompiling.
//
// All three steps (library, function, pipeline) can fail with
// distinguishable error messages, which we surface verbatim via
// err_buf so the caller can see what the compiler complained about.

int64_t mtl_kernel_create(const char *msl_source, const char *fn_name,
                          char *err_buf, size_t err_buf_len) {
    if (msl_source == NULL || fn_name == NULL) {
        write_err(err_buf, err_buf_len,
            "mtl_kernel_create: source or fn_name is NULL");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        NSString *src  = [NSString stringWithUTF8String:msl_source];
        NSString *name = [NSString stringWithUTF8String:fn_name];
        if (src == nil || name == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_kernel_create: source or fn_name is not valid UTF-8");
            return 0;
        }

        NSError *err = nil;
        id<MTLLibrary> lib = [gDevice newLibraryWithSource:src options:nil error:&err];
        if (lib == nil) {
            const char *msg = [[err localizedDescription] UTF8String];
            snprintf(err_buf, err_buf_len, "MSL compile failed: %s",
                msg ? msg : "(no description)");
            return 0;
        }

        id<MTLFunction> fn = [lib newFunctionWithName:name];
        if (fn == nil) {
            snprintf(err_buf, err_buf_len,
                "MSL compile succeeded but function '%s' not found in the library",
                fn_name);
            return 0;
        }

        id<MTLComputePipelineState> pipeline =
            [gDevice newComputePipelineStateWithFunction:fn error:&err];
        if (pipeline == nil) {
            const char *msg = [[err localizedDescription] UTF8String];
            snprintf(err_buf, err_buf_len,
                "pipeline state creation failed for '%s': %s",
                fn_name, msg ? msg : "(no description)");
            return 0;
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gKernels[@(handle)] = pipeline;
        [gLock unlock];
        return handle;
    }
}

void mtl_kernel_release(int64_t handle) {
    [gLock lock];
    if (gKernels != nil) {
        [gKernels removeObjectForKey:@(handle)];
    }
    [gLock unlock];
}

// ── Buffers (float32 storage) ───────────────────────────────────────────────
//
// Shared-storage MTLBuffer wrappers. On Apple Silicon the buffer's
// bytes are CPU-visible without any explicit synchronisation; reading
// after a dispatch's command buffer has completed is safe because the
// completion callback fires after the GPU's writes are visible.
//
// Phase 1 keeps this strictly float32 — adding int32/uint8/struct
// variants now would be speculation (no consumer needs them yet).
// When a real workload demands another type, add the explicit
// variant rather than reaching for a generic typed-pointer API.

int64_t mtl_buffer_create_f32(const float *data, int count,
                              char *err_buf, size_t err_buf_len) {
    if (count <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_create_f32: count must be positive");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        size_t byteLength = (size_t)count * sizeof(float);
        // Try the recycle pool. If we get a buffer, overwrite its
        // contents with the caller's source bytes (no zero pass
        // needed — memcpy fills the whole region).
        id<MTLBuffer> buf = pool_try_acquire(byteLength);
        if (buf != nil) {
            memcpy([buf contents], data, byteLength);
        } else {
            buf = [gDevice newBufferWithBytes:data
                                       length:byteLength
                                      options:MTLResourceStorageModeShared];
            if (buf == nil) {
                write_err(err_buf, err_buf_len,
                    "mtl_buffer_create_f32: newBufferWithBytes returned nil");
                return 0;
            }
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gBuffers[@(handle)] = buf;
        [gLock unlock];
        return handle;
    }
}

int64_t mtl_buffer_alloc(int byte_length,
                         char *err_buf, size_t err_buf_len) {
    if (byte_length <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_alloc: byte_length must be positive");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        // Try the recycle pool first — a same-size buffer from a
        // recent release. Note: pool entries still contain whatever
        // bytes were in them on release. For mtl_buffer_alloc we
        // need a zero-filled buffer, so explicitly zero on reuse.
        id<MTLBuffer> buf = pool_try_acquire((size_t)byte_length);
        if (buf != nil) {
            memset([buf contents], 0, (size_t)byte_length);
        } else {
            // Pool miss — fresh allocation. newBufferWithLength:
            // zero-initialises at the OS level for free.
            buf = [gDevice newBufferWithLength:(NSUInteger)byte_length
                                       options:MTLResourceStorageModeShared];
            if (buf == nil) {
                write_err(err_buf, err_buf_len,
                    "mtl_buffer_alloc: newBufferWithLength returned nil");
                return 0;
            }
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gBuffers[@(handle)] = buf;
        [gLock unlock];
        return handle;
    }
}

int mtl_buffer_count_f32(int64_t handle) {
    [gLock lock];
    id<MTLBuffer> buf = (gBuffers != nil) ? gBuffers[@(handle)] : nil;
    [gLock unlock];
    if (buf == nil) return -1;
    return (int)([buf length] / sizeof(float));
}

int mtl_buffer_read_f32(int64_t handle, float *out_data, int count,
                        char *err_buf, size_t err_buf_len) {
    if (out_data == NULL) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_read_f32: out_data is NULL");
        return -1;
    }

    [gLock lock];
    id<MTLBuffer> buf = (gBuffers != nil) ? gBuffers[@(handle)] : nil;
    [gLock unlock];
    if (buf == nil) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_read_f32: unknown buffer handle");
        return -1;
    }
    size_t available = [buf length] / sizeof(float);
    if ((size_t)count != available) {
        snprintf(err_buf, err_buf_len,
            "mtl_buffer_read_f32: requested %d floats but buffer holds %zu",
            count, available);
        return -1;
    }
    // Shared-storage buffers: contents() returns a CPU pointer to the
    // same bytes the GPU sees. memcpy is correct and zero-syscall on
    // Apple Silicon.
    memcpy(out_data, [buf contents], (size_t)count * sizeof(float));
    return 0;
}

int mtl_buffer_read_u32(int64_t handle, uint32_t *out_data, int count,
                        char *err_buf, size_t err_buf_len) {
    if (out_data == NULL) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_read_u32: out_data is NULL");
        return -1;
    }
    [gLock lock];
    id<MTLBuffer> buf = (gBuffers != nil) ? gBuffers[@(handle)] : nil;
    [gLock unlock];
    if (buf == nil) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_read_u32: unknown buffer handle");
        return -1;
    }
    size_t available = [buf length] / sizeof(uint32_t);
    if ((size_t)count != available) {
        snprintf(err_buf, err_buf_len,
            "mtl_buffer_read_u32: requested %d uint32s but buffer holds %zu",
            count, available);
        return -1;
    }
    memcpy(out_data, [buf contents], (size_t)count * sizeof(uint32_t));
    return 0;
}

void mtl_buffer_release(int64_t handle) {
    // Fetch + remove inside the lock; release into the pool outside.
    // Removing from gBuffers drops our strong reference, so without
    // the pool the underlying MTLBuffer would be freed. Pushing into
    // the pool re-strong-refs it so it survives for reuse.
    id<MTLBuffer> buf = nil;
    [gLock lock];
    if (gBuffers != nil) {
        buf = gBuffers[@(handle)];
        if (buf != nil) {
            [gBuffers removeObjectForKey:@(handle)];
        }
    }
    [gLock unlock];
    if (buf != nil) {
        pool_release(buf);
    }
}

// uint32 buffer creator. Shares the same gBuffers table as f32 —
// MTLBuffer is opaque, so the bridge doesn't track element type;
// the kLex side is responsible for using the right primitive
// (_mtlBuffer vs _mtlBufferU32) depending on what the kernel
// expects. Mixing them on the wrong slot is silent corruption,
// no API enforcement.
int64_t mtl_buffer_create_u32(const uint32_t *data, int count,
                              char *err_buf, size_t err_buf_len) {
    if (count <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_buffer_create_u32: count must be positive");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        size_t byteLength = (size_t)count * sizeof(uint32_t);
        id<MTLBuffer> buf = pool_try_acquire(byteLength);
        if (buf != nil) {
            memcpy([buf contents], data, byteLength);
        } else {
            buf = [gDevice newBufferWithBytes:data
                                       length:byteLength
                                      options:MTLResourceStorageModeShared];
            if (buf == nil) {
                write_err(err_buf, err_buf_len,
                    "mtl_buffer_create_u32: newBufferWithBytes returned nil");
                return 0;
            }
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gBuffers[@(handle)] = buf;
        [gLock unlock];
        return handle;
    }
}

// ── Dispatch ────────────────────────────────────────────────────────────────
//
// mtl_dispatch encodes a single compute command:
//   1. Look up the compiled pipeline state by handle.
//   2. Look up every texture / buffer binding.
//   3. Create a command buffer + compute command encoder.
//   4. Set the pipeline, bind textures + buffers in declaration order.
//   5. Dispatch with the requested grid size; Metal computes the
//      threadgroup decomposition from the pipeline's
//      threadExecutionWidth and maxTotalThreadsPerThreadgroup.
//   6. End encoding, commit, wait for completion.
//
// The function blocks until the GPU is done. The Go side calls it
// from a goroutine spawned by runAsyncSingle, so the kLex thread
// stays responsive — that's the whole point of the async pivot.
//
// Synchronisation: while the dispatch is in flight, gLock is NOT
// held — concurrent reads of other handles are fine, and even another
// dispatch on a different goroutine can proceed (Metal's command
// queue is thread-safe).

// ── Acceleration structure build ────────────────────────────────────────────
//
// Build steps (Apple's canonical pattern):
//
//   1. Describe the geometry  (MTLAccelerationStructureTriangleGeometryDescriptor)
//   2. Describe the AS itself (MTLPrimitiveAccelerationStructureDescriptor)
//   3. Query required sizes   (accelerationStructureSizesWithDescriptor:)
//   4. Allocate the AS        (newAccelerationStructureWithSize:)
//   5. Allocate scratch       (newBufferWithLength: ... StorageModePrivate)
//   6. Encode the build       (accelerationStructureCommandEncoder)
//   7. Commit + wait
//
// Synchronous on the bridge side; the Go layer wraps this in an
// async goroutine + Channel so kLex never waits.

int64_t mtl_accel_build_triangles(int64_t vertex_buffer_handle,
                                  int vertex_count,
                                  char *err_buf, size_t err_buf_len) {
    if (vertex_count <= 0 || (vertex_count % 3) != 0) {
        write_err(err_buf, err_buf_len,
            "mtl_accel_build_triangles: vertex_count must be a positive multiple of 3");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        [gLock lock];
        id<MTLBuffer> vbuf = (gBuffers != nil) ? gBuffers[@(vertex_buffer_handle)] : nil;
        [gLock unlock];
        if (vbuf == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_accel_build_triangles: unknown vertex buffer handle");
            return 0;
        }
        // Each vertex is a float3 (12 bytes). Validate the buffer has
        // enough storage before describing the geometry.
        NSUInteger needed = (NSUInteger)vertex_count * sizeof(float) * 3;
        if ([vbuf length] < needed) {
            snprintf(err_buf, err_buf_len,
                "mtl_accel_build_triangles: vertex buffer too small (need %lu bytes for %d float3 vertices, have %lu)",
                (unsigned long)needed, vertex_count, (unsigned long)[vbuf length]);
            return 0;
        }

        // Geometry descriptor: triangle list, float3 positions,
        // no index buffer (vertices stride through in order, 3 per
        // triangle).
        MTLAccelerationStructureTriangleGeometryDescriptor *geomDesc =
            [MTLAccelerationStructureTriangleGeometryDescriptor descriptor];
        geomDesc.vertexBuffer       = vbuf;
        geomDesc.vertexBufferOffset = 0;
        geomDesc.vertexFormat       = MTLAttributeFormatFloat3;
        geomDesc.vertexStride       = sizeof(float) * 3;
        geomDesc.triangleCount      = (NSUInteger)(vertex_count / 3);

        MTLPrimitiveAccelerationStructureDescriptor *accelDesc =
            [MTLPrimitiveAccelerationStructureDescriptor descriptor];
        accelDesc.geometryDescriptors = @[geomDesc];

        MTLAccelerationStructureSizes sizes =
            [gDevice accelerationStructureSizesWithDescriptor:accelDesc];

        id<MTLAccelerationStructure> accel =
            [gDevice newAccelerationStructureWithSize:sizes.accelerationStructureSize];
        if (accel == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_accel_build_triangles: newAccelerationStructureWithSize returned nil");
            return 0;
        }

        // Scratch buffer for the build. Private storage = GPU-only;
        // we never touch it from the CPU, only the GPU writes to it.
        id<MTLBuffer> scratch =
            [gDevice newBufferWithLength:sizes.buildScratchBufferSize
                                 options:MTLResourceStorageModePrivate];
        if (scratch == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_accel_build_triangles: scratch buffer allocation failed");
            return 0;
        }

        id<MTLCommandBuffer> cb = [gQueue commandBuffer];
        id<MTLAccelerationStructureCommandEncoder> enc =
            [cb accelerationStructureCommandEncoder];
        [enc buildAccelerationStructure:accel
                             descriptor:accelDesc
                          scratchBuffer:scratch
                    scratchBufferOffset:0];
        [enc endEncoding];
        [cb commit];
        [cb waitUntilCompleted];

        if (cb.error != nil) {
            const char *desc = [[cb.error localizedDescription] UTF8String];
            snprintf(err_buf, err_buf_len,
                "accel build GPU command failed: %s",
                desc ? desc : "(no description)");
            return 0;
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gAccels[@(handle)] = accel;
        // Remember the vertex buffer so dispatch can useResource: it
        // when this accel appears in a kernel's binding list.
        gAccelVertBufs[@(handle)] = vbuf;
        [gLock unlock];
        return handle;
    }
}

// Indexed-AS build. Same descriptor sequence as the unindexed
// variant, plus indexBuffer + indexBufferOffset + indexType set on
// the geometry descriptor. The index buffer must be a uint32 buffer
// previously created via mtl_buffer_create_u32.
int64_t mtl_accel_build_indexed_triangles(int64_t vertex_buffer_handle,
                                          int vertex_count,
                                          int64_t index_buffer_handle,
                                          int index_count,
                                          char *err_buf, size_t err_buf_len) {
    if (vertex_count <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_accel_build_indexed_triangles: vertex_count must be positive");
        return 0;
    }
    if (index_count <= 0 || (index_count % 3) != 0) {
        write_err(err_buf, err_buf_len,
            "mtl_accel_build_indexed_triangles: index_count must be a positive multiple of 3");
        return 0;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }

    @autoreleasepool {
        [gLock lock];
        id<MTLBuffer> vbuf = (gBuffers != nil) ? gBuffers[@(vertex_buffer_handle)] : nil;
        id<MTLBuffer> ibuf = (gBuffers != nil) ? gBuffers[@(index_buffer_handle)] : nil;
        [gLock unlock];
        if (vbuf == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_accel_build_indexed_triangles: unknown vertex buffer handle");
            return 0;
        }
        if (ibuf == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_accel_build_indexed_triangles: unknown index buffer handle");
            return 0;
        }

        NSUInteger neededV = (NSUInteger)vertex_count * sizeof(float) * 3;
        NSUInteger neededI = (NSUInteger)index_count  * sizeof(uint32_t);
        if ([vbuf length] < neededV) {
            snprintf(err_buf, err_buf_len,
                "vertex buffer too small (need %lu bytes for %d float3 vertices, have %lu)",
                (unsigned long)neededV, vertex_count, (unsigned long)[vbuf length]);
            return 0;
        }
        if ([ibuf length] < neededI) {
            snprintf(err_buf, err_buf_len,
                "index buffer too small (need %lu bytes for %d uint32 indices, have %lu)",
                (unsigned long)neededI, index_count, (unsigned long)[ibuf length]);
            return 0;
        }

        MTLAccelerationStructureTriangleGeometryDescriptor *geomDesc =
            [MTLAccelerationStructureTriangleGeometryDescriptor descriptor];
        geomDesc.vertexBuffer       = vbuf;
        geomDesc.vertexBufferOffset = 0;
        geomDesc.vertexFormat       = MTLAttributeFormatFloat3;
        geomDesc.vertexStride       = sizeof(float) * 3;
        geomDesc.indexBuffer        = ibuf;
        geomDesc.indexBufferOffset  = 0;
        geomDesc.indexType          = MTLIndexTypeUInt32;
        geomDesc.triangleCount      = (NSUInteger)(index_count / 3);

        MTLPrimitiveAccelerationStructureDescriptor *accelDesc =
            [MTLPrimitiveAccelerationStructureDescriptor descriptor];
        accelDesc.geometryDescriptors = @[geomDesc];

        MTLAccelerationStructureSizes sizes =
            [gDevice accelerationStructureSizesWithDescriptor:accelDesc];

        id<MTLAccelerationStructure> accel =
            [gDevice newAccelerationStructureWithSize:sizes.accelerationStructureSize];
        if (accel == nil) {
            write_err(err_buf, err_buf_len,
                "newAccelerationStructureWithSize returned nil");
            return 0;
        }

        id<MTLBuffer> scratch =
            [gDevice newBufferWithLength:sizes.buildScratchBufferSize
                                 options:MTLResourceStorageModePrivate];
        if (scratch == nil) {
            write_err(err_buf, err_buf_len, "scratch buffer allocation failed");
            return 0;
        }

        id<MTLCommandBuffer> cb = [gQueue commandBuffer];
        id<MTLAccelerationStructureCommandEncoder> enc =
            [cb accelerationStructureCommandEncoder];
        [enc buildAccelerationStructure:accel
                             descriptor:accelDesc
                          scratchBuffer:scratch
                    scratchBufferOffset:0];
        [enc endEncoding];
        [cb commit];
        [cb waitUntilCompleted];

        if (cb.error != nil) {
            const char *desc = [[cb.error localizedDescription] UTF8String];
            snprintf(err_buf, err_buf_len,
                "indexed-accel build GPU command failed: %s",
                desc ? desc : "(no description)");
            return 0;
        }

        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gAccels[@(handle)] = accel;
        // Remember the vertex buffer for useResource: at dispatch
        // time. Indexed geometry's index buffer is held internally
        // by the AS — no need to track it separately for
        // intersection_query.
        gAccelVertBufs[@(handle)] = vbuf;
        [gLock unlock];
        return handle;
    }
}

void mtl_accel_release(int64_t handle) {
    [gLock lock];
    if (gAccels != nil) {
        [gAccels removeObjectForKey:@(handle)];
    }
    if (gAccelVertBufs != nil) {
        [gAccelVertBufs removeObjectForKey:@(handle)];
    }
    [gLock unlock];
}

// ── MPS matmul ─────────────────────────────────────────────────────────────
//
// Wraps MPSMatrixMultiplication. Significantly faster than our naive
// kernel for non-trivial sizes — Apple ships a tiled, simdgroup-
// matrix-instruction implementation that hits >90% of M-series peak
// for large GEMMs.

// matmul_cache_lookup_or_build returns a cached MPSMatrixMultiplication
// kernel for the given (m, k, n). First call for a shape pays the
// kernel-selection cost (~ms); subsequent calls return the same
// object. Caller doesn't release — the cache owns the retain.
static MPSMatrixMultiplication *
matmul_cache_lookup_or_build(int m, int k, int n) {
    NSString *key = [NSString stringWithFormat:@"m=%d,k=%d,n=%d", m, k, n];
    [gLock lock];
    MPSMatrixMultiplication *mm = gMatmulCache[key];
    [gLock unlock];
    if (mm != nil) {
        return mm;
    }

    mm = [[MPSMatrixMultiplication alloc]
        initWithDevice:gDevice
         transposeLeft:NO
        transposeRight:NO
            resultRows:(NSUInteger)m
         resultColumns:(NSUInteger)n
       interiorColumns:(NSUInteger)k
                 alpha:1.0
                  beta:0.0];

    if (mm == nil) {
        return nil;
    }

    [gLock lock];
    // Race: another thread may have populated the same key. Last
    // writer wins — both kernels are equivalent, no semantic issue.
    gMatmulCache[key] = mm;
    [gLock unlock];

    return mm;
}

int mtl_matmul_mps(int64_t a_handle, int64_t b_handle, int64_t c_handle,
                   int m, int k, int n,
                   int64_t go_handle,
                   char *err_buf, size_t err_buf_len) {
    if (m <= 0 || k <= 0 || n <= 0) {
        write_err(err_buf, err_buf_len,
            "mtl_matmul_mps: m, k, n must be positive");
        return -1;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return -1;
    }

    @autoreleasepool {
        [gLock lock];
        id<MTLBuffer> aBuf = (gBuffers != nil) ? gBuffers[@(a_handle)] : nil;
        id<MTLBuffer> bBuf = (gBuffers != nil) ? gBuffers[@(b_handle)] : nil;
        id<MTLBuffer> cBuf = (gBuffers != nil) ? gBuffers[@(c_handle)] : nil;
        [gLock unlock];
        if (aBuf == nil) {
            write_err(err_buf, err_buf_len, "mtl_matmul_mps: unknown A buffer handle");
            return -1;
        }
        if (bBuf == nil) {
            write_err(err_buf, err_buf_len, "mtl_matmul_mps: unknown B buffer handle");
            return -1;
        }
        if (cBuf == nil) {
            write_err(err_buf, err_buf_len, "mtl_matmul_mps: unknown C buffer handle");
            return -1;
        }

        // Buffer size checks. Each float = 4 bytes, row-major layout
        // assumed (so row stride = columns * sizeof(float)).
        size_t needA = (size_t)m * (size_t)k * sizeof(float);
        size_t needB = (size_t)k * (size_t)n * sizeof(float);
        size_t needC = (size_t)m * (size_t)n * sizeof(float);
        if ([aBuf length] < needA) {
            snprintf(err_buf, err_buf_len,
                "A buffer too small (need %lu bytes for %dx%d, have %lu)",
                (unsigned long)needA, m, k, (unsigned long)[aBuf length]);
            return -1;
        }
        if ([bBuf length] < needB) {
            snprintf(err_buf, err_buf_len,
                "B buffer too small (need %lu bytes for %dx%d, have %lu)",
                (unsigned long)needB, k, n, (unsigned long)[bBuf length]);
            return -1;
        }
        if ([cBuf length] < needC) {
            snprintf(err_buf, err_buf_len,
                "C buffer too small (need %lu bytes for %dx%d, have %lu)",
                (unsigned long)needC, m, n, (unsigned long)[cBuf length]);
            return -1;
        }

        // MPSMatrixDescriptors describe the rows/cols/stride/type of
        // each matrix.  rowBytes for a tight row-major float layout
        // is just `columns * sizeof(float)`.
        MPSMatrixDescriptor *aDesc = [MPSMatrixDescriptor
            matrixDescriptorWithRows:(NSUInteger)m
                             columns:(NSUInteger)k
                            rowBytes:(NSUInteger)k * sizeof(float)
                            dataType:MPSDataTypeFloat32];
        MPSMatrixDescriptor *bDesc = [MPSMatrixDescriptor
            matrixDescriptorWithRows:(NSUInteger)k
                             columns:(NSUInteger)n
                            rowBytes:(NSUInteger)n * sizeof(float)
                            dataType:MPSDataTypeFloat32];
        MPSMatrixDescriptor *cDesc = [MPSMatrixDescriptor
            matrixDescriptorWithRows:(NSUInteger)m
                             columns:(NSUInteger)n
                            rowBytes:(NSUInteger)n * sizeof(float)
                            dataType:MPSDataTypeFloat32];

        MPSMatrix *aMat = [[MPSMatrix alloc] initWithBuffer:aBuf descriptor:aDesc];
        MPSMatrix *bMat = [[MPSMatrix alloc] initWithBuffer:bBuf descriptor:bDesc];
        MPSMatrix *cMat = [[MPSMatrix alloc] initWithBuffer:cBuf descriptor:cDesc];

        // alpha=1, beta=0  →  C := A · B  (no accumulation onto C).
        // Cached per (m, k, n) so the kernel-selection cost is paid
        // only on the first call for each shape.
        MPSMatrixMultiplication *mm = matmul_cache_lookup_or_build(m, k, n);
        if (mm == nil) {
            write_err(err_buf, err_buf_len,
                "MPSMatrixMultiplication kernel creation failed");
            return -1;
        }

        id<MTLCommandBuffer> cb = [gQueue commandBuffer];
        [mm encodeToCommandBuffer:cb
                       leftMatrix:aMat
                      rightMatrix:bMat
                     resultMatrix:cMat];
        return commit_or_install_completion(cb, go_handle,
            "MPS matmul GPU error: ", err_buf, err_buf_len);
    }
}

// ── MPSGraph FFT ───────────────────────────────────────────────────────────
//
// 1D complex-to-complex FFT via MPSGraph using the compile-once /
// run-many pattern.  The first call for a given (n, inverse) pair
// builds an MPSGraph, compiles it to an MPSGraphExecutable, and
// caches the executable + the input/output tensor references.
// Subsequent calls hit the cache and skip straight to the runWith…
// dispatch, which is essentially just setBuffer + commit.
//
// Apple guidance: MPSGraphExecutable is the canonical way to
// amortise the graph-compilation cost across many runs (per the
// MPSGraphCompilationDescriptor docs).  For real-time audio at
// 1000+ FFTs/sec on the same N this is the difference between the
// GPU keeping up and falling behind.
//
// Layout: input and output are interleaved (re, im) float32 — the
// same memory layout NumPy / SciPy use for complex64 arrays.

// MtlFFTEntry caches everything needed to run one (n, inverse) FFT
// without rebuilding the graph: the compiled executable plus the
// feed/target tensor handles the executable expects in its inputs
// and results arrays.
@interface MtlFFTEntry : NSObject
@property (nonatomic) int n;
@property (nonatomic) int inverse;
@property (nonatomic, strong) MPSGraphExecutable *executable;
@property (nonatomic, strong) MPSGraphTensor *inputTensor;
@property (nonatomic, strong) MPSGraphTensor *outputTensor;
@end
@implementation MtlFFTEntry
@end

static MtlFFTEntry *fft_cache_lookup_or_build(int n, int inverse,
                                              char *err_buf, size_t err_buf_len) {
    NSString *key = [NSString stringWithFormat:@"n=%d,inv=%d", n, inverse];
    [gLock lock];
    MtlFFTEntry *entry = gFFTCache[key];
    [gLock unlock];
    if (entry != nil) {
        return entry;
    }

    // Cache miss — build + compile.
    MPSGraph *graph = [MPSGraph new];
    MPSGraphTensor *input = [graph placeholderWithShape:@[@(n)]
                                               dataType:MPSDataTypeComplexFloat32
                                                   name:nil];
    MPSGraphFFTDescriptor *desc = [MPSGraphFFTDescriptor descriptor];
    desc.inverse = (inverse != 0);

    MPSGraphTensor *output = [graph fastFourierTransformWithTensor:input
                                                              axes:@[@0]
                                                        descriptor:desc
                                                              name:nil];

    // Compile to an executable.  feeds dictionary maps each
    // placeholder to its shaped tensor descriptor (the executable
    // is specialised for these exact input shapes / dtypes).
    MPSGraphShapedType *inputShaped =
        [[MPSGraphShapedType alloc] initWithShape:@[@(n)]
                                         dataType:MPSDataTypeComplexFloat32];

    MPSGraphCompilationDescriptor *compileDesc =
        [MPSGraphCompilationDescriptor new];

    MPSGraphExecutable *exe =
        [graph compileWithDevice:[MPSGraphDevice deviceWithMTLDevice:gDevice]
                           feeds:@{input: inputShaped}
                   targetTensors:@[output]
                targetOperations:nil
           compilationDescriptor:compileDesc];

    if (exe == nil) {
        snprintf(err_buf, err_buf_len,
            "MPSGraph FFT compile failed for n=%d", n);
        return nil;
    }

    entry = [MtlFFTEntry new];
    entry.n = n;
    entry.inverse = inverse;
    entry.executable = exe;
    entry.inputTensor = input;
    entry.outputTensor = output;

    [gLock lock];
    // Race: another thread may have populated the same key. Last
    // writer wins — that's harmless; both entries are equivalent.
    gFFTCache[key] = entry;
    [gLock unlock];

    return entry;
}

int mtl_fft_complex_1d(int64_t in_handle, int64_t out_handle, int n,
                       int inverse,
                       int64_t go_handle,
                       char *err_buf, size_t err_buf_len) {
    if (n <= 0) {
        write_err(err_buf, err_buf_len, "mtl_fft_complex_1d: n must be positive");
        return -1;
    }
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return -1;
    }

    @autoreleasepool {
        [gLock lock];
        id<MTLBuffer> inBuf  = (gBuffers != nil) ? gBuffers[@(in_handle)]  : nil;
        id<MTLBuffer> outBuf = (gBuffers != nil) ? gBuffers[@(out_handle)] : nil;
        [gLock unlock];
        if (inBuf == nil) {
            write_err(err_buf, err_buf_len, "mtl_fft_complex_1d: unknown input buffer handle");
            return -1;
        }
        if (outBuf == nil) {
            write_err(err_buf, err_buf_len, "mtl_fft_complex_1d: unknown output buffer handle");
            return -1;
        }
        size_t need = (size_t)n * 8;
        if ([inBuf length] < need) {
            snprintf(err_buf, err_buf_len,
                "FFT input buffer too small (need %lu bytes for %d complex, have %lu)",
                (unsigned long)need, n, (unsigned long)[inBuf length]);
            return -1;
        }
        if ([outBuf length] < need) {
            snprintf(err_buf, err_buf_len,
                "FFT output buffer too small (need %lu bytes for %d complex, have %lu)",
                (unsigned long)need, n, (unsigned long)[outBuf length]);
            return -1;
        }

        MtlFFTEntry *entry = fft_cache_lookup_or_build(n, inverse, err_buf, err_buf_len);
        if (entry == nil) {
            return -1;
        }

        MPSGraphTensorData *inputData = [[MPSGraphTensorData alloc]
            initWithMTLBuffer:inBuf
                        shape:@[@(n)]
                     dataType:MPSDataTypeComplexFloat32];
        MPSGraphTensorData *outputData = [[MPSGraphTensorData alloc]
            initWithMTLBuffer:outBuf
                        shape:@[@(n)]
                     dataType:MPSDataTypeComplexFloat32];

        // encodeToCommandBuffer:inputsArray:resultsArray: — the
        // executable already knows the graph; we just bind the
        // concrete buffers in the feed/target tensor order.
        id<MTLCommandBuffer> rawCb = [gQueue commandBuffer];
        MPSCommandBuffer *mpsCb = [MPSCommandBuffer commandBufferWithCommandBuffer:rawCb];
        [entry.executable encodeToCommandBuffer:mpsCb
                                   inputsArray:@[inputData]
                                  resultsArray:@[outputData]
                           executionDescriptor:nil];
        // MPSCommandBuffer conforms to MTLCommandBuffer protocol, so
        // the same async-completion helper works.
        return commit_or_install_completion((id<MTLCommandBuffer>)mpsCb,
            go_handle, "MPSGraph FFT GPU error: ", err_buf, err_buf_len);
    }
}

// encode_dispatch_on_cb does everything except commit — used by
// both mtl_dispatch (creates its own cb) and mtl_batch_dispatch
// (shares the batch's cb). Returns 0 on success, -1 on validation
// failure with err_buf populated.
static int encode_dispatch_on_cb(id<MTLCommandBuffer> cb,
                                  const MtlDispatchSpec *spec,
                                  char *err_buf, size_t err_buf_len) {
    if (spec == NULL) {
        write_err(err_buf, err_buf_len, "dispatch: spec is NULL");
        return -1;
    }
    if (spec->grid_x <= 0 || spec->grid_y <= 0 || spec->grid_z <= 0) {
        write_err(err_buf, err_buf_len,
            "dispatch: grid dimensions must all be positive");
        return -1;
    }

    // Resolve every handle BEFORE encoding so we error early
    // rather than partway through a command buffer setup.
    [gLock lock];
    id<MTLComputePipelineState> pipeline = (gKernels != nil)
        ? gKernels[@(spec->kernel_handle)] : nil;

    id<MTLTexture> textures[MTL_MAX_BIND] = {0};
    for (int i = 0; i < spec->texture_count; i++) {
        textures[i] = (gSurfaces != nil)
            ? gSurfaces[@(spec->texture_handles[i])] : nil;
    }
    id<MTLBuffer> buffers[MTL_MAX_BIND] = {0};
    for (int i = 0; i < spec->buffer_count; i++) {
        buffers[i] = (gBuffers != nil)
            ? gBuffers[@(spec->buffer_handles[i])] : nil;
    }
    id<MTLAccelerationStructure> accels[MTL_MAX_BIND] = {0};
    id<MTLBuffer> accelVerts[MTL_MAX_BIND] = {0};
    for (int i = 0; i < spec->accel_count; i++) {
        NSNumber *key = @(spec->accel_handles[i]);
        accels[i]     = (gAccels        != nil) ? gAccels[key]        : nil;
        accelVerts[i] = (gAccelVertBufs != nil) ? gAccelVertBufs[key] : nil;
    }
    [gLock unlock];

    if (pipeline == nil) {
        write_err(err_buf, err_buf_len, "dispatch: unknown kernel handle");
        return -1;
    }
    for (int i = 0; i < spec->texture_count; i++) {
        if (textures[i] == nil) {
            snprintf(err_buf, err_buf_len,
                "dispatch: unknown texture handle at slot %d", i);
            return -1;
        }
    }
    for (int i = 0; i < spec->buffer_count; i++) {
        if (buffers[i] == nil) {
            snprintf(err_buf, err_buf_len,
                "dispatch: unknown buffer handle at slot %d", i);
            return -1;
        }
    }
    for (int i = 0; i < spec->accel_count; i++) {
        if (accels[i] == nil) {
            snprintf(err_buf, err_buf_len,
                "dispatch: unknown accel handle at slot %d", i);
            return -1;
        }
    }
    if (spec->buffer_count + spec->accel_count > MTL_MAX_BIND) {
        snprintf(err_buf, err_buf_len,
            "dispatch: buffer_count (%d) + accel_count (%d) exceeds MTL_MAX_BIND (%d)",
            spec->buffer_count, spec->accel_count, MTL_MAX_BIND);
        return -1;
    }

    id<MTLComputeCommandEncoder> enc = [cb computeCommandEncoder];
    [enc setComputePipelineState:pipeline];

    for (int i = 0; i < spec->texture_count; i++) {
        [enc setTexture:textures[i] atIndex:i];
    }
    for (int i = 0; i < spec->buffer_count; i++) {
        [enc setBuffer:buffers[i] offset:0 atIndex:i];
    }
    for (int i = 0; i < spec->accel_count; i++) {
        [enc setAccelerationStructure:accels[i]
                        atBufferIndex:(NSUInteger)(spec->buffer_count + i)];
        if (accelVerts[i] != nil) {
            [enc useResource:accelVerts[i] usage:MTLResourceUsageRead];
        }
    }

    MTLSize gridSize = MTLSizeMake((NSUInteger)spec->grid_x,
                                   (NSUInteger)spec->grid_y,
                                   (NSUInteger)spec->grid_z);
    NSUInteger w = pipeline.threadExecutionWidth;
    NSUInteger maxThreads = pipeline.maxTotalThreadsPerThreadgroup;
    NSUInteger h = (spec->grid_y > 1) ? maxThreads / w : 1;
    if (h == 0) h = 1;
    MTLSize tgSize = MTLSizeMake(w, h, 1);

    [enc dispatchThreads:gridSize threadsPerThreadgroup:tgSize];
    [enc endEncoding];
    return 0;
}

int mtl_dispatch(const MtlDispatchSpec *spec,
                 int64_t go_handle,
                 char *err_buf, size_t err_buf_len) {
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return -1;
    }

    @autoreleasepool {
        id<MTLCommandBuffer> cb = [gQueue commandBuffer];
        int rc = encode_dispatch_on_cb(cb, spec, err_buf, err_buf_len);
        if (rc != 0) return rc;
        return commit_or_install_completion(cb, go_handle,
            "GPU dispatch failed: ", err_buf, err_buf_len);
    }
}

// ── Batch dispatch ──────────────────────────────────────────────────────────

int64_t mtl_batch_begin(char *err_buf, size_t err_buf_len) {
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return 0;
    }
    @autoreleasepool {
        id<MTLCommandBuffer> cb = [gQueue commandBuffer];
        if (cb == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_batch_begin: failed to allocate command buffer");
            return 0;
        }
        int64_t handle = 0;
        [gLock lock];
        handle = gNextHandle++;
        gBatches[@(handle)] = cb;
        [gLock unlock];
        return handle;
    }
}

int mtl_batch_dispatch(int64_t batch_handle, const MtlDispatchSpec *spec,
                       char *err_buf, size_t err_buf_len) {
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return -1;
    }
    @autoreleasepool {
        [gLock lock];
        id<MTLCommandBuffer> cb = (gBatches != nil) ? gBatches[@(batch_handle)] : nil;
        [gLock unlock];
        if (cb == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_batch_dispatch: unknown batch handle");
            return -1;
        }
        return encode_dispatch_on_cb(cb, spec, err_buf, err_buf_len);
    }
}

void mtl_batch_release(int64_t batch_handle) {
    // Drop the command buffer without committing it. Used as the
    // error-path cleanup so a failed mid-batch dispatch doesn't leak
    // the cb's strong reference in gBatches forever. ARC frees the
    // cb (and every encoder we attached to it) when the strong
    // reference dies.
    [gLock lock];
    if (gBatches != nil) {
        [gBatches removeObjectForKey:@(batch_handle)];
    }
    [gLock unlock];
}

int mtl_batch_commit(int64_t batch_handle, int64_t go_handle,
                     char *err_buf, size_t err_buf_len) {
    if (bridge_init(err_buf, err_buf_len) != 0) {
        return -1;
    }
    @autoreleasepool {
        [gLock lock];
        id<MTLCommandBuffer> cb = (gBatches != nil) ? gBatches[@(batch_handle)] : nil;
        if (cb != nil) {
            [gBatches removeObjectForKey:@(batch_handle)];
        }
        [gLock unlock];
        if (cb == nil) {
            write_err(err_buf, err_buf_len,
                "mtl_batch_commit: unknown batch handle");
            return -1;
        }
        return commit_or_install_completion(cb, go_handle,
            "batch commit failed: ", err_buf, err_buf_len);
    }
}
