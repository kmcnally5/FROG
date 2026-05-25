// Phase 2d3+2d4 — Cornell-box-lite via indexed AS + multi-sample AA + shadows.
//
// Scene: a 2×2×2 box, open toward the camera. White floor/ceiling/back,
//   red left wall, green right wall. One point light on the ceiling.
//
// 8 unique vertices, 10 triangles, 3 materials, 5 walls.
//
// Each sample jitters the sub-pixel position. The kernel accumulates
// samples into a buffer and writes the running average to the surface.
// 64 samples produces a noticeably smoother image than 1.

fn render() {
    let src = `#include <metal_stdlib>
#include <metal_raytracing>
using namespace metal;
using namespace raytracing;

// Cheap hash for per-pixel jitter. Output in [0, 1).
float hash21(uint2 p, uint sample) {
    uint x = p.x * 2654435761u + p.y * 2246822519u + sample * 3266489917u;
    x = (x ^ (x >> 16)) * 2246822519u;
    x = (x ^ (x >> 13)) * 3266489917u;
    return float(x & 0xFFFFFFu) / float(0x1000000);
}

kernel void rt_cornell(
    texture2d<float, access::write>     out             [[texture(0)]],
    constant float*                     u               [[buffer(0)]],   // 12 floats
    constant float*                     materials       [[buffer(1)]],   // 4/material
    constant float*                     triMaterialIdx  [[buffer(2)]],   // 1/triangle
    constant float*                     vertices        [[buffer(3)]],   // 3/vertex
    constant uint*                      indices         [[buffer(4)]],   // 3/triangle
    device   float*                     accumulator     [[buffer(5)]],   // 4/pixel
    primitive_acceleration_structure    as              [[buffer(6)]],
    uint2 gid [[thread_position_in_grid]]
) {
    uint W = out.get_width();
    uint H = out.get_height();
    if (gid.x >= W || gid.y >= H) return;

    float3 eye      = float3(u[0], u[1], u[2]);
    float  fovHalf  = u[3];
    float3 lightPos = float3(u[4], u[5], u[6]);
    uint   sampleIdx = uint(u[7]);

    // Jittered sub-pixel offset for this sample.
    float jx = hash21(gid, sampleIdx)            - 0.5;
    float jy = hash21(gid, sampleIdx + 99991u)   - 0.5;

    float aspect = float(W) / float(H);
    float ux = ((float(gid.x) + 0.5 + jx) / float(W) - 0.5) * 2.0 * aspect;
    float vy = ((float(gid.y) + 0.5 + jy) / float(H) - 0.5) * 2.0;
    float t  = tan(fovHalf);

    ray r;
    r.origin       = eye;
    r.direction    = normalize(float3(ux * t, -vy * t, -1.0));
    r.min_distance = 0.001;
    r.max_distance = 1000.0;

    intersector<triangle_data> rqi;
    intersection_result<triangle_data> result = rqi.intersect(r, as);

    float3 sampleColour;
    if (result.type == intersection_type::triangle) {
        uint primId = result.primitive_id;
        uint matIdx = uint(triMaterialIdx[primId]);
        float3 albedo = float3(materials[matIdx*4+0], materials[matIdx*4+1], materials[matIdx*4+2]);

        // Reconstruct hit point + geometric normal from the triangle's
        // indexed vertices.
        uint i0 = indices[primId*3+0];
        uint i1 = indices[primId*3+1];
        uint i2 = indices[primId*3+2];
        float3 v0 = float3(vertices[i0*3+0], vertices[i0*3+1], vertices[i0*3+2]);
        float3 v1 = float3(vertices[i1*3+0], vertices[i1*3+1], vertices[i1*3+2]);
        float3 v2 = float3(vertices[i2*3+0], vertices[i2*3+1], vertices[i2*3+2]);
        float3 n  = normalize(cross(v1 - v0, v2 - v0));
        if (dot(n, r.direction) > 0.0) n = -n;

        float3 hitPoint = r.origin + r.direction * result.distance;

        // Shadow ray toward the point light.
        float3 toLight = lightPos - hitPoint;
        float  lightDist = length(toLight);
        float3 lightDir  = toLight / lightDist;

        ray sh;
        sh.origin       = hitPoint + n * 0.001;  // offset to avoid self-hit
        sh.direction    = lightDir;
        sh.min_distance = 0.001;
        sh.max_distance = lightDist - 0.002;

        intersector<triangle_data> shadowQI;
        shadowQI.accept_any_intersection(true);  // any-hit terminator
        intersection_result<triangle_data> shadow = shadowQI.intersect(sh, as);
        float visibility = (shadow.type == intersection_type::none) ? 1.0 : 0.0;

        float ndotl = max(0.0, dot(n, lightDir));
        // Lambertian + ambient. Shadow factor only modulates the
        // direct-light term — the ambient still contributes when in
        // shadow so the result isn't pitch black.
        float3 col = albedo * (0.15 + 0.85 * ndotl * visibility);
        sampleColour = col;
    } else {
        sampleColour = float3(0.02, 0.02, 0.05);   // outside the box
    }

    // Accumulate into the running-average buffer + write to surface.
    uint pi = (gid.y * W + gid.x) * 4;
    accumulator[pi+0] += sampleColour.r;
    accumulator[pi+1] += sampleColour.g;
    accumulator[pi+2] += sampleColour.b;
    float inv = 1.0 / float(sampleIdx + 1);
    float3 avg = float3(accumulator[pi+0], accumulator[pi+1], accumulator[pi+2]) * inv;
    out.write(float4(avg, 1.0), gid);
}`

    let kernel, err = _mtlKernel(src, "rt_cornell")
    if err != null {
        println("kernel compile failed:")
        println(err)
        return false
    }

    // 8 unique vertices (8 corners of the box).
    //   Back-bottom-left  (-1, -1, -2)  V0
    //   Back-bottom-right ( 1, -1, -2)  V1
    //   Back-top-right    ( 1,  1, -2)  V2
    //   Back-top-left     (-1,  1, -2)  V3
    //   Front-bottom-left (-1, -1,  0)  V4
    //   Front-bottom-right( 1, -1,  0)  V5
    //   Front-top-right   ( 1,  1,  0)  V6
    //   Front-top-left    (-1,  1,  0)  V7
    let verts = [
        -1.0, -1.0, -2.0,
         1.0, -1.0, -2.0,
         1.0,  1.0, -2.0,
        -1.0,  1.0, -2.0,
        -1.0, -1.0,  0.0,
         1.0, -1.0,  0.0,
         1.0,  1.0,  0.0,
        -1.0,  1.0,  0.0
    ]
    let vbuf, _ = _mtlBuffer(verts)

    // 10 triangles, 30 indices. Each face = 2 triangles.
    // Winding chosen so the OUTWARD normal (cross of edge1 × edge2) is
    // correct; the shader flips back-faces anyway.
    let indices = [
        // Floor   (white, prim 0, 1)
        0, 1, 5,    0, 5, 4,
        // Ceiling (white, prim 2, 3)
        3, 7, 6,    3, 6, 2,
        // Back    (white, prim 4, 5)
        0, 2, 1,    0, 3, 2,
        // Left    (red,   prim 6, 7)
        0, 4, 7,    0, 7, 3,
        // Right   (green, prim 8, 9)
        1, 2, 6,    1, 6, 5
    ]
    let ibuf, _ = _mtlBufferU32(indices)

    println("Building indexed acceleration structure (8 verts, 10 tris)...")
    let ch = _mtlAccelIndexed(vbuf, 8, ibuf, 30)
    let val, _ = recv(ch)
    let accel, aerr = val
    if aerr != null {
        println("accel build failed:", aerr)
        return false
    }
    println("  accel handle =", accel)

    // 3 materials: white walls, red wall, green wall.
    let materials = [
        0.90, 0.90, 0.90, 0.0,
        0.85, 0.15, 0.15, 0.0,
        0.15, 0.85, 0.15, 0.0
    ]
    let matBuf, _ = _mtlBuffer(materials)

    // Per-triangle material index — must match the indices order above.
    let triMatIdx, _ = _mtlBuffer([0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 1.0, 1.0, 2.0, 2.0])

    // Index buffer SLOT — we bind ibuf as a regular u32 buffer too,
    // so the kernel can read indices for normal reconstruction.
    // (The AS holds it internally for intersection.)

    let W = 1024
    let H = 768
    let surface, _ = _mtlSurface(W, H)

    // Per-pixel accumulator: 4 floats × W × H.
    let accSize = W * H * 4
    let accZero = makeArray(accSize, 0.0)
    let accBuf, _ = _mtlBuffer(accZero)

    // 64 samples. Per-iteration the uniform's sample-index slot
    // changes; the kernel reads it and uses it for jitter + averaging.
    let samples = 64
    let eye_z = 1.5

    println("Rendering", samples, "samples...")
    let i = 0
    while i < samples {
        // Light at the ceiling, slightly forward of the back wall.
        let uniforms = [
            0.0, 0.0, eye_z,         // eye
            0.392699,                 // fov_half
            0.0, 0.85, -1.0,          // light_pos
            float(i),                 // sample_index
            float(samples),           // total_samples
            0.0, 0.0, 0.0             // pad
        ]
        let uBuf, _ = _mtlBuffer(uniforms)

        ch = _mtlDispatch(kernel, {
            "textures": [surface],
            "buffers":  [uBuf, matBuf, triMatIdx, vbuf, ibuf, accBuf],
            "accels":   [accel]
        }, [W, H, 1])
        val, _ = recv(ch)
        let _, derr = val
        if derr != null {
            println("sample", i, "dispatch failed:", derr)
            _mtlBufferRelease(uBuf)
            return false
        }
        _mtlBufferRelease(uBuf)

        if (i + 1) % 16 == 0 {
            println("  ", (i+1), "/", samples, "samples done")
        }
        i = i + 1
    }

    println("Saving /tmp/mtl_cornell.png ...")
    let pngCh = _mtlSurfaceSavePng(surface, "/tmp/mtl_cornell.png")
    val, _ = recv(pngCh)
    let _, perr = val
    if perr != null {
        println("save failed:", perr)
        return false
    }
    println("  done")

    _mtlBufferRelease(accBuf)
    _mtlBufferRelease(triMatIdx)
    _mtlBufferRelease(matBuf)
    _mtlBufferRelease(ibuf)
    _mtlSurfaceRelease(surface)
    _mtlAccelRelease(accel)
    _mtlBufferRelease(vbuf)
    _mtlKernelRelease(kernel)
    return true
}

let ok = render()
if ok {
    println("")
    println("Cornell-box-lite rendered. Open /tmp/mtl_cornell.png to view.")
}
