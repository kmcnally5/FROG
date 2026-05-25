package eval

// builtins_bytes.go — builtins for the Bytes type.
//
// kLex's `bytes` is an arbitrary byte sequence — not utf-8-validated like
// `string`. These builtins cover the four things bridge authors and binary
// users actually need: construction, encoding/decoding to text-safe forms,
// and conversion to/from strings.
//
// All decoders return a (value, err) tuple — same shape as parseInt, so
// callers can use safe() / err.code patterns consistently.

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"klex/ast"
	"math"
	"unicode/utf8"
)

func init() {

	// ── bytes(x) → bytes ────────────────────────────────────────────────────
	//
	// Construct a bytes value:
	//   bytes("hello")           — utf-8 encode the string
	//   bytes([72, 101, 108])    — pack an integer array (each 0..255)
	//   bytes(5)                 — N zero bytes
	//   bytes(b"...")            — defensive copy
	Builtins["bytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bytes expects 1 argument", ast.Pos{})
		}
		switch a := args[0].(type) {
		case *String:
			return &Bytes{Value: []byte(a.Value)}
		case *Bytes:
			out := make([]byte, len(a.Value))
			copy(out, a.Value)
			return &Bytes{Value: out}
		case *Integer:
			if a.Value < 0 {
				return runtimeError(fmt.Sprintf("bytes: length must be non-negative, got %d", a.Value), ast.Pos{})
			}
			return &Bytes{Value: make([]byte, a.Value)}
		case *Array:
			out := make([]byte, len(a.Elements))
			for i, el := range a.Elements {
				n, ok := el.(*Integer)
				if !ok {
					return typeError(fmt.Sprintf("bytes: array[%d] must be integer, got %s", i, el.Type()), ast.Pos{})
				}
				if n.Value < 0 || n.Value > 255 {
					return runtimeError(fmt.Sprintf("bytes: array[%d] = %d out of range [0, 255]", i, n.Value), ast.Pos{})
				}
				out[i] = byte(n.Value)
			}
			return &Bytes{Value: out}
		default:
			return typeError(fmt.Sprintf("bytes: cannot construct from %s", args[0].Type()), ast.Pos{})
		}
	}}

	// ── strToBytes(s: string) → bytes ───────────────────────────────────────
	//
	// utf-8 encode a string. Strings are already utf-8 internally so this is
	// effectively a typed copy — but having it as an explicit builtin keeps
	// the conversion direction obvious at the call site.
	Builtins["strToBytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("strToBytes expects 1 argument (str)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("strToBytes: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		return &Bytes{Value: []byte(s.Value)}
	}}

	// ── bytesToStr(b: bytes) → (string, error) ──────────────────────────────
	//
	// utf-8 decode bytes. Returns (str, null) on success, (null, err) when
	// the bytes are not valid utf-8 — err.code is "BYTES_INVALID_UTF8". Use
	// this when binary may be text; if the call site is sure the bytes are
	// utf-8 (e.g. JSON response bodies) unwrap with `?`.
	Builtins["bytesToStr"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bytesToStr expects 1 argument (bytes)", ast.Pos{})
		}
		b, ok := args[0].(*Bytes)
		if !ok {
			return typeError(fmt.Sprintf("bytesToStr: argument must be bytes, got %s", args[0].Type()), ast.Pos{})
		}
		if !utf8.Valid(b.Value) {
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        "BYTES_INVALID_UTF8",
				Message:     "bytesToStr: input is not valid utf-8",
			}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(b.Value)}, NULL}}
	}}

	// ── bytesToBase64(b: bytes) → string ────────────────────────────────────
	//
	// Standard base64 encoding (RFC 4648, with padding). Reverse of
	// base64ToBytes. Total-encoding wraps into a single string with no
	// internal line breaks.
	Builtins["bytesToBase64"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bytesToBase64 expects 1 argument (bytes)", ast.Pos{})
		}
		b, ok := args[0].(*Bytes)
		if !ok {
			return typeError(fmt.Sprintf("bytesToBase64: argument must be bytes, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: base64.StdEncoding.EncodeToString(b.Value)}
	}}

	// ── base64ToBytes(s: string) → (bytes, error) ───────────────────────────
	//
	// Decode standard base64. Returns (bytes, null) on success, (null, err)
	// with code "BASE64_INVALID" on bad input.
	Builtins["base64ToBytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("base64ToBytes expects 1 argument (str)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("base64ToBytes: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		decoded, err := base64.StdEncoding.DecodeString(s.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        "BASE64_INVALID",
				Message:     "base64ToBytes: " + err.Error(),
			}}}
		}
		return &Tuple{Elements: []Object{&Bytes{Value: decoded}, NULL}}
	}}

	// ── bytesToHex(b: bytes) → string ───────────────────────────────────────
	//
	// Lowercase hex encoding, two characters per byte, no separator.
	// "deadbeef" style. Reverse of hexToBytes.
	Builtins["bytesToHex"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bytesToHex expects 1 argument (bytes)", ast.Pos{})
		}
		b, ok := args[0].(*Bytes)
		if !ok {
			return typeError(fmt.Sprintf("bytesToHex: argument must be bytes, got %s", args[0].Type()), ast.Pos{})
		}
		return &String{Value: hex.EncodeToString(b.Value)}
	}}

	// ── hexToBytes(s: string) → (bytes, error) ──────────────────────────────
	//
	// Decode a hex string. Accepts both upper and lower case. Returns
	// (bytes, null) on success, (null, err) with code "HEX_INVALID" on
	// bad input (odd length, non-hex character).
	Builtins["hexToBytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("hexToBytes expects 1 argument (str)", ast.Pos{})
		}
		s, ok := args[0].(*String)
		if !ok {
			return typeError(fmt.Sprintf("hexToBytes: argument must be string, got %s", args[0].Type()), ast.Pos{})
		}
		decoded, err := hex.DecodeString(s.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        "HEX_INVALID",
				Message:     "hexToBytes: " + err.Error(),
			}}}
		}
		return &Tuple{Elements: []Object{&Bytes{Value: decoded}, NULL}}
	}}

	// ── floatsToBytes(arr) → bytes ──────────────────────────────────────────
	//
	// Pack a kLex float array as little-endian IEEE-754 float32 bytes —
	// the standard wire format for embeddings, scientific data, audio
	// samples. Output length is 4 × len(arr). Integer elements are
	// accepted and converted (so a literal `[1, 2, 3]` works).
	//
	// Round-trips with bytesToFloats. Use for disk-resident vector
	// indexes (one float32 per element matches Metal MTLBuffer<float>
	// layout, so the same bytes go straight into _mtlBuffer without
	// re-packing).
	Builtins["floatsToBytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("floatsToBytes expects 1 argument (array)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("floatsToBytes: argument must be an array, got %s", args[0].Type()), ast.Pos{})
		}
		out := make([]byte, len(arr.Elements)*4)
		for i, el := range arr.Elements {
			var f float32
			switch v := el.(type) {
			case *Float:
				f = float32(v.Value)
			case *Integer:
				f = float32(v.Value)
			default:
				return typeError(fmt.Sprintf("floatsToBytes: array[%d] must be number, got %s", i, el.Type()), ast.Pos{})
			}
			binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
		}
		return &Bytes{Value: out}
	}}

	// ── bytesToFloats(bs) → array ───────────────────────────────────────────
	//
	// Unpack little-endian IEEE-754 float32 bytes into a kLex float
	// array. Counterpart to floatsToBytes. len(bs) must be a multiple
	// of 4. Returns (null, err) on size mismatch so callers can branch
	// cleanly.
	Builtins["bytesToFloats"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bytesToFloats expects 1 argument (bytes)", ast.Pos{})
		}
		bs, ok := args[0].(*Bytes)
		if !ok {
			return typeError(fmt.Sprintf("bytesToFloats: argument must be bytes, got %s", args[0].Type()), ast.Pos{})
		}
		if len(bs.Value)%4 != 0 {
			return &Tuple{Elements: []Object{NULL, &Error{
				IsUserError: true,
				Code:        "BYTES_LEN_INVALID",
				Message:     fmt.Sprintf("bytesToFloats: length %d is not a multiple of 4", len(bs.Value)),
			}}}
		}
		n := len(bs.Value) / 4
		out := &Array{Elements: make([]Object, n)}
		for i := 0; i < n; i++ {
			f := math.Float32frombits(binary.LittleEndian.Uint32(bs.Value[i*4:]))
			out.Elements[i] = &Float{Value: float64(f)}
		}
		return &Tuple{Elements: []Object{out, NULL}}
	}}

	// ── bytesConcat(arr: array of bytes) → bytes ────────────────────────────
	//
	// Merge an array of *Bytes into a single *Bytes via Go-level memcpy.
	// The pre-existing workaround was: byte-index each into an int array
	// then call `bytes(intArray)` — fine for small payloads but ~40ms per
	// 393kB merge in the interpreter. Doing it here as a Go builtin is one
	// allocation plus N copy() calls, microsecond-scale.
	//
	// Empty array → empty bytes (0-length). Single-element array → a copy
	// of that element (NOT a reference share). Mixed-type array → typeError
	// identifying the first non-bytes index.
	Builtins["bytesConcat"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return runtimeError("bytesConcat expects 1 argument (array of bytes)", ast.Pos{})
		}
		arr, ok := args[0].(*Array)
		if !ok {
			return typeError(fmt.Sprintf("bytesConcat: argument must be array, got %s", args[0].Type()), ast.Pos{})
		}
		// Total length first so we allocate once.
		total := 0
		for i, el := range arr.Elements {
			b, ok := el.(*Bytes)
			if !ok {
				return typeError(fmt.Sprintf("bytesConcat: array[%d] must be bytes, got %s", i, el.Type()), ast.Pos{})
			}
			total += len(b.Value)
		}
		out := make([]byte, total)
		pos := 0
		for _, el := range arr.Elements {
			b := el.(*Bytes) // type checked above
			copy(out[pos:], b.Value)
			pos += len(b.Value)
		}
		return &Bytes{Value: out}
	}}
}
