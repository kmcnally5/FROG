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

	// bytes — construct a bytes value from a string, array, int, or bytes.
	//
	// Four forms: a string (utf-8 encode), an integer array (pack each 0..255), an
	// integer N (N zero bytes), or another bytes value (defensive copy). A bytes
	// value prints as bytes(N) — the length only, never the raw content.
	//
	// @sig     bytes(x: any) -> bytes
	// @param   x  a string, an integer array (each 0..255), a non-negative int (length), or a bytes value
	// @returns a new bytes value
	// @errors  TypeError for unsupported input; RuntimeError if an array element or length is out of range
	// @example bytes([72, 105, 33])   → bytes(3)
	// @example bytes(4)               → bytes(4)
	// @since   0.1.0
	// @see     strToBytes, bytesToStr, bytesConcat
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

	// strToBytes — utf-8 encode a string into bytes.
	//
	// Strings are already utf-8 internally, so this is effectively a typed copy —
	// but as an explicit builtin it keeps the conversion direction obvious at the
	// call site. The inverse is bytesToStr.
	//
	// @sig     strToBytes(s: string) -> bytes
	// @param   s  the string to encode
	// @returns the utf-8 bytes of s
	// @errors  TypeError if s is not a string
	// @example strToBytes("Hi")   → bytes(2)
	// @since   0.1.0
	// @see     bytesToStr, bytes
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

	// bytesToStr — utf-8 decode bytes into a string, returning (value, err).
	//
	// Use this when binary may be text. If the call site is sure the bytes are
	// utf-8 (e.g. JSON response bodies), unwrap with `?`.
	//
	// @sig     bytesToStr(b: bytes) -> (string, error)
	// @param   b  the bytes to decode
	// @returns (str, null) on success; (null, error) with code "BYTES_INVALID_UTF8" if b isn't valid utf-8
	// @errors  TypeError if b is not bytes; the utf-8 failure is returned in the tuple, not raised
	// @example bytesToStr(bytes("Hi"))   → (Hi, null)
	// @since   0.1.0
	// @see     strToBytes, bytes
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

	// bytesToBase64 — standard base64 encode (RFC 4648, padded).
	//
	// Encodes the whole input into a single line with no internal breaks. The
	// inverse is base64ToBytes.
	//
	// @sig     bytesToBase64(b: bytes) -> string
	// @param   b  the bytes to encode
	// @returns the base64 string (with padding)
	// @errors  TypeError if b is not bytes
	// @example bytesToBase64(bytes("Hi"))   → SGk=
	// @since   0.1.0
	// @see     base64ToBytes, bytesToHex
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

	// base64ToBytes — decode standard base64, returning (value, err).
	//
	// @sig     base64ToBytes(s: string) -> (bytes, error)
	// @param   s  a standard (padded) base64 string
	// @returns (bytes, null) on success; (null, error) with code "BASE64_INVALID" on bad input
	// @errors  TypeError if s is not a string; decode failures are returned in the tuple, not raised
	// @example base64ToBytes("SGk=")   → (bytes(2), null)
	// @since   0.1.0
	// @see     bytesToBase64, hexToBytes
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

	// bytesToHex — lowercase hex encode, two chars per byte, no separator.
	//
	// "deadbeef" style. The inverse is hexToBytes.
	//
	// @sig     bytesToHex(b: bytes) -> string
	// @param   b  the bytes to encode
	// @returns the lowercase hex string (2 chars per byte)
	// @errors  TypeError if b is not bytes
	// @example bytesToHex(bytes([222, 173, 190, 239]))   → deadbeef
	// @since   0.1.0
	// @see     hexToBytes, bytesToBase64
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

	// hexToBytes — decode a hex string (upper or lower case), returning (value, err).
	//
	// @sig     hexToBytes(s: string) -> (bytes, error)
	// @param   s  a hex string; case-insensitive, even length
	// @returns (bytes, null) on success; (null, error) with code "HEX_INVALID" on bad input (odd length or non-hex char)
	// @errors  TypeError if s is not a string; decode failures are returned in the tuple, not raised
	// @example hexToBytes("deadbeef")   → (bytes(4), null)
	// @since   0.1.0
	// @see     bytesToHex, base64ToBytes
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

	// floatsToBytes — pack a number array as little-endian float32 bytes.
	//
	// The standard wire format for embeddings, scientific data, and audio
	// samples. Output length is 4 × len(arr). Integer elements are accepted and
	// converted, so `[1, 2, 3]` works. Round-trips with bytesToFloats, and the
	// layout matches Metal's MTLBuffer<float> so the bytes go straight to the GPU.
	//
	// @sig     floatsToBytes(arr: array) -> bytes
	// @param   arr  an array of numbers (floats or ints)
	// @returns the little-endian float32 encoding, 4 bytes per element
	// @errors  TypeError if arr isn't an array or an element isn't numeric
	// @example floatsToBytes([1.0, 2.0])   → bytes(8)
	// @since   0.1.0
	// @see     bytesToFloats
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

	// bytesToFloats — unpack little-endian float32 bytes into a number array.
	//
	// Counterpart to floatsToBytes. The input length must be a multiple of 4.
	//
	// @sig     bytesToFloats(bs: bytes) -> (array, error)
	// @param   bs  bytes whose length is a multiple of 4
	// @returns (array-of-floats, null) on success; (null, error) with code "BYTES_LEN_INVALID" if the length isn't a multiple of 4
	// @errors  TypeError if bs is not bytes; the length mismatch is returned in the tuple, not raised
	// @example bytesToFloats(floatsToBytes([1.0, 2.0]))   → ([1, 2], null)
	// @since   0.1.0
	// @see     floatsToBytes
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

	// bytesConcat — merge an array of bytes values into one.
	//
	// Single-allocation memcpy of every element in order — far faster than
	// byte-indexing into an int array and calling bytes(). An empty array yields
	// empty bytes; a single element yields a copy (not a reference share).
	//
	// @sig     bytesConcat(arr: array) -> bytes
	// @param   arr  an array whose elements are all bytes values
	// @returns one bytes value: every element concatenated in order
	// @errors  TypeError if arr isn't an array or any element isn't bytes (the error names the first bad index)
	// @example bytesConcat([bytes("foo"), bytes("bar")])   → bytes(6)
	// @since   0.1.0
	// @see     bytes, concat
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
