// stdlib/mtl_fx.lex — Tier 1 image filters (Metal + CPU dual-path).
//
// Every filter takes an RGBA8 image (bytes value + width + height) plus
// its tuning parameters, and returns a new RGBA8 image of the same
// dimensions. The original bytes are never mutated — callers can keep
// the source around for "Reset" semantics in slider UIs.
//
// On macOS each filter runs as a Metal compute kernel — sub-millisecond
// at 1024² on Apple Silicon. On Linux/Windows the same kLex API falls
// back to Go-backed pixel-loop builtins (~5-20 ms at 1024²) so scripts
// stay portable.
//
// Color space: filters operate on sRGB-encoded byte values, not on
// linear light. Matches Photoshop / Lightroom defaults.
//
// Usage:
//   import "stdlib/mtl_fx.lex" as fx
//
//   bright, _   = fx.exposure(srcBytes, w, h, 0.5)        // +½ stop
//   punchy, _   = fx.contrast(bright,  w, h, 0.3)
//   final,  _   = fx.vignette(punchy,  w, h, 0.6, 0.9)
//   saveImage(makeImageFromBytes(final, w, h), "out.png")
//
// All 12 Tier 1 filters:
//   exposure(rgba, w, h, stops)              # ±4 typical
//   brightness(rgba, w, h, amount)           # -1..+1
//   contrast(rgba, w, h, amount)             # -1..+1
//   saturation(rgba, w, h, amount)           # -1..+1
//   hueShift(rgba, w, h, degrees)            # any float; wraps mod 360
//   gamma(rgba, w, h, gamma)                 # > 0; 2.2 is screen-ish
//   levels(rgba, w, h, inB, inW, g, oB, oW)  # Photoshop levels
//   channelMixer(rgba, w, h, matrix9)        # 3×3 RGB transform
//   invert(rgba, w, h)
//   desaturate(rgba, w, h)
//   sepia(rgba, w, h, strength)              # 0..1
//   vignette(rgba, w, h, strength, radius)   # strength 0..1, radius > 0

import "stdlib/mtl.lex" as mtl


// ── MSL kernels ─────────────────────────────────────────────────────
//
// Each kernel takes:
//   texture(0) — source RGBA8 (read)
//   texture(1) — dest   RGBA8 (write)
//   buffer(0)  — float[] params (omitted for zero-param filters)
//
// Common MSL preamble for every kernel.
const _MSL_HEADER = "#include <metal_stdlib>\nusing namespace metal;\n\n"

const _MSL_EXPOSURE = _MSL_HEADER +
"kernel void exposure(texture2d<float, access::read>  src [[texture(0)]],
                     texture2d<float, access::write> dst [[texture(1)]],
                     constant float *p [[buffer(0)]],
                     uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float mul = pow(2.0, p[0]);
    c.rgb *= mul;
    dst.write(c, gid);
\}"

const _MSL_BRIGHTNESS = _MSL_HEADER +
"kernel void brightness(texture2d<float, access::read>  src [[texture(0)]],
                       texture2d<float, access::write> dst [[texture(1)]],
                       constant float *p [[buffer(0)]],
                       uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    c.rgb += p[0];
    dst.write(c, gid);
\}"

const _MSL_CONTRAST = _MSL_HEADER +
"kernel void contrast(texture2d<float, access::read>  src [[texture(0)]],
                     texture2d<float, access::write> dst [[texture(1)]],
                     constant float *p [[buffer(0)]],
                     uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float k = 1.0 + p[0];
    c.rgb = (c.rgb - 0.5) * k + 0.5;
    dst.write(c, gid);
\}"

const _MSL_SATURATION = _MSL_HEADER +
"kernel void saturation(texture2d<float, access::read>  src [[texture(0)]],
                       texture2d<float, access::write> dst [[texture(1)]],
                       constant float *p [[buffer(0)]],
                       uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float y = dot(c.rgb, float3(0.2126, 0.7152, 0.0722));
    float k = 1.0 + p[0];
    c.rgb = y + (c.rgb - y) * k;
    dst.write(c, gid);
\}"

const _MSL_HUESHIFT = _MSL_HEADER +
"kernel void hueShift(texture2d<float, access::read>  src [[texture(0)]],
                     texture2d<float, access::write> dst [[texture(1)]],
                     constant float *p [[buffer(0)]],
                     uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float r = c.r, g = c.g, b = c.b;
    float mx = max(r, max(g, b));
    float mn = min(r, min(g, b));
    float d  = mx - mn;
    float h = 0.0;
    if (d > 0.0) \{
        if (mx == r)      h = fmod((g - b) / d + 6.0, 6.0);
        else if (mx == g) h = (b - r) / d + 2.0;
        else              h = (r - g) / d + 4.0;
        h *= 60.0;
    \}
    float s = (mx > 0.0) ? (d / mx) : 0.0;
    float v = mx;
    h = fmod(h + p[0], 360.0);
    if (h < 0.0) h += 360.0;
    h /= 60.0;
    int   ih = (int)floor(h) % 6;
    float fh = h - floor(h);
    float pp = v * (1.0 - s);
    float qq = v * (1.0 - s * fh);
    float tt = v * (1.0 - s * (1.0 - fh));
    float3 out = float3(v, v, v);
    if      (ih == 0) out = float3(v, tt, pp);
    else if (ih == 1) out = float3(qq, v, pp);
    else if (ih == 2) out = float3(pp, v, tt);
    else if (ih == 3) out = float3(pp, qq, v);
    else if (ih == 4) out = float3(tt, pp, v);
    else              out = float3(v, pp, qq);
    dst.write(float4(out, c.a), gid);
\}"

const _MSL_GAMMA = _MSL_HEADER +
"kernel void gammaFx(texture2d<float, access::read>  src [[texture(0)]],
                    texture2d<float, access::write> dst [[texture(1)]],
                    constant float *p [[buffer(0)]],
                    uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float inv = 1.0 / p[0];
    c.r = pow(max(c.r, 0.0), inv);
    c.g = pow(max(c.g, 0.0), inv);
    c.b = pow(max(c.b, 0.0), inv);
    dst.write(c, gid);
\}"

const _MSL_LEVELS = _MSL_HEADER +
"kernel void levels(texture2d<float, access::read>  src [[texture(0)]],
                   texture2d<float, access::write> dst [[texture(1)]],
                   constant float *p [[buffer(0)]],
                   uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float inB = p[0];
    float inW = p[1];
    float gm  = p[2];
    float oB  = p[3];
    float oW  = p[4];
    float span = inW - inB;
    float invG = 1.0 / gm;
    float outSpan = oW - oB;
    float3 n = clamp((c.rgb - inB) / span, 0.0, 1.0);
    float3 result;
    result.r = oB + pow(n.r, invG) * outSpan;
    result.g = oB + pow(n.g, invG) * outSpan;
    result.b = oB + pow(n.b, invG) * outSpan;
    dst.write(float4(result, c.a), gid);
\}"

const _MSL_CHANNELMIXER = _MSL_HEADER +
"kernel void channelMixer(texture2d<float, access::read>  src [[texture(0)]],
                         texture2d<float, access::write> dst [[texture(1)]],
                         constant float *m [[buffer(0)]],
                         uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float3 o;
    o.r = m[0]*c.r + m[1]*c.g + m[2]*c.b;
    o.g = m[3]*c.r + m[4]*c.g + m[5]*c.b;
    o.b = m[6]*c.r + m[7]*c.g + m[8]*c.b;
    dst.write(float4(o, c.a), gid);
\}"

const _MSL_INVERT = _MSL_HEADER +
"kernel void invertFx(texture2d<float, access::read>  src [[texture(0)]],
                     texture2d<float, access::write> dst [[texture(1)]],
                     uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    c.rgb = 1.0 - c.rgb;
    dst.write(c, gid);
\}"

const _MSL_DESATURATE = _MSL_HEADER +
"kernel void desaturate(texture2d<float, access::read>  src [[texture(0)]],
                       texture2d<float, access::write> dst [[texture(1)]],
                       uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float y = dot(c.rgb, float3(0.2126, 0.7152, 0.0722));
    dst.write(float4(y, y, y, c.a), gid);
\}"

const _MSL_SEPIA = _MSL_HEADER +
"kernel void sepia(texture2d<float, access::read>  src [[texture(0)]],
                  texture2d<float, access::write> dst [[texture(1)]],
                  constant float *p [[buffer(0)]],
                  uint2 gid [[thread_position_in_grid]]) \{
    if (gid.x >= dst.get_width() || gid.y >= dst.get_height()) return;
    float4 c = src.read(gid);
    float s = clamp(p[0], 0.0, 1.0);
    float tr = 0.393*c.r + 0.769*c.g + 0.189*c.b;
    float tg = 0.349*c.r + 0.686*c.g + 0.168*c.b;
    float tb = 0.272*c.r + 0.534*c.g + 0.131*c.b;
    float3 o = c.rgb + (float3(tr, tg, tb) - c.rgb) * s;
    dst.write(float4(o, c.a), gid);
\}"

const _MSL_VIGNETTE = _MSL_HEADER +
"kernel void vignette(texture2d<float, access::read>  src [[texture(0)]],
                     texture2d<float, access::write> dst [[texture(1)]],
                     constant float *p [[buffer(0)]],
                     uint2 gid [[thread_position_in_grid]]) \{
    uint w = dst.get_width();
    uint h = dst.get_height();
    if (gid.x >= w || gid.y >= h) return;
    float strength = clamp(p[0], 0.0, 1.0);
    float radius   = p[1];
    float cx = (float(w) - 1.0) * 0.5;
    float cy = (float(h) - 1.0) * 0.5;
    float halfDiag = sqrt(cx*cx + cy*cy);
    float dx = float(gid.x) - cx;
    float dy = float(gid.y) - cy;
    float d  = sqrt(dx*dx + dy*dy) / halfDiag;
    float t  = clamp(d / radius, 0.0, 1.0);
    float falloff = t * t * (3.0 - 2.0 * t);    // smoothstep
    float factor  = 1.0 - strength * falloff;
    float4 c = src.read(gid);
    c.rgb *= factor;
    dst.write(c, gid);
\}"


// ── Kernel cache ────────────────────────────────────────────────────
//
// Each MSL source string compiles once via _mtlKernel and is reused
// across every dispatch in the session. Cache key = the kernel
// function name (matches the [[kernel]] entry point in the MSL above).

let _kernelCache = {}

fn _getKernel(name, mslSrc) {
    let cached = _kernelCache[name]
    if cached != null { return cached, null }
    let k, err = mtl.kernel(mslSrc, name)
    if err != null { return null, err }
    _kernelCache[name] = k
    return k, null
}


// _runMetal uploads `srcBytes` to a Metal surface, allocates a
// destination surface, dispatches the named kernel, reads the result
// back, and frees every Metal resource it created. `params` is null
// for zero-parameter filters or a flat array of floats otherwise.
//
// On any Metal error, returns (null, errString) so the calling
// public function can fall back to the CPU path.
fn _runMetal(srcBytes, w, h, name, mslSrc, params) {
    let src, sErr = mtl.surfaceFromBytes(srcBytes, w, h)
    if sErr != null { return null, sErr }
    let dst, dErr = mtl.surface(w, h)
    if dErr != null {
        mtl.releaseSurface(src)
        return null, dErr
    }

    let k, kErr = _getKernel(name, mslSrc)
    if kErr != null {
        mtl.releaseSurface(src)
        mtl.releaseSurface(dst)
        return null, kErr
    }

    let bindings = {"textures": [src, dst], "buffers": makeArray(0)}
    let paramBuf = 0
    let hasParams = false
    if params != null && len(params) > 0 {
        let paramBuf, pbErr = mtl.buffer(params)
        if pbErr != null {
            mtl.releaseSurface(src)
            mtl.releaseSurface(dst)
            return null, pbErr
        }
        bindings["buffers"] = [paramBuf]
        hasParams = true
    }

    let _, dispErr = mtl.dispatchAndWait(k, bindings, [w, h, 1])
    if hasParams {
        mtl.releaseBuffer(paramBuf)
    }
    if dispErr != null {
        mtl.releaseSurface(src)
        mtl.releaseSurface(dst)
        return null, dispErr
    }

    let out, rErr = mtl.surfaceToBytes(dst)
    mtl.releaseSurface(src)
    mtl.releaseSurface(dst)
    if rErr != null { return null, rErr }
    return out, null
}


// _useMetal decides whether to take the GPU path for this call.
// Cached on first call so isAvailable doesn't re-probe per filter.
let _metalChecked   = false
let _metalAvailable = false

fn _useMetal() {
    if _metalChecked { return _metalAvailable }
    _metalAvailable = mtl.isAvailable()
    _metalChecked   = true
    return _metalAvailable
}


// ── Public filters ──────────────────────────────────────────────────

// exposure multiplies RGB by 2^stops. Stops in roughly [-4, +4].
fn exposure(rgba, w, h, stops) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "exposure", _MSL_EXPOSURE, [stops])
        if err == null { return out, null }
    }
    return _imgExposure(rgba, w, h, stops)
}

// brightness adds `amount` to each RGB channel (in [0,1] space).
// amount in [-1, +1]; 0 = no change.
fn brightness(rgba, w, h, amount) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "brightness", _MSL_BRIGHTNESS, [amount])
        if err == null { return out, null }
    }
    return _imgBrightness(rgba, w, h, amount)
}

// contrast scales each channel around mid-gray (0.5). amount in
// [-1, +1]; 0 = no change, +1 doubles contrast, -1 flattens to grey.
fn contrast(rgba, w, h, amount) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "contrast", _MSL_CONTRAST, [amount])
        if err == null { return out, null }
    }
    return _imgContrast(rgba, w, h, amount)
}

// saturation pulls each pixel toward (amount<0) or away from (amount>0)
// its luma. amount in [-1, +1].
fn saturation(rgba, w, h, amount) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "saturation", _MSL_SATURATION, [amount])
        if err == null { return out, null }
    }
    return _imgSaturation(rgba, w, h, amount)
}

// hueShift rotates hue by `degrees` (any real number; wraps mod 360).
fn hueShift(rgba, w, h, degrees) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "hueShift", _MSL_HUESHIFT, [degrees])
        if err == null { return out, null }
    }
    return _imgHueShift(rgba, w, h, degrees)
}

// gamma raises each channel to the power 1/gamma. gamma > 1 brightens
// midtones; gamma < 1 darkens. Must be > 0.
fn gamma(rgba, w, h, g) {
    if g <= 0 {
        return null, error("MTL_FX_BAD_ARGS", "gamma must be > 0")
    }
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "gammaFx", _MSL_GAMMA, [g])
        if err == null { return out, null }
    }
    return _imgGamma(rgba, w, h, g)
}

// levels remaps [inB, inW] of the input to [oB, oW] of the output
// with an intermediate gamma adjustment. All five sliders in [0, 1]
// (g > 0).
fn levels(rgba, w, h, inB, inW, g, oB, oW) {
    if g <= 0 {
        return null, error("MTL_FX_BAD_ARGS", "levels gamma must be > 0")
    }
    if inW == inB {
        return null, error("MTL_FX_BAD_ARGS", "levels inBlack and inWhite must differ")
    }
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "levels", _MSL_LEVELS,
            [inB, inW, g, oB, oW])
        if err == null { return out, null }
    }
    return _imgLevels(rgba, w, h, inB, inW, g, oB, oW)
}

// channelMixer applies a 3×3 row-major matrix to each pixel's RGB.
// matrix9 = [rr, rg, rb, gr, gg, gb, br, bg, bb]. Identity = no change.
fn channelMixer(rgba, w, h, matrix9) {
    if len(matrix9) != 9 {
        return null, error("MTL_FX_BAD_ARGS", "channelMixer: matrix9 must have 9 elements")
    }
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "channelMixer", _MSL_CHANNELMIXER, matrix9)
        if err == null { return out, null }
    }
    return _imgChannelMixer(rgba, w, h, matrix9)
}

// invert produces a photo negative — 1 - c on each RGB channel.
fn invert(rgba, w, h) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "invertFx", _MSL_INVERT, null)
        if err == null { return out, null }
    }
    return _imgInvert(rgba, w, h)
}

// desaturate collapses every pixel to its Rec.709 luma.
fn desaturate(rgba, w, h) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "desaturate", _MSL_DESATURATE, null)
        if err == null { return out, null }
    }
    return _imgDesaturate(rgba, w, h)
}

// sepia mixes the original pixel with its sepia-toned counterpart by
// `strength` (0 = no change, 1 = full sepia).
fn sepia(rgba, w, h, strength) {
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "sepia", _MSL_SEPIA, [strength])
        if err == null { return out, null }
    }
    return _imgSepia(rgba, w, h, strength)
}

// vignette darkens pixels away from the image centre.
// strength in [0,1]; radius in (0, ∞) — 0.5 tight, 1.0 gentle, 1.5
// only touches the corners.
fn vignette(rgba, w, h, strength, radius) {
    if radius <= 0 {
        return null, error("MTL_FX_BAD_ARGS", "vignette: radius must be > 0")
    }
    if _useMetal() {
        let out, err = _runMetal(rgba, w, h, "vignette", _MSL_VIGNETTE,
            [strength, radius])
        if err == null { return out, null }
    }
    return _imgVignette(rgba, w, h, strength, radius)
}
