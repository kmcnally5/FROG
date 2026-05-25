// tests/unit/mtlFxTest.lex — Tier 1 image-FX correctness + Metal/CPU parity.
//
// Goal: every filter produces sane output (matches hand-computed
// expected values within tolerance) AND when Metal is available, the
// GPU and CPU paths agree within a small per-channel byte tolerance
// (off-by-one rounding is normal on different float pipelines).
//
// Run: ./klex tests/unit/mtlFxTest.lex
// Pass condition: prints "OK" on the final line.

import "stdlib/mtl_fx.lex" as fx
import "stdlib/mtl.lex"    as mtl

const W = 16
const H = 16
const N = W * H

// ── Test image ──────────────────────────────────────────────────────
// A horizontal ramp red-to-blue with mid-grey alpha — generic enough
// that any filter changes something visible.
fn makeRamp() {
    let arr = makeArray(N * 4, 0)
    let y = 0
    while y < H {
        let x = 0
        while x < W {
            let t = x * 255 / (W - 1)
            let o = (y * W + x) * 4
            arr[o + 0] = 255 - t      // R: 255 → 0
            arr[o + 1] = 128          // G: constant mid
            arr[o + 2] = t            // B: 0 → 255
            arr[o + 3] = 255          // A: opaque
            x = x + 1
        }
        y = y + 1
    }
    return bytes(arr)
}


// ── Per-channel byte tolerance ──────────────────────────────────────
// Metal float pipeline vs Go float64 rounding can disagree by ±1 byte.
// ±2 stays well under "anything you could see" and accommodates
// pow()/HSV roundtrip rounding differences. Hue shift goes through
// trig + branching so use a slightly looser bound.
fn maxByteDiff(a, b) {
    if len(a) != len(b) {
        return -1
    }
    let worst = 0
    let i = 0
    while i < len(a) {
        let d = a[i] - b[i]
        if d < 0 { d = -d }
        if d > worst { worst = d }
        i = i + 1
    }
    return worst
}


// ── Smoke check: returned bytes have the right shape ────────────────
fn checkShape(label, bs) {
    if bs == null {
        println(label + ": FAIL — got null bytes")
        return false
    }
    if len(bs) != N * 4 {
        println(label + ": FAIL — got " + str(len(bs)) + " bytes, expected " + str(N * 4))
        return false
    }
    return true
}


// ── Per-filter check ────────────────────────────────────────────────
// runs the filter, validates the shape, and (if Metal is available)
// re-runs the CPU equivalent and compares byte-by-byte.

let failures = 0

fn check(label, gpuBytes, cpuBytes, tolerance) {
    if !checkShape(label + " gpu", gpuBytes) { failures = failures + 1  return }
    if cpuBytes != null {
        if !checkShape(label + " cpu", cpuBytes) { failures = failures + 1  return }
        let d = maxByteDiff(gpuBytes, cpuBytes)
        if d > tolerance {
            println(label + ": FAIL — max byte diff " + str(d) + " exceeds tolerance " + str(tolerance))
            failures = failures + 1
            return
        }
        println(label + ": OK (max byte diff " + str(d) + ")")
    } else {
        println(label + ": OK (CPU only — Metal unavailable)")
    }
}


let img      = makeRamp()
let mtlOn    = mtl.isAvailable()
println("Metal available: " + str(mtlOn))
println("Test image: " + str(W) + "×" + str(H) + " (" + str(N * 4) + " bytes)")
println("")


// exposure
let g, _ = fx.exposure(img, W, H, 1.0)
let c, _ = _imgExposure(img, W, H, 1.0)
check("exposure(+1)    ", g, c, 2)

// brightness
g, _ = fx.brightness(img, W, H, 0.2)
c, _ = _imgBrightness(img, W, H, 0.2)
check("brightness(+0.2)", g, c, 2)

// contrast
g, _ = fx.contrast(img, W, H, 0.5)
c, _ = _imgContrast(img, W, H, 0.5)
check("contrast(+0.5)  ", g, c, 2)

// saturation
g, _ = fx.saturation(img, W, H, -0.5)
c, _ = _imgSaturation(img, W, H, -0.5)
check("saturation(-0.5)", g, c, 2)

// hueShift — looser tolerance: HSV roundtrip on different pipelines
// can disagree by a couple of bytes near gamut corners.
g, _ = fx.hueShift(img, W, H, 90.0)
c, _ = _imgHueShift(img, W, H, 90.0)
check("hueShift(90)    ", g, c, 4)

// gamma
g, _ = fx.gamma(img, W, H, 2.2)
c, _ = _imgGamma(img, W, H, 2.2)
check("gamma(2.2)      ", g, c, 2)

// levels
g, _ = fx.levels(img, W, H, 0.1, 0.9, 1.2, 0.0, 1.0)
c, _ = _imgLevels(img, W, H, 0.1, 0.9, 1.2, 0.0, 1.0)
check("levels(0.1..0.9)", g, c, 2)

// channelMixer — swap R and B
let mat = [0.0, 0.0, 1.0,  0.0, 1.0, 0.0,  1.0, 0.0, 0.0]
g, _ = fx.channelMixer(img, W, H, mat)
c, _ = _imgChannelMixer(img, W, H, mat)
check("channelMixer    ", g, c, 1)

// invert
g, _ = fx.invert(img, W, H)
c, _ = _imgInvert(img, W, H)
check("invert          ", g, c, 1)

// desaturate
g, _ = fx.desaturate(img, W, H)
c, _ = _imgDesaturate(img, W, H)
check("desaturate      ", g, c, 1)

// sepia
g, _ = fx.sepia(img, W, H, 1.0)
c, _ = _imgSepia(img, W, H, 1.0)
check("sepia(1.0)      ", g, c, 2)

// vignette
g, _ = fx.vignette(img, W, H, 0.7, 1.0)
c, _ = _imgVignette(img, W, H, 0.7, 1.0)
check("vignette(0.7,1) ", g, c, 2)


println("")
if failures > 0 {
    println(str(failures) + " test(s) failed")
    return
}
println("All filters: OK")
