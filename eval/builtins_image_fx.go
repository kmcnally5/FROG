// builtins_image_fx.go — pure-Go RGBA8 image filters.
//
// These are the cross-platform fallback path for kLex's image effects.
// On macOS the `mtl_fx` stdlib prefers Metal compute kernels (one
// dispatch per filter, sub-millisecond at 1024²); on Linux/Windows
// (and as a smoke-test sanity check on macOS) it calls these Go
// builtins instead. Per-filter cost at 1024² is roughly 5-20 ms — fast
// enough for interactive slider UIs.
//
// Pixel layout matches Metal's MTLPixelFormatRGBA8Unorm and what
// browsers, Stable Diffusion endpoints, and stb_image all produce:
// row-major, four bytes per pixel (R, G, B, A in that order).
//
// Color space: filters operate on sRGB-encoded byte values, NOT on
// linear light. This matches Photoshop/Lightroom defaults — the math
// is "wrong" relative to scene-referred linear pipelines but matches
// user expectation, and skips an sRGB→linear→sRGB round-trip that
// would add ~10% per filter for negligible visual improvement at the
// adjustment magnitudes Tier 1 targets.
//
// Alpha is preserved verbatim — no Tier 1 filter touches alpha.

package eval

import (
	"fmt"
	"math"

	"klex/ast"
)

// ── byte ↔ float helpers ────────────────────────────────────────────

// b2f maps a byte in [0, 255] to a float in [0, 1].
func b2f(b byte) float64 { return float64(b) / 255.0 }

// f2b maps a float in [0, 1] (or outside; clamped) to a byte in [0, 255].
func f2b(f float64) byte {
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 255
	}
	// +0.5 for round-to-nearest. math.Round would also work; +0.5+truncate
	// is faster and produces identical results for non-negative inputs.
	return byte(f*255.0 + 0.5)
}

// luma709 returns Rec.709 luma for an sRGB-encoded pixel. The
// coefficients are the standard {0.2126, 0.7152, 0.0722} — used by
// HDTV, sRGB monitors, and modern image-processing pipelines.
func luma709(r, g, b float64) float64 {
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// ── arg-parsing helpers ─────────────────────────────────────────────

// parseImg pulls the (bytes, width, height) header that every filter
// builtin starts with. Returns the raw byte slice (writable copy is the
// caller's responsibility), width, height, and any parse error.
func parseImg(args []Object, name string) ([]byte, int, int, Object) {
	if len(args) < 3 {
		return nil, 0, 0, runtimeError(name+": expects (bytes, width, height, …)", ast.Pos{})
	}
	bs, bok := args[0].(*Bytes)
	w, wok := args[1].(*Integer)
	h, hok := args[2].(*Integer)
	if !bok || !wok || !hok {
		return nil, 0, 0, typeError(name+": first three args must be (bytes, int, int)", ast.Pos{})
	}
	if w.Value <= 0 || h.Value <= 0 {
		return nil, 0, 0, runtimeError(name+": width and height must be positive", ast.Pos{})
	}
	expected := w.Value * h.Value * 4
	if len(bs.Value) != expected {
		return nil, 0, 0, runtimeError(fmt.Sprintf(
			"%s: bytes length %d does not match width*height*4 = %d",
			name, len(bs.Value), expected), ast.Pos{})
	}
	return bs.Value, w.Value, h.Value, nil
}

// argFloat pulls the i-th argument as a float (accepts Integer too).
func argFloat(args []Object, i int, name, label string) (float64, Object) {
	if i >= len(args) {
		return 0, runtimeError(fmt.Sprintf("%s: missing argument %s", name, label), ast.Pos{})
	}
	switch v := args[i].(type) {
	case *Float:
		return v.Value, nil
	case *Integer:
		return float64(v.Value), nil
	}
	return 0, typeError(fmt.Sprintf("%s: %s must be a number", name, label), ast.Pos{})
}

// newOutBytes returns a writable byte slice of the same length as the
// input, with alpha pre-filled. Per-pixel filters only need to fill
// the RGB triples; alpha passthrough is handled here once.
func newOutBytes(in []byte) []byte {
	out := make([]byte, len(in))
	// Copy alpha first; loops that overwrite RGB can ignore alpha bytes.
	for i := 3; i < len(in); i += 4 {
		out[i] = in[i]
	}
	return out
}

// rgbToHsv / hsvToRgb — HSV operates on sRGB-encoded inputs in [0,1].
// Standard formulas; hue is in degrees [0, 360).
func rgbToHsv(r, g, b float64) (h, s, v float64) {
	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	v = maxC
	d := maxC - minC
	if maxC > 0 {
		s = d / maxC
	}
	if d == 0 {
		return 0, s, v
	}
	switch maxC {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	return h, s, v
}

func hsvToRgb(h, s, v float64) (r, g, b float64) {
	if s == 0 {
		return v, v, v
	}
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	h /= 60
	i := math.Floor(h)
	f := h - i
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	switch int(i) % 6 {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	default:
		return v, p, q
	}
}

// ── filter implementations ──────────────────────────────────────────

func init() {
	// _imgExposure(bytes, w, h, stops) → (bytes, err)
	//
	// Multiplies RGB by 2^stops — i.e. one stop doubles or halves the
	// linear-light intensity. Typical range -4..+4 stops.
	Builtins["_imgExposure"] = &Builtin{Fn: func(args []Object) Object {
		in, w, h, err := parseImg(args, "_imgExposure")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgExposure expects (bytes, w, h, stops)", ast.Pos{})
		}
		stops, e := argFloat(args, 3, "_imgExposure", "stops")
		if e != nil {
			return e
		}
		_ = w
		_ = h
		mul := math.Pow(2.0, stops)
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			out[i+0] = f2b(b2f(in[i+0]) * mul)
			out[i+1] = f2b(b2f(in[i+1]) * mul)
			out[i+2] = f2b(b2f(in[i+2]) * mul)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgBrightness(bytes, w, h, amount) → (bytes, err)
	//
	// Adds `amount` to RGB (each in [0,1] space). amount in [-1, +1];
	// 0 is no change. Additive, unlike exposure (which is multiplicative).
	Builtins["_imgBrightness"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgBrightness")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgBrightness expects (bytes, w, h, amount)", ast.Pos{})
		}
		amount, e := argFloat(args, 3, "_imgBrightness", "amount")
		if e != nil {
			return e
		}
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			out[i+0] = f2b(b2f(in[i+0]) + amount)
			out[i+1] = f2b(b2f(in[i+1]) + amount)
			out[i+2] = f2b(b2f(in[i+2]) + amount)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgContrast(bytes, w, h, amount) → (bytes, err)
	//
	// Scales each channel around 0.5 mid-gray: out = (in - 0.5)*(1+amount)+0.5.
	// amount in [-1, +1]; 0 is no change, +1 doubles contrast, -1 flattens
	// to mid-gray.
	Builtins["_imgContrast"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgContrast")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgContrast expects (bytes, w, h, amount)", ast.Pos{})
		}
		amount, e := argFloat(args, 3, "_imgContrast", "amount")
		if e != nil {
			return e
		}
		k := 1.0 + amount
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			out[i+0] = f2b((b2f(in[i+0])-0.5)*k + 0.5)
			out[i+1] = f2b((b2f(in[i+1])-0.5)*k + 0.5)
			out[i+2] = f2b((b2f(in[i+2])-0.5)*k + 0.5)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgSaturation(bytes, w, h, amount) → (bytes, err)
	//
	// Pulls each pixel toward (amount < 0) or away from (amount > 0)
	// its luma. amount in [-1, +1]: -1 fully desaturates, 0 no change,
	// +1 doubles saturation (may clip on already-saturated pixels).
	Builtins["_imgSaturation"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgSaturation")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgSaturation expects (bytes, w, h, amount)", ast.Pos{})
		}
		amount, e := argFloat(args, 3, "_imgSaturation", "amount")
		if e != nil {
			return e
		}
		k := 1.0 + amount
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			r, g, b := b2f(in[i+0]), b2f(in[i+1]), b2f(in[i+2])
			y := luma709(r, g, b)
			out[i+0] = f2b(y + (r-y)*k)
			out[i+1] = f2b(y + (g-y)*k)
			out[i+2] = f2b(y + (b-y)*k)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgHueShift(bytes, w, h, degrees) → (bytes, err)
	//
	// Rotates hue by `degrees` (any real number; wraps modulo 360).
	// Routes through HSV — saturation and value are unchanged.
	Builtins["_imgHueShift"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgHueShift")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgHueShift expects (bytes, w, h, degrees)", ast.Pos{})
		}
		deg, e := argFloat(args, 3, "_imgHueShift", "degrees")
		if e != nil {
			return e
		}
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			r, g, b := b2f(in[i+0]), b2f(in[i+1]), b2f(in[i+2])
			h, s, v := rgbToHsv(r, g, b)
			h += deg
			nr, ng, nb := hsvToRgb(h, s, v)
			out[i+0] = f2b(nr)
			out[i+1] = f2b(ng)
			out[i+2] = f2b(nb)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgGamma(bytes, w, h, gamma) → (bytes, err)
	//
	// Raises each channel to the power 1/gamma. gamma > 1 brightens
	// midtones; gamma < 1 darkens them. Must be > 0.
	Builtins["_imgGamma"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgGamma")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgGamma expects (bytes, w, h, gamma)", ast.Pos{})
		}
		gamma, e := argFloat(args, 3, "_imgGamma", "gamma")
		if e != nil {
			return e
		}
		if gamma <= 0 {
			return runtimeError("_imgGamma: gamma must be > 0", ast.Pos{})
		}
		inv := 1.0 / gamma
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			out[i+0] = f2b(math.Pow(b2f(in[i+0]), inv))
			out[i+1] = f2b(math.Pow(b2f(in[i+1]), inv))
			out[i+2] = f2b(math.Pow(b2f(in[i+2]), inv))
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgLevels(bytes, w, h, inBlack, inWhite, gamma, outBlack, outWhite) → (bytes, err)
	//
	// Classic Photoshop-style levels: remap [inBlack, inWhite] of the
	// input to [outBlack, outWhite] of the output with an intermediate
	// gamma adjustment. All five sliders are floats in [0,1] (gamma
	// is the usual "midtone" knob, > 0).
	Builtins["_imgLevels"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgLevels")
		if err != nil {
			return err
		}
		if len(args) != 8 {
			return runtimeError("_imgLevels expects (bytes, w, h, inB, inW, gamma, outB, outW)", ast.Pos{})
		}
		inB, e := argFloat(args, 3, "_imgLevels", "inBlack")
		if e != nil {
			return e
		}
		inW, e := argFloat(args, 4, "_imgLevels", "inWhite")
		if e != nil {
			return e
		}
		gamma, e := argFloat(args, 5, "_imgLevels", "gamma")
		if e != nil {
			return e
		}
		outB, e := argFloat(args, 6, "_imgLevels", "outBlack")
		if e != nil {
			return e
		}
		outW, e := argFloat(args, 7, "_imgLevels", "outWhite")
		if e != nil {
			return e
		}
		if gamma <= 0 {
			return runtimeError("_imgLevels: gamma must be > 0", ast.Pos{})
		}
		span := inW - inB
		if span == 0 {
			return runtimeError("_imgLevels: inBlack and inWhite must differ", ast.Pos{})
		}
		invG := 1.0 / gamma
		outSpan := outW - outB
		out := newOutBytes(in)
		remap := func(c float64) float64 {
			n := (c - inB) / span
			if n < 0 {
				n = 0
			}
			if n > 1 {
				n = 1
			}
			return outB + math.Pow(n, invG)*outSpan
		}
		for i := 0; i < len(in); i += 4 {
			out[i+0] = f2b(remap(b2f(in[i+0])))
			out[i+1] = f2b(remap(b2f(in[i+1])))
			out[i+2] = f2b(remap(b2f(in[i+2])))
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgChannelMixer(bytes, w, h, matrix9) → (bytes, err)
	//
	// matrix9 is a flat 9-element array of floats representing a
	// 3×3 row-major matrix applied to each pixel's RGB column:
	//   [ rr rg rb
	//     gr gg gb
	//     br bg bb ]
	// out.r = rr*in.r + rg*in.g + rb*in.b   (and similarly for g, b)
	// Identity matrix [1,0,0, 0,1,0, 0,0,1] = no change.
	Builtins["_imgChannelMixer"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgChannelMixer")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgChannelMixer expects (bytes, w, h, matrix9)", ast.Pos{})
		}
		arr, ok := args[3].(*Array)
		if !ok {
			return typeError("_imgChannelMixer: matrix9 must be a 9-element array", ast.Pos{})
		}
		if len(arr.Elements) != 9 {
			return runtimeError("_imgChannelMixer: matrix9 must have exactly 9 elements", ast.Pos{})
		}
		var m [9]float64
		for i, el := range arr.Elements {
			switch v := el.(type) {
			case *Float:
				m[i] = v.Value
			case *Integer:
				m[i] = float64(v.Value)
			default:
				return typeError("_imgChannelMixer: matrix elements must be numbers", ast.Pos{})
			}
		}
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			r, g, b := b2f(in[i+0]), b2f(in[i+1]), b2f(in[i+2])
			out[i+0] = f2b(m[0]*r + m[1]*g + m[2]*b)
			out[i+1] = f2b(m[3]*r + m[4]*g + m[5]*b)
			out[i+2] = f2b(m[6]*r + m[7]*g + m[8]*b)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgInvert(bytes, w, h) → (bytes, err)
	//
	// Negative — 1 - c on each RGB channel.
	Builtins["_imgInvert"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgInvert")
		if err != nil {
			return err
		}
		if len(args) != 3 {
			return runtimeError("_imgInvert expects (bytes, w, h)", ast.Pos{})
		}
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			out[i+0] = 255 - in[i+0]
			out[i+1] = 255 - in[i+1]
			out[i+2] = 255 - in[i+2]
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgDesaturate(bytes, w, h) → (bytes, err)
	//
	// Convenience equivalent to saturation(-1) but cheaper — every
	// channel becomes Rec.709 luma.
	Builtins["_imgDesaturate"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgDesaturate")
		if err != nil {
			return err
		}
		if len(args) != 3 {
			return runtimeError("_imgDesaturate expects (bytes, w, h)", ast.Pos{})
		}
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			y := f2b(luma709(b2f(in[i+0]), b2f(in[i+1]), b2f(in[i+2])))
			out[i+0] = y
			out[i+1] = y
			out[i+2] = y
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgSepia(bytes, w, h, strength) → (bytes, err)
	//
	// Mixes the original pixel with its sepia-toned counterpart by
	// `strength` (0 = no change, 1 = full sepia). Sepia transform is
	// the standard Photoshop matrix.
	Builtins["_imgSepia"] = &Builtin{Fn: func(args []Object) Object {
		in, _, _, err := parseImg(args, "_imgSepia")
		if err != nil {
			return err
		}
		if len(args) != 4 {
			return runtimeError("_imgSepia expects (bytes, w, h, strength)", ast.Pos{})
		}
		s, e := argFloat(args, 3, "_imgSepia", "strength")
		if e != nil {
			return e
		}
		if s < 0 {
			s = 0
		}
		if s > 1 {
			s = 1
		}
		out := newOutBytes(in)
		for i := 0; i < len(in); i += 4 {
			r, g, b := b2f(in[i+0]), b2f(in[i+1]), b2f(in[i+2])
			tr := 0.393*r + 0.769*g + 0.189*b
			tg := 0.349*r + 0.686*g + 0.168*b
			tb := 0.272*r + 0.534*g + 0.131*b
			out[i+0] = f2b(r + (tr-r)*s)
			out[i+1] = f2b(g + (tg-g)*s)
			out[i+2] = f2b(b + (tb-b)*s)
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}

	// _imgVignette(bytes, w, h, strength, radius) → (bytes, err)
	//
	// Darkens pixels away from the image centre. `strength` in [0,1]
	// is how dark the corners get (0 = no change, 1 = corners → black).
	// `radius` in (0, ∞) controls how quickly the falloff starts; 0.5
	// is a tight vignette, 1.0 is gentle, 1.5 only kisses the corners.
	// Distance is normalised to half-diagonal so the effect is
	// resolution-independent.
	Builtins["_imgVignette"] = &Builtin{Fn: func(args []Object) Object {
		in, w, h, err := parseImg(args, "_imgVignette")
		if err != nil {
			return err
		}
		if len(args) != 5 {
			return runtimeError("_imgVignette expects (bytes, w, h, strength, radius)", ast.Pos{})
		}
		strength, e := argFloat(args, 3, "_imgVignette", "strength")
		if e != nil {
			return e
		}
		radius, e := argFloat(args, 4, "_imgVignette", "radius")
		if e != nil {
			return e
		}
		if radius <= 0 {
			return runtimeError("_imgVignette: radius must be > 0", ast.Pos{})
		}
		if strength < 0 {
			strength = 0
		}
		if strength > 1 {
			strength = 1
		}
		cx := float64(w-1) * 0.5
		cy := float64(h-1) * 0.5
		halfDiag := math.Sqrt(cx*cx + cy*cy)
		out := newOutBytes(in)
		for y := 0; y < h; y++ {
			dy := float64(y) - cy
			for x := 0; x < w; x++ {
				dx := float64(x) - cx
				d := math.Sqrt(dx*dx+dy*dy) / halfDiag
				// Smoothstep-like falloff: 0 at d=0, 1 at d=radius.
				t := d / radius
				if t < 0 {
					t = 0
				}
				if t > 1 {
					t = 1
				}
				factor := 1.0 - strength*(t*t*(3-2*t))
				i := (y*w + x) * 4
				out[i+0] = f2b(b2f(in[i+0]) * factor)
				out[i+1] = f2b(b2f(in[i+1]) * factor)
				out[i+2] = f2b(b2f(in[i+2]) * factor)
			}
		}
		return &Tuple{Elements: []Object{&Bytes{Value: out}, NULL}}
	}}
}
